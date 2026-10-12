// Package webui serves the browser console and its JSON API. Assets are embedded
// so the whole product remains a single offline binary with no external CDN.
package webui

import (
	"context"
	"deploytool/internal/adapter"
	"deploytool/internal/artifact"
	"deploytool/internal/config"
	"deploytool/internal/engine"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed static
var staticFS embed.FS

type task struct {
	Done     bool              `json:"done"`
	Report   *engine.Report    `json:"report,omitempty"`
	Progress []engine.Progress `json:"progress,omitempty"`
}

// Server exposes the deploytool engine over HTTP. Mutating operations are
// serialized so that, for example, an install cannot race an uninstall, and a
// config reload cannot swap the engine out from under a running action.
type Server struct {
	O          *engine.Orchestrator
	oMu        sync.RWMutex
	ConfigPath string
	Initial    []byte
	// Native marks a native-window build: the portal link is opened in the
	// system default browser instead of a new tab inside the window.
	Native bool

	mu       sync.Mutex // guards tasks and nextID
	actionMu sync.Mutex // serializes mutating actions and config reloads
	tasks    map[string]*task
	nextID   int64
}

type actionRequest struct {
	Only    []string `json:"only"`
	Purge   bool     `json:"purge"`
	Package string   `json:"package"`
	Src     string   `json:"src"`
	Out     string   `json:"out"`
	Yaml    string   `json:"yaml"`
	Name    string   `json:"name"`
	Enabled *bool    `json:"enabled"`
	Start   bool     `json:"start"`
}

type serviceHealth struct {
	Type    string `json:"type,omitempty"`
	URL     string `json:"url,omitempty"`
	Address string `json:"address,omitempty"`
}

type serviceView struct {
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Enabled     bool                   `json:"enabled"`
	Workdir     string                 `json:"workdir"`
	DependsOn   []string               `json:"depends_on"`
	StartMode   string                 `json:"start_mode"`
	Start       string                 `json:"start"`
	Healthcheck *serviceHealth         `json:"healthcheck,omitempty"`
	PID         int                    `json:"pid"`
	Status      *adapter.ServiceStatus `json:"status"`
}

func (s *Server) Serve(host string, port int) error {
	if host == "" {
		host = "127.0.0.1"
	}
	return http.ListenAndServe(fmt.Sprintf("%s:%d", host, port), s.Handler())
}

// Handler builds the HTTP routes; it is exported separately so tests can exercise
// the API without binding a network port.
func (s *Server) Handler() http.Handler {
	if s.tasks == nil {
		s.tasks = map[string]*task{}
	}
	assets, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(assets))))
	mux.HandleFunc("/", s.index)
	mux.HandleFunc("/api/bootstrap", s.bootstrap)
	mux.HandleFunc("/api/overview", s.overview)
	mux.HandleFunc("/api/status", s.status)
	mux.HandleFunc("/api/config", s.config)
	mux.HandleFunc("/api/config/form", s.configForm)
	mux.HandleFunc("/api/config/service/form", s.configServiceForm)
	mux.HandleFunc("/api/config/reset", s.resetConfig)
	mux.HandleFunc("/api/config/service", s.setServiceEnabled)
	mux.HandleFunc("/api/install", s.install)
	mux.HandleFunc("/api/uninstall", s.uninstall)
	mux.HandleFunc("/api/start", s.start)
	mux.HandleFunc("/api/stop", s.stop)
	mux.HandleFunc("/api/update", s.update)
	mux.HandleFunc("/api/pack", s.pack)
	mux.HandleFunc("/api/tasks/", s.task)
	mux.HandleFunc("/api/logs", s.logs)
	mux.HandleFunc("/api/mode", s.mode)
	mux.HandleFunc("/api/open", s.open)
	mux.HandleFunc("/api/update/check", s.updateCheck)
	mux.HandleFunc("/api/update/diff", s.updateDiff)
	mux.HandleFunc("/api/update/dismiss", s.updateDismiss)
	mux.HandleFunc("/api/update/apply", s.updateApply)
	return mux
}

func (s *Server) orchestrator() *engine.Orchestrator {
	s.oMu.RLock()
	defer s.oMu.RUnlock()
	return s.O
}

func (s *Server) setOrchestrator(o *engine.Orchestrator) {
	s.oMu.Lock()
	s.O = o
	s.oMu.Unlock()
}

// reload rebuilds the engine from the current config file. It must be called while
// holding actionMu.
func (s *Server) reload() error {
	c, err := config.Load(s.ConfigPath)
	if err != nil {
		return err
	}
	o, err := engine.New(c, false)
	if err != nil {
		return err
	}
	old := s.orchestrator()
	s.setOrchestrator(o)
	if old != nil {
		_ = old.Close()
	}
	return nil
}

