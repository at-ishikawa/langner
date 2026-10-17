package database

import (
	"testing"
	"testing/fstest"
)

func TestMaxMigrationVersion(t *testing.T) {
	fsys := fstest.MapFS{
		"migrations/001_init.up.sql":   {Data: []byte("")},
		"migrations/001_init.down.sql": {Data: []byte("")},
		"migrations/009_x.up.sql":      {Data: []byte("")},
		"migrations/033_last.up.sql":   {Data: []byte("")},
		"migrations/033_last.down.sql": {Data: []byte("")},
		"migrations/readme.txt":        {Data: []byte("")},
	}
	got, err := maxMigrationVersion(fsys, "migrations")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 33 {
		t.Fatalf("maxMigrationVersion = %d, want 33", got)
	}
}

func TestMaxMigrationVersion_Empty(t *testing.T) {
	got, err := maxMigrationVersion(fstest.MapFS{"migrations/readme.txt": {Data: []byte("")}}, "migrations")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Fatalf("maxMigrationVersion = %d, want 0", got)
	}
}
