package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockHttpResponse creates a new http.Response from the given body and status code.
func mockHTTPResponse(body string, statusCode int) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Request: &http.Request{
			URL: &url.URL{
				Scheme: "http",
				Host:   "example.com",
			},
		},
	}
}

func TestParseFailedHTTPResponse(t *testing.T) {
	tests := []struct {
		name       string
		response   *http.Response
		wantErr    bool
		errMessage string
	}{
		{
			name:     "Valid JSON with string detail",
			response: mockHTTPResponse(`{"detail": "scene_uuid is not a valid UUID", "statusCode": 403}`, 403),
			wantErr:  false,
		},
		{
			name:     "Valid JSON with map detail",
			response: mockHTTPResponse(`{"detail":{"thumbnail": "Invalid image format. Only PNG and JPEG are supported."},"statusCode": 400}`, 400),
			wantErr:  false,
		},
		{
			name:       "Invalid JSON",
			response:   mockHTTPResponse("invalid json", 400),
			wantErr:    true,
			errMessage: "invalid json",
		},
		{
			name:     "Valid JSON with complex structure",
			response: mockHTTPResponse(`{"detail": "Limit of private storage exceeded. Limit is 1.0 B, 1.0 B is remaining. You tried to add 7.0 B", "addedSize": 7, "addedSizeFmt": "7.0 B", "code": "private_quota_limit", "freeQuota": 1, "freeQuotaFmt": "1.0 B", "quota": 1, "quotaFmt": "1.0 B"}`, 400),
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			JSON, bodyString, err := ParseFailedHTTPResponse(tt.response)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFailedHTTPResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errMessage) {
				t.Errorf("ParseFailedHTTPResponse() error = %v, wantErr containing %s", err, tt.errMessage)
			}
			if tt.wantErr {
				return
			}
			if !json.Valid(JSON) {
				t.Errorf("ParseFailedHTTPResponse() got invalid JSON")
			}
			if bodyString == "" {
				t.Errorf("ParseFailedHTTPResponse() got empty bodyString")
			}
		})
	}
}

func TestDictToParams(t *testing.T) {
	tests := []struct {
		name     string
		inputs   map[string]interface{}
		expected []map[string]string
	}{
		{
			name:     "Empty input",
			inputs:   map[string]interface{}{},
			expected: []map[string]string{},
		},
		{
			name: "String input",
			inputs: map[string]interface{}{
				"key": "value",
			},
			expected: []map[string]string{
				{"parameterType": "key", "value": "value"},
			},
		},
		{
			name: "String slice input",
			inputs: map[string]interface{}{
				"key": []string{"value1", "value2"},
			},
			expected: []map[string]string{
				{"parameterType": "key", "value": "value1,value2"},
			},
		},
		{
			name: "Bool input",
			inputs: map[string]interface{}{
				"key": true,
			},
			expected: []map[string]string{
				{"parameterType": "key", "value": "true"},
			},
		},
		{
			name: "Int input",
			inputs: map[string]interface{}{
				"int": int(42),
			},
			expected: []map[string]string{
				{"parameterType": "int", "value": "42"},
			},
		},
		{
			name: "Int input - negative",
			inputs: map[string]interface{}{
				"int32": int32(-42 * 1000 * 1000),
			},
			expected: []map[string]string{
				{"parameterType": "int32", "value": "-42000000"},
			},
		},
		{
			name: "Int input - huge",
			inputs: map[string]interface{}{
				"int64": int(42 * 1000 * 1000 * 1000 * 1000 * 1000),
			},
			expected: []map[string]string{
				{"parameterType": "int64", "value": "42000000000000000"},
			},
		},
		{
			name: "Float inputs",
			inputs: map[string]interface{}{
				"float32": float32(-3.000),
			},
			expected: []map[string]string{
				{"parameterType": "float32", "value": "-3"},
			},
		},
		{
			name: "Float input - with trailing zeros",
			inputs: map[string]interface{}{
				"float64": float64(3.123456789000),
			},
			expected: []map[string]string{
				{"parameterType": "float64", "value": "3.123456789"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DictToParams(tt.inputs)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("DictToParams(%v) = %v, expected %v", tt.inputs, result, tt.expected)
			}
		})
	}
}

// Test helper function to generate random connected Software.
func generateSoftware(seed uint64) Software {
	r := rand.New(rand.NewPCG(seed, seed))
	id := r.IntN(4000) + 1000
	version := fmt.Sprintf(
		"%d.%d.%d",
		r.IntN(100),
		r.IntN(100),
		r.IntN(100),
	)
	addon_version := fmt.Sprintf(
		"%d.%d.%d",
		r.IntN(100),
		r.IntN(100),
		r.IntN(100),
	)
	names := []string{"Blender", "Godot", "Maya", "Unreal"}
	index := r.IntN(len(names))
	software := Software{
		AppID:        id,
		Name:         names[index],
		Version:      version,
		AddonVersion: addon_version,
	}
	return software
}

// Test helper function to generate random map of connected Softwares of given size.
// Seed is used to keep the "random" values same on every run of the tests.
func generateSoftwares(size, seed uint64) map[int]Software {
	sm := map[int]Software{}
	for i := range size {
		software := generateSoftware(seed + i)
		sm[software.AppID] = software
	}
	return sm
}

func BenchmarkGetAvailableSoftwares(b *testing.B) {
	AvailableSoftwares = make(map[int]Software)
	AvailableSoftwaresMux = sync.Mutex{}
	benches := []struct {
		name        string
		softwareMap map[int]Software
	}{
		{
			name:        "0 running", // starting work
			softwareMap: map[int]Software{},
		},
		{
			name: "1 running", // this is like normal use case
			softwareMap: map[int]Software{
				1001: {AppID: 1001, Name: "Blender", Version: "4.2.1", AddonVersion: "3.13.0"},
			},
		},
		{
			name: "2 running", // quite normal use
			softwareMap: map[int]Software{
				1111: {AppID: 1001, Name: "Blender", Version: "4.2.1", AddonVersion: "3.13.0"},
				2222: {AppID: 2222, Name: "Godot", Version: "4.3.0", AddonVersion: "0.1.0"},
			},
		},
		{
			name: "4 running", // quite normal use
			softwareMap: map[int]Software{
				1111: {AppID: 1001, Name: "Blender", Version: "4.2.1", AddonVersion: "3.13.0"},
				2222: {AppID: 2222, Name: "Godot", Version: "4.3.0", AddonVersion: "0.1.0"},
				3333: {AppID: 3333, Name: "Maya", Version: "2027.2", AddonVersion: "0.2.0"},
				4444: {AppID: 4444, Name: "Unreal", Version: "5.8", AddonVersion: "0.0.11"},
			},
		},
		{
			name:        "8 running", // quite big usage
			softwareMap: generateSoftwares(8, 111),
		},
		{
			name:        "64 running", // unexpected extreme just for testing
			softwareMap: generateSoftwares(64, 111),
		},
	}
	for _, bench := range benches {
		b.Run(bench.name, func(b *testing.B) {
			AvailableSoftwaresMux.Lock()
			AvailableSoftwares = bench.softwareMap
			AvailableSoftwaresMux.Unlock()
			for b.Loop() {
				// TODO: looks that there is a space for improvement of the function perf
				GetAvailableSoftwares()
			}
		})
	}
	// clean global variables after the test
	AvailableSoftwares = make(map[int]Software)
	AvailableSoftwaresMux = sync.Mutex{}
}

func TestGetAvailableSoftwares(t *testing.T) {
	AvailableSoftwares = make(map[int]Software)
	AvailableSoftwaresMux = sync.Mutex{}

	tests := []struct {
		name          string
		softwareMap   map[int]Software
		expectedCount int
	}{
		{
			name:          "Empty map",
			softwareMap:   map[int]Software{},
			expectedCount: 0,
		},
		{
			name: "Map with one software",
			softwareMap: map[int]Software{
				1001: {AppID: 1001, Name: "Blender", Version: "4.2.1", AddonVersion: "3.13.0"},
			},
			expectedCount: 1,
		},
		{
			name: "Map with multiple softwares",
			softwareMap: map[int]Software{
				1001: {AppID: 1001, Name: "Blender", Version: "4.2.1", AddonVersion: "3.13.0"},
				2222: {AppID: 2222, Name: "Godot", Version: "4.3.0", AddonVersion: "0.1.0"},
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			AvailableSoftwaresMux.Lock()
			AvailableSoftwares = tt.softwareMap
			AvailableSoftwaresMux.Unlock()

			result := GetAvailableSoftwares()

			if len(result) != tt.expectedCount {
				t.Errorf("Expected %d softwares, got %d", tt.expectedCount, len(result))
			}

			for _, software := range result {
				if _, exists := tt.softwareMap[software.AppID]; !exists {
					t.Errorf("Software with AppID %d not found in original map", software.AppID)
				}
			}
		})
	}

	// clean global variables after the test
	AvailableSoftwares = make(map[int]Software)
	AvailableSoftwaresMux = sync.Mutex{}
}

