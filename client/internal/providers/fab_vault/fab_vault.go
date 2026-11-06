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

// Package fab_vault indexes local Epic Games / Fab / Quixel asset caches:
// the Epic Launcher VaultCache, the Fab plugin's FabLibrary, and Quixel
// Bridge's Megascans Library. See docs/research/epic_vault_paths.md in the
// plugin repository for the candidate paths per OS.
//
// Scanned roots come from the settings variable vault_roots; while that
// variable is unset the provider scans the auto-detected caches that exist —
// dirs the user's launcher/Bridge already pointed at. Once the user edits
// the list (even to empty), the configured list is authoritative.
package fab_vault

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/blenderkit/blenderkit/client/internal/index"
	"github.com/blenderkit/blenderkit/client/internal/providers"
)

// VarVaultRoots is the settings variable holding the JSON roots list.
const VarVaultRoots = "vault_roots"

// Provider scans Epic/Fab/Quixel vault caches.
type Provider struct {
	idx *index.Store      // nil when the index is unavailable — provider still serves
	cfg providers.Config // nil when the settings store is unavailable
}

// New returns the fab_vault provider. idx and cfg may be nil (degraded mode).
func New(idx *index.Store, cfg providers.Config) *Provider {
	return &Provider{idx: idx, cfg: cfg}
}

// Name returns the provider identifier.
func (*Provider) Name() string { return "fab_vault" }

// Kind returns "local" — assets come from on-disk caches.
func (*Provider) Kind() string { return "local" }

// RegisterRoutes mounts the provider's endpoints, bare + versioned.
func (p *Provider) RegisterRoutes(mux *http.ServeMux, vapi string) {
	for _, prefix := range []string{"/providers/fab_vault", "/" + vapi + "/providers/fab_vault"} {
		mux.HandleFunc(prefix+"/status", p.status)
		mux.HandleFunc(prefix+"/scan", p.scan)
		mux.HandleFunc(prefix+"/roots", p.roots)
	}
}

// Candidates returns the OS-specific default cache locations. Custom
// locations (launcher/Bridge settings) are resolved on top of these.
func Candidates() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		return []string{
			filepath.Join(os.Getenv("PROGRAMDATA"), `Epic\EpicGamesLauncher\VaultCache`),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "FabLibrary"),
			filepath.Join(home, "Documents", "Megascans Library", "Downloaded"),
		}
	case "darwin":
		return []string{
			"/Users/Shared/Epic Games/EpicGamesLauncher/VaultCache",
			filepath.Join(home, "Library", "Application Support", "Epic", "FabLibrary"),
			filepath.Join(home, "Documents", "Megascans Library", "Downloaded"),
		}
	default: // linux and others
		return []string{
			filepath.Join(home, ".local", "share", "Epic", "FabLibrary"),
			filepath.Join(home, "Documents", "Megascans Library", "Downloaded"),
			filepath.Join(home, "Megascans Library", "Downloaded"),
		}
	}
}

// detected returns the auto-detected candidates that currently exist.
func detected() []string {
	var out []string
	for _, r := range providers.ProbeRoots(Candidates()) {
		if r.Exists {
			out = append(out, r.Path)
		}
	}
	return out
}

// rootsList resolves the effective roots and where they came from.
func (p *Provider) rootsList() ([]string, string) {
	if roots, ok := providers.RootsFromVar(p.cfg, VarVaultRoots); ok {
		return roots, "configured"
	}
	if det := detected(); len(det) > 0 {
		return det, "detected"
	}
	return nil, "none"
}

func (p *Provider) status(w http.ResponseWriter, _ *http.Request) {
	roots, source := p.rootsList()
	providers.WriteJSON(w, map[string]any{
		"provider":         p.Name(),
		"kind":             p.Kind(),
		"index_available":  p.idx != nil,
		"roots":            providers.ProbeRoots(roots),
		"roots_source":     source,
		"candidate_roots":  providers.ProbeRoots(Candidates()),
	})
}

// scan walks the effective roots and indexes the asset files found.
func (p *Provider) scan(w http.ResponseWriter, r *http.Request) {
	roots, source := p.rootsList()
	sum := providers.ScanRoots(r.Context(), p.idx, p.Name(), roots)
	providers.WriteJSON(w, map[string]any{
		"provider":        p.Name(),
		"roots_source":    source,
		"scanned":         sum,
		"index_available": p.idx != nil,
	})
}

// roots handles GET (effective roots + detected candidates) and POST
// (replace the stored list with the request body's {"roots":[...]}).
func (p *Provider) roots(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		roots, source := p.rootsList()
		providers.WriteJSON(w, map[string]any{
			"roots":            providers.ProbeRoots(roots),
			"source":           source,
			"candidate_roots":  providers.ProbeRoots(Candidates()),
		})
	case http.MethodPost:
		if p.cfg == nil {
			http.Error(w, "settings store unavailable", http.StatusServiceUnavailable)
			return
		}
		var body struct {
			Roots []string `json:"roots"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "Error parsing JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer r.Body.Close()
		stored, err := providers.SaveRoots(p.cfg, VarVaultRoots, body.Roots)
		if err != nil {
			http.Error(w, "persist failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		providers.WriteJSON(w, map[string]any{
			"roots":  providers.ProbeRoots(stored),
			"source": "configured",
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
