package state

import "time"

type FileEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type Inverse struct {
	Action string `json:"action"`
	Path   string `json:"path"`
	Script string `json:"script,omitempty"`
}
type ServiceRecord struct {
	State     string      `json:"state"`
	PID       int         `json:"pid,omitempty"`
	Reason    string      `json:"reason,omitempty"`
	UpdatedAt time.Time   `json:"updated_at"`
	Files     []FileEntry `json:"files,omitempty"`
	Inverses  []Inverse   `json:"inverses,omitempty"`
}
type Data struct {
	Services   map[string]ServiceRecord `json:"services"`
	Operations []Operation              `json:"operations"`
}
type Operation struct {
	Time    time.Time `json:"time"`
	Action  string    `json:"action"`
	Service string    `json:"service"`
	Result  string    `json:"result"`
}
