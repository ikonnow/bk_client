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
)

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

// Registry is the set of active providers.
type Registry struct {
	providers []Provider
}

// NewRegistry returns a Registry holding the given providers.
func NewRegistry(ps ...Provider) *Registry {
	return &Registry{providers: ps}
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

	for _, p := range r.providers {
		p.RegisterRoutes(mux, vapi)
	}
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
