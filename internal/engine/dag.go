package engine

import (
	"deploytool/internal/config"
	"fmt"
	"sort"
)

// Batches returns deterministic, dependency-safe parallel execution groups.
//
// Only dependencies that are part of the supplied selection constrain the order:
// an operator who runs `stop web` on a project where `web` depends on `api` asked
// for exactly one service, so the missing dependency counts as satisfied rather
// than as an unsatisfiable (or cyclic) graph.
func Batches(services []config.Service, reverse bool) ([][]string, error) {
	by := map[string]config.Service{}
	degree := map[string]int{}
	next := map[string][]string{}
	for _, s := range services {
		by[s.Name] = s
		degree[s.Name] = 0
	}
	for _, s := range services {
		for _, d := range s.DependsOn {
			if _, selected := by[d]; !selected {
				continue
			}
			degree[s.Name]++
			next[d] = append(next[d], s.Name)
		}
	}
	ready := []string{}
	for n, d := range degree {
		if d == 0 {
			ready = append(ready, n)
		}
	}
	sort.Strings(ready)
	out := [][]string{}
	seen := 0
	for len(ready) > 0 {
		batch := ready
		ready = nil
		if reverse {
			for i, j := 0, len(batch)-1; i < j; i, j = i+1, j-1 {
				batch[i], batch[j] = batch[j], batch[i]
			}
		}
		out = append(out, batch)
		seen += len(batch)
		for _, n := range batch {
			for _, v := range next[n] {
				degree[v]--
				if degree[v] == 0 {
					ready = append(ready, v)
				}
			}
		}
		sort.Strings(ready)
	}
	if seen != len(services) {
		return nil, fmt.Errorf("dependency cycle: %s", cycle(by))
	}
	if reverse {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out, nil
}
func cycle(by map[string]config.Service) string {
	state := map[string]int{}
	stack := []string{}
	var visit func(string) string
	visit = func(n string) string {
		state[n] = 1
		stack = append(stack, n)
		for _, d := range by[n].DependsOn {
			if state[d] == 1 {
				for i, x := range stack {
					if x == d {
						return fmt.Sprintf("%v", append(stack[i:], d))
					}
				}
			}
			if state[d] == 0 {
				if r := visit(d); r != "" {
					return r
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[n] = 2
		return ""
	}
	for n := range by {
		if state[n] == 0 {
			if r := visit(n); r != "" {
				return r
			}
		}
	}
	return "unknown"
}
