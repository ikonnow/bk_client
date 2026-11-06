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
// external drives). Roots live in the Client settings store under the
// "assets2godot" namespace — nothing is ever scanned that the user did not
// add. The A2G_LOCAL_DIRS environment variable is a read-only development
// fallback when nothing is configured.
package local_dir

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/blenderkit/blenderkit/client/internal/index"
	"github.com/blenderkit/blenderkit/client/internal/providers"
)

// VarLocalDirs is the settings variable holding the JSON roots list.
const VarLocalDirs = "local_dirs"

// EnvVar lists local asset directories separated by the OS path list
// separator (';' on Windows, ':' elsewhere). Read-only dev fallback.
const EnvVar = "A2G_LOCAL_DIRS"

// Provider scans user-configured local directories.
type Provider struct {
	idx *index.Store      // nil when the index is unavailable — provider still serves
	cfg providers.Config // nil when the settings store is unavailable
}

// New returns the local_dir provider. idx and cfg may be nil (degraded mode).
func New(idx *index.Store, cfg providers.Config) *Provider {
	return &Provider{idx: idx, cfg: cfg}
}

// Name returns the provider identifier.
func (*Provider) Name() string { return "local_dir" }

// Kind returns "local" — assets come from on-disk directories.
func (*Provider) Kind() string { return "local" }

// RegisterRoutes mounts the provider's endpoints, bare + versioned.
func (p *Provider) RegisterRoutes(mux *http.ServeMux, vapi string) {
	for _, prefix := range []string{"/providers/local_dir", "/" + vapi + "/providers/local_dir"} {
		mux.HandleFunc(prefix+"/status", p.status)
		mux.HandleFunc(prefix+"/scan", p.scan)
		mux.HandleFunc(prefix+"/roots", p.roots)
	}
}

// roots resolves the effective roots and where they came from: the settings
// store wins, then the env fallback, then nothing. An explicitly stored
// empty list means the user removed everything — source "configured".
func (p *Provider) rootsList() ([]string, string) {
	if roots, ok := providers.RootsFromVar(p.cfg, VarLocalDirs); ok {
		return roots, "configured"
	}
	if env := envRoots(); len(env) > 0 {
		return env, "env"
	}
	return nil, "none"
}

func envRoots() []string {
	var roots []string
	for _, dir := range strings.Split(os.Getenv(EnvVar), string(os.PathListSeparator)) {
		if dir = strings.TrimSpace(dir); dir != "" {
			roots = append(roots, dir)
		}
	}
	return roots
}

func (p *Provider) status(w http.ResponseWriter, _ *http.Request) {
	roots, source := p.rootsList()
	providers.WriteJSON(w, map[string]any{
		"provider":        p.Name(),
		"kind":            p.Kind(),
		"index_available": p.idx != nil,
		"roots":           providers.ProbeRoots(roots),
		"roots_source":    source,
	})
}

// scan walks the configured roots and indexes the asset files found.
// Unreachable roots (disconnected drives) are skipped, not fatal.
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

// roots handles GET (effective roots + source) and POST (replace the stored
// list with the request body's {"roots":[...]}).
func (p *Provider) roots(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		roots, source := p.rootsList()
		providers.WriteJSON(w, map[string]any{
			"roots":  providers.ProbeRoots(roots),
			"source": source,
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
		stored, err := providers.SaveRoots(p.cfg, VarLocalDirs, body.Roots)
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
