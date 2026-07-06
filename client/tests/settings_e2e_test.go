//go:build integration

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

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

// testServer is the (fake) upstream server the Client is told to use. No real
// requests are made to it in these tests — only the Client's own local HTTP API
// is exercised — so it just needs to be a stable, recognizable value.
const testServer = "https://devel.blendkit.com"

// testAppID is the fake process ID used to simulate a connected Blender add-on.
const testAppID = 424242

var (
	baseURL   string    // e.g. http://localhost:62485
	clientCmd *exec.Cmd // the running Client process
	tempDir   string    // scratch dir for the built binary and settings file
)

// TestMain builds the real Client binary, starts it as a child process on a free
// port, waits until its HTTP server is ready, runs the suite, then shuts the
// Client down cleanly.
func TestMain(m *testing.M) {
	if err := setup(); err != nil {
		fmt.Fprintln(os.Stderr, "integration setup failed:", err)
		teardown()
		os.Exit(1)
	}
	code := m.Run()
	teardown()
	os.Exit(code)
}

// setup builds and launches the Client.
func setup() error {
	var err error
	tempDir, err = os.MkdirTemp("", "blendkit-client-e2e-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}
	clientDir := filepath.Dir(wd) // .../client (parent of .../client/tests)

	exeName := "blendkit-client-e2e"
	if runtime.GOOS == "windows" {
		exeName += ".exe"
	}
	exePath := filepath.Join(tempDir, exeName)

	build := exec.Command("go", "build", "-o", exePath, ".")
	build.Dir = clientDir
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("building client: %w", err)
	}

	port, err := freePort()
	if err != nil {
		return fmt.Errorf("finding free port: %w", err)
	}
	baseURL = "http://localhost:" + port

	settingsPath := filepath.Join(tempDir, "settings.json")

	// Pass --version so the Client is treated as add-on-spawned (not standalone):
	// no system tray, no single-instance guard, and it listens headlessly.
	clientCmd = exec.Command(exePath,
		"--port", port,
		"--server", testServer,
		"--version", "3.13.0",
		"--software", "Blender",
		"--pid", strconv.Itoa(testAppID),
	)
	clientCmd.Env = append(os.Environ(), "BLENDKIT_CLIENT_SETTINGS="+settingsPath)
	clientCmd.Stdout = os.Stderr
	clientCmd.Stderr = os.Stderr
	if err := clientCmd.Start(); err != nil {
		return fmt.Errorf("starting client: %w", err)
	}

	return waitReady(15 * time.Second)
}

// teardown asks the Client to shut down and reaps the process.
func teardown() {
	if clientCmd != nil && clientCmd.Process != nil {
		// Best-effort graceful shutdown via the Client's own endpoint.
		if resp, err := http.Post(baseURL+"/shutdown", "application/json", nil); err == nil {
			resp.Body.Close()
		}
		done := make(chan struct{})
		go func() { clientCmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			clientCmd.Process.Kill()
		}
	}
	if tempDir != "" {
		os.RemoveAll(tempDir)
	}
}

// freePort asks the OS for an unused TCP port and returns it as a string.
func freePort() (string, error) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return "", err
	}
	defer ln.Close()
	return strconv.Itoa(ln.Addr().(*net.TCPAddr).Port), nil
}

// waitReady polls the Client's index page until it responds or the deadline hits.
func waitReady(timeout time.Duration) error {
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("client did not become ready at %s within %s", baseURL, timeout)
}

// --- shared JSON shapes (mirror the Client's public responses) ---------------

type snapshot struct {
	Version  string `json:"version"`
	Revision uint64 `json:"revision"`
	Shared   struct {
		Server string `json:"server"`
	} `json:"shared"`
	GlobalVariables map[string]string            `json:"global_variables"`
	PluginVariables map[string]map[string]string `json:"plugin_variables"`
}

type reportTask struct {
	TaskType string          `json:"task_type"`
	Status   string          `json:"status"`
	Result   json.RawMessage `json:"result"`
}

type clientStatus struct {
	ClientVersion string `json:"clientVersion"`
	Softwares     []struct {
		Name  string `json:"name"`
		AppID int    `json:"appID"`
	} `json:"softwares"`
}

// --- helpers -----------------------------------------------------------------

