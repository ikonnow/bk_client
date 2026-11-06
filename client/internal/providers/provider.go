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

// Package providers defines the Assets2Godot provider contract: each asset
// source (BlenderKit API, Epic/Fab vault caches, custom local directories)
// implements Provider and registers its HTTP endpoints under
// /providers/<name>/... . All endpoints are additive; existing Client
// endpoints are never touched, keeping upstream merges conflict-free.
package providers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/blenderkit/blenderkit/client/internal/index"
	"github.com/blenderkit/blenderkit/client/internal/settings"
)

// PluginName is the settings namespace every Assets2Godot variable is stored
// under (Snapshot.PluginVariables["assets2godot"]).
const PluginName = "assets2godot"

// Config is the slice of the Client settings store providers need to read and
// persist their user-configured roots. *settings.Store satisfies it; it may be
// nil in degraded mode, which providers must tolerate.
type Config interface {
	GetVariable(plugin, variable string) (string, bool)
	SetVariable(plugin, variable, value string) (settings.Snapshot, error)
}

// RootsFromVar decodes a JSON string-list variable. The second return value
// is true when the variable exists at all — an explicitly stored empty list
// means "user removed every root", which differs from "never configured".
func RootsFromVar(cfg Config, variable string) ([]string, bool) {
	if cfg == nil {
		return nil, false
	}
	v, ok := cfg.GetVariable(PluginName, variable)
	if !ok {
		return nil, false
	}
	var out []string
	if json.Unmarshal([]byte(v), &out) != nil || out == nil {
		out = []string{}
	}
	return out, true
}

// SaveRoots normalizes and persists a roots list as a JSON variable,
// returning the stored list.
func SaveRoots(cfg Config, variable string, roots []string) ([]string, error) {
	norm := NormalizeRoots(roots)
	data, _ := json.Marshal(norm)
	_, err := cfg.SetVariable(PluginName, variable, string(data))
	return norm, err
}

// NormalizeRoots cleans each path, drops empties and duplicates, and sorts
// for stable settings diffs and UI output.
func NormalizeRoots(roots []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		r = strings.TrimSpace(r)
		if r == "" {
			continue // "" would otherwise resolve to the working directory
		}
		if abs, err := filepath.Abs(r); err == nil {
			r = abs
		}
		r = filepath.Clean(r)
		if r == "." || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// Provider is one source of 3D assets.
type Provider interface {
	// Name is the provider's stable identifier, used as its route segment
	// (/providers/<name>/...) and index key.
	Name() string
	// Kind describes where assets come from: "remote" or "local".
	Kind() string
	// RegisterRoutes mounts the provider's endpoints on mux, both bare
	// (/providers/<name>/...) and versioned (/<vapi>/providers/<name>/...).
	RegisterRoutes(mux *http.ServeMux, vapi string)
}

// Registry is the set of active providers, plus the shared asset index used
// by the unified /providers/search endpoint (nil in degraded mode).
type Registry struct {
	providers []Provider
	idx       *index.Store
}

// NewRegistry returns a Registry holding the shared index and providers.
func NewRegistry(idx *index.Store, ps ...Provider) *Registry {
	return &Registry{providers: ps, idx: idx}
}

// All returns the registered providers in registration order.
func (r *Registry) All() []Provider {
	return r.providers
}

// RegisterRoutes mounts /providers/list (both bare and versioned) and then
// lets each provider mount its own routes.
func (r *Registry) RegisterRoutes(mux *http.ServeMux, vapi string) {
	list := func(w http.ResponseWriter, _ *http.Request) {
		type entry struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		}
		out := make([]entry, 0, len(r.providers))
		for _, p := range r.providers {
			out = append(out, entry{Name: p.Name(), Kind: p.Kind()})
		}
		WriteJSON(w, out)
	}
	mux.HandleFunc("/providers/list", list)
	mux.HandleFunc("/"+vapi+"/providers/list", list)
	mux.HandleFunc("/providers/search", r.search)
	mux.HandleFunc("/"+vapi+"/providers/search", r.search)

	for _, p := range r.providers {
		p.RegisterRoutes(mux, vapi)
	}
}

// search is the unified index query: /providers/search?q=<text>[&provider=<name>][&limit=<n>].
// With no index it reports index_available instead of failing.
func (r *Registry) search(w http.ResponseWriter, req *http.Request) {
	if r.idx == nil {
		WriteJSON(w, map[string]any{"results": []any{}, "index_available": false})
		return
	}
	q := req.URL.Query().Get("q")
	provider := req.URL.Query().Get("provider")
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	assets, err := r.idx.Search(req.Context(), provider, q, limit)
	if err != nil {
		http.Error(w, "search failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	WriteJSON(w, map[string]any{"results": assets, "index_available": true})
}

// WriteJSON is the shared response helper for provider endpoints.
func WriteJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// RootStatus reports one probed root directory: where it is and whether it
// is reachable right now. Unreachable roots (missing drives, deleted
// folders) are reported, not fatal.
type RootStatus struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

// ProbeRoots stat-checks each candidate and returns one RootStatus per path.
func ProbeRoots(candidates []string) []RootStatus {
	roots := make([]RootStatus, 0, len(candidates))
	for _, c := range candidates {
		info, err := os.Stat(c)
		roots = append(roots, RootStatus{Path: c, Exists: err == nil && info.IsDir()})
	}
	return roots
}