func TestUpdateAvailableSoftware(t *testing.T) {
	AvailableSoftwares = make(map[int]Software)
	AvailableSoftwaresMux = sync.Mutex{}

	tests := []struct {
		name                   string
		inputSoftware          Software
		expectedNew            bool
		expectedSoftwaresCount int
		expectedAppID          int
		expectedName           string
		expectedAssetsPath     string
	}{
		{
			name:                   "New Blender connected",
			inputSoftware:          Software{AppID: 2422, Name: "Blender", Version: "4.2.2", AddonVersion: "3.13.0", AssetsPath: "/home/me/blenderkit_data"},
			expectedNew:            true,
			expectedSoftwaresCount: 1,
			expectedAppID:          2422,
			expectedName:           "Blender",
			expectedAssetsPath:     "/home/me/blenderkit_data",
		},
		{
			name:                   "Update connected Blender",
			inputSoftware:          Software{AppID: 2422, Name: "Blender", Version: "4.2.2", AddonVersion: "3.13.0", AssetsPath: "/home/me/another_path"},
			expectedNew:            false,
			expectedSoftwaresCount: 1,
			expectedAppID:          2422,
			expectedName:           "Blender",
			expectedAssetsPath:     "/home/me/another_path",
		},
		{
			name:                   "New Godot connected",
			inputSoftware:          Software{AppID: 7431, Name: "Godot", Version: "4.3.1", AddonVersion: "0.1.0", AssetsPath: "/home/me/godot/my_project"},
			expectedNew:            true,
			expectedSoftwaresCount: 2,
			expectedAppID:          7431,
			expectedName:           "Godot",
			expectedAssetsPath:     "/home/me/godot/my_project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beforeTime := time.Now()
			isNew := updateAvailableSoftware(tt.inputSoftware)
			afterTime := time.Now()
			if isNew != tt.expectedNew {
				t.Errorf("Expected new software: %v, got: %v", tt.expectedNew, isNew)
			}

			AvailableSoftwaresMux.Lock()
			defer AvailableSoftwaresMux.Unlock()

			if len(AvailableSoftwares) != tt.expectedSoftwaresCount {
				t.Errorf("Expected %d softwares, got %d", tt.expectedSoftwaresCount, len(AvailableSoftwares))
			}

			software, exists := AvailableSoftwares[tt.expectedAppID]
			if !exists {
				t.Errorf("Software with appID %d not found in AvailableSoftwares", tt.expectedAppID)
			}

			if software.Name != tt.expectedName {
				t.Errorf("Expected software name %s, got %s", tt.expectedName, software.Name)
			}

			if software.AssetsPath != tt.expectedAssetsPath {
				t.Errorf("Expected AssetsPath %s, got %s", tt.expectedAssetsPath, software.AssetsPath)
			}

			if !(beforeTime.Before(software.lastTimeConnected)) {
				t.Errorf("Time before (%v) calling update is not Before lastTimeConnected (%v)", beforeTime, software.lastTimeConnected)
			}

			if !(afterTime.After(software.lastTimeConnected)) {
				t.Errorf("Time after (%v) calling update is not After lastTimeConnected (%v)", afterTime, software.lastTimeConnected)
			}
		})
	}
	// clean global variables after the test
	AvailableSoftwares = make(map[int]Software)
	AvailableSoftwaresMux = sync.Mutex{}
}

// Test if the Client handles the CORS preflight checks (OPTIONS requests) correctly.
// For correct origins the allow-methods, allow-headers and allow-private-network should be set.
func TestBkclientjsPreflightResponses(t *testing.T) {
	tests := []struct {
		name                        string
		endpoint                    string
		origin                      string
		expectedStatus              int
		expectedAllowCredentials    string
		expectedAllowMethods        string
		expectedAllowHeaders        string
		expectedAllowPrivateNetwork string
	}{
		{ // bkclientjsStatusHandler
			name:                        "/status CORS preflight headers are set for localhost",
			endpoint:                    "/status",
			origin:                      "http://localhost:8765",
			expectedStatus:              http.StatusNoContent,
			expectedAllowCredentials:    "true",
			expectedAllowMethods:        "GET, POST, OPTIONS",
			expectedAllowHeaders:        "Content-Type",
			expectedAllowPrivateNetwork: "true",
		}, {
			name:                        "/status CORS preflight headers are set for blendkit.com",
			endpoint:                    "/status",
			origin:                      "https://blendkit.com",
			expectedStatus:              http.StatusNoContent,
			expectedAllowCredentials:    "true",
			expectedAllowMethods:        "GET, POST, OPTIONS",
			expectedAllowHeaders:        "Content-Type",
			expectedAllowPrivateNetwork: "true",
		}, {
			name:                        "/status CORS preflight headers are set for test.blendkit.com",
			endpoint:                    "/status",
			origin:                      "https://test.blendkit.com",
			expectedStatus:              http.StatusNoContent,
			expectedAllowCredentials:    "true",
			expectedAllowMethods:        "GET, POST, OPTIONS",
			expectedAllowHeaders:        "Content-Type",
			expectedAllowPrivateNetwork: "true",
		}, {
			name:                        "/status CORS preflight not allowed for wrong domain",
			endpoint:                    "/status",
			origin:                      "https://hacker.net",
			expectedStatus:              http.StatusForbidden,
			expectedAllowCredentials:    "",
			expectedAllowMethods:        "",
			expectedAllowHeaders:        "",
			expectedAllowPrivateNetwork: "",
		},

		{ // bkclientjsGetAssetHandler
			name:                        "/get_asset CORS preflight headers are set for localhost",
			endpoint:                    "/get_asset",
			origin:                      "http://localhost:8765",
			expectedStatus:              http.StatusNoContent,
			expectedAllowCredentials:    "true",
			expectedAllowMethods:        "GET, POST, OPTIONS",
			expectedAllowHeaders:        "Content-Type",
			expectedAllowPrivateNetwork: "true",
		}, {
			name:                        "/get_asset CORS preflight headers are set for blendkit.com",
			endpoint:                    "/get_asset",
			origin:                      "https://blendkit.com",
			expectedStatus:              http.StatusNoContent,
			expectedAllowCredentials:    "true",
			expectedAllowMethods:        "GET, POST, OPTIONS",
			expectedAllowHeaders:        "Content-Type",
			expectedAllowPrivateNetwork: "true",
		}, {
			name:                        "/get_asset CORS preflight headers are set for test.blendkit.com",
			endpoint:                    "/get_asset",
			origin:                      "https://test.blendkit.com",
			expectedStatus:              http.StatusNoContent,
			expectedAllowCredentials:    "true",
			expectedAllowMethods:        "GET, POST, OPTIONS",
			expectedAllowHeaders:        "Content-Type",
			expectedAllowPrivateNetwork: "true",
		}, {
			name:                        "/get_asset CORS preflight not allowed for wrong domain",
			endpoint:                    "/get_asset",
			origin:                      "https://hacker.net",
			expectedStatus:              http.StatusForbidden,
			expectedAllowCredentials:    "",
			expectedAllowMethods:        "",
			expectedAllowHeaders:        "",
			expectedAllowPrivateNetwork: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("OPTIONS", "/status", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Origin", tt.origin)

			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(bkclientjsStatusHandler)
			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v, expected %v", status, tt.expectedStatus)
			}

			allow_credentials := rr.Header().Get("Access-Control-Allow-Credentials")
			if allow_credentials != tt.expectedAllowCredentials {
				t.Errorf("handler set wrong header Access-Control-Allow-Credentials: got %s, expected %s", allow_credentials, tt.expectedAllowCredentials)
			}

			allow_methods := rr.Header().Get("Access-Control-Allow-Methods")
			if allow_methods != tt.expectedAllowMethods {
				t.Errorf("handler set wrong header Access-Control-Allow-Methods: got %s, expected %s", allow_methods, tt.expectedAllowMethods)
			}

			allow_headers := rr.Header().Get("Access-Control-Allow-Headers")
			if allow_headers != tt.expectedAllowHeaders {
				t.Errorf("handler set wrong header Access-Control-Allow-Headers: got %s, expected %s", allow_headers, tt.expectedAllowHeaders)
			}

			allow_private_network := rr.Header().Get("Access-Control-Allow-Private-Network")
			if allow_private_network != tt.expectedAllowPrivateNetwork {
				t.Errorf("handler set wrong header Access-Control-Allow-Private-Network: got %s, expected %s", allow_private_network, tt.expectedAllowPrivateNetwork)
			}
		})
	}
}

