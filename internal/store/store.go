// Package store persists application data (profiles, credential metadata,
// per-profile preferences) as JSON in the user config directory.
//
// It deliberately holds NO secrets. Credentials are persisted with a SecretRef
// only; the password itself lives in the OS credential store.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"tobiro/internal/domain"
)

const (
	appDirName   = "tobiro"
	dataFileName = "data.json"

	// schemaVer 3 added update check preferences.
	schemaVer = 3
)

// Data is the on-disk document.
type Data struct {
	SchemaVersion int                                  `json:"schemaVersion"`
	Profiles      []domain.VPNProfile                  `json:"profiles"`
	Credentials   []domain.Credential                  `json:"credentials"`
	Preferences   map[string]domain.ProfilePreferences `json:"preferences"`

	// Groups is keyed by group name, which is the same string profiles carry.
	// Only groups with non-default presentation are kept here.
	Groups map[string]domain.GroupSettings `json:"groups"`

	// LastNativeFillProfileID is which connection ⌘I fills after restart.
	LastNativeFillProfileID string `json:"lastNativeFillProfileId,omitempty"`

	// FillHotkey stores the global fill shortcut, e.g. "cmd+i". Empty means default.
	FillHotkey string `json:"fillHotkey,omitempty"`

	// FillHotkeyIntroSeen suppresses the first-run fill shortcut dialog.
	FillHotkeyIntroSeen bool `json:"fillHotkeyIntroSeen,omitempty"`

	// AutoCheckUpdates enables a daily GitHub release check at startup.
	AutoCheckUpdates bool `json:"autoCheckUpdates,omitempty"`

	// LastUpdateCheckAt is when Tobiro last contacted GitHub (RFC3339).
	LastUpdateCheckAt string `json:"lastUpdateCheckAt,omitempty"`
}

func newData() *Data {
	return &Data{
		SchemaVersion:     schemaVer,
		Profiles:          []domain.VPNProfile{},
		Credentials:       []domain.Credential{},
		Preferences:       map[string]domain.ProfilePreferences{},
		Groups:            map[string]domain.GroupSettings{},
		AutoCheckUpdates:  true,
	}
}

// Store is a mutex-guarded JSON document with atomic writes.
type Store struct {
	mu   sync.RWMutex
	path string
	data *Data
}

// DefaultPath resolves the platform user-config location for the data file.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(dir, appDirName, dataFileName), nil
}

// Open loads the store at path, creating an empty document if absent.
func Open(path string) (*Store, error) {
	s := &Store{path: path, data: newData()}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, s.persist()
		}
		return nil, fmt.Errorf("read data file: %w", err)
	}
	if len(raw) == 0 {
		return s, nil
	}

	var loaded Data
	if err := json.Unmarshal(raw, &loaded); err != nil {
		return nil, fmt.Errorf("parse data file %s: %w", path, err)
	}
	if loaded.Preferences == nil {
		loaded.Preferences = map[string]domain.ProfilePreferences{}
	}
	if loaded.Profiles == nil {
		loaded.Profiles = []domain.VPNProfile{}
	}
	if loaded.Credentials == nil {
		loaded.Credentials = []domain.Credential{}
	}
	if loaded.Groups == nil {
		loaded.Groups = map[string]domain.GroupSettings{}
	}
	// Every profile written before SSH tunnels existed is a VPN profile. Saying
	// so on load means nothing downstream has to keep asking what an empty kind
	// meant.
	for i := range loaded.Profiles {
		loaded.Profiles[i].Kind = loaded.Profiles[i].Kind.Normalise()
	}
	// Defensive: a secret must never round-trip through this file even if an
	// older build or hand edit put one there.
	for i := range loaded.Credentials {
		loaded.Credentials[i].Secret = ""
	}
	if loaded.SchemaVersion < 3 {
		loaded.AutoCheckUpdates = true
	}
	loaded.SchemaVersion = schemaVer
	s.data = &loaded
	return s, nil
}

// Path reports the data file location (shown in Settings).
func (s *Store) Path() string { return s.path }

// Read runs fn against a snapshot of the data under a read lock.
func (s *Store) Read(fn func(d *Data)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fn(s.data)
}

// Update mutates the document and persists it atomically. The change is rolled
// back in memory if the write fails, so disk and memory cannot diverge.
func (s *Store) Update(fn func(d *Data) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot, err := json.Marshal(s.data)
	if err != nil {
		return fmt.Errorf("snapshot data: %w", err)
	}
	if err := fn(s.data); err != nil {
		return err
	}
	if err := s.persist(); err != nil {
		var restored Data
		if jsonErr := json.Unmarshal(snapshot, &restored); jsonErr == nil {
			s.data = &restored
		}
		return err
	}
	return nil
}

// persist writes the document via a temp file + rename. Callers hold the lock.
func (s *Store) persist() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode data: %w", err)
	}

	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".data-*.json")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("replace data file: %w", err)
	}
	return nil
}
