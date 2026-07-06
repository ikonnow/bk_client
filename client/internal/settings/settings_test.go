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

package settings

import (
	"path/filepath"
	"testing"
)

func TestOpenSeedsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	s, err := Open(path, "1.10.0", Shared{Server: "https://example.test"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	snap := s.Snapshot()
	if snap.Version != "1.10.0" {
		t.Errorf("Version = %q, want 1.10.0", snap.Version)
	}
	if snap.Shared.Server != "https://example.test" {
		t.Errorf("Server = %q, want seeded default", snap.Shared.Server)
	}
	if snap.Revision != 1 {
		t.Errorf("Revision = %d, want 1", snap.Revision)
	}
}

func TestSetSharedBumpsRevisionAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	s, err := Open(path, "1.10.0", Shared{Server: "https://a.test"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	before := s.Snapshot().Revision

	snap, err := s.SetShared(Shared{Server: "https://b.test"})
	if err != nil {
		t.Fatalf("SetShared() error = %v", err)
	}
	if snap.Shared.Server != "https://b.test" {
		t.Errorf("Server = %q, want https://b.test", snap.Shared.Server)
	}
	if snap.Revision != before+1 {
		t.Errorf("Revision = %d, want %d", snap.Revision, before+1)
	}

	// Reopen from disk: the change must have been persisted.
	s2, err := Open(path, "1.10.0", Shared{Server: "https://ignored.test"})
	if err != nil {
		t.Fatalf("reopen Open() error = %v", err)
	}
	if got := s2.Snapshot().Shared.Server; got != "https://b.test" {
		t.Errorf("persisted Server = %q, want https://b.test", got)
	}
}

func TestVariablesGlobalAndPluginScoped(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	s, err := Open(path, "1.10.0", Shared{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if _, err := s.SetVariable("", "value", "1"); err != nil {
		t.Fatalf("SetVariable global error = %v", err)
	}
	if _, err := s.SetVariable("blender", "value", "2"); err != nil {
		t.Fatalf("SetVariable plugin error = %v", err)
	}

	if v, ok := s.GetVariable("", "value"); !ok || v != "1" {
		t.Errorf("global value = %q,%v, want 1,true", v, ok)
	}
	if v, ok := s.GetVariable("blender", "value"); !ok || v != "2" {
		t.Errorf("blender value = %q,%v, want 2,true", v, ok)
	}
	if _, ok := s.GetVariable("godot", "value"); ok {
		t.Errorf("godot value should be absent")
	}
}

func TestSetVariableNoOpDoesNotBumpRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	s, _ := Open(path, "1.10.0", Shared{})
	s.SetVariable("blender", "path", "x")
	rev := s.Snapshot().Revision

	if _, err := s.SetVariable("blender", "path", "x"); err != nil {
		t.Fatalf("SetVariable error = %v", err)
	}
	if got := s.Snapshot().Revision; got != rev {
		t.Errorf("Revision changed on no-op write: %d, want %d", got, rev)
	}
}

func TestDeleteVariable(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	s, _ := Open(path, "1.10.0", Shared{})
	s.SetVariable("", "k", "v")

	if _, err := s.DeleteVariable("", "k"); err != nil {
		t.Fatalf("DeleteVariable error = %v", err)
	}
	if _, ok := s.GetVariable("", "k"); ok {
		t.Errorf("variable still present after delete")
	}
}

func TestNewVersionInheritsFromMostRecentPrevious(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)

	// Older versions populate the file.
	s19, _ := Open(path, "1.9.0", Shared{Server: "https://old.test"})
	s19.SetVariable("blender", "executable", "/old/blender")

	s195, _ := Open(path, "1.9.5", Shared{Server: "https://ignored.test"})
	s195.SetShared(Shared{Server: "https://mid.test"})
	s195.SetVariable("blender", "executable", "/mid/blender")

	// New version with no settings should inherit from 1.9.5 (most recent < 1.10.0),
	// not from 1.9.0.
	s110, err := Open(path, "1.10.0", Shared{Server: "https://fallback.test"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	snap := s110.Snapshot()
	if snap.Shared.Server != "https://mid.test" {
		t.Errorf("inherited Server = %q, want https://mid.test", snap.Shared.Server)
	}
	if v, ok := s110.GetVariable("blender", "executable"); !ok || v != "/mid/blender" {
		t.Errorf("inherited variable = %q,%v, want /mid/blender,true", v, ok)
	}

	// Inheritance must not mutate the previous version.
	s195again, _ := Open(path, "1.9.5", Shared{})
	if got := s195again.Snapshot().Shared.Server; got != "https://mid.test" {
		t.Errorf("previous version mutated: Server = %q", got)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.9.0", "1.10.0", -1},
		{"1.10.0", "1.9.0", 1},
		{"1.10.0", "1.10.0", 0},
		{"1.2", "1.2.0", 0},
		{"2.0.0", "1.99.99", 1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