// Test the actual request comming to get the bkclientjs /status.
// Browsers only do this request after the OPTIONS preflight check for CORS and private_network passes.
func TestBkclientjsStatusHandler(t *testing.T) {
	tests := []struct {
		name                 string
		origin               string
		availableSoftwares   map[int]Software
		expectedStatus       int
		expectedClientStatus ClientStatus
	}{
		{
			name:               "Empty software list",
			origin:             "http://localhost:8765",
			availableSoftwares: map[int]Software{},
			expectedStatus:     http.StatusOK,
			expectedClientStatus: ClientStatus{
				ClientVersion: ClientVersion,
				Softwares:     nil,
			},
		},
		{
			name:               "Origin from blendkit.com",
			origin:             "https://blendkit.com",
			availableSoftwares: map[int]Software{},
			expectedStatus:     http.StatusOK,
			expectedClientStatus: ClientStatus{
				ClientVersion: ClientVersion,
				Softwares:     nil,
			},
		},
		{
			name:               "Origin from subdomain test.blendkit.com",
			origin:             "https://test.blendkit.com",
			availableSoftwares: map[int]Software{},
			expectedStatus:     http.StatusOK,
			expectedClientStatus: ClientStatus{
				ClientVersion: ClientVersion,
				Softwares:     nil,
			},
		},
		{
			name:   "Single software",
			origin: "http://localhost:8765",
			availableSoftwares: map[int]Software{
				1001: {AppID: 1001, Name: "Blender", Version: "4.2.1", AddonVersion: "3.13.0"},
			},
			expectedStatus: http.StatusOK,
			expectedClientStatus: ClientStatus{
				ClientVersion: ClientVersion,
				Softwares: []Software{
					{AppID: 1001, Name: "Blender", Version: "4.2.1", AddonVersion: "3.13.0"},
				},
			},
		},
		{
			name:   "Multiple softwares",
			origin: "http://localhost:8765",
			availableSoftwares: map[int]Software{
				1001: {AppID: 1001, Name: "Blender", Version: "4.2.1", AddonVersion: "3.13.0"},
				2222: {AppID: 2222, Name: "Godot", Version: "4.3.0", AddonVersion: "0.1.0"},
			},
			expectedStatus: http.StatusOK,
			expectedClientStatus: ClientStatus{
				ClientVersion: ClientVersion,
				Softwares: []Software{
					{AppID: 1001, Name: "Blender", Version: "4.2.1", AddonVersion: "3.13.0"},
					{AppID: 2222, Name: "Godot", Version: "4.3.0", AddonVersion: "0.1.0"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			AvailableSoftwares = tt.availableSoftwares
			AvailableSoftwaresMux = sync.Mutex{}

			req, err := http.NewRequest("GET", "/status", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Origin", tt.origin)

			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(bkclientjsStatusHandler)
			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v, expected %v", status, tt.expectedStatus)
			}

			var clientStatus ClientStatus
			err = json.Unmarshal(rr.Body.Bytes(), &clientStatus)
			if err != nil {
				t.Fatalf("Could not unmarshal response body: %v", err)
			}

			sortSoftwares(clientStatus.Softwares)
			sortSoftwares(tt.expectedClientStatus.Softwares)
			if !reflect.DeepEqual(clientStatus, tt.expectedClientStatus) {
				t.Errorf("handler returned unexpected clientStatus: got %v, expected %v", clientStatus, tt.expectedClientStatus)
			}

			allow_origin := rr.Header().Get("Access-Control-Allow-Origin")
			if allow_origin != tt.origin {
				t.Errorf("handler set wrong Access-Control-Allow-Origin: got %s, expected %s", allow_origin, tt.origin)
			}

			// clean global variables after the test
			AvailableSoftwares = make(map[int]Software)
			AvailableSoftwaresMux = sync.Mutex{}
		})
	}
}

// Helper function to sort the Software slice by AppID
// so we can run DeepEqual robustly.
func sortSoftwares(softwares []Software) {
	sort.Slice(softwares, func(i, j int) bool {
		return softwares[i].AppID < softwares[j].AppID
	})
}

func BenchmarkTaskFinish(b *testing.B) {
	benches := []struct {
		name            string
		status          string
		startMessage    string
		finalizeMessage string
	}{
		{
			name:            "Finish task with empty initial message",
			status:          "running",
			startMessage:    "",
			finalizeMessage: "Task completed successfully",
		},
		{
			name:            "Finish already finished task",
			status:          "finished",
			startMessage:    "Task already done",
			finalizeMessage: "Attempting to finish again",
		},
	}
	for _, bench := range benches {
		b.Run(bench.name, func(b *testing.B) {
			for b.Loop() {
				task := &Task{
					Status:  bench.status,
					Message: bench.startMessage,
				}
				task.Finish(bench.finalizeMessage)
			}
		})
	}
}

func TestTaskFinish(t *testing.T) {
	tests := []struct {
		name           string
		initialStatus  string
		initialMessage string
		finishMessage  string
		expectedStatus string
	}{
		{
			name:           "Finish task with empty initial message",
			initialStatus:  "running",
			initialMessage: "",
			finishMessage:  "Task completed successfully",
			expectedStatus: "finished",
		},
		{
			name:           "Finish task with existing message",
			initialStatus:  "processing",
			initialMessage: "Processing data",
			finishMessage:  "Data processing complete",
			expectedStatus: "finished",
		},
		{
			name:           "Finish already finished task",
			initialStatus:  "finished",
			initialMessage: "Task already done",
			finishMessage:  "Attempting to finish again",
			expectedStatus: "finished",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{
				Status:  tt.initialStatus,
				Message: tt.initialMessage,
			}
			task.Finish(tt.finishMessage)

			if task.Status != tt.expectedStatus {
				t.Errorf("Expected status %s, got %s", tt.expectedStatus, task.Status)
			}
			if task.Message != tt.finishMessage {
				t.Errorf("Expected message %s, got %s", tt.finishMessage, task.Message)
			}
		})
	}
}

func BenchmarkNewTask(b *testing.B) {
	benches := []struct {
		name     string
		data     interface{}
		appID    int
		taskID   string
		taskType string
	}{
		{
			name:     "New task with nil data",
			data:     nil,
			appID:    1001,
			taskID:   "task1",
			taskType: "download",
		},
		{
			name:     "New task with map data",
			data:     map[string]interface{}{"key": "value"},
			appID:    2000,
			taskID:   "task2",
			taskType: "upload",
		},
		{
			name:     "New task with slice data",
			data:     []string{"item1", "item2"},
			appID:    3000,
			taskID:   "task3",
			taskType: "process",
		},
	}

	for _, bench := range benches {
		b.Run(bench.name, func(b *testing.B) {
			for b.Loop() {
				NewTask(bench.data, bench.appID, bench.taskID, bench.taskType)
			}
		})
	}

}

func TestNewTask(t *testing.T) {
	tests := []struct {
		name     string
		data     interface{}
		appID    int
		taskID   string
		taskType string
	}{
		{
			name:     "New task with nil data",
			data:     nil,
			appID:    1001,
			taskID:   "task1",
			taskType: "download",
		},
		{
			name:     "New task with map data",
			data:     map[string]interface{}{"key": "value"},
			appID:    2000,
			taskID:   "task2",
			taskType: "upload",
		},
		{
			name:     "New task with slice data",
			data:     []string{"item1", "item2"},
			appID:    3000,
			taskID:   "task3",
			taskType: "process",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := NewTask(tt.data, tt.appID, tt.taskID, tt.taskType)

			if task == nil {
				t.Fatal("NewTask returned nil")
			}

			if tt.data == nil {
				if _, ok := task.Data.(map[string]interface{}); !ok {
					t.Errorf("Expected Data to be map[string]interface{}, got %T", task.Data)
				}
			} else {
				if !reflect.DeepEqual(task.Data, tt.data) {
					t.Errorf("Expected Data %v, got %v", tt.data, task.Data)
				}
			}

			if task.AppID != tt.appID {
				t.Errorf("Expected AppID %d, got %d", tt.appID, task.AppID)
			}

			if task.TaskID != tt.taskID {
				t.Errorf("Expected TaskID %s, got %s", tt.taskID, task.TaskID)
			}

			if task.TaskType != tt.taskType {
				t.Errorf("Expected TaskType %s, got %s", tt.taskType, task.TaskType)
			}

			if task.Status != "created" {
				t.Errorf("Expected Status 'created', got %s", task.Status)
			}

			if task.Progress != 0 {
				t.Errorf("Expected Progress 0, got %d", task.Progress)
			}

			if task.Message != "" {
				t.Errorf("Expected empty Message, got %s", task.Message)
			}

			if task.MessageDetailed != "" {
				t.Errorf("Expected empty MessageDetailed, got %s", task.MessageDetailed)
			}

			if result, ok := task.Result.(map[string]interface{}); !ok {
				t.Errorf("Expected Result to be map[string]interface{}, got %T", task.Data)
			} else {
				if len(result) != 0 {
					t.Errorf("Expected Result to be empty map[string]interface{}, got %T of length %d", result, len(result))
				}
			}

			if task.Error != nil {
				t.Errorf("Expected nil Error, got %v", task.Error)
			}

			if task.Ctx == nil {
				t.Error("Expected non-nil Ctx")
			}

			if task.Cancel == nil {
				t.Error("Expected non-nil Cancel function")
			}
		})
	}
}
func TestGetAssetInstance(t *testing.T) {
	originalServer := Server
	tempServer := "http://test-server.com"
	Server = &tempServer
	defer func() { Server = originalServer }()

	tests := []struct {
		name        string
		assetBaseID string
		mockResp    *http.Response
		want        Asset
		wantErr     bool
		errContains string
	}{
		{
			name:        "Successful asset retrieval",
			assetBaseID: "abc123",
			mockResp: &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(bytes.NewBufferString(`{
					"results": [{
						"id": "abc123",
						"name": "Test Asset",
						"description": "Test Description"
					}]
				}`)),
			},
			want: Asset{
				ID:          "abc123",
				Name:        "Test Asset",
				Description: "Test Description",
			},
			wantErr: false,
		},
		{
			name:        "Empty results",
			assetBaseID: "nonexistent",
			mockResp: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`{"results": []}`)),
			},
			wantErr:     true,
			errContains: "0 assets found with asset_base_id=nonexistent",
		},
		{
			name:        "Multiple results",
			assetBaseID: "duplicate",
			mockResp: &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(bytes.NewBufferString(`{
					"results": [
						{"id": "1", "name": "Asset 1"},
						{"id": "2", "name": "Asset 2"}
					]
				}`)),
			},
			want: Asset{
				ID:   "1",
				Name: "Asset 1",
			},
			wantErr: false,
		},
		{
			name:        "Invalid JSON response",
			assetBaseID: "invalid",
			mockResp: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`invalid json`)),
			},
			wantErr: true,
		},
		{
			name:        "Server error",
			assetBaseID: "error",
			mockResp: &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(bytes.NewBufferString(`{"detail": "Internal server error"}`)),
			},
			wantErr:     true,
			errContains: "error getting asset",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalClient := ClientAPI
			mockClient := &http.Client{
				Transport: &mockTransport{
					response: tt.mockResp,
				},
			}
			ClientAPI = mockClient
			defer func() { ClientAPI = originalClient }()

			got, err := GetAssetInstance(tt.assetBaseID)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAssetInstance() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("GetAssetInstance() error = %v, want error containing %v", err, tt.errContains)
				}
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetAssetInstance() = %v, want %v", got, tt.want)
			}
		})
	}
}

type mockTransport struct {
	response *http.Response
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.response == nil {
		return nil, fmt.Errorf("no response configured")
	}
	m.response.Request = req
	return m.response, nil
}

