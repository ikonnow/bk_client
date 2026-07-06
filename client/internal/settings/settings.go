/*##### BEGIN GPL LICENSE BLOCK #####

  This program is free software; you can redistribute it and/or
  modify it under the terms of the GNU General Public License
  as published by the Free Software Foundation; either version 2
  of the License, or (at your option) any later version.

  This program is distributed in the hope that it will be useful,
  but WITHOUT ANY WARRANTY; without even the implied warranty of
  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
  GNU General Public License for more details.

  You should have received a copy of the GNU General Public License
  along with this program; if not, write to the Free Software Foundation,
  Inc., 51 Franklin Street, Fifth Floor, Boston, MA 02110-1301, USA.

##### END GPL LICENSE BLOCK #####*/

// Package settings implements the Client-owned settings store.
//
// The Client is the single source of truth for settings shared with connected
// plugins (Blender, Godot, ...). Plugins are required to sync from the Client:
// the current settings ride along on every /report response together with a
// monotonically increasing Revision, so a plugin can never permanently miss an
// update — the next poll always reconciles it. Plugins compare Revision and
// apply whenever it grows.
//
// Two kinds of data are stored:
//
//   - Shared settings: typed, Client-wide values every plugin must mirror. In
//     phase one this is just the Server the Client is connected to.
//   - Variables: free-form key/value pairs a plugin can ask the Client to keep
//     on its behalf. They can be stored globally (no plugin association) or
//     namespaced under a plugin name (e.g. "blender" -> "executable").
//
// VERSIONING: settings are stored per Client version. Each version keeps its own
// isolated set, and all previous versions remain available in the same file.
// When a new version starts without settings, it inherits a copy of the most
// recent previous version's settings (if any), so users keep their configuration
// across upgrades while never mutating the old version's data.
//
// CONCURRENCY: reads (the hot /report path) are lock-free via an atomically
// published, immutable Snapshot. Mutations take a mutex, update the on-disk file
// atomically, then publish a fresh Snapshot.
package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// EnvSettingsPath is the environment variable that, when set, overrides the
// default settings file location.
const EnvSettingsPath = "BLENDKIT_CLIENT_SETTINGS"

// FileName is the default settings file name used when the settings are stored
// next to the executable.
const FileName = "blendkit-client-settings.json"

// Shared holds the typed, Client-wide settings every connected plugin must
// mirror. Kept intentionally small in phase one.
type Shared struct {
	// Server is the Blendkit server address the Client is connected to and
	// that all plugins must use as the source of truth.
	Server string `json:"server"`
}

// versionSettings is the persisted, mutable state for a single Client version.
type versionSettings struct {
	Revision        uint64                       `json:"revision"`
	UpdatedAt       time.Time                    `json:"updated_at"`
	Shared          Shared                       `json:"shared"`
	GlobalVariables map[string]string            `json:"global_variables"`
	PluginVariables map[string]map[string]string `json:"plugin_variables"`
}

// diskFile is the on-disk layout: every known Client version's settings kept
// together so previous versions remain available for inheritance and history.
type diskFile struct {
	CurrentVersion string                      `json:"current_version"`
	Versions       map[string]*versionSettings `json:"versions"`
}

// Snapshot is an immutable, point-in-time view of the current version's
// settings. It is what gets broadcast to plugins and returned by the HTTP
// endpoints. All fields (including maps) must be treated as read-only.
type Snapshot struct {
	Version         string                       `json:"version"`
	Revision        uint64                       `json:"revision"`
	UpdatedAt       time.Time                    `json:"updated_at"`
	Shared          Shared                       `json:"shared"`
	GlobalVariables map[string]string            `json:"global_variables"`
	PluginVariables map[string]map[string]string `json:"plugin_variables"`
}

// Store is the concurrency-safe, persistent settings store.
type Store struct {
	mu      sync.Mutex               // guards data + disk writes
	path    string                   // on-disk file path
	version string                   // current Client version
	data    *diskFile                // full multi-version state (persisted)
	cur     atomic.Pointer[Snapshot] // lock-free published view of current version
}

// Path resolves the settings file path.
//
// It returns the value of the BLENDKIT_CLIENT_SETTINGS environment variable
// when set, otherwise the FileName located next to the running executable.
//
// Returns:
//
//	The resolved settings file path, or an error if the executable path
//	cannot be determined.
func Path() (string, error) {
	if p := os.Getenv(EnvSettingsPath); p != "" {
		return p, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("settings: cannot determine executable path: %w", err)
	}
	return filepath.Join(filepath.Dir(exe), FileName), nil
}

