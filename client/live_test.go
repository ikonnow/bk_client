//go:build live

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

// Live integration tests that hit a real Blendkit server.
//
// These are gated behind the "live" build tag so they never run in normal
// `go test ./...` or CI. Run them explicitly against the devel server with a
// dedicated test account's key:
//
//	# PowerShell
//	$env:BLENDKIT_API_KEY="..."; $env:BLENDKIT_SERVER="https://devel.blendkit.com"
//	go test -tags=live -run TestLive ./...
//
//	# bash
//	BLENDKIT_API_KEY=... BLENDKIT_SERVER=https://devel.blendkit.com \
//	  go test -tags=live -run TestLive ./...
//
// When BLENDKIT_API_KEY is unset the auth-requiring tests skip themselves, so
// the suite is safe to invoke without credentials.

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// liveServer returns the server to test against, defaulting to the devel server
// so live tests never accidentally hit production.
func liveServer() string {
	if s := os.Getenv("BLENDKIT_SERVER"); s != "" {
		return s
	}
	return "https://devel.blendkit.com"
}

// liveAPIKey returns the test API key, or skips the test when it is not set.
func liveAPIKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("BLENDKIT_API_KEY")
	if key == "" {
		t.Skip("set BLENDKIT_API_KEY (and optionally BLENDKIT_SERVER) to run live tests")
	}
	return key
}

// liveHTTPClient is a plain client with a sane timeout; live tests do not rely
// on the proxy/TLS plumbing configured by CreateHTTPClients.
func liveHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

// TestLiveSearchPublic verifies the public search endpoint responds without
// authentication. This catches gross connectivity/server-shape regressions.
func TestLiveSearchPublic(t *testing.T) {
	if os.Getenv("BLENDKIT_LIVE") == "" && os.Getenv("BLENDKIT_API_KEY") == "" {
		t.Skip("set BLENDKIT_LIVE=1 or BLENDKIT_API_KEY to run live tests")
	}

	url := liveServer() + "/api/v1/search/?query=test&page_size=1"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header = getHeaders("", *getSystemID(), "0.0.0", "", 0)

	resp, err := liveHTTPClient().Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		t.Fatalf("search returned %s: %s", resp.Status, body)
	}

	var payload struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decoding search response: %v", err)
	}
}

// TestLiveGetUserProfile verifies that the configured API key authenticates
// against the real /me/ endpoint. Skips when no key is provided.
func TestLiveGetUserProfile(t *testing.T) {
	key := liveAPIKey(t)

	url := liveServer() + "/api/v1/me/"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header = getHeaders(key, *getSystemID(), "0.0.0", "", 0)

	resp, err := liveHTTPClient().Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		t.Fatalf("/me/ returned %s (is BLENDKIT_API_KEY valid for %s?): %s", resp.Status, liveServer(), body)
	}

	var profile struct {
		User struct {
			ID    int    `json:"id"`
			Email string `json:"email"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		t.Fatalf("decoding /me/ response: %v", err)
	}
	if profile.User.ID == 0 {
		t.Fatalf("/me/ returned no user id; response shape may have changed")
	}
	t.Logf("authenticated as user id %d on %s", profile.User.ID, liveServer())
}
