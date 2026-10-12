package artifact

import (
	"archive/tar"
	"compress/gzip"
	"deploytool/internal/state"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ExtractTarGz(src, dest string) ([]state.FileEntry, error) {
	f, e := os.Open(src)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	g, e := gzip.NewReader(f)
	if e != nil {
		return nil, e
	}
	defer g.Close()
	tr := tar.NewReader(g)
	out := []state.FileEntry{}
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
		target := filepath.Join(dest, filepath.FromSlash(h.Name))
		clean := filepath.Clean(target)
		base := filepath.Clean(dest) + string(os.PathSeparator)
		if clean != filepath.Clean(dest) && !strings.HasPrefix(clean, base) {
			return nil, fmt.Errorf("archive contains unsafe path %q", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if e = os.MkdirAll(clean, os.FileMode(h.Mode)); e != nil {
				return nil, e
			}
		case tar.TypeReg:
			if e = os.MkdirAll(filepath.Dir(clean), 0755); e != nil {
				return nil, e
			}
			o, e := os.OpenFile(clean, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(h.Mode))
			if e != nil {
				return nil, e
			}
			_, e = io.Copy(o, tr)
			ce := o.Close()
			if e != nil {
				return nil, e
			}
			if ce != nil {
				return nil, ce
			}
			sum, size, e := HashFile(clean)
			if e != nil {
				return nil, e
			}
			out = append(out, state.FileEntry{Path: clean, SHA256: sum, Size: size})
		}
	}
	return out, nil
}
