//go:build !cgo

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

// runTray is the CGO-free stand-in for the systray-based runTray: fyne.io/
// systray needs CGO, so CGO-free builds ship without a tray icon. The Client
// keeps serving until /shutdown or process exit — the same end state the
// tray's Quit menu reaches via os.Exit(0).
func runTray(_, listenAddr string) {
	BKLog.Printf("%s CGO-free build: no system tray; Client serving on %s", EmoInfo, listenAddr)
	select {}
}
