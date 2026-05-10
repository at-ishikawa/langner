package testrunner

import "testing"

func TestSubstitute(t *testing.T) {
	deps := Deps{
		TempDir: "/tmp/work",
		Vars:    map[string]string{"NB": "alpha"},
	}
	got := substitute("$TEMP_DIR/$NB.yml", deps)
	want := "/tmp/work/alpha.yml"
	if got != want {
		t.Fatalf("substitute: want %q, got %q", want, got)
	}
}
