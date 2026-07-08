package database

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"
)

// mongoSampleSize is how many documents GetSchema samples to infer a schema.
const mongoSampleSize = 100

// MongoAdapter implements DatabaseClient for MongoDB.
//
// MongoDB is not SQL, so the interface is mapped as follows:
//   - ListTables lists collections.
//   - GetSchema samples documents and reports each field's observed BSON types.
//   - RunReadonlyQuery accepts a JSON find/aggregate spec (see runMongoQuery).
type MongoAdapter struct {
	client *mongo.Client
	db     *mongo.Database
}

// NewMongoAdapter connects using a standard MongoDB connection URI. The URI must
// include the database name, e.g. mongodb://host:27017/mydb.
func NewMongoAdapter(ctx context.Context, dsn string) (*MongoAdapter, error) {
	cs, err := connstring.ParseAndValidate(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid mongodb URI: %w", err)
	}
	if cs.Database == "" {
		return nil, fmt.Errorf("mongodb URI must include a database name, e.g. mongodb://host:27017/mydb")
	}

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(dsn))
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		_ = client.Disconnect(ctx)
		return nil, fmt.Errorf("unable to ping database: %w", err)
	}

	return &MongoAdapter{client: client, db: client.Database(cs.Database)}, nil
}

func (m *MongoAdapter) ListTables(ctx context.Context) ([]string, error) {
	names, err := m.db.ListCollectionNames(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to list collections: %w", err)
	}
	sort.Strings(names)
	return names, nil
}