// Mock the real func SubscribeNewApp. Keep the global Tasks map manipulation.
// But ignore spawning goroutines fetching the online resources.
func mockSubscribeNewApp(data MinimalTaskData) {
	Tasks[data.AppID] = make(map[string]*Task)
	// ignore rest
}

func mockReportHandler(w http.ResponseWriter, r *http.Request) {
	reportHandlerDo(w, r, mockSubscribeNewApp)
}

func BenchmarkReportHandler(b *testing.B) {
	testCases := []struct {
		name                    string
		accessingSoftwares      []GetReportData
		accessingSoftwaresJsons [][]byte
	}{
		{
			name:                    "Single software spams",
			accessingSoftwaresJsons: [][]byte{},
			accessingSoftwares: []GetReportData{
				{
					ProjectName: "first project",
					MinimalTaskData: MinimalTaskData{
						AppID:           1111,
						BlenderVersion:  "5.0.1",
						AddonVersion:    "3.20.1",
						PlatformVersion: "macOS-26.6.2-arm64-arm-64bit-Mach-O",
					},
				},
			},
		},
		{
			name: "Two softwares spams",
			accessingSoftwares: []GetReportData{
				{
					ProjectName: "first project",
					MinimalTaskData: MinimalTaskData{
						AppID:           1111,
						BlenderVersion:  "5.0.1",
						AddonVersion:    "3.20.1",
						PlatformVersion: "macOS-26.6.2-arm64-arm-64bit-Mach-O",
					},
				},
				{
					ProjectName: "second project",
					MinimalTaskData: MinimalTaskData{
						AppID:           2222,
						BlenderVersion:  "5.0.2",
						AddonVersion:    "3.20.2",
						PlatformVersion: "macOS-26.6.2-arm64-arm-64bit-Mach-O",
					},
				},
			},
		},
		{
			name: "Four softwares spams",
			accessingSoftwares: []GetReportData{
				{
					ProjectName: "first project",
					MinimalTaskData: MinimalTaskData{
						AppID:           1111,
						BlenderVersion:  "5.0.1",
						AddonVersion:    "3.20.1",
						PlatformVersion: "macOS-26.6.2-arm64-arm-64bit-Mach-O",
					},
				},
				{
					ProjectName: "second project",
					MinimalTaskData: MinimalTaskData{
						AppID:           2222,
						BlenderVersion:  "5.0.2",
						AddonVersion:    "3.20.2",
						PlatformVersion: "macOS-26.6.2-arm64-arm-64bit-Mach-O",
					},
				},
				{
					ProjectName: "third project",
					MinimalTaskData: MinimalTaskData{
						AppID:           3333,
						BlenderVersion:  "5.0.3",
						AddonVersion:    "3.20.3",
						PlatformVersion: "macOS-26.6.2-arm64-arm-64bit-Mach-O",
					},
				},
				{
					ProjectName: "fourth project",
					MinimalTaskData: MinimalTaskData{
						AppID:           4444,
						BlenderVersion:  "5.0.4",
						AddonVersion:    "3.20.4",
						PlatformVersion: "macOS-26.6.2-arm64-arm-64bit-Mach-O",
					},
				},
			},
		},
	}

	// prepare the JSON data before the test
	for i, testCase := range testCases {
		for _, accessingSoftware := range testCase.accessingSoftwares {
			jsonData, err := json.Marshal(accessingSoftware)
			if err != nil {
				fmt.Println("cannot marshal test data")
			}
			testCases[i].accessingSoftwaresJsons = append(testCases[i].accessingSoftwaresJsons, jsonData)
		}
	}

	// discard stdout of called func during the tests
	BKLog = log.New(io.Discard, "⬡  ", log.LstdFlags|log.Lmicroseconds)
	for _, testCase := range testCases {
		b.Run(testCase.name, func(b *testing.B) {
			TasksMux.Lock()
			Tasks = make(map[int]map[string]*Task)
			TasksMux.Unlock()
			lastReportAccessMux.Lock()
			lastReportAccess = time.Time{}
			lastReportAccessMux.Unlock()

			b.RunParallel(func(pb *testing.PB) {
				i := 0
				handler := http.HandlerFunc(mockReportHandler)

				for pb.Next() {
					i++
					jsonData := testCase.accessingSoftwaresJsons[i%len(testCase.accessingSoftwares)]

					responseRecorder := httptest.NewRecorder()
					req, err := http.NewRequest("GET", "/report", bytes.NewBuffer(jsonData))
					if err != nil {
						b.Errorf("could not create testing request: %v", err)
					}
					handler.ServeHTTP(responseRecorder, req)
					if status := responseRecorder.Code; status != http.StatusOK {
						b.Errorf("handler returned wrong status code: %v", status)
					}
				}
			})
		})
	}
	BKLog = log.New(os.Stdout, "⬡  ", log.LstdFlags|log.Lmicroseconds)
}

