package engine

import (
	"context"
	"deploytool/internal/adapter"
	"deploytool/internal/artifact"
	"deploytool/internal/config"
	"deploytool/internal/logger"
	"deploytool/internal/process"
	"deploytool/internal/state"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"
)

type Report struct {
	Success   bool                    `json:"success"`
	Services  []adapter.ServiceStatus `json:"services"`
	StartedAt time.Time               `json:"started_at"`
	EndedAt   time.Time               `json:"ended_at"`
}

// Progress reports one service's transition during a lifecycle operation. Index and
// Total are zero-based positions within the selected set, so a UI can render a
// progress bar for a known service list without knowing the engine's batch order.
type Progress struct {
	Service string `json:"service"`
	Phase   string `json:"phase"` // running | done | failed | skipped
	Reason  string `json:"reason,omitempty"`
	Index   int    `json:"index"`
	Total   int    `json:"total"`
}
type Orchestrator struct {
	Config      *config.Config
	Store       *state.Store
	Registry    *adapter.Registry
	Log         *logger.Logger
	Root        string
	interactive bool
	dryRun      bool
}

func New(c *config.Config, dry bool) (*Orchestrator, error) {
	root := c.Vars["install_root"]
	if root == "" {
		return nil, fmt.Errorf("vars.install_root is required")
	}
	if !filepath.IsAbs(root) {
		if abs, err := filepath.Abs(root); err == nil {
			root = abs
		}
	}
	c.Vars["install_root"] = root
	for i := range c.Services {
		if wd := c.Services[i].Workdir; wd != "" && !filepath.IsAbs(wd) {
			c.Services[i].Workdir = filepath.Join(root, filepath.FromSlash(wd))
		}
	}
	st, e := state.Open(root)
	if e != nil {
		return nil, e
	}
	r := adapter.NewRegistry()
	adapter.RegisterBuiltins(r)
	l, e := logger.New(root)
	if e != nil {
		return nil, e
	}
	return &Orchestrator{Config: c, Store: st, Registry: r, Log: l, Root: root, dryRun: dry}, nil
}

// Close releases resources owned by the orchestrator, in particular the log file.
func (o *Orchestrator) Close() error {
	if o.Log != nil {
		return o.Log.Close()
	}
	return nil
}

// SetInteractive marks whether the operator is driving this process from an
// interactive terminal. Only in that case is start_mode "console" honored; a web
// server or desktop launcher must always run such services in the background.
func (o *Orchestrator) SetInteractive(v bool) { o.interactive = v }

func (o *Orchestrator) selected(only []string) ([]config.Service, error) {
	want := map[string]bool{}
	for _, n := range only {
		if n != "" {
			want[n] = true
		}
	}
	out := []config.Service{}
	for _, s := range o.Config.Services {
		if len(want) == 0 || want[s.Name] {
			out = append(out, s)
		}
	}
	if len(want) > 0 && len(out) != len(want) {
		return nil, fmt.Errorf("one or more requested services do not exist")
	}
	return out, nil
}
func (o *Orchestrator) Install(ctx context.Context, only []string, progress ...func(Progress)) *Report {
	return o.install(ctx, only, true, progress...)
}

// InstallWithoutStart installs services but leaves them stopped, so starting is a
// separate, explicit step.
func (o *Orchestrator) InstallWithoutStart(ctx context.Context, only []string, progress ...func(Progress)) *Report {
	return o.install(ctx, only, false, progress...)
}
func (o *Orchestrator) install(ctx context.Context, only []string, start bool, progress ...func(Progress)) *Report {
	return o.run(ctx, "install", only, false, start, progress...)
}
func (o *Orchestrator) Start(ctx context.Context, only []string, progress ...func(Progress)) *Report {
	return o.run(ctx, "start", only, false, false, progress...)
}
func (o *Orchestrator) Stop(ctx context.Context, only []string, progress ...func(Progress)) *Report {
	return o.run(ctx, "stop", only, true, false, progress...)
}
func (o *Orchestrator) Uninstall(ctx context.Context, only []string, purge bool, progress ...func(Progress)) *Report {
	r := o.run(ctx, "uninstall", only, true, false, progress...)
	if purge && r.Success && !o.dryRun {
		_ = o.Store.Purge()
	}
	return r
}

