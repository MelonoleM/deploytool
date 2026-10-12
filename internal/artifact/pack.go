package artifact

import (
	"archive/tar"
	"compress/gzip"
	"deploytool/internal/state"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"
)

func Pack(src, out, project, version string) error {
	f, e := os.Create(out)
	if e != nil {
		return e
	}
	defer f.Close()
	g := gzip.NewWriter(f)
	defer g.Close()
	tw := tar.NewWriter(g)
	defer tw.Close()
	m := Manifest{Project: project, Version: version, GeneratedAt: time.Now().UTC().Format(time.RFC3339)}
	e = filepath.Walk(src, func(p string, i os.FileInfo, e error) error {
		if e != nil || i.IsDir() {
			return e
		}
		rel, e := filepath.Rel(src, p)
		if e != nil {
			return e
		}
		if rel == "manifest.json" {
			return nil
		}
		sum, size, e := HashFile(p)
		if e != nil {
			return e
		}
		m.Files = append(m.Files, state.FileEntry{Path: filepath.ToSlash(rel), SHA256: sum, Size: size})
		h, e := tar.FileInfoHeader(i, "")
		if e != nil {
			return e
		}
		h.Name = filepath.ToSlash(rel)
		if e = tw.WriteHeader(h); e != nil {
			return e
		}
		in, e := os.Open(p)
		if e != nil {
			return e
		}
		_, e = io.Copy(tw, in)
		in.Close()
		return e
	})
	if e != nil {
		return e
	}
	b, e := json.MarshalIndent(m, "", "  ")
	if e != nil {
		return e
	}
	if e = tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0644, Size: int64(len(b))}); e != nil {
		return e
	}
	_, e = tw.Write(b)
	return e
}