// overlayExists reports whether a runtime override exists for the base config.
func (s *Server) overlayExists() bool {
	overlay, err := config.OverlayPathFor(s.ConfigPath)
	if err != nil {
		return false
	}
	_, err = os.Stat(overlay)
	return err == nil
}

// writeOverlay ensures the runtime override exists, applies fn to it, and reloads.
// On failure it restores the previous override content.
func (s *Server) writeOverlay(installRoot string, fn func(overlay string) error) error {
	oldRoot := ""
	if installRoot != "" {
		var err error
		if oldRoot, err = config.CurrentRuntimeDir(s.ConfigPath); err != nil {
			return err
		}
		if err := config.SetRuntimeDir(s.ConfigPath, installRoot); err != nil {
			return err
		}
	}
	overlay, err := config.EnsureOverlay(s.ConfigPath)
	if err != nil {
		return err
	}
	previous, _ := os.ReadFile(overlay)
	if err := fn(overlay); err != nil {
		return err
	}
	s.actionMu.Lock()
	err = s.reload()
	s.actionMu.Unlock()
	if err != nil {
		_ = os.WriteFile(overlay, previous, 0644)
		s.actionMu.Lock()
		_ = s.reload()
		s.actionMu.Unlock()
		return err
	}
	// When the install directory changed, the old root's override and logs
	// (including the startup deploytool.jsonl) are stale and would otherwise
	// remain on disk at the previous location.
	if oldRoot != "" && !config.SamePath(oldRoot, installRoot) {
		_ = os.Remove(config.OverlayPath(oldRoot))
		_ = os.RemoveAll(filepath.Join(oldRoot, ".deploytool", "logs"))
	}
	return nil
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	b, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "index not embedded", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}

func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) {
	o := s.orchestrator()
	if s.provisioned() {
		_ = config.RecordBaseline(s.ConfigPath, o.Root)
	}
	welcome := o.Config.Welcome.Defaults()
	writeJSON(w, map[string]any{
		"project":      o.Config.Project,
		"install_root": o.Root,
		"installed":    o.Store.Names(),
		"provisioned":  s.provisioned(),
		"welcome":      welcome,
	})
}