// Open loads the settings file at path and returns a ready-to-use Store scoped
// to the given Client version.
//
// If the file does not exist it is created. If the current version has no stored
// settings yet, it inherits a deep copy of the most recent previous version's
// settings (if any); otherwise it starts from defaults. The provided defaults
// are used only for a brand-new version with nothing to inherit, and any empty
// shared field is backfilled from defaults so first-run seeding works.
//
// Args:
//
//	path:     Path to the JSON settings file.
//	version:  Current Client version (e.g. "1.10.0").
//	defaults: Shared values to seed a first-ever version with.
//
// Returns:
//
//	The opened Store, or an error if the file existed but could not be read or
//	parsed, or if the initialized state could not be saved.
func Open(path, version string, defaults Shared) (*Store, error) {
	s := &Store{path: path, version: version}

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		s.data = &diskFile{}
		if err := json.Unmarshal(data, s.data); err != nil {
			return nil, fmt.Errorf("settings: parsing %q: %w", path, err)
		}
	case os.IsNotExist(err):
		s.data = &diskFile{}
	default:
		return nil, fmt.Errorf("settings: reading %q: %w", path, err)
	}
	if s.data.Versions == nil {
		s.data.Versions = make(map[string]*versionSettings)
	}

	changed := s.ensureVersion(defaults)
	s.data.CurrentVersion = version
	s.publish()

	// Persist first-run creation / inheritance so the file reflects reality.
	if changed || err != nil {
		if serr := s.save(); serr != nil {
			return nil, serr
		}
	}
	return s, nil
}

// ensureVersion makes sure the current version exists in the file, inheriting
// from the most recent previous version when created fresh. It reports whether
// the in-memory state was changed (and therefore needs saving).
//
// The caller must hold s.mu, except during Open where the Store is not yet
// shared with other goroutines.
func (s *Store) ensureVersion(defaults Shared) bool {
	if vs := s.data.Versions[s.version]; vs != nil {
		// Backfill an empty server from defaults (e.g. first startup after an
		// upgrade that introduced the field) without bumping the revision.
		if vs.Shared.Server == "" && defaults.Server != "" {
			vs.Shared.Server = defaults.Server
			return true
		}
		return false
	}

	vs := &versionSettings{
		Revision:        1,
		UpdatedAt:       time.Now().UTC(),
		GlobalVariables: make(map[string]string),
		PluginVariables: make(map[string]map[string]string),
	}
	if prev := s.data.Versions[s.mostRecentPreviousVersion()]; prev != nil {
		vs.Shared = prev.Shared
		vs.GlobalVariables = cloneStringMap(prev.GlobalVariables)
		vs.PluginVariables = clonePluginMap(prev.PluginVariables)
	}
	if vs.Shared.Server == "" {
		vs.Shared.Server = defaults.Server
	}
	s.data.Versions[s.version] = vs
	return true
}

// mostRecentPreviousVersion returns the highest stored version strictly lower
// than the current one, or "" if there is none.
func (s *Store) mostRecentPreviousVersion() string {
	best := ""
	for v := range s.data.Versions {
		if v == s.version {
			continue
		}
		if compareVersions(v, s.version) >= 0 {
			continue
		}
		if best == "" || compareVersions(v, best) > 0 {
			best = v
		}
	}
	return best
}

// current returns the current version's settings. The caller must hold s.mu.
func (s *Store) current() *versionSettings {
	return s.data.Versions[s.version]
}

// Snapshot returns the current immutable view. Safe for concurrent, lock-free
// use on the hot /report path.
func (s *Store) Snapshot() Snapshot {
	return *s.cur.Load()
}

// SetShared replaces the shared settings, bumps the revision and persists.
//
// Args:
//
//	shared: The new shared settings to store.
//
// Returns:
//
//	The new published Snapshot and an error if persisting failed. Even on a
//	save error the in-memory state is updated and returned.
func (s *Store) SetShared(shared Shared) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vs := s.current()
	vs.Shared = shared
	return s.commitLocked()
}

