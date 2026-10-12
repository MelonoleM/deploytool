package artifact

import "deploytool/internal/state"

type Diff struct{ Added, Modified, Removed []state.FileEntry }

func Compare(old, fresh []state.FileEntry) Diff {
	om := map[string]state.FileEntry{}
	nm := map[string]state.FileEntry{}
	for _, f := range old {
		om[f.Path] = f
	}
	for _, f := range fresh {
		nm[f.Path] = f
	}
	d := Diff{}
	for p, n := range nm {
		if o, ok := om[p]; !ok {
			d.Added = append(d.Added, n)
		} else if o.SHA256 != n.SHA256 {
			d.Modified = append(d.Modified, n)
		}
	}
	for p, o := range om {
		if _, ok := nm[p]; !ok {
			d.Removed = append(d.Removed, o)
		}
	}
	return d
}
