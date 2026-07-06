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
	"net/http"
)

// devDashboardHTML is the self-contained developer dashboard page, embedded into
// the binary at build time.
//
//go:embed dev_dashboard.html
var devDashboardHTML []byte

// devDashboardHandler serves the developer dashboard: a same-origin HTML page
// with buttons to call the Client's endpoints and view their raw responses.
//
// Because the page is served by the Client itself, all requests it makes are
// same-origin, so the settings endpoints (which do not emit CORS headers) work
// directly from the browser. This is a manual-testing aid, not a production UI.
func devDashboardHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(devDashboardHTML)
}