func Test_parseThumbnailsOnAsset(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		asset          Asset
		index          int
		appID          int
		tempDir        string
		addonVersion   string
		blenderVersion *BlenderVersionStruct
		want1          *Task
		want2          *Task
		want3          *Task
		want4          *Task
	}{
		{ // https://www.blendkit.com/api/v1/assets/4f3f607d-4210-4b0a-bbaa-906b8e2a1fed/
			name: "WebP available & supported",
			asset: Asset{
				AssetBaseID: "975dcf32-d010-4a9a-b093-4d6966175590",
				DisplayName: "Triple wall hook",
				AssetType:   "model",
				//WebpGeneratedTimestamp: 1787831335,
				Files: []AssetFile{
					{
						FileType:    "blend",
						DownloadURL: "https://www.blendkit.com/api/v1/downloads/a3c2b935-a394-4b08-a867-260083418d2d/",
					},
					{
						FileType:                        "thumbnail",
						DownloadURL:                     "https://www.blendkit.com/api/v1/downloads/478d21f8-dad9-43c2-a99e-e9bbc474399f/",
						ThumbnailSmallUrl:               "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.256x256_q85_crop-,.jpg",
						ThumbnailSmallUrlWebp:           "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.256x256_q85_crop-,.jpg.webp?webp_generated=1766636291",
						ThumbnailMiddleUrl:              "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.512x512_q85_crop-,.jpg",
						ThumbnailMiddleUrlWebp:          "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.512x512_q85_crop-,.jpg.webp?webp_generated=1766636291",
						ThumbnailLargeUrlNonsquared:     "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.1024x1024_q85.jpg",
						ThumbnailLargeUrlNonsquaredWebp: "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.1024x1024_q85.jpg.webp?webp_generated=1766636291",
					},
				},
			},
			index:        1,
			appID:        1111,
			tempDir:      "/tmp/bk_client",
			addonVersion: "5.2.2",
			blenderVersion: &BlenderVersionStruct{
				Major: 5,
				Minor: 1,
				Patch: 1,
			},
			want1: &Task{
				Data: DownloadThumbnailData{
					AddonVersion:  "3.21.5",
					ThumbnailType: "small",
					ImagePath:     "/tmp/bk_client/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.256x256_q85_crop-%2C.jpg.webp",
					ImageURL:      "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.256x256_q85_crop-,.jpg.webp?webp_generated=1766636291",
					AssetBaseID:   "975dcf32-d010-4a9a-b093-4d6966175590",
					Index:         5,
				},
				AppID:    1111,
				TaskType: "thumbnail_download",
			},
			want2: &Task{
				Data: DownloadThumbnailData{
					AddonVersion:  "3.21.5",
					ThumbnailType: "full",
					ImagePath:     "/tmp/bk_client/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.512x512_q85_crop-%2C.jpg.webp",
					ImageURL:      "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.512x512_q85_crop-,.jpg.webp?webp_generated=1766636291",
					AssetBaseID:   "975dcf32-d010-4a9a-b093-4d6966175590",
					Index:         5,
				},
				AppID:    1111,
				TaskType: "thumbnail_download",
			},
			want3: nil,
			want4: nil,
		},
		{ // https://www.blendkit.com/api/v1/assets/4f3f607d-4210-4b0a-bbaa-906b8e2a1fed/
			name: "WebP not available",
			asset: Asset{
				AssetBaseID: "975dcf32-d010-4a9a-b093-4d6966175590",
				DisplayName: "Triple wall hook",
				AssetType:   "model",
				//WebpGeneratedTimestamp: 0,
				Files: []AssetFile{
					{
						FileType:    "blend",
						DownloadURL: "https://www.blendkit.com/api/v1/downloads/a3c2b935-a394-4b08-a867-260083418d2d/",
					},
					{
						FileType:                        "thumbnail",
						DownloadURL:                     "https://www.blendkit.com/api/v1/downloads/478d21f8-dad9-43c2-a99e-e9bbc474399f/",
						ThumbnailSmallUrl:               "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.256x256_q85_crop-,.jpg",
						ThumbnailSmallUrlWebp:           "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.256x256_q85_crop-,.jpg.webp?webp_generated=None",
						ThumbnailMiddleUrl:              "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.512x512_q85_crop-,.jpg",
						ThumbnailMiddleUrlWebp:          "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.512x512_q85_crop-,.jpg.webp?webp_generated=None",
						ThumbnailLargeUrlNonsquared:     "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.1024x1024_q85.jpg",
						ThumbnailLargeUrlNonsquaredWebp: "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.1024x1024_q85.jpg.webp?webp_generated=None",
					},
				},
			},
			index:        1,
			appID:        1111,
			tempDir:      "/tmp/bk_client",
			addonVersion: "5.2.2",
			blenderVersion: &BlenderVersionStruct{
				Major: 5,
				Minor: 1,
				Patch: 1,
			},
			want1: &Task{
				Data: DownloadThumbnailData{
					AddonVersion:  "3.21.5",
					ThumbnailType: "small",
					ImagePath:     "/tmp/bk_client/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.256x256_q85_crop-%2C.jpg",
					ImageURL:      "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.256x256_q85_crop-,.jpg",
					AssetBaseID:   "975dcf32-d010-4a9a-b093-4d6966175590",
					Index:         5,
				},
				AppID:    1111,
				TaskType: "thumbnail_download",
			},
			want2: &Task{
				Data: DownloadThumbnailData{
					AddonVersion:  "3.21.5",
					ThumbnailType: "full",
					ImagePath:     "/tmp/bk_client/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.512x512_q85_crop-%2C.jpg",
					ImageURL:      "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.512x512_q85_crop-,.jpg",
					AssetBaseID:   "975dcf32-d010-4a9a-b093-4d6966175590",
					Index:         5,
				},
				AppID:    1111,
				TaskType: "thumbnail_download",
			},
			want3: nil,
			want4: nil,
		},
		{ // https://www.blendkit.com/api/v1/assets/4f3f607d-4210-4b0a-bbaa-906b8e2a1fed/
			name: "WebP not supported",
			asset: Asset{
				AssetBaseID: "975dcf32-d010-4a9a-b093-4d6966175590",
				DisplayName: "Triple wall hook",
				AssetType:   "model",
				//WebpGeneratedTimestamp: 1766636291,
				Files: []AssetFile{
					{
						FileType:    "blend",
						DownloadURL: "https://www.blendkit.com/api/v1/downloads/a3c2b935-a394-4b08-a867-260083418d2d/",
					},
					{
						FileType:                        "thumbnail",
						DownloadURL:                     "https://www.blendkit.com/api/v1/downloads/478d21f8-dad9-43c2-a99e-e9bbc474399f/",
						ThumbnailSmallUrl:               "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.256x256_q85_crop-,.jpg",
						ThumbnailSmallUrlWebp:           "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.256x256_q85_crop-,.jpg.webp?webp_generated=1766636291",
						ThumbnailMiddleUrl:              "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.512x512_q85_crop-,.jpg",
						ThumbnailMiddleUrlWebp:          "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.512x512_q85_crop-,.jpg.webp?webp_generated=1766636291",
						ThumbnailLargeUrlNonsquared:     "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.1024x1024_q85.jpg",
						ThumbnailLargeUrlNonsquaredWebp: "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.1024x1024_q85.jpg.webp?webp_generated=1766636291",
					},
				},
			},
			index:        1,
			appID:        1111,
			tempDir:      "/tmp/bk_client",
			addonVersion: "5.2.2",
			blenderVersion: &BlenderVersionStruct{
				Major: 3,
				Minor: 0,
				Patch: 1,
			},
			want1: &Task{
				Data: DownloadThumbnailData{
					AddonVersion:  "3.21.5",
					ThumbnailType: "small",
					ImagePath:     "/tmp/bk_client/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.256x256_q85_crop-%2C.jpg",
					ImageURL:      "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.256x256_q85_crop-,.jpg",
					AssetBaseID:   "975dcf32-d010-4a9a-b093-4d6966175590",
					Index:         5,
				},
				AppID:    1111,
				TaskType: "thumbnail_download",
			},
			want2: &Task{
				Data: DownloadThumbnailData{
					AddonVersion:  "3.21.5",
					ThumbnailType: "full",
					ImagePath:     "/tmp/bk_client/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.512x512_q85_crop-%2C.jpg",
					ImageURL:      "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.512x512_q85_crop-,.jpg",
					AssetBaseID:   "975dcf32-d010-4a9a-b093-4d6966175590",
					Index:         5,
				},
				AppID:    1111,
				TaskType: "thumbnail_download",
			},
			want3: nil,
			want4: nil,
		},
		{ // https://www.blendkit.com/api/v1/assets/4f3f607d-4210-4b0a-bbaa-906b8e2a1fed/
			name: "Photo & wire thumbs available in webp",
			asset: Asset{
				AssetBaseID: "975dcf32-d010-4a9a-b093-4d6966175590",
				DisplayName: "Triple wall hook",
				AssetType:   "model",
				//WebpGeneratedTimestamp: 1766636291,
				Files: []AssetFile{
					{
						FileType:    "blend",
						DownloadURL: "https://www.blendkit.com/api/v1/downloads/a3c2b935-a394-4b08-a867-260083418d2d/",
					},
					{
						FileType:               "thumbnail",
						DownloadURL:            "https://www.blendkit.com/api/v1/downloads/478d21f8-dad9-43c2-a99e-e9bbc474399f/",
						ThumbnailSmallUrl:      "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.256x256_q85_crop-,.jpg",
						ThumbnailSmallUrlWebp:  "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.256x256_q85_crop-,.jpg.webp?webp_generated=1766636291",
						ThumbnailMiddleUrl:     "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.512x512_q85_crop-,.jpg",
						ThumbnailMiddleUrlWebp: "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.512x512_q85_crop-,.jpg.webp?webp_generated=1766636291",
					},
					{
						FileType:               "photo_thumbnail",
						DownloadURL:            "https://www.blendkit.com/api/v1/downloads/478d21f8-dad9-43c2-a99e-e9bbc474399f/",
						ThumbnailMiddleUrl:     "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/photo_thumbnail_39d3d601-c032-4215-b413-2b0bf3446dfe.jpg.512x512_q85_crop-,.jpg",
						ThumbnailMiddleUrlWebp: "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/photo_thumbnail_39d3d601-c032-4215-b413-2b0bf3446dfe.jpg.512x512_q85_crop-,.jpg.webp?webp_generated=1766636291",
					},
					{
						FileType:               "wire_thumbnail",
						DownloadURL:            "https://www.blendkit.com/api/v1/downloads/478d21f8-dad9-43c2-a99e-e9bbc474399f/",
						ThumbnailMiddleUrl:     "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/wire_thumbnail_39d3d601-c032-4215-b413-2b0bf3446dfe.jpg.512x512_q85_crop-,.jpg",
						ThumbnailMiddleUrlWebp: "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/wire_thumbnail_39d3d601-c032-4215-b413-2b0bf3446dfe.jpg.512x512_q85_crop-,.jpg.webp?webp_generated=1766636291",
					},
				},
			},
			index:        1,
			appID:        1111,
			tempDir:      "/tmp/bk_client",
			addonVersion: "5.2.2",
			blenderVersion: &BlenderVersionStruct{
				Major: 5,
				Minor: 1,
				Patch: 1,
			},
			want1: &Task{
				Data: DownloadThumbnailData{
					AddonVersion:  "3.21.5",
					ThumbnailType: "small",
					ImagePath:     "/tmp/bk_client/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.256x256_q85_crop-%2C.jpg.webp",
					ImageURL:      "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.256x256_q85_crop-,.jpg.webp?webp_generated=1766636291",
					AssetBaseID:   "975dcf32-d010-4a9a-b093-4d6966175590",
					Index:         5,
				},
				AppID:    1111,
				TaskType: "thumbnail_download",
			},
			want2: &Task{
				Data: DownloadThumbnailData{
					AddonVersion:  "3.21.5",
					ThumbnailType: "full",
					ImagePath:     "/tmp/bk_client/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.512x512_q85_crop-%2C.jpg.webp",
					ImageURL:      "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.512x512_q85_crop-,.jpg.webp?webp_generated=1766636291",
					AssetBaseID:   "975dcf32-d010-4a9a-b093-4d6966175590",
					Index:         5,
				},
				AppID:    1111,
				TaskType: "thumbnail_download",
			},
			want3: &Task{
				Data: DownloadThumbnailData{
					AddonVersion:  "3.21.5",
					ThumbnailType: "photo_full",
					ImagePath:     "/tmp/bk_client/photo_thumbnail_39d3d601-c032-4215-b413-2b0bf3446dfe.jpg.512x512_q85_crop-%2C.jpg.webp",
					ImageURL:      "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/photo_thumbnail_39d3d601-c032-4215-b413-2b0bf3446dfe.jpg.512x512_q85_crop-,.jpg.webp?webp_generated=1766636291",
					AssetBaseID:   "975dcf32-d010-4a9a-b093-4d6966175590",
					Index:         5,
				},
				AppID:    1111,
				TaskType: "thumbnail_download",
			},
			want4: &Task{
				Data: DownloadThumbnailData{
					AddonVersion:  "3.21.5",
					ThumbnailType: "wire_full",
					ImagePath:     "/tmp/bk_client/wire_thumbnail_39d3d601-c032-4215-b413-2b0bf3446dfe.jpg.512x512_q85_crop-%2C.jpg.webp",
					ImageURL:      "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/wire_thumbnail_39d3d601-c032-4215-b413-2b0bf3446dfe.jpg.512x512_q85_crop-,.jpg.webp?webp_generated=1766636291",
					AssetBaseID:   "975dcf32-d010-4a9a-b093-4d6966175590",
					Index:         5,
				},
				AppID:    1111,
				TaskType: "thumbnail_download",
			},
		},
		{ // https://www.blendkit.com/api/v1/assets/4f3f607d-4210-4b0a-bbaa-906b8e2a1fed/
			name: "Photo & wire thumbs available without webp",
			asset: Asset{
				AssetBaseID: "975dcf32-d010-4a9a-b093-4d6966175590",
				DisplayName: "Triple wall hook",
				AssetType:   "model",
				//WebpGeneratedTimestamp: 1766636291,
				Files: []AssetFile{
					{
						FileType:    "blend",
						DownloadURL: "https://www.blendkit.com/api/v1/downloads/a3c2b935-a394-4b08-a867-260083418d2d/",
					},
					{
						FileType:               "thumbnail",
						DownloadURL:            "https://www.blendkit.com/api/v1/downloads/478d21f8-dad9-43c2-a99e-e9bbc474399f/",
						ThumbnailSmallUrl:      "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.256x256_q85_crop-,.jpg",
						ThumbnailSmallUrlWebp:  "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.256x256_q85_crop-,.jpg.webp?webp_generated=None",
						ThumbnailMiddleUrl:     "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.512x512_q85_crop-,.jpg",
						ThumbnailMiddleUrlWebp: "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.512x512_q85_crop-,.jpg.webp?webp_generated=None",
					},
					{
						FileType:               "photo_thumbnail",
						DownloadURL:            "https://www.blendkit.com/api/v1/downloads/478d21f8-dad9-43c2-a99e-e9bbc474399f/",
						ThumbnailMiddleUrl:     "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/photo_thumbnail_39d3d601-c032-4215-b413-2b0bf3446dfe.jpg.512x512_q85_crop-,.jpg",
						ThumbnailMiddleUrlWebp: "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/photo_thumbnail_39d3d601-c032-4215-b413-2b0bf3446dfe.jpg.512x512_q85_crop-,.jpg.webp?webp_generated=None",
					},
					{
						FileType:               "wire_thumbnail",
						DownloadURL:            "https://www.blendkit.com/api/v1/downloads/478d21f8-dad9-43c2-a99e-e9bbc474399f/",
						ThumbnailMiddleUrl:     "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/wire_thumbnail_39d3d601-c032-4215-b413-2b0bf3446dfe.jpg.512x512_q85_crop-,.jpg",
						ThumbnailMiddleUrlWebp: "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/wire_thumbnail_39d3d601-c032-4215-b413-2b0bf3446dfe.jpg.512x512_q85_crop-,.jpg.webp?webp_generated=None",
					},
				},
			},
			index:        1,
			appID:        1111,
			tempDir:      "/tmp/bk_client",
			addonVersion: "5.2.2",
			blenderVersion: &BlenderVersionStruct{
				Major: 5,
				Minor: 1,
				Patch: 1,
			},
			want1: &Task{
				Data: DownloadThumbnailData{
					AddonVersion:  "3.21.5",
					ThumbnailType: "small",
					ImagePath:     "/tmp/bk_client/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.256x256_q85_crop-%2C.jpg",
					ImageURL:      "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.256x256_q85_crop-,.jpg",
					AssetBaseID:   "975dcf32-d010-4a9a-b093-4d6966175590",
					Index:         5,
				},
				AppID:    1111,
				TaskType: "thumbnail_download",
			},
			want2: &Task{
				Data: DownloadThumbnailData{
					AddonVersion:  "3.21.5",
					ThumbnailType: "full",
					ImagePath:     "/tmp/bk_client/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.512x512_q85_crop-%2C.jpg",
					ImageURL:      "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/thumbnail_e1016411-e8f6-4998-ae47-4b0801a4f210.jpg.512x512_q85_crop-,.jpg",
					AssetBaseID:   "975dcf32-d010-4a9a-b093-4d6966175590",
					Index:         5,
				},
				AppID:    1111,
				TaskType: "thumbnail_download",
			},
			want3: &Task{
				Data: DownloadThumbnailData{
					AddonVersion:  "3.21.5",
					ThumbnailType: "photo_full",
					ImagePath:     "/tmp/bk_client/photo_thumbnail_39d3d601-c032-4215-b413-2b0bf3446dfe.jpg.512x512_q85_crop-%2C.jpg",
					ImageURL:      "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/photo_thumbnail_39d3d601-c032-4215-b413-2b0bf3446dfe.jpg.512x512_q85_crop-,.jpg",
					AssetBaseID:   "975dcf32-d010-4a9a-b093-4d6966175590",
					Index:         5,
				},
				AppID:    1111,
				TaskType: "thumbnail_download",
			},
			want4: &Task{
				Data: DownloadThumbnailData{
					AddonVersion:  "3.21.5",
					ThumbnailType: "wire_full",
					ImagePath:     "/tmp/bk_client/wire_thumbnail_39d3d601-c032-4215-b413-2b0bf3446dfe.jpg.512x512_q85_crop-%2C.jpg",
					ImageURL:      "https://public.blenderkit.com/thumbnails/assets/4f3f607d42104b0abbaa906b8e2a1fed/files/wire_thumbnail_39d3d601-c032-4215-b413-2b0bf3446dfe.jpg.512x512_q85_crop-,.jpg",
					AssetBaseID:   "975dcf32-d010-4a9a-b093-4d6966175590",
					Index:         5,
				},
				AppID:    1111,
				TaskType: "thumbnail_download",
			},
		},
	}

	for _, tt := range tests {
		// CHECK SMALL THUMB
		if tt.want1 != nil {
			got, _, _, _ := parseThumbnailsOnAsset(tt.asset, tt.index, tt.appID, tt.tempDir, tt.addonVersion, tt.blenderVersion)
			want := tt.want1
			t.Run(tt.name+"-small_thumb-task_basics", func(t *testing.T) {
				if got.AppID != want.AppID {
					t.Errorf("parseThumbnailsOnAsset() task.AppID = %v, want %v", got.AppID, want.AppID)
				}
				if got.TaskType != want.TaskType {
					t.Errorf("parseThumbnailsOnAsset() task.TaskType = %v, want %v", got.TaskType, want.TaskType)
				}
			})

			t.Run(tt.name+"-small_thumb-data_correct", func(t *testing.T) {
				if got.Data.(DownloadThumbnailData).ThumbnailType != want.Data.(DownloadThumbnailData).ThumbnailType {
					t.Errorf("parseThumbnailsOnAsset() task.Data.ThumbnailType = %v, want %v", got.Data.(DownloadThumbnailData).ThumbnailType, want.Data.(DownloadThumbnailData).ThumbnailType)
				}
				if got.Data.(DownloadThumbnailData).ImagePath != want.Data.(DownloadThumbnailData).ImagePath {
					t.Errorf("parseThumbnailsOnAsset() task.Data.ImagePath = %v, want %v", got.Data.(DownloadThumbnailData).ImagePath, want.Data.(DownloadThumbnailData).ImagePath)
				}
				if got.Data.(DownloadThumbnailData).ImageURL != want.Data.(DownloadThumbnailData).ImageURL {
					t.Errorf("parseThumbnailsOnAsset() task.Data.ImageURL = %v, want %v", got.Data.(DownloadThumbnailData).ImageURL, want.Data.(DownloadThumbnailData).ImageURL)
				}
			})
		}

		// CHECK FULL THUMB
		if tt.want2 != nil {
			_, got, _, _ := parseThumbnailsOnAsset(tt.asset, tt.index, tt.appID, tt.tempDir, tt.addonVersion, tt.blenderVersion)
			want := tt.want2
			t.Run(tt.name+"-full_thumb-task_basics", func(t *testing.T) {
				if got.AppID != want.AppID {
					t.Errorf("parseThumbnailsOnAsset() task.AppID = %v, want %v", got.AppID, want.AppID)
				}
				if got.TaskType != want.TaskType {
					t.Errorf("parseThumbnailsOnAsset() task.TaskType = %v, want %v", got.TaskType, want.TaskType)
				}
			})
			t.Run(tt.name+"-full_thumb-data_correct", func(t *testing.T) {
				if got.Data.(DownloadThumbnailData).ThumbnailType != want.Data.(DownloadThumbnailData).ThumbnailType {
					t.Errorf("parseThumbnailsOnAsset() task.Data.ThumbnailType = %v, want %v", got.Data.(DownloadThumbnailData).ThumbnailType, want.Data.(DownloadThumbnailData).ThumbnailType)
				}
				if got.Data.(DownloadThumbnailData).ImagePath != want.Data.(DownloadThumbnailData).ImagePath {
					t.Errorf("parseThumbnailsOnAsset() task.Data.ImagePath = %v, want %v", got.Data.(DownloadThumbnailData).ImagePath, want.Data.(DownloadThumbnailData).ImagePath)
				}
				if got.Data.(DownloadThumbnailData).ImageURL != want.Data.(DownloadThumbnailData).ImageURL {
					t.Errorf("parseThumbnailsOnAsset() task.Data.ImageURL = %v, want %v", got.Data.(DownloadThumbnailData).ImageURL, want.Data.(DownloadThumbnailData).ImageURL)
				}
			})
		}

		// CHECK PHOTO THUMBNAIL
		if tt.want3 != nil {
			_, _, got, _ := parseThumbnailsOnAsset(tt.asset, tt.index, tt.appID, tt.tempDir, tt.addonVersion, tt.blenderVersion)
			want := tt.want3
			t.Run(tt.name+"-full_photo-task_basics", func(t *testing.T) {
				if got.AppID != want.AppID {
					t.Errorf("parseThumbnailsOnAsset() task.AppID = %v, want %v", got.AppID, want.AppID)
				}
				if got.TaskType != want.TaskType {
					t.Errorf("parseThumbnailsOnAsset() task.TaskType = %v, want %v", got.TaskType, want.TaskType)
				}
			})
			t.Run(tt.name+"-full_photo-data_correct", func(t *testing.T) {
				if got.Data.(DownloadThumbnailData).ThumbnailType != want.Data.(DownloadThumbnailData).ThumbnailType {
					t.Errorf("parseThumbnailsOnAsset() task.Data.ThumbnailType = %v, want %v", got.Data.(DownloadThumbnailData).ThumbnailType, want.Data.(DownloadThumbnailData).ThumbnailType)
				}
				if got.Data.(DownloadThumbnailData).ImagePath != want.Data.(DownloadThumbnailData).ImagePath {
					t.Errorf("parseThumbnailsOnAsset() task.Data.ImagePath = %v, want %v", got.Data.(DownloadThumbnailData).ImagePath, want.Data.(DownloadThumbnailData).ImagePath)
				}
				if got.Data.(DownloadThumbnailData).ImageURL != want.Data.(DownloadThumbnailData).ImageURL {
					t.Errorf("parseThumbnailsOnAsset() task.Data.ImageURL = %v, want %v", got.Data.(DownloadThumbnailData).ImageURL, want.Data.(DownloadThumbnailData).ImageURL)
				}
			})
		}

		// CHECK WIREFRAME THUMBNAIL
		if tt.want4 != nil {
			_, _, _, got := parseThumbnailsOnAsset(tt.asset, tt.index, tt.appID, tt.tempDir, tt.addonVersion, tt.blenderVersion)
			want := tt.want4
			t.Run(tt.name+"-full_wite-task_basics", func(t *testing.T) {
				if got.AppID != want.AppID {
					t.Errorf("parseThumbnailsOnAsset() task.AppID = %v, want %v", got.AppID, want.AppID)
				}
				if got.TaskType != want.TaskType {
					t.Errorf("parseThumbnailsOnAsset() task.TaskType = %v, want %v", got.TaskType, want.TaskType)
				}
			})
			t.Run(tt.name+"-full_wire-data_correct", func(t *testing.T) {
				if got.Data.(DownloadThumbnailData).ThumbnailType != want.Data.(DownloadThumbnailData).ThumbnailType {
					t.Errorf("parseThumbnailsOnAsset() task.Data.ThumbnailType = %v, want %v", got.Data.(DownloadThumbnailData).ThumbnailType, want.Data.(DownloadThumbnailData).ThumbnailType)
				}
				if got.Data.(DownloadThumbnailData).ImagePath != want.Data.(DownloadThumbnailData).ImagePath {
					t.Errorf("parseThumbnailsOnAsset() task.Data.ImagePath = %v, want %v", got.Data.(DownloadThumbnailData).ImagePath, want.Data.(DownloadThumbnailData).ImagePath)
				}
				if got.Data.(DownloadThumbnailData).ImageURL != want.Data.(DownloadThumbnailData).ImageURL {
					t.Errorf("parseThumbnailsOnAsset() task.Data.ImageURL = %v, want %v", got.Data.(DownloadThumbnailData).ImageURL, want.Data.(DownloadThumbnailData).ImageURL)
				}
			})
		}
	}
}

