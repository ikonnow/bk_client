package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

// ReportUsagesData is expected from the add-on for the save-time usage report.
type ReportUsagesData struct {
	AppID           int             `json:"app_id"`
	ApiKey          string          `json:"api_key"`
	AddonVersion    string          `json:"addon_version"`
	PlatformVersion string          `json:"platform_version"`
	Report          json.RawMessage `json:"report"` // forwarded to the server untouched
}

// ReportUsagesHandler accepts a usage report (the Blendkit assets present in a
// file at a save or a render) and forwards it to the server as a
// "report_usages" task. Dropped without a task when the user opted out of
// sending usage data (Shared.UsageDataOptOut).
func ReportUsagesHandler(w http.ResponseWriter, r *http.Request) {
	var data ReportUsagesData
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		es := fmt.Sprintf("error parsing JSON: %v", err)
		fmt.Println(es)
		http.Error(w, es, http.StatusBadRequest)
		return
	}
	if len(data.Report) == 0 {
		http.Error(w, "missing report", http.StatusBadRequest)
		return
	}
	if SettingsStore != nil && SettingsStore.Snapshot().Shared.UsageDataOptOut {
		w.WriteHeader(http.StatusOK)
		return
	}

	go ReportUsages(data)
	w.WriteHeader(http.StatusOK)
}

// ReportUsages posts the report to the server and finishes or errors the task.
func ReportUsages(data ReportUsagesData) {
	taskUUID := uuid.New().String()
	AddTaskCh <- NewTask(data, data.AppID, taskUUID, "report_usages")

	url := *Server + "/api/v1/scene_save_reports/"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(data.Report))
	if err != nil {
		TaskErrorCh <- &TaskError{AppID: data.AppID, TaskID: taskUUID, Error: fmt.Errorf("usage report - making request: %w", err)}
		return
	}
	req.Header = getHeaders(data.ApiKey, *SystemID, data.AddonVersion, data.PlatformVersion, data.AppID)

	resp, err := ClientAPI.Do(req)
	if err != nil {
		TaskErrorCh <- &TaskError{AppID: data.AppID, TaskID: taskUUID, Error: fmt.Errorf("usage report: %w", err)}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, respString, _ := ParseFailedHTTPResponse(resp)
		TaskErrorCh <- &TaskError{AppID: data.AppID, TaskID: taskUUID, Error: fmt.Errorf("usage report: %s (%s)", respString, resp.Status)}
		return
	}
	TaskFinishCh <- &TaskFinish{AppID: data.AppID, TaskID: taskUUID, Message: "Usage report sent", Result: map[string]string{}}
}
