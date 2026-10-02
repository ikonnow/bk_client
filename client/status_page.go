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
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// statusPageHTML is the Client status page served at /, embedded into the
// binary at build time.
//
//go:embed status_page.html
var statusPageHTML string

// statusPageClientPlaceholder in statusPageHTML is replaced by the Client info
// as JSON. json.Marshal escapes <, > and &, so the result is safe in a <script>.
const statusPageClientPlaceholder = "/*CLIENT_INFO*/null"

// statusPageClientInfo is the Client info rendered into the status page.
// The connected softwares are fetched by the page itself from /addons/list.
type statusPageClientInfo struct {
	Version     string `json:"version"` // Fields are shown in this order.
	Port        string `json:"port"`
	StartedFrom string `json:"startedFrom"`
	Platform    string `json:"platform"`
	SystemID    string `json:"systemID"`
	PID         int    `json:"pid"`
}

// Handler for the index of the Client: the human-readable status page.
func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/favicon.ico" { // Reuses the tray icon: PNG on Linux, ICO elsewhere.
		w.Header().Set("Content-Type", http.DetectContentType(trayIcon))
		w.Write(trayIcon)
		return
	}
	if r.URL.Path != "/" { // Go handles "/" path as catchall, so refusing non-index stuff here
		BKLog.Printf("%v Access to unknown path: %v", EmoWarning, r.URL.Path)
		http.Error(w, "Not found: "+r.URL.Path, http.StatusNotFound)
		return
	}

	startedFrom := "started manually"
	if *StartingAddonVersion != "" {
		startedFrom = fmt.Sprintf("%s add-on v%s", *StartingSoftwareName, *StartingAddonVersion)
	}
	info := statusPageClientInfo{
		Version:     ClientVersion,
		Port:        *Port,
		StartedFrom: startedFrom,
		Platform:    GetPlatformVersion(),
		SystemID:    *SystemID,
		PID:         os.Getpid(),
	}

	infoJSON, err := json.Marshal(info)
	if err != nil {
		http.Error(w, "Error converting to JSON: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(strings.Replace(statusPageHTML, statusPageClientPlaceholder, string(infoJSON), 1)))
}
