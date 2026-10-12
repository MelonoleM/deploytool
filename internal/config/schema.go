package config

import "time"

type Config struct {
	APIVersion    string            `yaml:"apiVersion"`
	ConfigVersion string            `yaml:"config_version"`
	Project       Project           `yaml:"project"`
	Vars          map[string]string `yaml:"vars"`
	Services      []Service         `yaml:"services"`
	Provision     []Step            `yaml:"provision"`
	Welcome       Welcome           `yaml:"welcome"`
	UpdatePolicy  UpdatePolicy      `yaml:"update_policy"`
	FailFast      bool              `yaml:"fail_fast"`
	// BaseDir is the directory of the configuration file. Relative source paths in
	// install steps resolve against it, so a config and its artifacts stay portable
	// regardless of the process working directory. It is populated by Load.
	BaseDir string `yaml:"-" json:"-"`
}
type Project struct {
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`
	// Portal is the project's external home page (for example a web dashboard).
	// It is opened from the console and is not edited through the UI.
	Portal string `yaml:"portal" json:"portal"`
}
type Welcome struct {
	Title       string   `yaml:"title" json:"title"`
	Description string   `yaml:"description" json:"description"`
	Guide       string   `yaml:"guide" json:"guide"`
	Features    []string `yaml:"features" json:"features"`
}

// Defaults returns the welcome content with sensible fallbacks applied, so both the
// console and the form editor show the same text when the config omits it.
func (w Welcome) Defaults() Welcome {
	if w.Title == "" {
		w.Title = "欢迎使用 deploytool"
	}
	if w.Description == "" {
		w.Description = "一个离线优先的声明式部署与运维控制台。完成一次配置后，就能在这里统一完成服务的安装、启动、监控与更新。"
	}
	if w.Guide == "" {
		w.Guide = "准备好了吗？让我们开始配置吧"
	}
	if len(w.Features) == 0 {
		w.Features = []string{
			"声明式配置 · 可保存可重置",
			"按依赖顺序安装 · 进度可视",
			"已装 / 未装分区 · 一键启停",
		}
	}
	return w
}

type UpdatePolicy struct {
	Strategy           string `yaml:"strategy"`
	BackupBeforeUpdate bool   `yaml:"backup_before_update"`
	RollbackOnFailure  bool   `yaml:"rollback_on_failure"`
}
type Service struct {
	Name        string            `yaml:"name"`
	Type        string            `yaml:"type"`
	Enabled     *bool             `yaml:"enabled"`
	Workdir     string            `yaml:"workdir"`
	DependsOn   []string          `yaml:"depends_on"`
	Install     Install           `yaml:"install"`
	Start       string            `yaml:"start"`
	StartMode   string            `yaml:"start_mode"` // background (default) | console
	Stop        string            `yaml:"stop"`
	StopSignal  string            `yaml:"stop_signal"`
	Env         map[string]string `yaml:"env"`
	Healthcheck Healthcheck       `yaml:"healthcheck"`
	Update      Update            `yaml:"update"`
}
type Install struct {
	Artifact string `yaml:"artifact"`
	Steps    []Step `yaml:"steps"`
}
type Step struct {
	Name       string `yaml:"name"`
	Action     string `yaml:"action"`
	Src        string `yaml:"src"`
	Dest       string `yaml:"dest"`
	Path       string `yaml:"path"`
	Mode       string `yaml:"mode"`
	Content    string `yaml:"content"`
	Command    string `yaml:"command"`
	UndoScript string `yaml:"undo_script"`
}
type Healthcheck struct {
	Type     string        `yaml:"type"`
	URL      string        `yaml:"url"`
	Address  string        `yaml:"address"`
	Command  string        `yaml:"command"`
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
	Retries  int           `yaml:"retries"`
}
type Update struct {
	Strategy      string `yaml:"strategy"`
	PreStopHook   string `yaml:"pre_stop_hook"`
	PostStartHook string `yaml:"post_start_hook"`
}

func (s Service) IsEnabled() bool { return s.Enabled == nil || *s.Enabled }