func Test_isWebpSupported(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		asset          Asset
		blenderVersion *BlenderVersionStruct
		want           bool
	}{
		{
			name: "Blender supports",
			want: true,
			blenderVersion: &BlenderVersionStruct{
				Major: 5,
				Minor: 1,
				Patch: 1,
			},
		},
		{
			name: "Blender is old",
			want: false,
			blenderVersion: &BlenderVersionStruct{
				Major: 3,
				Minor: 3,
				Patch: 1,
			},
		},
		{
			name: "Oldest Blender supporting webp",
			want: true,
			blenderVersion: &BlenderVersionStruct{
				Major: 3,
				Minor: 4,
				Patch: 0,
			},
		},
		{
			name:           "blenderVersion is nil",
			want:           false,
			blenderVersion: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isWebpSupported(tt.blenderVersion)
			if got != tt.want {
				t.Errorf("isWebpSupported() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getFullThumbnailURL(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		assetType string
		assetFile AssetFile
		useWebp   bool
		want      string
	}{
		{
			name:      "HDR webp supported & present",
			want:      "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.1024x1024_q85.jpg.webp?webp_generated=1784732123",
			useWebp:   true,
			assetType: "hdr",
			assetFile: AssetFile{
				ThumbnailSmallUrl:               "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.256x256_q85_crop-%2C.jpg",
				ThumbnailMiddleUrl:              "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.512x512_q85_crop-%2C.jpg",
				ThumbnailLargeUrlNonsquared:     "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.1024x1024_q85.jpg",
				ThumbnailSmallUrlWebp:           "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.256x256_q85_crop-%2C.jpg.webp?webp_generated=1784732123",
				ThumbnailMiddleUrlWebp:          "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.512x512_q85_crop-%2C.jpg.webp?webp_generated=1784732123",
				ThumbnailLargeUrlNonsquaredWebp: "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.1024x1024_q85.jpg.webp?webp_generated=1784732123",
			},
		},
		{
			name:      "HDR webp unsupported",
			want:      "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.1024x1024_q85.jpg",
			useWebp:   false,
			assetType: "hdr",
			assetFile: AssetFile{
				ThumbnailSmallUrl:               "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.256x256_q85_crop-%2C.jpg",
				ThumbnailMiddleUrl:              "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.512x512_q85_crop-%2C.jpg",
				ThumbnailLargeUrlNonsquared:     "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.1024x1024_q85.jpg",
				ThumbnailSmallUrlWebp:           "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.256x256_q85_crop-%2C.jpg.webp?webp_generated=1784732123",
				ThumbnailMiddleUrlWebp:          "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.512x512_q85_crop-%2C.jpg.webp?webp_generated=1784732123",
				ThumbnailLargeUrlNonsquaredWebp: "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.1024x1024_q85.jpg.webp?webp_generated=1784732123",
			},
		},
		{
			name:      "HDR webp not present",
			want:      "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.1024x1024_q85.jpg",
			useWebp:   true,
			assetType: "hdr",
			assetFile: AssetFile{
				ThumbnailSmallUrl:               "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.256x256_q85_crop-%2C.jpg",
				ThumbnailMiddleUrl:              "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.512x512_q85_crop-%2C.jpg",
				ThumbnailLargeUrlNonsquared:     "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.1024x1024_q85.jpg",
				ThumbnailSmallUrlWebp:           "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.256x256_q85_crop-%2C.jpg.webp?webp_generated=None",
				ThumbnailMiddleUrlWebp:          "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.512x512_q85_crop-%2C.jpg.webp?webp_generated=None",
				ThumbnailLargeUrlNonsquaredWebp: "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.1024x1024_q85.jpg.webp?webp_generated=None",
			},
		},
		{
			name:      "Model webp supported & present",
			want:      "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.512x512_q85_crop-%2C.jpg.webp?webp_generated=1784732123",
			useWebp:   true,
			assetType: "model",
			assetFile: AssetFile{
				ThumbnailSmallUrl:               "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.256x256_q85_crop-%2C.jpg",
				ThumbnailMiddleUrl:              "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.512x512_q85_crop-%2C.jpg",
				ThumbnailLargeUrlNonsquared:     "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.1024x1024_q85.jpg",
				ThumbnailSmallUrlWebp:           "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.256x256_q85_crop-%2C.jpg.webp?webp_generated=1784732123",
				ThumbnailMiddleUrlWebp:          "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.512x512_q85_crop-%2C.jpg.webp?webp_generated=1784732123",
				ThumbnailLargeUrlNonsquaredWebp: "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.1024x1024_q85.jpg.webp?webp_generated=1784732123",
			},
		},
		{
			name:      "Printable webp not supported",
			want:      "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.512x512_q85_crop-%2C.jpg",
			useWebp:   false,
			assetType: "printable",
			assetFile: AssetFile{
				ThumbnailSmallUrl:               "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.256x256_q85_crop-%2C.jpg",
				ThumbnailMiddleUrl:              "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.512x512_q85_crop-%2C.jpg",
				ThumbnailLargeUrlNonsquared:     "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.1024x1024_q85.jpg",
				ThumbnailSmallUrlWebp:           "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.256x256_q85_crop-%2C.jpg.webp?webp_generated=1784732123",
				ThumbnailMiddleUrlWebp:          "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.512x512_q85_crop-%2C.jpg.webp?webp_generated=1784732123",
				ThumbnailLargeUrlNonsquaredWebp: "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.1024x1024_q85.jpg.webp?webp_generated=1784732123",
			},
		},
		{
			name:      "Material webp not present",
			want:      "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.512x512_q85_crop-%2C.jpg",
			useWebp:   true,
			assetType: "material",
			assetFile: AssetFile{
				ThumbnailSmallUrl:               "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.256x256_q85_crop-%2C.jpg",
				ThumbnailMiddleUrl:              "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.512x512_q85_crop-%2C.jpg",
				ThumbnailLargeUrlNonsquared:     "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.1024x1024_q85.jpg",
				ThumbnailSmallUrlWebp:           "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.256x256_q85_crop-%2C.jpg.webp?webp_generated=None",
				ThumbnailMiddleUrlWebp:          "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.512x512_q85_crop-%2C.jpg.webp?webp_generated=None",
				ThumbnailLargeUrlNonsquaredWebp: "https://public.blenderkit.com/thumbnails/assets/7fed2ece1a9a4fdeba4d6cc2ea1f749e/files/thumbnail_4a697c7d-5a7a-4625-bb96-4282ca6890fc.jpg.1024x1024_q85.jpg.webp?webp_generated=None",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getFullThumbnailURL(tt.assetFile, tt.assetType, tt.useWebp)

			if got != tt.want {
				t.Errorf("getFullThumbnailURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_createThumbnailDownloadTask(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		assetBaseId      string
		assetDisplayName string
		index            int
		thumbnailUrl     string
		thumbnailType    string
		appId            int
		addonVersion     string
		tempDir          string
		want             *Task
	}{
		{ // https://www.blendkit.com/api/v1/assets/c2973368-9754-4b63-8e37-2c655de7fe4a/
			name:             "Small PNG",
			assetBaseId:      "839f9e10-4a5a-4831-a4cd-638911339c83",
			assetDisplayName: "Kittenrial",
			index:            1,
			thumbnailUrl:     "https://public.blenderkit.com/thumbnails/assets/c297336897544b638e372c655de7fe4a/files/thumbnail_20e8d297-9f3b-4073-b6a7-5cd8348518a5.png.256x256_q85_crop-,.png",
			thumbnailType:    "small",
			appId:            1111,
			addonVersion:     "3.21.1",
			tempDir:          "/tmp/blendkit/assets",
			want: &Task{
				Data: DownloadThumbnailData{
					AddonVersion:  "3.21.1",
					ThumbnailType: "small",
					ImagePath:     "/tmp/blendkit/assets/thumbnail_20e8d297-9f3b-4073-b6a7-5cd8348518a5.png.256x256_q85_crop-%2C.png",
					ImageURL:      "https://public.blenderkit.com/thumbnails/assets/c297336897544b638e372c655de7fe4a/files/thumbnail_20e8d297-9f3b-4073-b6a7-5cd8348518a5.png.256x256_q85_crop-,.png",
					AssetBaseID:   "839f9e10-4a5a-4831-a4cd-638911339c83",
					Index:         1,
				},
				AppID:    1111,
				TaskType: "thumbnail_download",
				Error:    nil,
			},
		},
		{
			name:             "Small webp",
			assetBaseId:      "839f9e10-4a5a-4831-a4cd-638911339c83",
			assetDisplayName: "Kittenrial",
			index:            2,
			thumbnailUrl:     "https://public.blenderkit.com/thumbnails/assets/c297336897544b638e372c655de7fe4a/files/thumbnail_20e8d297-9f3b-4073-b6a7-5cd8348518a5.png.256x256_q85_crop-,.png.webp?webp_generated=1766466225",
			thumbnailType:    "small",
			appId:            2222,
			addonVersion:     "3.21.2",
			tempDir:          "/tmp/blendkit/assets",
			want: &Task{
				Data: DownloadThumbnailData{
					AddonVersion:  "3.21.2",
					ThumbnailType: "small",
					ImagePath:     "/tmp/blendkit/assets/thumbnail_20e8d297-9f3b-4073-b6a7-5cd8348518a5.png.256x256_q85_crop-%2C.png.webp",
					ImageURL:      "https://public.blenderkit.com/thumbnails/assets/c297336897544b638e372c655de7fe4a/files/thumbnail_20e8d297-9f3b-4073-b6a7-5cd8348518a5.png.256x256_q85_crop-,.png.webp?webp_generated=1766466225",
					AssetBaseID:   "839f9e10-4a5a-4831-a4cd-638911339c83",
					Index:         2,
				},
				AppID:    2222,
				TaskType: "thumbnail_download",
				Error:    nil,
			},
		},
		{
			name:             "Full png",
			assetBaseId:      "839f9e10-4a5a-4831-a4cd-638911339c83",
			assetDisplayName: "Kittenrial",
			index:            3,
			thumbnailUrl:     "https://public.blenderkit.com/thumbnails/assets/c297336897544b638e372c655de7fe4a/files/thumbnail_20e8d297-9f3b-4073-b6a7-5cd8348518a5.png.1024x1024_q85_crop-,.png",
			thumbnailType:    "full",
			appId:            3333,
			addonVersion:     "3.21.3",
			tempDir:          "/tmp/blendkit/assets",
			want: &Task{
				Data: DownloadThumbnailData{
					AddonVersion:  "3.21.3",
					ThumbnailType: "full",
					ImagePath:     "/tmp/blendkit/assets/thumbnail_20e8d297-9f3b-4073-b6a7-5cd8348518a5.png.1024x1024_q85_crop-%2C.png",
					ImageURL:      "https://public.blenderkit.com/thumbnails/assets/c297336897544b638e372c655de7fe4a/files/thumbnail_20e8d297-9f3b-4073-b6a7-5cd8348518a5.png.1024x1024_q85_crop-,.png",
					AssetBaseID:   "839f9e10-4a5a-4831-a4cd-638911339c83",
					Index:         3,
				},
				AppID:    3333,
				TaskType: "thumbnail_download",
				Error:    nil,
			},
		},
		{
			name:             "Full webp",
			assetBaseId:      "839f9e10-4a5a-4831-a4cd-638911339c83",
			assetDisplayName: "Kittenrial",
			index:            4,
			thumbnailUrl:     "https://public.blenderkit.com/thumbnails/assets/c297336897544b638e372c655de7fe4a/files/thumbnail_20e8d297-9f3b-4073-b6a7-5cd8348518a5.png.1024x1024_q85_crop-,.png.webp?webp_generated=1766466225",
			thumbnailType:    "full",
			appId:            4444,
			addonVersion:     "3.21.4",
			tempDir:          "/tmp/blendkit/assets",
			want: &Task{
				Data: DownloadThumbnailData{
					AddonVersion:  "3.21.4",
					ThumbnailType: "full",
					ImagePath:     "/tmp/blendkit/assets/thumbnail_20e8d297-9f3b-4073-b6a7-5cd8348518a5.png.1024x1024_q85_crop-%2C.png.webp",
					ImageURL:      "https://public.blenderkit.com/thumbnails/assets/c297336897544b638e372c655de7fe4a/files/thumbnail_20e8d297-9f3b-4073-b6a7-5cd8348518a5.png.1024x1024_q85_crop-,.png.webp?webp_generated=1766466225",
					AssetBaseID:   "839f9e10-4a5a-4831-a4cd-638911339c83",
					Index:         4,
				},
				AppID:    4444,
				TaskType: "thumbnail_download",
				Error:    nil,
			},
		},
		{
			name:             "ExtractFilenameFromURL error is propagated to task error",
			assetBaseId:      "839f9e10-4a5a-4831-a4cd-638911339c83",
			assetDisplayName: "Kittenrial",
			index:            5,
			thumbnailUrl:     "", // Empty URL so we expect underlying ExtractFilenameFromURL() to fail
			thumbnailType:    "full",
			appId:            5555,
			addonVersion:     "3.21.5",
			tempDir:          "/tmp/blendkit/assets",
			want: &Task{
				Data: DownloadThumbnailData{
					AddonVersion:  "3.21.5",
					ThumbnailType: "full",
					ImagePath:     "/tmp/blendkit/assets",
					ImageURL:      "",
					AssetBaseID:   "839f9e10-4a5a-4831-a4cd-638911339c83",
					Index:         5,
				},
				AppID:    5555,
				TaskType: "thumbnail_download",
				Error:    fmt.Errorf("error extracting filename from URL: empty URL for asset Kittenrial"),
			},
		},
		{
			name:             "Unsupported thumbnailType is rejected",
			assetBaseId:      "839f9e10-4a5a-4831-a4cd-638911339c83",
			assetDisplayName: "Kittenrial",
			index:            6,
			thumbnailUrl:     "", // Empty URL so we expect underlying ExtractFilenameFromURL() to fail
			thumbnailType:    "photo_small",
			appId:            6666,
			addonVersion:     "3.21.6",
			tempDir:          "/tmp/blendkit/assets",
			want:             nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := createThumbnailDownloadTask(tt.assetBaseId, tt.assetDisplayName, tt.index, tt.thumbnailUrl, tt.thumbnailType, tt.appId, tt.addonVersion, tt.tempDir)

			if tt.want == nil {
				if got != tt.want {
					t.Errorf("createThumbnailDownloadTask()->task = %v, want %v", got, tt.want)
				}
				return
			}

			if got.AppID != tt.want.AppID {
				t.Errorf("createThumbnailDownloadTask()->task.AppID = %v, want %v", got.AppID, tt.want.AppID)
			}
			if got.TaskType != tt.want.TaskType {
				t.Errorf("createThumbnailDownloadTask()->task.TaskType = %v, want %v", got.TaskType, tt.want.TaskType)
			}
			if tt.want.Error != nil {
				if got.Error.Error() != tt.want.Error.Error() {
					t.Errorf("createThumbnailDownloadTask()->task.Error = %v, want %v", got.Error, tt.want.Error)
				}
			} else {
				if got.Error != nil {
					t.Errorf("createThumbnailDownloadTask()->task.Error = %v, want %v", got.Error, tt.want.Error)
				}
			}

			// TEST TASK.DATA
			if got.Data.(DownloadThumbnailData).AddonVersion != tt.want.Data.(DownloadThumbnailData).AddonVersion {
				t.Errorf("createThumbnailDownloadTask()->task.Data.AddonVersion = %v, want %v", got.Data.(DownloadThumbnailData).AddonVersion, tt.want.Data.(DownloadThumbnailData).AddonVersion)
			}
			if got.Data.(DownloadThumbnailData).ThumbnailType != tt.want.Data.(DownloadThumbnailData).ThumbnailType {
				t.Errorf("createThumbnailDownloadTask()->task.Data.ThumbnailType = %v, want %v", got.Data.(DownloadThumbnailData).ThumbnailType, tt.want.Data.(DownloadThumbnailData).ThumbnailType)
			}
			if got.Data.(DownloadThumbnailData).ImagePath != tt.want.Data.(DownloadThumbnailData).ImagePath {
				t.Errorf("createThumbnailDownloadTask()->task.Data.ImagePath = %v, want %v", got.Data.(DownloadThumbnailData).ImagePath, tt.want.Data.(DownloadThumbnailData).ImagePath)
			}
			if got.Data.(DownloadThumbnailData).ImageURL != tt.want.Data.(DownloadThumbnailData).ImageURL {
				t.Errorf("createThumbnailDownloadTask()->task.Data.ImageURL = %v, want %v", got.Data.(DownloadThumbnailData).ImageURL, tt.want.Data.(DownloadThumbnailData).ImageURL)
			}
			if got.Data.(DownloadThumbnailData).AssetBaseID != tt.want.Data.(DownloadThumbnailData).AssetBaseID {
				t.Errorf("createThumbnailDownloadTask()->task.Data.AssetBaseID = %v, want %v", got.Data.(DownloadThumbnailData).AssetBaseID, tt.want.Data.(DownloadThumbnailData).AssetBaseID)
			}
			if got.Data.(DownloadThumbnailData).Index != tt.want.Data.(DownloadThumbnailData).Index {
				t.Errorf("createThumbnailDownloadTask()->task.Data.Index = %v, want %v", got.Data.(DownloadThumbnailData).Index, tt.want.Data.(DownloadThumbnailData).Index)
			}
		})
	}
}