// SetVariable stores a variable value. An empty plugin stores it globally
// (without plugin association); a non-empty plugin namespaces it under that
// plugin name. The revision is bumped only when the value actually changes.
//
// Args:
//
//	plugin:   Plugin name to namespace under, or "" for a global variable.
//	variable: Variable name.
//	value:    Value to store.
//
// Returns:
//
//	The published Snapshot and an error if persisting failed.
func (s *Store) SetVariable(plugin, variable, value string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vs := s.current()

	if plugin == "" {
		if vs.GlobalVariables == nil {
			vs.GlobalVariables = make(map[string]string)
		}
		if old, ok := vs.GlobalVariables[variable]; ok && old == value {
			return *s.cur.Load(), nil
		}
		vs.GlobalVariables[variable] = value
	} else {
		if vs.PluginVariables == nil {
			vs.PluginVariables = make(map[string]map[string]string)
		}
		if vs.PluginVariables[plugin] == nil {
			vs.PluginVariables[plugin] = make(map[string]string)
		}
		if old, ok := vs.PluginVariables[plugin][variable]; ok && old == value {
			return *s.cur.Load(), nil
		}
		vs.PluginVariables[plugin][variable] = value
	}
	return s.commitLocked()
}

// GetVariable returns a stored variable. An empty plugin reads a global
// variable; a non-empty plugin reads from that plugin's namespace.
//
// Returns:
//
//	The value and true if present, otherwise "" and false.
func (s *Store) GetVariable(plugin, variable string) (string, bool) {
	snap := s.Snapshot()
	if plugin == "" {
		v, ok := snap.GlobalVariables[variable]
		return v, ok
	}
	if m, ok := snap.PluginVariables[plugin]; ok {
		v, ok := m[variable]
		return v, ok
	}
	return "", false
}

// DeleteVariable removes a stored variable and bumps the revision if it existed.
// An empty plugin targets a global variable; a non-empty plugin targets that
// plugin's namespace.
//
// Returns:
//
//	The published Snapshot and an error if persisting failed.
func (s *Store) DeleteVariable(plugin, variable string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vs := s.current()

	if plugin == "" {
		if _, ok := vs.GlobalVariables[variable]; !ok {
			return *s.cur.Load(), nil
		}
		delete(vs.GlobalVariables, variable)
	} else {
		m, ok := vs.PluginVariables[plugin]
		if !ok {
			return *s.cur.Load(), nil
		}
		if _, ok := m[variable]; !ok {
			return *s.cur.Load(), nil
		}
		delete(m, variable)
	}
	return s.commitLocked()
}

// commitLocked bumps revision + timestamp, publishes a fresh Snapshot and saves
// to disk. The caller must hold s.mu.
func (s *Store) commitLocked() (Snapshot, error) {
	vs := s.current()
	vs.Revision++
	vs.UpdatedAt = time.Now().UTC()
	s.publish()
	if err := s.save(); err != nil {
		return *s.cur.Load(), err
	}
	return *s.cur.Load(), nil
}

// publish builds an immutable Snapshot of the current version and stores it for
// lock-free reads. The caller must hold s.mu (except during Open).
func (s *Store) publish() {
	vs := s.current()
	snap := &Snapshot{
		Version:         s.version,
		Revision:        vs.Revision,
		UpdatedAt:       vs.UpdatedAt,
		Shared:          vs.Shared,
		GlobalVariables: cloneStringMap(vs.GlobalVariables),
		PluginVariables: clonePluginMap(vs.PluginVariables),
	}
	if snap.GlobalVariables == nil {
		snap.GlobalVariables = map[string]string{}
	}
	if snap.PluginVariables == nil {
		snap.PluginVariables = map[string]map[string]string{}
	}
	s.cur.Store(snap)
}

// save atomically writes the full multi-version state to disk. The caller must
// hold s.mu (except during Open).
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("settings: marshalling: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("settings: creating dir %q: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, FileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("settings: creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("settings: writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("settings: closing temp file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("settings: renaming temp file into place: %w", err)
	}
	return nil
}

// compareVersions compares two dotted version strings (e.g. "1.10.0") numerically
// per component. Missing or non-numeric components are treated as 0.
//
// Returns:
//
//	-1 if a < b, 0 if equal, 1 if a > b.
func compareVersions(a, b string) int {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var ai, bi int
		if i < len(pa) {
			ai, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			bi, _ = strconv.Atoi(pb[i])
		}
		if ai != bi {
			if ai < bi {
				return -1
			}
			return 1
		}
	}
	return 0
}

func cloneStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func clonePluginMap(m map[string]map[string]string) map[string]map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]map[string]string, len(m))
	for k, v := range m {
		out[k] = cloneStringMap(v)
	}
	return out
}