func doJSON(t *testing.T, method, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshalling body: %v", err)
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, baseURL+path, rdr)
	if err != nil {
		t.Fatalf("building request %s %s: %v", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func getSnapshot(t *testing.T) snapshot {
	t.Helper()
	resp := doJSON(t, http.MethodPost, "/settings/get", nil, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/settings/get status = %d, want 200", resp.StatusCode)
	}
	var snap snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatalf("decoding snapshot: %v", err)
	}
	return snap
}

// subscribeApp performs a /report so the Client registers our fake Blender app.
func subscribeApp(t *testing.T) {
	t.Helper()
	body := map[string]any{
		"app_id":           testAppID,
		"api_key":          "",
		"addon_version":    "3.13.0",
		"blender_version":  "4.2.2",
		"platform_version": "e2e-test",
		"project_name":     "e2e",
	}
	resp := doJSON(t, http.MethodPost, "/report", body, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/report status = %d, want 200", resp.StatusCode)
	}
}

// --- tests -------------------------------------------------------------------

// TestSettingsGetSeedsServer verifies the Client seeds shared.server from its
// startup --server value and exposes it via /settings/get.
func TestSettingsGetSeedsServer(t *testing.T) {
	snap := getSnapshot(t)
	if snap.Shared.Server != testServer {
		t.Errorf("shared.server = %q, want %q", snap.Shared.Server, testServer)
	}
	if snap.Revision < 1 {
		t.Errorf("revision = %d, want >= 1", snap.Revision)
	}
}

// TestSettingsSetServerBumpsRevisionAndPersists changes the shared server via
// /settings/set and confirms the revision grows and the change is readable back.
func TestSettingsSetServerBumpsRevisionAndPersists(t *testing.T) {
	before := getSnapshot(t)

	const newServer = "https://e2e.blendkit.test"
	resp := doJSON(t, http.MethodPost, "/settings/set", map[string]any{"server": newServer}, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/settings/set status = %d, want 200", resp.StatusCode)
	}
	var updated snapshot
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatalf("decoding set response: %v", err)
	}
	if updated.Shared.Server != newServer {
		t.Errorf("set: shared.server = %q, want %q", updated.Shared.Server, newServer)
	}
	if updated.Revision <= before.Revision {
		t.Errorf("set: revision = %d, want > %d", updated.Revision, before.Revision)
	}

	// A fresh read must reflect the change (single source of truth).
	if got := getSnapshot(t).Shared.Server; got != newServer {
		t.Errorf("after set: shared.server = %q, want %q", got, newServer)
	}
}

// TestSettingsSetVariable stores both a global and a plugin-scoped variable and
// reads them back from the snapshot.
func TestSettingsSetVariable(t *testing.T) {
	global := doJSON(t, http.MethodPost, "/settings/set_variable",
		map[string]any{"variable": "value", "value": "1"}, nil)
	global.Body.Close()
	if global.StatusCode != http.StatusOK {
		t.Fatalf("set_variable (global) status = %d, want 200", global.StatusCode)
	}

	plugin := doJSON(t, http.MethodPost, "/settings/set_variable",
		map[string]any{"plugin": "blender", "variable": "executable", "value": "/usr/bin/blender"}, nil)
	plugin.Body.Close()
	if plugin.StatusCode != http.StatusOK {
		t.Fatalf("set_variable (plugin) status = %d, want 200", plugin.StatusCode)
	}

	snap := getSnapshot(t)
	if got := snap.GlobalVariables["value"]; got != "1" {
		t.Errorf("global var value = %q, want 1", got)
	}
	if got := snap.PluginVariables["blender"]["executable"]; got != "/usr/bin/blender" {
		t.Errorf("plugin var blender.executable = %q, want /usr/bin/blender", got)
	}
}

// TestReportBroadcastsSettings verifies every /report response carries the
// current settings as a "settings" task, so plugins always converge.
func TestReportBroadcastsSettings(t *testing.T) {
	body := map[string]any{
		"app_id":           testAppID,
		"api_key":          "",
		"addon_version":    "3.13.0",
		"blender_version":  "4.2.2",
		"platform_version": "e2e-test",
		"project_name":     "e2e",
	}
	resp := doJSON(t, http.MethodPost, "/report", body, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/report status = %d, want 200", resp.StatusCode)
	}

	var tasks []reportTask
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		t.Fatalf("decoding /report tasks: %v", err)
	}

	var found bool
	for _, task := range tasks {
		if task.TaskType != "settings" {
			continue
		}
		found = true
		var snap snapshot
		if err := json.Unmarshal(task.Result, &snap); err != nil {
			t.Fatalf("decoding settings task result: %v", err)
		}
		if snap.Shared.Server == "" {
			t.Errorf("settings task carried empty shared.server")
		}
	}
	if !found {
		t.Errorf("no settings task found in /report response (got %d tasks)", len(tasks))
	}
}

// TestBkclientjsStatusListsConnected subscribes the fake app then verifies it
// appears in /bkclientjs/status (with a permitted CORS origin).
func TestBkclientjsStatusListsConnected(t *testing.T) {
	subscribeApp(t)

	resp := doJSON(t, http.MethodGet, "/bkclientjs/status", nil,
		map[string]string{"Origin": "https://blendkit.com"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/bkclientjs/status status = %d, want 200", resp.StatusCode)
	}

	var status clientStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("decoding status: %v", err)
	}

	var found bool
	for _, sw := range status.Softwares {
		if sw.AppID == testAppID && sw.Name == "Blender" {
			found = true
		}
	}
	if !found {
		t.Errorf("connected app %d not listed in /bkclientjs/status softwares: %+v", testAppID, status.Softwares)
	}
}
