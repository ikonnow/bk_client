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
	"net/url"
	"os"
	"strings"
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

// isCloudflareChallenge reports whether a response looks like a Cloudflare
// managed "Just a moment..." challenge page rather than a real API answer.
func isCloudflareChallenge(status int, body []byte) bool {
	if status != http.StatusForbidden && status != http.StatusServiceUnavailable {
		return false
	}
	b := strings.ToLower(string(body))
	return strings.Contains(b, "just a moment") ||
		strings.Contains(b, "challenge-platform") ||
		strings.Contains(b, "cf_chl") ||
		strings.Contains(b, "cloudflare")
}

// TestLiveSearchAddonReproduction replays the exact search the Blender add-on
// makes (the "bricks asset_type:brush order:_score" query with addon_version
// and blender_version params) that Cloudflare challenged in CI, sending it under
// several User-Agent values to reveal which one the WAF blocks. Run with -v to
// see the per-User-Agent outcome.
func TestLiveSearchAddonReproduction(t *testing.T) {
	if os.Getenv("BLENDKIT_LIVE") == "" && os.Getenv("BLENDKIT_API_KEY") == "" {
		t.Skip("set BLENDKIT_LIVE=1 or BLENDKIT_API_KEY to run live tests")
	}

	q := url.Values{}
	q.Set("query", "bricks asset_type:brush order:_score")
	q.Set("dict_parameters", "1")
	q.Set("page_size", "15")
	q.Set("addon_version", "3.21.1")
	q.Set("blender_version", "3.1.2")
	searchURL := liveServer() + "/api/v1/search/?" + q.Encode()

	// "" means leave Go's default User-Agent ("Go-http-client/1.1"); "-" sends
	// no User-Agent header at all; anything else is sent verbatim.
	userAgents := []struct {
		name string
		ua   string
	}{
		{"go-default", ""},
		{"python-requests", "python-requests/2.31.0"},
		{"blenderkit-addon", "Blender/3.1.2 BlenderKit/3.21.1"},
		{"no-user-agent", "-"},
	}

	for _, tc := range userAgents {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, searchURL, nil)
			if err != nil {
				t.Fatalf("building request: %v", err)
			}
			req.Header = getHeaders(os.Getenv("BLENDKIT_API_KEY"), *getSystemID(), "3.21.1", "3.1.2", 0)
			switch tc.ua {
			case "":
				// keep Go's default User-Agent
			case "-":
				req.Header.Set("User-Agent", "") // Go omits the header when set to ""
			default:
				req.Header.Set("User-Agent", tc.ua)
			}

			resp, err := liveHTTPClient().Do(req)
			if err != nil {
				t.Fatalf("GET %s: %v", searchURL, err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

			if isCloudflareChallenge(resp.StatusCode, body) {
				t.Fatalf("Cloudflare challenge for User-Agent %q (status %s)", tc.ua, resp.Status)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("search returned %s for User-Agent %q: %s", resp.Status, tc.ua, body)
			}
			t.Logf("User-Agent %q -> %s (OK)", tc.ua, resp.Status)
		})
	}
}
