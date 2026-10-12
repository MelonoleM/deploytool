package state

import (
	"testing"
)

// Each recorded operation must keep its own fields; a shared JSON tag would
// silently collapse action/service/result into one key on disk.
func TestLogRoundTripsOperationFields(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Log("install", "nodejs-web", "success"); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	if len(reopened.data.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %#v", reopened.data.Operations)
	}
	op := reopened.data.Operations[0]
	if op.Action != "install" || op.Service != "nodejs-web" || op.Result != "success" {
		t.Fatalf("operation fields were not preserved: %#v", op)
	}
}

func TestPutGetAndDelete(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Put("web", ServiceRecord{State: "Running", PID: 42}); err != nil {
		t.Fatal(err)
	}
	rec, ok := s.Get("web")
	if !ok || rec.PID != 42 || rec.State != "Running" {
		t.Fatalf("unexpected record: %#v ok=%t", rec, ok)
	}
	if err = s.Delete("web"); err != nil {
		t.Fatal(err)
	}
	if _, ok = s.Get("web"); ok {
		t.Fatal("record should have been deleted")
	}
}
