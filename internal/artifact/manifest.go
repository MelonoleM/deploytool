package artifact

import (
	"crypto/sha256"
	"deploytool/internal/state"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Manifest struct {
	Project, Version string
	GeneratedAt      string            `json:"generated_at"`
	Files            []state.FileEntry `json:"files"`
}

func HashFile(path string) (string, int64, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", 0, e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, f)
	if e != nil {
		return "", 0, e
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
func ReadManifest(root string) (Manifest, error) {
	var m Manifest
	b, e := os.ReadFile(filepath.Join(root, "manifest.json"))
	if e != nil {
		return m, e
	}
	e = json.Unmarshal(b, &m)
	return m, e
}
func Verify(root string, m Manifest) error {
	for _, f := range m.Files {
		p := filepath.Join(root, filepath.FromSlash(f.Path))
		sum, size, e := HashFile(p)
		if e != nil {
			return fmt.Errorf("verify %s: %w", f.Path, e)
		}
		if sum != f.SHA256 || size != f.Size {
			return fmt.Errorf("integrity check failed for %s", f.Path)
		}
	}
	return nil
}