// Update consumes a self-contained tar.gz package with a manifest.json. Files are copied
// only after checksum verification; a per-service backup is restored on any failure.
func (o *Orchestrator) Update(ctx context.Context, pkg string, only []string, progress ...func(Progress)) *Report {
	notify := func(Progress) {}
	if len(progress) > 0 && progress[0] != nil {
		notify = progress[0]
	}
	begin := time.Now()
	r := Report{Success: true, StartedAt: begin}
	tmp, e := os.MkdirTemp("", "deploytool-update-")
	if e == nil {
		defer os.RemoveAll(tmp)
		_, e = artifact.ExtractTarGz(pkg, tmp)
	}
	if e != nil {
		r.Success = false
		r.Services = []adapter.ServiceStatus{{Reason: "unpack update package: " + e.Error()}}
		r.EndedAt = time.Now()
		return &r
	}
	m, e := artifact.ReadManifest(tmp)
	if e == nil {
		e = artifact.Verify(tmp, m)
	}
	if e != nil {
		r.Success = false
		r.Services = []adapter.ServiceStatus{{Reason: "verify update package: " + e.Error()}}
		r.EndedAt = time.Now()
		return &r
	}
	svcs, e := o.selected(only)
	if e != nil {
		r.Success = false
		r.Services = []adapter.ServiceStatus{{Reason: e.Error()}}
		r.EndedAt = time.Now()
		return &r
	}
	for i, s := range svcs {
		if !s.IsEnabled() {
			notify(Progress{Service: s.Name, Phase: "skipped", Reason: "disabled by configuration", Index: i, Total: len(svcs)})
			r.Services = append(r.Services, adapter.ServiceStatus{Name: s.Name, Skipped: true, Reason: "disabled by configuration"})
			continue
		}
		// The installed record is only a gate here: startOne persists the new PID and
		// Running state, so writing the pre-stop record back would retain a stale PID.
		if _, ok := o.Store.Get(s.Name); !ok {
			notify(Progress{Service: s.Name, Phase: "failed", Reason: "update failed: service is not installed", Index: i, Total: len(svcs)})
			r.Success = false
			r.Services = append(r.Services, adapter.ServiceStatus{Name: s.Name, Reason: "update failed: service is not installed"})
			continue
		}
		notify(Progress{Service: s.Name, Phase: "running", Index: i, Total: len(svcs)})
		backup := filepath.Join(backupDir(o.Root, s.Name), fmt.Sprint(time.Now().UnixNano()))
		changed := []state.FileEntry{}
		for _, f := range m.Files {
			src := filepath.Join(tmp, filepath.FromSlash(f.Path))
			if _, e := os.Stat(src); e != nil {
				continue
			}
			changed = append(changed, f)
		}
		er := os.MkdirAll(backup, 0755)
		if er == nil {
			er = o.stopOne(ctx, s)
		}
		if er == nil {
			for _, f := range changed {
				dst := filepath.Join(s.Workdir, filepath.FromSlash(f.Path))
				if _, e := os.Stat(dst); e == nil {
					er = copyFile(dst, filepath.Join(backup, filepath.FromSlash(f.Path)))
					if er != nil {
						break
					}
				}
				if er = copyFile(filepath.Join(tmp, filepath.FromSlash(f.Path)), dst); er != nil {
					break
				}
			}
		}
		if er == nil {
			er = o.startOne(ctx, s)
		}
		rolled := false
		if er != nil && o.Config.UpdatePolicy.RollbackOnFailure {
			rolled = true
			_ = o.stopOne(ctx, s)
			_ = restoreTree(backup, s.Workdir)
			_ = o.startOne(ctx, s)
		}
		if er != nil {
			notify(Progress{Service: s.Name, Phase: "failed", Reason: "update failed: " + er.Error(), Index: i, Total: len(svcs)})
			r.Success = false
			r.Services = append(r.Services, adapter.ServiceStatus{Name: s.Name, Installed: true, RolledBack: rolled, Reason: "update failed: " + er.Error()})
			_ = o.Store.Log("update", s.Name, "failed")
		} else {
			notify(Progress{Service: s.Name, Phase: "done", Index: i, Total: len(svcs)})
			r.Services = append(r.Services, o.status(ctx, s))
			_ = o.Store.Log("update", s.Name, "success")
		}
	}
	r.EndedAt = time.Now()
	return &r
}
func (o *Orchestrator) Status(ctx context.Context, only []string) *Report {
	begin := time.Now()
	svcs, e := o.selected(only)
	r := Report{Success: e == nil, StartedAt: begin}
	if e != nil {
		r.Services = []adapter.ServiceStatus{{Reason: e.Error()}}
		r.EndedAt = time.Now()
		return &r
	}
	for _, s := range svcs {
		r.Services = append(r.Services, o.status(ctx, s))
	}
	for _, x := range r.Services {
		if !x.Skipped && (!x.Installed || !x.Healthy && x.Running) {
			r.Success = false
		}
	}
	r.EndedAt = time.Now()
	return &r
}
func (o *Orchestrator) run(ctx context.Context, action string, only []string, reverse bool, start bool, progress ...func(Progress)) *Report {
	notify := func(Progress) {}
	if len(progress) > 0 && progress[0] != nil {
		notify = progress[0]
	}
	begin := time.Now()
	r := Report{Success: true, StartedAt: begin}
	svcs, e := o.selected(only)
	if e != nil {
		r.Success = false
		r.Services = []adapter.ServiceStatus{{Reason: e.Error()}}
		r.EndedAt = time.Now()
		return &r
	}
	batches, e := Batches(svcs, reverse)
	if e != nil {
		r.Success = false
		r.Services = []adapter.ServiceStatus{{Reason: e.Error()}}
		r.EndedAt = time.Now()
		return &r
	}
	by := map[string]config.Service{}
	for _, s := range svcs {
		by[s.Name] = s
	}
	// Assign stable progress indices once, in the order batches are processed.
	total := len(svcs)
	idxBy := map[string]int{}
	idx := 0
	for _, batch := range batches {
		for _, name := range batch {
			idxBy[name] = idx
			idx++
		}
	}

	type outcome struct {
		status adapter.ServiceStatus
		failed bool
	}

	known := map[string]adapter.ServiceStatus{}
	for _, batch := range batches {
		results := make([]outcome, len(batch))
		skips := make([]string, len(batch))

		var wg sync.WaitGroup
		for i, name := range batch {
			s := by[name]
			if !s.IsEnabled() {
				skips[i] = "disabled by configuration"
				results[i] = outcome{status: adapter.ServiceStatus{Name: name, Skipped: true, Reason: "disabled by configuration"}}
				continue
			}
			if !reverse && blocked(s, known) {
				skips[i] = "dependency was not successfully deployed"
				results[i] = outcome{status: adapter.ServiceStatus{Name: name, Skipped: true, Reason: "dependency was not successfully deployed"}, failed: true}
				continue
			}
			wg.Add(1)
			go func(name string, s config.Service, pos int) {
				defer wg.Done()
				notify(Progress{Service: name, Phase: "running", Index: idxBy[name], Total: total})
				var er error
				switch action {
				case "install":
					er = o.installOne(ctx, s, start)
				case "start":
					er = o.startOne(ctx, s)
				case "stop":
					er = o.stopOne(ctx, s)
				case "uninstall":
					er = o.uninstallOne(ctx, s)
				}
				x := o.status(ctx, s)
				if er != nil {
					o.Log.Error("service operation failed", "service", name, "action", action, "error", er)
					x.Reason = fmt.Sprintf("%s failed: %v", action, er)
					_ = o.Store.Log(action, name, "failed")
					notify(Progress{Service: name, Phase: "failed", Reason: x.Reason, Index: idxBy[name], Total: total})
				} else {
					o.Log.Info("service operation completed", "service", name, "action", action)
					_ = o.Store.Log(action, name, "success")
					notify(Progress{Service: name, Phase: "done", Index: idxBy[name], Total: total})
				}
				results[pos] = outcome{status: x, failed: er != nil}
			}(name, s, i)
		}
		wg.Wait()

		batchFailed := false
		for i, name := range batch {
			res := results[i]
			if skips[i] != "" {
				notify(Progress{Service: name, Phase: "skipped", Reason: skips[i], Index: idxBy[name], Total: total})
			}
			known[name] = res.status
			r.Services = append(r.Services, res.status)
			if res.failed {
				r.Success = false
				batchFailed = true
			}
		}
		if batchFailed && o.Config.FailFast {
			r.EndedAt = time.Now()
			return &r
		}
	}
	r.EndedAt = time.Now()
	return &r
}
func blocked(s config.Service, k map[string]adapter.ServiceStatus) bool {
	for _, d := range s.DependsOn {
		x, ok := k[d]
		// Reason may be an informational status message (for example, no health
		// check is configured); it is not itself a dependency failure.
		if ok && (!x.Installed || x.Skipped) {
			return true
		}
	}
	return false
}
func (o *Orchestrator) installOne(ctx context.Context, s config.Service, start bool) error {
	if o.dryRun {
		return nil
	}
	rec, _ := o.Store.Get(s.Name)
	if rec.State == string(Installed) || rec.State == string(Running) || rec.State == string(Stopped) {
		// Install is idempotent: the files are already in place, so only make sure
		// the service runs. startOne is a no-op for a process that is still alive.
		if s.Start != "" {
			return o.startOne(ctx, s)
		}
		return nil
	}
	if e := o.Store.Put(s.Name, state.ServiceRecord{State: string(Installing)}); e != nil {
		return e
	}
	inv, files, e := o.executeSteps(ctx, s, s.Install.Steps)
	if e != nil {
		_ = o.Store.Put(s.Name, state.ServiceRecord{State: string(Failed), Reason: e.Error()})
		return e
	}
	rec = state.ServiceRecord{State: string(Installed), Files: files, Inverses: inv}
	if e = o.Store.Put(s.Name, rec); e != nil {
		return e
	}
	if start && s.Start != "" {
		return o.startOne(ctx, s)
	}
	return nil
}
func (o *Orchestrator) startOne(ctx context.Context, s config.Service) error {
	if o.dryRun {
		return nil
	}
	rec, ok := o.Store.Get(s.Name)
	if !ok || rec.State == string(NotInstalled) {
		return fmt.Errorf("service is not installed")
	}
	if rec.PID > 0 && process.Alive(rec.PID) {
		return nil
	}
	if s.StartMode == "console" && o.interactive && process.IsTerminal(os.Stdout) {
		return o.startConsole(ctx, s)
	}
	var pid int
	var e error
	pidPath := pidFilePath(o.Root, s.Name)
	if s.StartMode == "console" {
		pid, e = process.RunConsoleWindow(s.Start, s.Workdir, s.Env, pidPath)
	} else {
		pid, e = process.RunBackground(s.Start, s.Workdir, filepath.Join(o.Root, ".deploytool", "logs", s.Name+".log"), s.Env, pidPath)
	}
	if e != nil {
		return e
	}
	rec.PID = pid
	rec.State = string(Running)
	rec.Reason = ""
	if e = o.Store.Put(s.Name, rec); e != nil {
		return e
	}
	okHealth, why := process.Check(ctx, s.Healthcheck, pid)
	if !okHealth {
		rec.State = string(Failed)
		rec.Reason = why
		_ = o.Store.Put(s.Name, rec)
		return fmt.Errorf("health check: %s", why)
	}
	return nil
}

