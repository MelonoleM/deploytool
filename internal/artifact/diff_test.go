package artifact

import (
	"deploytool/internal/state"
	"testing"
)

func TestCompare(t *testing.T) {
	old := []state.FileEntry{{Path: "same", SHA256: "1"}, {Path: "mod", SHA256: "1"}, {Path: "gone", SHA256: "1"}}
	fresh := []state.FileEntry{{Path: "same", SHA256: "1"}, {Path: "mod", SHA256: "2"}, {Path: "add", SHA256: "1"}}
	d := Compare(old, fresh)
	if len(d.Added) != 1 || len(d.Modified) != 1 || len(d.Removed) != 1 {
		t.Fatalf("unexpected diff: %#v", d)
	}
}
