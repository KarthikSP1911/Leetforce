package catalog

import "testing"

func TestStatementAndStarters(t *testing.T) {
	c, err := Load("../../../problems")
	if err != nil {
		t.Fatal(err)
	}
	if c.Statement("fizz-count") == "" {
		t.Error("fizz-count has no statement")
	}
	st := c.Starters("fizz-count")
	if len(st) != 4 || st["python"] == "" {
		t.Errorf("starters = %v", st)
	}
	st["python"] = "tampered"
	if c.Starters("fizz-count")["python"] == "tampered" {
		t.Error("Starters must return a copy")
	}
	if c.Statement("nope") != "" || len(c.Starters("nope")) != 0 {
		t.Error("unknown slug must have no content")
	}
}
