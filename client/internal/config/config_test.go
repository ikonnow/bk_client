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

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefault(t *testing.T) {
	c := Default()
	if c.Server != DefaultServer {
		t.Errorf("Server = %q, want %q", c.Server, DefaultServer)
	}
	if c.PreferredPort != DefaultPort {
		t.Errorf("PreferredPort = %q, want %q", c.PreferredPort, DefaultPort)
	}
	if c.ProxyWhich != DefaultProxyWhich {
		t.Errorf("ProxyWhich = %q, want %q", c.ProxyWhich, DefaultProxyWhich)
	}
	if !c.AutoShutdown {
		t.Errorf("AutoShutdown = false, want true")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)

	want := Default()
	want.Server = "https://example.test"
	want.PreferredPort = "12345"
	want.ProxyWhich = "CUSTOM"
	want.ProxyAddress = "http://127.0.0.1:8080"
	want.AutoShutdown = false

	if err := want.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, existed, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !existed {
		t.Fatalf("Load() existed = false, want true")
	}
	if got != want {
		t.Errorf("round trip mismatch:\n got = %+v\nwant = %+v", got, want)
	}
}

func TestLoadMissingReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")

	got, existed, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil for missing file", err)
	}
	if existed {
		t.Errorf("Load() existed = true, want false for missing file")
	}
	if got != Default() {
		t.Errorf("Load() = %+v, want defaults %+v", got, Default())
	}
}

func TestLoadIgnoresUnknownKeysAndKeepsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)

	// Only overrides Server; omits the rest and includes an unknown key and an
	// api_key that must never be honoured.
	raw := `{"server":"https://partial.test","api_key":"secret","unknown":1}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, existed, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !existed {
		t.Fatalf("existed = false, want true")
	}
	if got.Server != "https://partial.test" {
		t.Errorf("Server = %q, want overridden value", got.Server)
	}
	if got.PreferredPort != DefaultPort {
		t.Errorf("PreferredPort = %q, want default %q", got.PreferredPort, DefaultPort)
	}
}

func TestSaveDoesNotPersistSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)

	if err := Default().Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, secret := range []string{"api_key", "apikey", "token", "password"} {
		if strings.Contains(strings.ToLower(string(data)), secret) {
			t.Errorf("config file unexpectedly contains secret-like key %q:\n%s", secret, data)
		}
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, existed, err := Load(path)
	if err == nil {
		t.Fatalf("Load() error = nil, want parse error")
	}
	if !existed {
		t.Errorf("existed = false, want true (file is present)")
	}
	if got != Default() {
		t.Errorf("Load() on bad JSON = %+v, want defaults", got)
	}
}

func TestDefaultPathFor(t *testing.T) {
	got := DefaultPathFor(filepath.Join("some", "dir", "bk_client"))
	want := filepath.Join("some", "dir", FileName)
	if got != want {
		t.Errorf("DefaultPathFor() = %q, want %q", got, want)
	}
}

func TestPathHonoursEnvOverride(t *testing.T) {
	want := filepath.Join(t.TempDir(), "custom-config.json")
	t.Setenv(EnvConfigPath, want)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}
	if got != want {
		t.Errorf("Path() = %q, want env override %q", got, want)
	}
}
