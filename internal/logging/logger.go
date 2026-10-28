// Package logging provides an in-app activity log that is safe by construction:
// secrets registered with the logger are redacted from every message before it
// is stored or printed.
package logging

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Level is the severity of a log entry.
type Level string

const (
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// Entry is one line in the activity log shown in the UI.
type Entry struct {
	Time    time.Time `json:"time"`
	Level   Level     `json:"level"`
	Message string    `json:"message"`
}

const maxEntries = 500

// sensitiveKeyPattern catches key=value shapes for sensitive keys, so even a
// caller that formats its own string cannot leak a password.
var sensitiveKeyPattern = regexp.MustCompile(`(?i)\b(password|passwd|pwd|secret|otp|token|passcode|pin)\s*[=:]\s*\S+`)

// Logger records activity for the UI and stdout, redacting secrets.
//
// Callers register a secret for the duration of a connect attempt; every
// message is scrubbed of that literal value before being written anywhere.
type Logger struct {
	mu      sync.RWMutex
	entries []Entry
	secrets map[string]struct{}

	sink func(Entry)
}

// New creates a logger.
func New() *Logger {
	return &Logger{
		entries: make([]Entry, 0, 64),
		secrets: make(map[string]struct{}),
	}
}

// SetSink installs a callback invoked for every entry (used to stream the log
// to the frontend).
func (l *Logger) SetSink(fn func(Entry)) {
	l.mu.Lock()
	l.sink = fn
	l.mu.Unlock()
}

// Protect registers a literal secret value to redact from all future messages.
// Returns a function that unregisters it once the operation completes.
func (l *Logger) Protect(secret string) func() {
	if len(secret) < 3 {
		// Too short to redact meaningfully without mangling ordinary text.
		return func() {}
	}
	l.mu.Lock()
	l.secrets[secret] = struct{}{}
	l.mu.Unlock()

	return func() {
		l.mu.Lock()
		delete(l.secrets, secret)
		l.mu.Unlock()
	}
}

// Redact removes registered secrets and sensitive key=value pairs from a string.
func (l *Logger) Redact(msg string) string {
	l.mu.RLock()
	secrets := make([]string, 0, len(l.secrets))
	for s := range l.secrets {
		secrets = append(secrets, s)
	}
	l.mu.RUnlock()

	for _, s := range secrets {
		msg = strings.ReplaceAll(msg, s, "***")
	}
	return sensitiveKeyPattern.ReplaceAllStringFunc(msg, func(match string) string {
		idx := strings.IndexAny(match, "=:")
		if idx < 0 {
			return "***"
		}
		return match[:idx+1] + "***"
	})
}

func (l *Logger) write(level Level, format string, args ...any) {
	entry := Entry{
		Time:    time.Now(),
		Level:   level,
		Message: l.Redact(fmt.Sprintf(format, args...)),
	}

	l.mu.Lock()
	l.entries = append(l.entries, entry)
	if len(l.entries) > maxEntries {
		l.entries = l.entries[len(l.entries)-maxEntries:]
	}
	sink := l.sink
	l.mu.Unlock()

	log.Printf("[%s] %s", level, entry.Message)
	if sink != nil {
		sink(entry)
	}
}

// Info records an informational entry.
func (l *Logger) Info(format string, args ...any) { l.write(LevelInfo, format, args...) }

// Warn records a warning entry.
func (l *Logger) Warn(format string, args ...any) { l.write(LevelWarn, format, args...) }

// Error records an error entry.
func (l *Logger) Error(format string, args ...any) { l.write(LevelError, format, args...) }

// Entries returns a copy of the retained log.
func (l *Logger) Entries() []Entry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]Entry, len(l.entries))
	copy(out, l.entries)
	return out
}

// Clear empties the retained log.
func (l *Logger) Clear() {
	l.mu.Lock()
	l.entries = l.entries[:0]
	l.mu.Unlock()
}
