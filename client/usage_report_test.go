package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/blenderkit/blenderkit/client/internal/settings"
)

// drainTaskChannels collects what ReportUsages puts on the task channels.
func drainTaskChannels(t *testing.T) (finished *TaskFinish, failed *TaskError) {
	t.Helper()
	<-AddTaskCh
	select {
	case finished = <-TaskFinishCh:
	case failed = <-TaskErrorCh:
	}
	return finished, failed
}

func withFakeServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	oldServer, oldClient := Server, ClientAPI
	serverURL := server.URL
	Server = &serverURL
	ClientAPI = server.Client()
	t.Cleanup(func() { Server, ClientAPI = oldServer, oldClient; server.Close() })
	return server
}

func TestReportUsagesForwardsTheReportUntouchedAndFinishesTheTask(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	withFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		gotBody = buf.String()
		w.WriteHeader(http.StatusCreated)
	})
	report := `{"scene":"9b675a0c-615e-4ff2-80aa-f4d2907f9cc2","event":"save","assetusageSet":[]}`

	go ReportUsages(ReportUsagesData{AppID: 1, ApiKey: "secret-token", Report: json.RawMessage(report)})
	finished, failed := drainTaskChannels(t)

	if failed != nil {
		t.Fatalf("task errored: %v", failed.Error)
	}
	if finished.AppID != 1 || finished.Message != "Usage report sent" {
		t.Errorf("finish = %+v", finished)
	}
	if gotPath != "/api/v1/scene_save_reports/" {
		t.Errorf("posted to %q", gotPath)
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotBody != report {
		t.Errorf("body = %q; want the report untouched", gotBody)
	}
}

func TestReportUsagesAnonymousSendsNoAuthorizationAndErrorsOnRejection(t *testing.T) {
	var gotAuth string
	withFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		http.Error(w, `{"detail":"nope"}`, http.StatusBadRequest)
	})

	go ReportUsages(ReportUsagesData{AppID: 2, Report: json.RawMessage(`{}`)})
	finished, failed := drainTaskChannels(t)

	if gotAuth != "" {
		t.Errorf("Authorization sent for a logged-out add-on: %q", gotAuth)
	}
	if finished != nil || failed == nil || failed.AppID != 2 {
		t.Fatalf("want a task error for the rejection, got finish=%+v err=%+v", finished, failed)
	}
}

func TestReportUsagesHandlerRejectsBadInput(t *testing.T) {
	for name, body := range map[string]string{"not json": "{not json", "missing report": `{"app_id": 1}`} {
		req := httptest.NewRequest(http.MethodPost, "/report_usages", bytes.NewBufferString(body))
		w := httptest.NewRecorder()
		ReportUsagesHandler(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d; want 400", name, w.Code)
		}
	}
}

func TestReportUsagesHandlerDropsReportsWhenOptedOut(t *testing.T) {
	store, err := settings.Open(filepath.Join(t.TempDir(), "settings.json"), "test", settings.Shared{Server: "https://x.test"})
	if err != nil {
		t.Fatal(err)
	}
	oldStore := SettingsStore
	SettingsStore = store
	t.Cleanup(func() { SettingsStore = oldStore })
	if _, err := store.SetShared(settings.Shared{Server: "https://x.test", UsageDataOptOut: true}); err != nil {
		t.Fatal(err)
	}
	posted := false
	withFakeServer(t, func(w http.ResponseWriter, r *http.Request) { posted = true })

	req := httptest.NewRequest(http.MethodPost, "/report_usages", bytes.NewBufferString(`{"app_id":1,"report":{"scene":"x"}}`))
	w := httptest.NewRecorder()
	ReportUsagesHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200 (opt-out is not an error)", w.Code)
	}
	select {
	case task := <-AddTaskCh:
		t.Errorf("a task was created despite the opt-out: %+v", task)
	default:
	}
	if posted {
		t.Error("report reached the server despite the opt-out")
	}
}

func TestSetSettingsHandlerStoresTheOptOut(t *testing.T) {
	store, err := settings.Open(filepath.Join(t.TempDir(), "settings.json"), "test", settings.Shared{Server: "https://x.test"})
	if err != nil {
		t.Fatal(err)
	}
	oldStore := SettingsStore
	SettingsStore = store
	t.Cleanup(func() { SettingsStore = oldStore })

	req := httptest.NewRequest(http.MethodPost, "/settings/set", bytes.NewBufferString(`{"usage_data_opt_out": true}`))
	w := httptest.NewRecorder()
	setSettingsHandler(w, req)

	snap := store.Snapshot()
	if !snap.Shared.UsageDataOptOut {
		t.Error("opt-out not stored")
	}
	if snap.Shared.Server != "https://x.test" {
		t.Errorf("server changed to %q", snap.Shared.Server)
	}
}