// provisioned reports whether installation is complete: every enabled service has
// reached a settled state (installed, running, or stopped). A service that never
// finished installing, or that failed, keeps the console in the first-run flow.
func (s *Server) provisioned() bool {
	o := s.orchestrator()
	for _, svc := range o.Config.Services {
		if !svc.IsEnabled() {
			continue
		}
		rec, ok := o.Store.Get(svc.Name)
		if !ok {
			return false
		}
		switch rec.State {
		case string(engine.Installed), string(engine.Running), string(engine.Stopped):
		default:
			return false
		}
	}
	return true
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	o := s.orchestrator()
	report := o.Status(r.Context(), nil)
	byName := map[string]adapter.ServiceStatus{}
	for _, st := range report.Services {
		byName[st.Name] = st
	}
	out := struct {
		Project struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"project"`
		InstallRoot string        `json:"install_root"`
		Services    []serviceView `json:"services"`
	}{}
	out.Project.Name = o.Config.Project.Name
	out.Project.Version = o.Config.Project.Version
	out.InstallRoot = o.Root
	out.Services = []serviceView{}
	for _, svc := range o.Config.Services {
		pid := 0
		if rec, ok := o.Store.Get(svc.Name); ok {
			pid = rec.PID
		}
		st := byName[svc.Name]
		mode := svc.StartMode
		if mode == "" {
			mode = "background"
		}
		v := serviceView{
			Name:      svc.Name,
			Type:      svc.Type,
			Enabled:   svc.IsEnabled(),
			Workdir:   svc.Workdir,
			DependsOn: svc.DependsOn,
			StartMode: mode,
			Start:     svc.Start,
			PID:       pid,
			Status:    &st,
		}
		if svc.Healthcheck.Type != "" || svc.Healthcheck.URL != "" || svc.Healthcheck.Address != "" {
			v.Healthcheck = &serviceHealth{
				Type:    svc.Healthcheck.Type,
				URL:     svc.Healthcheck.URL,
				Address: svc.Healthcheck.Address,
			}
		}
		out.Services = append(out.Services, v)
	}
	writeJSON(w, out)
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.orchestrator().Status(r.Context(), nil))
}

func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		path := s.ConfigPath
		if overlay, err := config.OverlayPathFor(s.ConfigPath); err == nil {
			if _, statErr := os.Stat(overlay); statErr == nil {
				path = overlay
			}
		}
		b, err := os.ReadFile(path)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]any{
			"yaml":     string(b),
			"modified": s.overlayExists(),
			"path":     s.ConfigPath,
		})
	case http.MethodPost:
		req := decodeAction(r)
		if strings.TrimSpace(req.Yaml) == "" {
			writeError(w, http.StatusBadRequest, "yaml is required")
			return
		}
		if err := s.writeOverlay("", func(overlay string) error {
			return os.WriteFile(overlay, []byte(req.Yaml), 0644)
		}); err != nil {
			writeError(w, http.StatusBadRequest, "配置无效："+err.Error())
			return
		}
		writeJSON(w, map[string]bool{"modified": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) resetConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	if err := config.RemoveOverlay(s.ConfigPath); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.actionMu.Lock()
	err := s.reload()
	s.actionMu.Unlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"modified": false})
}

func (s *Server) setServiceEnabled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	req := decodeAction(r)
	if req.Name == "" || req.Enabled == nil {
		writeError(w, http.StatusBadRequest, "name and enabled are required")
		return
	}
	if err := s.writeOverlay("", func(overlay string) error {
		return config.SetServiceEnabled(overlay, req.Name, *req.Enabled)
	}); err != nil {
		writeError(w, http.StatusBadRequest, "配置无效："+err.Error())
		return
	}
	writeJSON(w, map[string]bool{"modified": true})
}

func (s *Server) configForm(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		form, err := config.ReadForm(s.ConfigPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]any{
			"form":     form,
			"modified": s.overlayExists(),
		})
	case http.MethodPost:
		var form config.Form
		if err := json.NewDecoder(r.Body).Decode(&form); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.writeOverlay(form.InstallRoot, func(overlay string) error {
			return config.WriteForm(overlay, form)
		}); err != nil {
			writeError(w, http.StatusBadRequest, "配置无效："+err.Error())
			return
		}
		writeJSON(w, map[string]bool{"modified": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) configServiceForm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var svc config.FormService
	if err := json.NewDecoder(r.Body).Decode(&svc); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if svc.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := s.writeOverlay("", func(overlay string) error {
		return config.WriteService(overlay, svc)
	}); err != nil {
		writeError(w, http.StatusBadRequest, "配置无效："+err.Error())
		return
	}
	writeJSON(w, map[string]bool{"modified": true})
}

func (s *Server) install(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	req := decodeAction(r)
	id := s.runTask(func(progress func(engine.Progress)) *engine.Report {
		if req.Start {
			return s.orchestrator().Install(context.Background(), req.Only, progress)
		}
		return s.orchestrator().InstallWithoutStart(context.Background(), req.Only, progress)
	})
	writeJSON(w, map[string]string{"task_id": id})
}

func (s *Server) uninstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	req := decodeAction(r)
	id := s.runTask(func(progress func(engine.Progress)) *engine.Report {
		return s.orchestrator().Uninstall(context.Background(), req.Only, req.Purge, progress)
	})
	writeJSON(w, map[string]string{"task_id": id})
}

func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	req := decodeAction(r)
	id := s.runTask(func(progress func(engine.Progress)) *engine.Report {
		return s.orchestrator().Start(context.Background(), req.Only, progress)
	})
	writeJSON(w, map[string]string{"task_id": id})
}

func (s *Server) stop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	req := decodeAction(r)
	id := s.runTask(func(progress func(engine.Progress)) *engine.Report {
		return s.orchestrator().Stop(context.Background(), req.Only, progress)
	})
	writeJSON(w, map[string]string{"task_id": id})
}

func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	req := decodeAction(r)
	if strings.TrimSpace(req.Package) == "" {
		writeError(w, http.StatusBadRequest, "package path is required")
		return
	}
	id := s.runTask(func(progress func(engine.Progress)) *engine.Report {
		return s.orchestrator().Update(context.Background(), req.Package, req.Only, progress)
	})
	writeJSON(w, map[string]string{"task_id": id})
}

func (s *Server) pack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	req := decodeAction(r)
	if strings.TrimSpace(req.Src) == "" || strings.TrimSpace(req.Out) == "" {
		writeError(w, http.StatusBadRequest, "src and out are required")
		return
	}
	id := s.runTask(func(progress func(engine.Progress)) *engine.Report {
		rep := &engine.Report{Success: true, StartedAt: time.Now()}
		if e := artifact.Pack(req.Src, req.Out, s.orchestrator().Config.Project.Name, s.orchestrator().Config.Project.Version); e != nil {
			rep.Success = false
			rep.Services = []adapter.ServiceStatus{{Reason: e.Error()}}
		} else {
			rep.Services = []adapter.ServiceStatus{{Name: "pack", Installed: true, Reason: "wrote " + req.Out}}
		}
		rep.EndedAt = time.Now()
		return rep
	})
	writeJSON(w, map[string]string{"task_id": id})
}

func (s *Server) task(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	s.mu.Lock()
	t, ok := s.tasks[id]
	s.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, t)
}

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	if tail <= 0 || tail > 2000 {
		tail = 200
	}
	var rel string
	if service == "" {
		rel = filepath.Join(".deploytool", "logs", "deploytool.jsonl")
	} else if s.isService(service) {
		rel = filepath.Join(".deploytool", "logs", service+".log")
	} else {
		writeError(w, http.StatusNotFound, "unknown service")
		return
	}
	data, err := tailLines(filepath.Join(s.orchestrator().Root, rel), tail)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(data))
}

// mode reports the runtime context the frontend needs to open the project portal:
// whether it is running inside a native window and the configured portal URL.
func (s *Server) mode(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"native": s.Native,
		"portal": s.orchestrator().Config.Project.Portal,
	})
}

// open launches the requested URL in the system default browser. It exists for
// native-window builds where window.open would otherwise open a new WebView tab.
func (s *Server) open(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req struct {
		URL string `json:"url"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if strings.TrimSpace(req.URL) == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	if err := openDefaultBrowser(req.URL); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"opened": true})
}

