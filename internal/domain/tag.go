package domain

import (
	"errors"
	"fmt"
	"strings"
)

// NameTagKey is the reserved tag key that always mirrors the connection name.
const NameTagKey = "Name"

const (
	maxTagsPerProfile = 50
	maxTagKeyLen      = 128
	maxTagValueLen    = 256
)

// Tag is a key/value label on a connection. The Name tag is not
// stored; it is implied by VPNProfile.Name.
type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// DisplayTags returns all tags for UI and search, including the virtual Name tag.
func DisplayTags(name string, tags []Tag) []Tag {
	out := make([]Tag, 0, 1+len(tags))
	out = append(out, Tag{Key: NameTagKey, Value: strings.TrimSpace(name)})
	for _, t := range tags {
		out = append(out, t)
	}
	return out
}

// NormaliseTags trims keys and values and drops empty keys.
func NormaliseTags(tags []Tag) []Tag {
	if len(tags) == 0 {
		return nil
	}
	out := make([]Tag, 0, len(tags))
	for _, t := range tags {
		key := strings.TrimSpace(t.Key)
		if key == "" {
			continue
		}
		out = append(out, Tag{
			Key:   key,
			Value: strings.TrimSpace(t.Value),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ValidateTags enforces tag limits and rejects the reserved Name key.
func ValidateTags(tags []Tag) error {
	tags = NormaliseTags(tags)
	if len(tags) > maxTagsPerProfile {
		return fmt.Errorf("at most %d tags are allowed", maxTagsPerProfile)
	}
	seen := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		if t.Key == NameTagKey {
			return fmt.Errorf("tag key %q is reserved; connection name is the Name tag", NameTagKey)
		}
		if len(t.Key) > maxTagKeyLen {
			return fmt.Errorf("tag key is too long (max %d characters)", maxTagKeyLen)
		}
		if len(t.Value) > maxTagValueLen {
			return fmt.Errorf("tag value for %q is too long (max %d characters)", t.Key, maxTagValueLen)
		}
		if _, dup := seen[t.Key]; dup {
			return fmt.Errorf("duplicate tag key %q", t.Key)
		}
		seen[t.Key] = struct{}{}
	}
	return nil
}

// MatchTagQuery reports whether a connection matches a dashboard tag query.
//
//   - "key=value" matches an exact key (case-sensitive) whose value contains value (case-insensitive).
//   - "Name=x" matches profile name containing x (case-insensitive).
//   - "key=" matches any connection that has that tag key (any value).
func MatchTagQuery(name string, tags []Tag, query string) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return true
	}
	key, value, hasEq := strings.Cut(query, "=")
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	value = strings.TrimSpace(value)

	display := DisplayTags(name, tags)
	if hasEq {
		if key == NameTagKey {
			if value == "" {
				return true
			}
			return strings.Contains(strings.ToLower(strings.TrimSpace(name)), strings.ToLower(value))
		}
		for _, t := range display {
			if t.Key != key {
				continue
			}
			if value == "" {
				return true
			}
			return strings.Contains(strings.ToLower(t.Value), strings.ToLower(value))
		}
		return false
	}
	// Plain substring: match any tag key or value.
	lower := strings.ToLower(query)
	for _, t := range display {
		if strings.Contains(strings.ToLower(t.Key), lower) ||
			strings.Contains(strings.ToLower(t.Value), lower) {
			return true
		}
	}
	return false
}

// ErrReservedNameTag is returned when input tries to store a Name tag explicitly.
var ErrReservedNameTag = errors.New("Name tag is reserved")