func (m *MongoAdapter) GetSchema(ctx context.Context, tableName string) (string, error) {
	names, err := m.db.ListCollectionNames(ctx, bson.M{"name": tableName})
	if err != nil {
		return "", fmt.Errorf("failed to look up collection: %w", err)
	}
	if len(names) == 0 {
		return "", fmt.Errorf("collection '%s' not found", tableName)
	}

	cur, err := m.db.Collection(tableName).Find(ctx, bson.M{}, options.Find().SetLimit(mongoSampleSize))
	if err != nil {
		return "", fmt.Errorf("failed to sample collection: %w", err)
	}
	defer cur.Close(ctx)

	// field name -> set of observed BSON type names.
	fields := map[string]map[string]struct{}{}
	sampled := 0
	for cur.Next(ctx) {
		var raw bson.Raw
		if err := cur.Decode(&raw); err != nil {
			return "", fmt.Errorf("failed to decode document: %w", err)
		}
		elems, err := raw.Elements()
		if err != nil {
			return "", fmt.Errorf("failed to read document: %w", err)
		}
		for _, el := range elems {
			key := el.Key()
			if fields[key] == nil {
				fields[key] = map[string]struct{}{}
			}
			fields[key][el.Value().Type.String()] = struct{}{}
		}
		sampled++
	}
	if err := cur.Err(); err != nil {
		return "", fmt.Errorf("cursor error: %w", err)
	}

	type fieldSchema struct {
		Field string   `json:"field"`
		Types []string `json:"types"`
	}
	schema := make([]fieldSchema, 0, len(fields))
	for name, types := range fields {
		ts := make([]string, 0, len(types))
		for t := range types {
			ts = append(ts, t)
		}
		sort.Strings(ts)
		schema = append(schema, fieldSchema{Field: name, Types: ts})
	}
	sort.Slice(schema, func(i, j int) bool { return schema[i].Field < schema[j].Field })

	out, err := json.MarshalIndent(map[string]interface{}{
		"collection":      tableName,
		"sampled_docs":    sampled,
		"inferred_schema": schema,
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal schema: %w", err)
	}
	return string(out), nil
}

// mongoQuerySpec is the JSON shape accepted by RunReadonlyQuery. Either a find
// (collection + optional filter/projection/sort/limit) or an aggregation
// (collection + pipeline). It is read-only by construction: only find and
// aggregate are ever executed, and write stages ($out/$merge) are rejected.
type mongoQuerySpec struct {
	Collection string          `json:"collection"`
	Filter     json.RawMessage `json:"filter"`
	Projection json.RawMessage `json:"projection"`
	Sort       json.RawMessage `json:"sort"`
	Limit      *int64          `json:"limit"`
	Pipeline   json.RawMessage `json:"pipeline"`
}

func (m *MongoAdapter) RunReadonlyQuery(ctx context.Context, query string) (string, error) {
	var spec mongoQuerySpec
	if err := json.Unmarshal([]byte(query), &spec); err != nil {
		return "", fmt.Errorf("invalid MongoDB query JSON: %w (expected a find/aggregate spec object)", err)
	}
	if spec.Collection == "" {
		return "", fmt.Errorf(`query must include a "collection" field`)
	}
	coll := m.db.Collection(spec.Collection)

	// Cap the result set. A caller-supplied limit may only lower the cap.
	limit := int64(MaxQueryRows)
	if spec.Limit != nil && *spec.Limit > 0 && *spec.Limit < limit {
		limit = *spec.Limit
	}

	var cur *mongo.Cursor
	var err error
	if len(spec.Pipeline) > 0 {
		pipeline, perr := parseReadonlyPipeline(spec.Pipeline)
		if perr != nil {
			return "", perr
		}
		cur, err = coll.Aggregate(ctx, pipeline)
	} else {
		filter := bson.M{}
		if len(spec.Filter) > 0 {
			if uerr := bson.UnmarshalExtJSON(spec.Filter, false, &filter); uerr != nil {
				return "", fmt.Errorf("invalid filter: %w", uerr)
			}
		}
		findOpts := options.Find().SetLimit(limit)
		if len(spec.Projection) > 0 {
			var proj bson.M
			if uerr := bson.UnmarshalExtJSON(spec.Projection, false, &proj); uerr != nil {
				return "", fmt.Errorf("invalid projection: %w", uerr)
			}
			findOpts.SetProjection(proj)
		}
		if len(spec.Sort) > 0 {
			var s bson.D
			if uerr := bson.UnmarshalExtJSON(spec.Sort, false, &s); uerr != nil {
				return "", fmt.Errorf("invalid sort: %w", uerr)
			}
			findOpts.SetSort(s)
		}
		cur, err = coll.Find(ctx, filter, findOpts)
	}
	if err != nil {
		return "", fmt.Errorf("failed to execute query: %w", err)
	}
	defer cur.Close(ctx)

	results := []json.RawMessage{}
	for cur.Next(ctx) {
		if len(results) >= MaxQueryRows {
			break
		}
		var raw bson.Raw
		if derr := cur.Decode(&raw); derr != nil {
			return "", fmt.Errorf("failed to decode document: %w", derr)
		}
		// Relaxed Extended JSON keeps the output human/JSON friendly.
		ejson, merr := bson.MarshalExtJSON(raw, false, false)
		if merr != nil {
			return "", fmt.Errorf("failed to marshal document: %w", merr)
		}
		results = append(results, json.RawMessage(ejson))
	}
	if len(results) < MaxQueryRows {
		if cerr := cur.Err(); cerr != nil {
			return "", fmt.Errorf("cursor error: %w", cerr)
		}
	}

	out, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal results: %w", err)
	}
	return string(out), nil
}

// parseReadonlyPipeline decodes an aggregation pipeline and rejects any stage
// that writes to the database, keeping the query read-only.
func parseReadonlyPipeline(raw json.RawMessage) (bson.A, error) {
	var pipeline bson.A
	if err := bson.UnmarshalExtJSON(raw, false, &pipeline); err != nil {
		return nil, fmt.Errorf("invalid pipeline: %w", err)
	}
	for _, stage := range pipeline {
		doc, ok := stage.(bson.D)
		if !ok {
			return nil, fmt.Errorf("each pipeline stage must be an object")
		}
		for _, e := range doc {
			if e.Key == "$out" || e.Key == "$merge" {
				return nil, fmt.Errorf("pipeline stage %q writes data and is not allowed in read-only queries", e.Key)
			}
		}
	}
	return pipeline, nil
}

// Close disconnects the client.
func (m *MongoAdapter) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return m.client.Disconnect(ctx)
}
