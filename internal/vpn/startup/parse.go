package startup

import (
	"regexp"
	"strings"
)

var disabledLine = regexp.MustCompile(`"([^"]+)"\s*=>\s*(enabled|disabled)`)

func parseDisabledMap(raw string) map[string]bool {
	out := map[string]bool{}
	for _, match := range disabledLine.FindAllStringSubmatch(raw, -1) {
		out[match[1]] = match[2] == "disabled"
	}
	return out
}

func matchesName(want []string, got string) bool {
	got = strings.ToLower(strings.TrimSpace(got))
	for _, name := range want {
		if strings.ToLower(name) == got {
			return true
		}
	}
	return false
}
