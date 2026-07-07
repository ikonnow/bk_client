/*##### BEGIN GPL LICENSE BLOCK #####

  This program is free software; you can redistribute it and/or
  modify it under the terms of the GNU General Public License
  as published by the Free Software Foundation; either version 2
  of the License, or (at your option) any later version.

##### END GPL LICENSE BLOCK #####*/

// Tool discovery for connected plugins.
//
// GET /tools/list enumerates the Python recipes bundled *inside* the
// client binary (the same tools/*.py embedded for /run_blender_script),
// so a plugin can learn which tools it can run without hard-coding
// script IDs. Because the recipes AND their manifests are embedded, the
// list is guaranteed to match exactly what /tools/run can execute — the
// binary and its tools ship as one unit, so a released binary can never
// advertise a tool it doesn't carry.
//
// POST /tools/run is the canonical alias for /run_blender_script: same
// handler, same RunBlenderScriptData body (script_id + params). New
// callers should prefer /tools/run; /run_blender_script stays for
// backward compatibility.

package main

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"sort"
	"strings"
)

// ToolParam describes a single parameter accepted by a bundled tool.
// It is purely advisory: the client never validates params, it only
// forwards them to the recipe as params.json.
type ToolParam struct {
	Name        string      `json:"name"`
	Type        string      `json:"type,omitempty"`
	Required    bool        `json:"required,omitempty"`
	Default     interface{} `json:"default,omitempty"`
	Description string      `json:"description,omitempty"`
}

// ToolManifest describes one bundled recipe. The ID is always derived
// from the .py filename; the optional companion tools/<id>.json supplies
// the human-facing name, description and parameter schema.
type ToolManifest struct {
	ID          string      `json:"id"`
	Name        string      `json:"name,omitempty"`
	Description string      `json:"description,omitempty"`
	Software    string      `json:"software,omitempty"`
	Params      []ToolParam `json:"params,omitempty"`
}

// listBundledTools enumerates tools/*.py embedded in the binary and
// merges each recipe's optional tools/<id>.json manifest. Helper files
// (dunder names like __init__.py) are skipped. The result is sorted by
// ID for stable output.
func listBundledTools() ([]ToolManifest, error) {
	entries, err := fs.ReadDir(bundledTools, "tools")
	if err != nil {
		return nil, err
	}
	tools := make([]ToolManifest, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".py") || strings.HasPrefix(name, "__") {
			continue
		}
		id := strings.TrimSuffix(name, ".py")
		m := ToolManifest{ID: id}
		if data, err := bundledTools.ReadFile("tools/" + id + ".json"); err == nil {
			// Ignore a malformed manifest rather than hiding the tool;
			// the id alone is still enough for a plugin to run it.
			_ = json.Unmarshal(data, &m)
			m.ID = id // filename is authoritative — manifest can't rename the tool
		}
		tools = append(tools, m)
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].ID < tools[j].ID })
	return tools, nil
}

// listToolsHandler serves GET /tools/list.
func listToolsHandler(w http.ResponseWriter, r *http.Request) {
	tools, err := listBundledTools()
	if err != nil {
		http.Error(w, "listing tools: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"client_version": ClientVersion,
		"tools":          tools,
	}); err != nil {
		http.Error(w, "encoding tools: "+err.Error(), http.StatusInternalServerError)
	}
}