// startConsole runs a service in the foreground and blocks until it exits. It is
// only used when the caller has an interactive terminal.
func (o *Orchestrator) startConsole(ctx context.Context, s config.Service) error {
	rec, _ := o.Store.Get(s.Name)
	rec.PID = 0
	rec.State = string(Running)
	rec.Reason = "running in attached console; Ctrl+C stops the service"
	if e := o.Store.Put(s.Name, rec); e != nil {
		return e
	}
	e := process.RunConsole(ctx, s.Start, s.Workdir, s.Env)
	if e != nil {
		rec.State, rec.Reason = string(Failed), "console process exited: "+e.Error()
	} else {
		rec.State, rec.Reason = string(Stopped), "console process exited"
	}
	_ = o.Store.Put(s.Name, rec)
	return e
}

func (o *Orchestrator) stopOne(_ context.Context, s config.Service) error {
	if o.dryRun {
		return nil
	}
	rec, ok := o.Store.Get(s.Name)
	if !ok {
		return nil
	}
	if e := process.Stop(s.Stop, s.Workdir, rec.PID); e != nil {
		return e
	}
	rec.PID = 0
	rec.State = string(Stopped)
	rec.Reason = ""
	_ = process.RemovePID(pidFilePath(o.Root, s.Name))
	return o.Store.Put(s.Name, rec)
}
func (o *Orchestrator) uninstallOne(ctx context.Context, s config.Service) error {
	if o.dryRun {
		return nil
	}
	rec, ok := o.Store.Get(s.Name)
	if !ok {
		return nil
	}
	_ = o.stopOne(ctx, s)
	// Clear tool-owned runtime bookkeeping before replaying the recorded inverse
	// actions, so the service work directory can be removed once it is empty.
	removeBackups(backupDir(o.Root, s.Name))
	for i := len(rec.Inverses) - 1; i >= 0; i-- {
		v := rec.Inverses[i]
		if e := undo(ctx, v); e != nil {
			return fmt.Errorf("undo %s: %w", v.Action, e)
		}
	}
	return o.Store.Delete(s.Name)
}
func (o *Orchestrator) status(ctx context.Context, s config.Service) adapter.ServiceStatus {
	if !s.IsEnabled() {
		return adapter.ServiceStatus{Name: s.Name, Skipped: true, Reason: "disabled by configuration"}
	}
	rec, ok := o.Store.Get(s.Name)
	if !ok {
		return adapter.ServiceStatus{Name: s.Name, Reason: "not installed"}
	}
	run := process.Alive(rec.PID)
	// Services without a start command are file-only; there is no process state to
	// report, so they fall through to the configured health check.
	if !run && strings.TrimSpace(s.Start) != "" {
		// Probing a deliberately stopped service would only report a connection
		// refusal, which reads as a fault instead of the state the operator asked for.
		why := "stopped"
		if rec.PID > 0 {
			why = "process is not running"
		}
		if rec.Reason != "" {
			why = rec.Reason
		}
		return adapter.ServiceStatus{Name: s.Name, Installed: true, Reason: why}
	}
	healthy, why := process.Check(ctx, s.Healthcheck, rec.PID)
	if rec.Reason != "" {
		why = rec.Reason
	}
	return adapter.ServiceStatus{Name: s.Name, Installed: true, Running: run, Healthy: healthy, Reason: why}
}
func (o *Orchestrator) executeSteps(ctx context.Context, s config.Service, steps []config.Step) ([]state.Inverse, []state.FileEntry, error) {
	out := []state.Inverse{}
	files := []state.FileEntry{}
	for _, step := range steps {
		e := o.step(ctx, s, step, &out, &files)
		if e != nil {
			return out, files, fmt.Errorf("%s: %w", step.Action, e)
		}
	}
	return out, files, nil
}
func (o *Orchestrator) step(ctx context.Context, s config.Service, x config.Step, inv *[]state.Inverse, files *[]state.FileEntry) error {
	val := func(v string) (string, error) { return config.Render(v, s, o.Config.Vars) }
	// Source artifacts are config-relative; output paths keep using {{ .workdir }}.
	srcPath := func(v string) (string, error) {
		out, err := val(v)
		if err != nil {
			return "", err
		}
		if out == "" || filepath.IsAbs(out) || o.Config.BaseDir == "" {
			return out, nil
		}
		return filepath.Join(o.Config.BaseDir, out), nil
	}
	switch x.Action {
	case "mkdir":
		p, e := val(x.Path)
		if e != nil {
			return e
		}
		if e = os.MkdirAll(p, 0755); e != nil {
			return e
		}
		*inv = append(*inv, state.Inverse{Action: "rmdir_if_empty", Path: p})
		return nil
	case "extract":
		src, e := srcPath(x.Src)
		if e != nil {
			return e
		}
		dest, e := val(x.Dest)
		if e != nil {
			return e
		}
		fs, e := artifact.ExtractTarGz(src, dest)
		if e != nil {
			return e
		}
		*files = append(*files, fs...)
		// Replay removes files before directories, so record directory pruning first.
		recordFileParents(inv, fs)
		for _, f := range fs {
			*inv = append(*inv, state.Inverse{Action: "remove_file", Path: f.Path})
		}
		return nil
	case "copy_file":
		src, e := srcPath(x.Src)
		if e != nil {
			return e
		}
		dst, e := val(x.Dest)
		if e != nil {
			return e
		}
		return copyTracked(src, dst, inv, files)
	case "render_template":
		src, e := srcPath(x.Src)
		if e != nil {
			return e
		}
		dst, e := val(x.Dest)
		if e != nil {
			return e
		}
		b, e := os.ReadFile(src)
		if e != nil {
			return e
		}
		t, e := template.New("file").Option("missingkey=error").Parse(string(b))
		if e != nil {
			return e
		}
		var w strings.Builder
		if e = t.Execute(&w, map[string]any{"Vars": o.Config.Vars, "workdir": s.Workdir, "env": s.Env, "install": s.Install}); e != nil {
			return e
		}
		recordCreatedDirs(inv, filepath.Dir(dst))
		if e = os.MkdirAll(filepath.Dir(dst), 0755); e != nil {
			return e
		}
		if e = os.WriteFile(dst, []byte(w.String()), 0644); e != nil {
			return e
		}
		return track(dst, inv, files)
	case "write_env":
		p, e := val(x.Path)
		if e != nil {
			return e
		}
		recordCreatedDirs(inv, filepath.Dir(p))
		if e = os.MkdirAll(filepath.Dir(p), 0755); e != nil {
			return e
		}
		if e = os.WriteFile(p, []byte(x.Content), 0600); e != nil {
			return e
		}
		return track(p, inv, files)
	case "set_permission":
		p, e := val(x.Path)
		if e != nil {
			return e
		}
		var mode os.FileMode
		_, e = fmt.Sscanf(x.Mode, "%o", &mode)
		if e != nil {
			return e
		}
		return os.Chmod(p, mode)
	case "run_script":
		if x.UndoScript == "" {
			*inv = append(*inv, state.Inverse{Action: "manual", Path: "", Script: "run_script lacks undo_script"})
		}
		return run(ctx, x.Command)
	default:
		return fmt.Errorf("unsupported install action %q", x.Action)
	}
}
func track(p string, inv *[]state.Inverse, files *[]state.FileEntry) error {
	sum, size, e := artifact.HashFile(p)
	if e != nil {
		return e
	}
	*files = append(*files, state.FileEntry{Path: p, SHA256: sum, Size: size})
	*inv = append(*inv, state.Inverse{Action: "remove_file", Path: p})
	return nil
}
func copyTracked(src, dst string, inv *[]state.Inverse, files *[]state.FileEntry) error {
	in, e := os.Open(src)
	if e != nil {
		return e
	}
	defer in.Close()
	recordCreatedDirs(inv, filepath.Dir(dst))
	if e = os.MkdirAll(filepath.Dir(dst), 0755); e != nil {
		return e
	}
	o, e := os.Create(dst)
	if e != nil {
		return e
	}
	_, e = io.Copy(o, in)
	ce := o.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return track(dst, inv, files)
}