// updateCheck reports whether a newer config template is available for a deployed
// console, and whether the operator previously deferred this version.
func (s *Server) updateCheck(w http.ResponseWriter, r *http.Request) {
	o := s.orchestrator()
	if !s.provisioned() {
		writeJSON(w, map[string]any{"has_update": false, "provisioned": false})
		return
	}
	current, _ := config.ConfigVersionFromPath(s.ConfigPath)
	baseline, _ := config.BaselineVersion(o.Root)
	st, _ := config.ReadUpdateState(o.Root)
	has := current != "" && config.CompareVersions(current, baseline) > 0
	dismissed := st.DismissedVersion != "" && config.CompareVersions(st.DismissedVersion, current) >= 0
	writeJSON(w, map[string]any{
		"has_update":       has,
		"provisioned":      true,
		"current_version":  current,
		"baseline_version": baseline,
		"dismissed":        dismissed,
	})
}

// updateDiff returns the field-level differences between the recorded baseline and
// the current base config.
func (s *Server) updateDiff(w http.ResponseWriter, r *http.Request) {
	o := s.orchestrator()
	changes, err := config.DiffUpdate(s.ConfigPath, o.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"changes": changes})
}

// updateDismiss records the current version as deferred so the startup prompt is
// suppressed until the next version ships.
func (s *Server) updateDismiss(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	o := s.orchestrator()
	current, err := config.ConfigVersionFromPath(s.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := config.WriteUpdateState(o.Root, config.UpdateState{DismissedVersion: current}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"dismissed": true})
}

// updateApply merges the official template changes into the user's override and
// reloads the engine.
func (s *Server) updateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	s.actionMu.Lock()
	defer s.actionMu.Unlock()
	if err := config.ApplyUpdate(s.ConfigPath, s.orchestrator().Root); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.reload(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"updated": true})
}

func (s *Server) isService(name string) bool {
	for _, svc := range s.orchestrator().Config.Services {
		if svc.Name == name {
			return true
		}
	}
	return false
}

func (s *Server) runTask(fn func(func(engine.Progress)) *engine.Report) string {
	s.mu.Lock()
	s.nextID++
	id := strconv.FormatInt(s.nextID, 36)
	t := &task{}
	s.tasks[id] = t
	s.mu.Unlock()
	go func() {
		s.actionMu.Lock()
		defer s.actionMu.Unlock()
		rep := fn(func(p engine.Progress) {
			s.mu.Lock()
			t.Progress = append(t.Progress, p)
			s.mu.Unlock()
		})
		s.mu.Lock()
		t.Report = rep
		t.Done = true
		s.mu.Unlock()
	}()
	return id
}

func decodeAction(r *http.Request) actionRequest {
	var req actionRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	return req
}

func tailLines(path string, n int) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(b) == 0 {
		return "", nil
	}
	lines := strings.Split(string(b), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n"), nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// openDefaultBrowser opens url in the system default browser. It is kept local to
// avoid importing internal/console (which imports this package and would cycle).
func openDefaultBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
