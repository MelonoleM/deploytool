package engine

import (
	"deploytool/internal/config"
	"strings"
	"testing"
)

func TestBatches(t *testing.T) {
	s := []config.Service{{Name: "db", Type: "exe", Workdir: "x"}, {Name: "api", Type: "exe", Workdir: "x", DependsOn: []string{"db"}}, {Name: "web", Type: "exe", Workdir: "x", DependsOn: []string{"api"}}}
	b, e := Batches(s, false)
	if e != nil {
		t.Fatal(e)
	}
	if len(b) != 3 || b[0][0] != "db" || b[2][0] != "web" {
		t.Fatalf("unexpected batches: %#v", b)
	}
}
func TestBatchesReportsCycle(t *testing.T) {
	s := []config.Service{{Name: "a", Type: "exe", Workdir: "x", DependsOn: []string{"b"}}, {Name: "b", Type: "exe", Workdir: "x", DependsOn: []string{"a"}}}
	_, e := Batches(s, false)
	if e == nil || !strings.Contains(e.Error(), "cycle") {
		t.Fatalf("expected cycle, got %v", e)
	}
}

// Selecting one service must not fail just because its dependencies were left out
// of the selection; that is what `stop <service>` and `start <service>` do.
func TestBatchesIgnoresDependenciesOutsideSelection(t *testing.T) {
	s := []config.Service{{Name: "api", Type: "exe", Workdir: "x"}, {Name: "web", Type: "exe", Workdir: "x", DependsOn: []string{"api"}}}
	b, e := Batches(s[1:], false)
	if e != nil {
		t.Fatalf("subset selection reported an error: %v", e)
	}
	if len(b) != 1 || len(b[0]) != 1 || b[0][0] != "web" {
		t.Fatalf("unexpected batches: %#v", b)
	}
}

func TestBatchesReverseStopsDependentsFirst(t *testing.T) {
	s := []config.Service{{Name: "db", Type: "exe", Workdir: "x"}, {Name: "api", Type: "exe", Workdir: "x", DependsOn: []string{"db"}}, {Name: "web", Type: "exe", Workdir: "x", DependsOn: []string{"api"}}}
	b, e := Batches(s, true)
	if e != nil {
		t.Fatal(e)
	}
	if len(b) != 3 || b[0][0] != "web" || b[1][0] != "api" || b[2][0] != "db" {
		t.Fatalf("unexpected reverse batches: %#v", b)
	}
}