// recordCreatedDirs records rmdir_if_empty inverses for the directory levels that
// MkdirAll is about to create, shallowest first so that reverse replay prunes
// leaves before their parents.
func recordCreatedDirs(inv *[]state.Inverse, dir string) {
	missing := []string{}
	for p := filepath.Clean(dir); ; {
		if _, e := os.Stat(p); e == nil {
			break
		}
		missing = append(missing, p)
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	for i := len(missing) - 1; i >= 0; i-- {
		*inv = append(*inv, state.Inverse{Action: "rmdir_if_empty", Path: missing[i]})
	}
}

// recordFileParents records the distinct parent directories of freshly extracted
// files so uninstall prunes them once the files themselves are gone.
func recordFileParents(inv *[]state.Inverse, files []state.FileEntry) {
	seen := map[string]bool{}
	for _, f := range files {
		d := filepath.Dir(f.Path)
		if d == "" || d == "." || seen[d] {
			continue
		}
		seen[d] = true
		*inv = append(*inv, state.Inverse{Action: "rmdir_if_empty", Path: d})
	}
}
func copyFile(src, dst string) error {
	in, e := os.Open(src)
	if e != nil {
		return e
	}
	defer in.Close()
	if e = os.MkdirAll(filepath.Dir(dst), 0755); e != nil {
		return e
	}
	out, e := os.Create(dst)
	if e != nil {
		return e
	}
	_, e = io.Copy(out, in)
	ce := out.Close()
	if e != nil {
		return e
	}
	return ce
}
func restoreTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, e error) error {
		if e != nil || info.IsDir() {
			return e
		}
		rel, e := filepath.Rel(src, p)
		if e != nil {
			return e
		}
		return copyFile(p, filepath.Join(dst, rel))
	})
}

