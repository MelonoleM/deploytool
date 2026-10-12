package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"deploytool/internal/config"
	"deploytool/internal/engine"
	"deploytool/internal/state"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	root := filepath.Join(t.TempDir(), "runtime")
	c := &config.Config{
		APIVersion: "v1",
		Project:    config.Project{Name: "demo", Version: "1.0.0"},
		Vars:       map[string]string{"install_root": root},
		Services: []config.Service{
			{Name: "app", Type: "exe", Workdir: filepath.Join(root, "app")},
		},
	}
	o, err := engine.New(c, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = o.Close() })
	return &Server{O: o}
}

func TestConsoleServesIndexAndOverview(t *testing.T) {
	ts := httptest.NewServer(newTestServer(t).Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("index status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("index content-type = %q", ct)
	}

	resp, err = http.Get(ts.URL + "/api/overview")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var ov struct {
		Project  struct{ Name string }
		Services []struct {
			Name   string
			Status struct {
				Installed bool
			}
		}
	}
	if err = json.NewDecoder(resp.Body).Decode(&ov); err != nil {
		t.Fatal(err)
	}
	if ov.Project.Name != "demo" || len(ov.Services) != 1 || ov.Services[0].Name != "app" {
		t.Fatalf("unexpected overview: %+v", ov)
	}
}

func TestInstallRunsAsPollableTask(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/install", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		TaskID string `json:"task_id"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if created.TaskID == "" {
		t.Fatal("install did not return a task id")
	}

	var tsk task
	for i := 0; i < 200; i++ {
		resp, err = http.Get(ts.URL + "/api/tasks/" + created.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.NewDecoder(resp.Body).Decode(&tsk); err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if tsk.Done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !tsk.Done || tsk.Report == nil || !tsk.Report.Success {
		t.Fatalf("install task did not succeed: %+v", tsk)
	}

	resp, err = http.Get(ts.URL + "/api/logs?service=app&tail=10")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logs status = %d", resp.StatusCode)
	}
}

func TestPackRejectsMissingPaths(t *testing.T) {
	ts := httptest.NewServer(newTestServer(t).Handler())
	defer ts.Close()
	resp, err := http.Post(ts.URL+"/api/pack", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("pack status = %d, want 400", resp.StatusCode)
	}
}

// The first-run flow must be driven by install completion, not browser memory, so
// a failed or never-completed install keeps returning to the welcome screen.
func TestProvisionedTracksCompletion(t *testing.T) {
	s := newTestServer(t)
	o := s.orchestrator()
	if s.provisioned() {
		t.Fatal("a fresh install root must not be provisioned")
	}
	if err := o.Store.Put("app", state.ServiceRecord{State: string(engine.Installed)}); err != nil {
		t.Fatal(err)
	}
	if !s.provisioned() {
		t.Fatal("installed service should mark the console provisioned")
	}
	if err := o.Store.Put("app", state.ServiceRecord{State: string(engine.Failed)}); err != nil {
		t.Fatal(err)
	}
	if s.provisioned() {
		t.Fatal("a failed service must keep the console unprovisioned")
	}
}
