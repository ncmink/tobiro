package sshconfig

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ParseFile reads an OpenSSH config path and follows Include directives.
// Paths in Include are relative to the file that contains them; globs are
// expanded. A file is included at most once per top-level parse (cycles are
// skipped). Match and ProxyJump are not expanded.
func ParseFile(path string) ([]Host, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	lines, err := readExpandedLines(abs, map[string]struct{}{})
	if err != nil {
		return nil, err
	}
	text := strings.Join(lines, "\n")
	return dedupeByAlias(Parse([]byte(text))), nil
}

func readExpandedLines(filePath string, visited map[string]struct{}) ([]string, error) {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return nil, err
	}
	if _, ok := visited[abs]; ok {
		return nil, nil
	}
	visited[abs] = struct{}{}

	raw, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	baseDir := filepath.Dir(abs)

	var out []string
	for _, line := range bytes.Split(raw, []byte{'\n'}) {
		trimmed := strings.TrimSpace(stripComment(string(line)))
		if trimmed == "" {
			out = append(out, string(line))
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 2 && strings.EqualFold(fields[0], "include") {
			pattern := strings.Join(fields[1:], " ")
			paths, err := expandIncludePatterns(baseDir, pattern)
			if err != nil {
				return nil, fmt.Errorf("%s: include %q: %w", abs, pattern, err)
			}
			for _, p := range paths {
				sub, err := readExpandedLines(p, visited)
				if err != nil {
					return nil, err
				}
				out = append(out, sub...)
			}
			continue
		}
		out = append(out, string(line))
	}
	return out, nil
}

func expandIncludePatterns(baseDir, pattern string) ([]string, error) {
	tokens := splitIncludePatterns(pattern)
	var paths []string
	for _, tok := range tokens {
		tok = expandHome(tok)
		globPath := tok
		if !filepath.IsAbs(globPath) {
			globPath = filepath.Join(baseDir, tok)
		}
		matches, err := filepath.Glob(globPath)
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil || info.IsDir() {
				continue
			}
			paths = append(paths, m)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func splitIncludePatterns(pattern string) []string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil
	}
	var out []string
	var b strings.Builder
	inQuote := false
	quote := byte(0)
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch {
		case !inQuote && (c == '"' || c == '\''):
			inQuote = true
			quote = c
		case inQuote && c == quote:
			inQuote = false
			out = append(out, b.String())
			b.Reset()
		case !inQuote && (c == ' ' || c == '\t'):
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
		default:
			b.WriteByte(c)
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

func expandHome(path string) string {
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func dedupeByAlias(hosts []Host) []Host {
	if len(hosts) < 2 {
		return hosts
	}
	index := make(map[string]int, len(hosts))
	out := make([]Host, 0, len(hosts))
	for _, h := range hosts {
		key := strings.ToLower(strings.TrimSpace(h.Alias))
		if i, ok := index[key]; ok {
			out[i] = h
		} else {
			index[key] = len(out)
			out = append(out, h)
		}
	}
	return out
}
