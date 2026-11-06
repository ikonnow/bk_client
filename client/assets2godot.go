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

// assets2godot.go is the single seam between upstream's main() and the
// Assets2Godot provider layer: all A2G wiring lives here so the diff inside
// main() stays one call site.

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/blenderkit/blenderkit/client/internal/index"
	"github.com/blenderkit/blenderkit/client/internal/providers"
	"github.com/blenderkit/blenderkit/client/internal/providers/blenderkit"
	"github.com/blenderkit/blenderkit/client/internal/providers/fab_vault"
	"github.com/blenderkit/blenderkit/client/internal/providers/local_dir"
)

// AssetIndex is the shared Assets2Godot SQLite index. It is nil when it
// could not be opened — providers then run degraded (no persistence/search)
// instead of failing.
var AssetIndex *index.Store

// indexDBPath resolves the index location: -global_dir override first, then
// the client's data-dir convention ~/blenderkit_data/assets2godot/index.db.
func indexDBPath() (string, error) {
	if GlobalDir != "" {
		return filepath.Join(GlobalDir, "assets2godot", "index.db"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "blenderkit_data", "assets2godot", "index.db"), nil
}

// initProviders opens the shared index (degrading gracefully when it can't)
// and mounts every provider's routes on mux.
func initProviders(mux *http.ServeMux, vapi string) {
	if path, err := indexDBPath(); err != nil {
		BKLog.Printf("%s Assets2Godot index path unavailable: %v", EmoWarning, err)
	} else if idx, err := index.Open(path); err != nil {
		BKLog.Printf("%s Assets2Godot index open failed (%s): %v", EmoWarning, path, err)
	} else {
		AssetIndex = idx
		BKLog.Printf("%s Assets2Godot index ready: %s", EmoOK, path)
	}

	reg := providers.NewRegistry(
		blenderkit.New(),
		fab_vault.New(AssetIndex),
		local_dir.New(AssetIndex),
	)
	reg.RegisterRoutes(mux, vapi)
	BKLog.Printf("%s Assets2Godot providers registered: blenderkit, fab_vault, local_dir", EmoOK)
}
