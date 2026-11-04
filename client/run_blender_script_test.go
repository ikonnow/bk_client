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

import "testing"

func TestParseRecipeProgressLine(t *testing.T) {
	tests := []struct {
		name         string
		line         string
		current      int
		wantProgress int
		wantMessage  string
		wantOK       bool
	}{
		{"startup load", `00:00.437  blend  | Read blend: "C:\a.blend"`, 10, 12, "Loading .blend", true},
		{"later load ignored", `00:05.187  blend  | Read blend: "C:\a.blend"`, 40, 40, "", false},
		{"status", "BK_STATUS Unpacking textures", 30, 30, "Unpacking textures", true},
		{"empty status", "BK_STATUS", 30, 30, "", false},
		{"progress start", "BK_PROGRESS 0.000 Opening blend", 12, 12, "Opening blend", true},
		{"progress middle", "BK_PROGRESS 0.500 Exporting materials 2/4", 12, 55, "Exporting materials 2/4", true},
		{"progress end", "BK_PROGRESS 1.000 done", 12, 99, "done", true},
		{"progress without message", "BK_PROGRESS 0.5", 12, 55, "", true},
		{"progress clamps", "BK_PROGRESS 7", 12, 99, "", true},
		{"progress never decreases", "BK_PROGRESS 0.100 Split materials", 60, 60, "Split materials", true},
		{"bad progress", "BK_PROGRESS nope", 30, 30, "", false},
		{"plain log", "[export_usd] unpack: rel_dir=//textures", 30, 30, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			progress, message, ok := parseRecipeProgressLine(tc.line, tc.current)
			if progress != tc.wantProgress || message != tc.wantMessage || ok != tc.wantOK {
				t.Fatalf("parseRecipeProgressLine(%q, %d) = (%d, %q, %v), want (%d, %q, %v)",
					tc.line, tc.current, progress, message, ok, tc.wantProgress, tc.wantMessage, tc.wantOK)
			}
		})
	}
}
