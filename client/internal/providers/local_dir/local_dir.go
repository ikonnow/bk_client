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

// Package local_dir indexes user-configured asset directories (HDDs, SSDs,
// external drives). Roots currently come from the A2G_LOCAL_DIRS environment
// variable; configuration through the Client settings store (managed by the
// plugin UI) replaces that once the settings surface exists.
package local_dir

import (
	"net/http"
	"os"
	"strings"

	"github.com/blenderkit/blenderkit/client/internal/index"
	"github.com/blenderkit/blenderkit/client/internal/providers"
)

// EnvVar lists local asset directories separated by the OS path list
// separator (';' on Windows, ':' elsewhere).
const EnvVar = "A2G_LOCAL_DIRS"

// Provider scans user-configured local directories.
type Provider struct {
	idx *index.Store // nil when the index is unavailable — provider still serves
}

// New returns the local_dir provider. idx may be nil (degraded mode).
func New(idx *index.Store) *Provider {
	return &Provider{idx: idx}
}

// Name returns the provider identifier.
func (*Provider) Name() string { return "local_dir" }

// Kind returns "local" — assets come from on-disk directories.
func (*Provider) Kind() string { return "local" }

// Roots returns the currently configured directories.
func Roots() []string {
	var roots []string
	for _, p := range strings.Split(os.Getenv(EnvVar), string(os.PathListSeparator)) {
		if p = strings.TrimSpace(p); p != "" {
			roots = append(roots, p)
		}
	}
	return roots
}

// RegisterRoutes mounts /providers/local_dir/{status,scan}, bare + versioned.
func (p *Provider) RegisterRoutes(mux *http.ServeMux, vapi string) {
	for _, prefix := range []string{"/providers/local_dir", "/" + vapi + "/providers/local_dir"} {
		mux.HandleFunc(prefix+"/status", p.status)
		mux.HandleFunc(prefix+"/scan", p.scan)
	}
}

func (p *Provider) status(w http.ResponseWriter, _ *http.Request) {
	providers.WriteJSON(w, map[string]any{
		"provider":         p.Name(),
		"kind":             p.Kind(),
		"index_available":  p.idx != nil,
		"configured_roots": providers.ProbeRoots(Roots()),
	})
}

// scan reports the reachable configured roots and their top-level entry
// counts; unreachable roots (disconnected drives) are skipped, not fatal.
func (p *Provider) scan(w http.ResponseWriter, _ *http.Request) {
	type scannedRoot struct {
		Path    string `json:"path"`
		Entries int    `json:"entries"`
	}
	scanned := []scannedRoot{}
	skipped := []string{}
	for _, root := range providers.ProbeRoots(Roots()) {
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
