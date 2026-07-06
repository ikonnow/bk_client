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

package main

import (
	"encoding/json"
	"net/http"

	"github.com/blenderkit/blenderkit/client/internal/settings"
)

// SettingsStore is the Client-owned, version-scoped settings store. The Client is
// the single source of truth for settings; plugins must sync from it. Initialized
// in main() before the HTTP server starts.
var SettingsStore *settings.Store

// SetSettingsData is the body of a /settings/set request. Only fields that are
// present (non-nil) are changed, so a plugin can patch a single setting without
// clobbering the rest.
type SetSettingsData struct {
	// Server, when non-nil, replaces the shared server address.
	Server *string `json:"server,omitempty"`
}

// SetVariableData is the body of a /settings/set_variable request. An empty
// Plugin stores the variable globally (without plugin association); a non-empty
// Plugin namespaces it under that plugin name.
type SetVariableData struct {
	Plugin   string `json:"plugin,omitempty"`
	Variable string `json:"variable"`
	Value    string `json:"value"`
}

// writeSnapshot writes a settings Snapshot to the client as JSON.
func writeSnapshot(w http.ResponseWriter, snap settings.Snapshot) {
	body, err := json.Marshal(snap)
	if err != nil {
		http.Error(w, "Error converting to JSON: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Blendkit-Client-Version", ClientVersion)
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

// getSettingsHandler returns the current settings Snapshot so a plugin can sync
// its local copy to the Client's source of truth.
func getSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if SettingsStore == nil {
		http.Error(w, "Settings store not initialized", http.StatusServiceUnavailable)
		return
	}
	writeSnapshot(w, SettingsStore.Snapshot())
}

// setSettingsHandler applies a change to the shared settings requested by a
// plugin, bumps the revision and returns the new Snapshot. The change is also
// broadcast to every connected plugin on their next /report poll.
func setSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if SettingsStore == nil {
		http.Error(w, "Settings store not initialized", http.StatusServiceUnavailable)
		return
	}
	var data SetSettingsData
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Error parsing JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	shared := SettingsStore.Snapshot().Shared
	if data.Server != nil {
		shared.Server = *data.Server
	}

	snap, err := SettingsStore.SetShared(shared)
	if err != nil {
		BKLog.Printf("%v Failed to persist settings: %v", EmoWarning, err)
	}
	writeSnapshot(w, snap)
}

// setVariableHandler stores a free-form variable on behalf of a plugin (globally
// or namespaced under a plugin name), bumps the revision and returns the new
// Snapshot. The change is broadcast to every connected plugin on their next
// /report poll.
func setVariableHandler(w http.ResponseWriter, r *http.Request) {
	if SettingsStore == nil {
		http.Error(w, "Settings store not initialized", http.StatusServiceUnavailable)
		return
	}
	var data SetVariableData
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Error parsing JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if data.Variable == "" {
		http.Error(w, "Missing required field: variable", http.StatusBadRequest)
		return
	}

	snap, err := SettingsStore.SetVariable(data.Plugin, data.Variable, data.Value)
	if err != nil {
		BKLog.Printf("%v Failed to persist settings: %v", EmoWarning, err)
	}
	writeSnapshot(w, snap)
}