// pidFilePath returns the tool-owned pid file location for a service, kept under
// the install root's .deploytool directory so the service work directory stays clean.
func pidFilePath(root, name string) string {
	return filepath.Join(root, ".deploytool", "pids", name+".pid")
}

// backupDir returns the per-service update backup directory under .deploytool.
func backupDir(root, name string) string {
	return filepath.Join(root, ".deploytool", "backups", name)
}

// removeBackups drops the per-service update backup directory. Backups only
// protect an in-flight update, so uninstall discards them.
func removeBackups(dir string) {
	if strings.TrimSpace(dir) == "" {
		return
	}
	_ = os.RemoveAll(dir)
}
func run(ctx context.Context, cmd string) error {
	if strings.TrimSpace(cmd) == "" {
		return nil
	}
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.CommandContext(ctx, "cmd", "/C", cmd)
	} else {
		c = exec.CommandContext(ctx, "sh", "-c", cmd)
	}
	c.SysProcAttr = process.HiddenProcAttr()
	return c.Run()
}
func undo(ctx context.Context, v state.Inverse) error {
	switch v.Action {
	case "remove_file":
		if e := os.Remove(v.Path); e != nil && !os.IsNotExist(e) {
			return e
		}
		return nil
	case "rmdir_if_empty":
		e := os.Remove(v.Path)
		if e != nil && !os.IsNotExist(e) {
			return nil
		}
		return nil
	case "manual":
		return fmt.Errorf("manual cleanup required: %s", v.Script)
	case "script":
		return run(ctx, v.Script)
	default:
		return fmt.Errorf("unknown inverse %q", v.Action)
	}
}
func sorted(xs []string) { sort.Strings(xs) }
