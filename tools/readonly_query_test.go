package tools

import "testing"

func TestIsReadonlyQuery(t *testing.T) {
	cases := []struct {
		query string
		want  bool
	}{
		{"SELECT * FROM users", true},
		{"select id, updated_at from orders", true}, // "update" substring must not trip
		{"SELECT * FROM inserted_rows", true},       // "insert" substring must not trip
		{"DELETE FROM users", false},
		{"delete from users", false},
		{"UPDATE users SET name='x'", false},
		{"INSERT INTO users VALUES (1)", false},
		{"DROP TABLE users", false},
		{"TRUNCATE users", false},
		{"ALTER TABLE users ADD c INT", false},
		{"SELECT 1; DELETE FROM users", false},
	}
	for _, c := range cases {
		if got := isReadonlyQuery(c.query); got != c.want {
			t.Errorf("isReadonlyQuery(%q) = %v, want %v", c.query, got, c.want)
		}
	}
}
