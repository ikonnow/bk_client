package apispec

import (
	"encoding/json"
	"fmt"
	"strings"
)

// VersionPrefix derives the versioned route prefix (e.g. "v1.9") from a full
// semantic version string (e.g. "1.9.0").
//
// Args:
//
//	version: Full semantic version, e.g. "1.9.0".
//
// Returns:
//
//	The "vMAJOR.MINOR" prefix, e.g. "v1.9". Falls back to "v" + version when
//	the version cannot be parsed.
func VersionPrefix(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return "v" + version
	}
	return fmt.Sprintf("v%s.%s", parts[0], parts[1])
}

// OpenAPI builds an OpenAPI 3.1 document for the Client API and returns it as
// indented JSON.
//
// Args:
//
//	version: Full Client version, e.g. "1.9.0".
//
// Returns:
//
//	The OpenAPI document encoded as indented JSON bytes, or an error if it
//	could not be marshalled.
func OpenAPI(version string) ([]byte, error) {
	prefix := VersionPrefix(version)

	paths := map[string]any{}
	for _, r := range Routes() {
		addPath(paths, r.Path, r, prefix)
		if r.Versioned {
			addPath(paths, "/"+prefix+r.Path, r, prefix)
		}
	}

	doc := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":   "Blendkit-Client API",
			"version": version,
			"description": "Local HTTP API exposed by the Blendkit-Client (formerly daemon). " +
				"The Client runs on localhost and bridges Blendkit DCC add-ons (Blender, Godot, " +
				"and embedders such as Maya and Rhino) with the Blendkit web service.\n\n" +
				"Most endpoints are registered twice: once under the bare path (e.g. `/report`) and " +
				"once under the versioned prefix (e.g. `/" + prefix + "/report`). Both are equivalent.\n\n" +
				"This document is generated from the Go route registry in " +
				"`internal/apispec` by `cmd/apidocgen`. Do not edit it by hand.",
			"license": map[string]any{
				"name": "GPL-2.0-or-later",
			},
		},
		"servers": []any{
			map[string]any{
				"url":         "http://localhost:{port}",
				"description": "Local Blendkit-Client. Default port is 62485.",
				"variables": map[string]any{
					"port": map[string]any{"default": "62485"},
				},
			},
		},
		"tags":  openAPITags(),
		"paths": paths,
	}

	return json.MarshalIndent(doc, "", "  ")
}

func openAPITags() []any {
	tags := make([]any, 0, len(Tags))
	for _, t := range Tags {
		tags = append(tags, map[string]any{"name": t})
	}
	return tags
}

func addPath(paths map[string]any, path string, r Route, prefix string) {
	item, ok := paths[path].(map[string]any)
	if !ok {
		item = map[string]any{}
		paths[path] = item
	}
	for _, m := range r.Methods {
		method := strings.ToLower(m)
		if method == "options" {
			continue // CORS preflight, not a documented operation
		}
		item[method] = operation(r)
	}
}

func operation(r Route) map[string]any {
	op := map[string]any{
		"tags":        []any{r.Tag},
		"summary":     r.Summary,
		"description": operationDescription(r),
		"operationId": r.Handler,
		"responses": map[string]any{
			"200": map[string]any{"description": "Successful response."},
			"400": map[string]any{"description": "Bad request (invalid or unparseable body)."},
			"500": map[string]any{"description": "Internal server error."},
		},
	}

	if r.RequestType != "" {
		op["requestBody"] = map[string]any{
			"required": true,
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{
						"type":        "object",
						"description": "JSON body. See Go struct `" + r.RequestType + "` in package main (client/structs.go and related files).",
						"x-go-type":   r.RequestType,
					},
				},
			},
		}
	}

	if r.RequiresAPIKey {
		op["x-requires-api-key"] = true
	}

	return op
}

func operationDescription(r Route) string {
	desc := r.Description
	if r.RequestNote != "" {
		desc += "\n\n" + r.RequestNote
	}
	if r.RequiresAPIKey {
		desc += "\n\nRequires a logged-in Blendkit API key."
	}
	return desc
}
