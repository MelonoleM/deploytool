package process

import (
	"context"
	"deploytool/internal/config"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func Check(ctx context.Context, h config.Healthcheck, pid int) (bool, string) {
	if h.Type == "" {
		return true, "no healthcheck configured"
	}
	timeout := h.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	tries := h.Retries
	if tries < 1 {
		tries = 1
	}
	interval := h.Interval
	if interval <= 0 {
		interval = time.Second
	}
	for i := 0; i < tries; i++ {
		ok, why := once(ctx, h, pid, timeout)
		if ok {
			return true, "healthy"
		}
		if i == tries-1 {
			return false, why
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err().Error()
		case <-time.After(interval):
		}
	}
	return false, "health check exhausted"
}
func once(ctx context.Context, h config.Healthcheck, pid int, t time.Duration) (bool, string) {
	switch h.Type {
	case "process":
		if Alive(pid) {
			return true, ""
		}
		return false, "process is not running"
	case "tcp":
		c, e := net.DialTimeout("tcp", h.Address, t)
		if e == nil {
			c.Close()
			return true, ""
		}
		return false, "TCP probe failed: " + e.Error()
	case "http":
		r, e := (&http.Client{Timeout: t}).Get(h.URL)
		if e == nil && r.Body != nil {
			r.Body.Close()
		}
		if e == nil && r.StatusCode >= 200 && r.StatusCode < 400 {
			return true, ""
		}
		if e != nil {
			return false, "HTTP probe failed: " + e.Error()
		}
		return false, fmt.Sprintf("HTTP probe returned %d", r.StatusCode)
	case "script":
		var c *exec.Cmd
		if runtime.GOOS == "windows" {
			c = exec.CommandContext(ctx, "cmd", "/C", h.Command)
		} else {
			c = exec.CommandContext(ctx, "sh", "-c", h.Command)
		}
		c.SysProcAttr = HiddenProcAttr()
		if e := c.Run(); e != nil {
			return false, "health script failed: " + e.Error()
		}
		return true, ""
	default:
		return false, "unsupported healthcheck type " + strings.TrimSpace(h.Type)
	}
}
