package database

import (
	"encoding/json"
	"testing"
)

func TestParseReadonlyPipelineRejectsWrites(t *testing.T) {
	writeStages := []string{
		`[{"$match":{"x":1}},{"$out":"copy"}]`,
		`[{"$merge":{"into":"other"}}]`,
	}
	for _, p := range writeStages {
		if _, err := parseReadonlyPipeline(json.RawMessage(p)); err == nil {
			t.Errorf("expected pipeline %s to be rejected", p)
		}
	}
}

func TestParseReadonlyPipelineAllowsReads(t *testing.T) {
	readPipeline := `[{"$match":{"active":true}},{"$group":{"_id":"$country","n":{"$sum":1}}}]`
	if _, err := parseReadonlyPipeline(json.RawMessage(readPipeline)); err != nil {
		t.Fatalf("expected read pipeline to be allowed, got: %v", err)
	}
}
