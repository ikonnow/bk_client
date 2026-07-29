package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestForwardEventPostsToServerWithHeaders(t *testing.T) {
	var gotPath, gotSystemID, gotOrigin, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotSystemID = r.Header.Get("System-Id")
		gotOrigin = r.Header.Get("Origin-Name")
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		gotBody = buf.String()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	oldServer, oldClient := Server, ClientAPI
	defer func() { Server, ClientAPI = oldServer, oldClient }()
	serverURL := server.URL
	Server = &serverURL
	ClientAPI = server.Client()

	AvailableSoftwaresMux.Lock()
	origSoftwares := AvailableSoftwares
	AvailableSoftwares = map[int]Software{1: {Name: "Blender", AppID: 1}}
	AvailableSoftwaresMux.Unlock()
	defer func() {
		AvailableSoftwaresMux.Lock()
		AvailableSoftwares = origSoftwares
		AvailableSoftwaresMux.Unlock()
	}()

	forwardEvent(ReportEventData{
		AppID:           1,
		AddonVersion:    "3.21.0.250101",
		PlatformVersion: "test-platform",
		Event:           "login_started",
		Data:            json.RawMessage(`{"placement":"premium_popup","signup":false}`),
	})

	if gotPath != "/api/v1/telemetry/events/" {
		t.Errorf("posted to %q; want /api/v1/telemetry/events/", gotPath)
	}
	if gotOrigin != "Blender" {
		t.Errorf("Origin-Name header: got %q, want Blender", gotOrigin)
	}
	if gotSystemID == "" {
		t.Error("System-Id header not set")
	}
	var payload struct {
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(gotBody), &payload); err != nil {
		t.Fatalf("body is not valid JSON: %v (%q)", err, gotBody)
	}
	if payload.Event != "login_started" {
		t.Errorf("event = %q; want login_started", payload.Event)
	}
}

func TestForwardEventSilentOnServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	oldServer, oldClient := Server, ClientAPI
	defer func() { Server, ClientAPI = oldServer, oldClient }()
	serverURL := server.URL
	Server = &serverURL
	ClientAPI = server.Client()

	// Must not panic and must not create any Task - just log.
	forwardEvent(ReportEventData{Event: "login_cancelled"})
}

func TestReportEventHandlerBadJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/report_event", bytes.NewBufferString("{not json"))
	w := httptest.NewRecorder()
	ReportEventHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want %d", w.Code, http.StatusBadRequest)
	}
}
