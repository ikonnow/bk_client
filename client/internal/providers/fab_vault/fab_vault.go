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
package fab_vault

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/blenderkit/blenderkit/client/internal/index"
	"github.com/blenderkit/blenderkit/client/internal/providers"
)

// Provider scans Epic/Fab/Quixel vault caches.
type Provider struct {
	idx *index.Store // nil when the index is unavailable — provider still serves
}

// New returns the fab_vault provider. idx may be nil (degraded mode).
func New(idx *index.Store) *Provider {
	return &Provider{idx: idx}
}

// Name returns the provider identifier.
func (*Provider) Name() string { return "fab_vault" }

// Kind returns "local" — assets come from on-disk caches.
func (*Provider) Kind() string { return "local" }

// RegisterRoutes mounts /providers/fab_vault/{status,scan}, bare + versioned.
func (p *Provider) RegisterRoutes(mux *http.ServeMux, vapi string) {
	for _, prefix := range []string{"/providers/fab_vault", "/" + vapi + "/providers/fab_vault"} {
		mux.HandleFunc(prefix+"/status", p.status)
		mux.HandleFunc(prefix+"/scan", p.scan)
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

func (p *Provider) status(w http.ResponseWriter, _ *http.Request) {
	providers.WriteJSON(w, map[string]any{
		"provider":         p.Name(),
		"kind":             p.Kind(),
		"index_available":  p.idx != nil,
		"candidate_roots":  providers.ProbeRoots(Candidates()),
	})
}

// scan reports the reachable roots and the number of top-level entries per
// root. Deep crawling + index writes land with the real scanner.
func (p *Provider) scan(w http.ResponseWriter, _ *http.Request) {
	type scannedRoot struct {
		Path    string `json:"path"`
		Entries int    `json:"entries"`
	}
	scanned := []scannedRoot{}
	skipped := []string{}
	for _, root := range providers.ProbeRoots(Candidates()) {
		if !root.Exists {
			skipped = append(skipped, root.Path)
			continue
		}
		entries, err := os.ReadDir(root.Path)
		if err != nil {
			skipped = append(skipped, root.Path)
			continue
		}
		scanned = append(scanned, scannedRoot{Path: root.Path, Entries: len(entries)})
	}
	providers.WriteJSON(w, map[string]any{
		"provider":        p.Name(),
		"scanned_roots":   scanned,
		"skipped_roots":   skipped,
		"index_available": p.idx != nil,
	})
}
