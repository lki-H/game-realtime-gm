package database

import "testing"

func TestSplitSQLHandlesQuotedSemicolons(t *testing.T) {
	statements := splitSQL("INSERT INTO t VALUES ('a;b'); CREATE TABLE t2 (v VARCHAR(10));")
	if len(statements) != 2 {
		t.Fatalf("expected two statements, got %d", len(statements))
	}
}
