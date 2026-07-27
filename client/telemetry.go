package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// ReportEventData is expected from the add-on for fire-and-forget telemetry events.
type ReportEventData struct {
	AppID           int             `json:"app_id"`
	ApiKey          string          `json:"api_key"`
	AddonVersion    string          `json:"addon_version"`
	PlatformVersion string          `json:"platform_version"`
	Event           string          `json:"event"`
	Data            json.RawMessage `json:"data"`
}

// ReportEventHandler accepts a telemetry event from the add-on and forwards it
// to the server in the background. Unlike wrappers it creates no Task and
// reports nothing back to the UI: telemetry must never surface errors to users,
// even while the server endpoint does not exist yet.
func ReportEventHandler(w http.ResponseWriter, r *http.Request) {
	var data ReportEventData
	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		es := fmt.Sprintf("error parsing JSON: %v", err)
		fmt.Println(es)
		http.Error(w, es, http.StatusBadRequest)
		return
	}

	go forwardEvent(data)
	w.WriteHeader(http.StatusOK)
}

// forwardEvent posts the event to the server with the standard headers
// (system_id, add-on version, platform, API key). Failures are only logged.
func forwardEvent(data ReportEventData) {
	body, err := json.Marshal(map[string]interface{}{
		"event": data.Event,
		"data":  data.Data,
	})
	if err != nil {
		BKLog.Printf("Telemetry event %q: cannot marshal: %v", data.Event, err)
		return
	}

	url := *Server + "/api/v1/telemetry/events/"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		BKLog.Printf("Telemetry event %q: cannot create request: %v", data.Event, err)
		return
	}
	req.Header = getHeaders(data.ApiKey, *SystemID, data.AddonVersion, data.PlatformVersion)

	resp, err := ClientAPI.Do(req)
	if err != nil {
		BKLog.Printf("Telemetry event %q: request failed: %v", data.Event, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		BKLog.Printf("Telemetry event %q: server returned %s", data.Event, resp.Status)
	}
}
