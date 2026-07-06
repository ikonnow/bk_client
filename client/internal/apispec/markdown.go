package apispec

import (
	"fmt"
	"strings"
)

// Markdown renders the route registry as a human-readable Markdown document.
//
// Args:
//
//	version: Full Client version, e.g. "1.9.0".
//
// Returns:
//
//	The generated Markdown document as a string.
func Markdown(version string) string {
	prefix := VersionPrefix(version)
	var b strings.Builder

	b.WriteString("# Blendkit-Client API\n\n")
	b.WriteString("> Generated from `internal/apispec` by `cmd/apidocgen`. Do not edit by hand.\n\n")
	fmt.Fprintf(&b, "**Client version:** `%s` &nbsp;&nbsp; **Versioned prefix:** `/%s`\n\n", version, prefix)
	b.WriteString("The Client is a local HTTP server (default port **62485**) that bridges Blendkit ")
	b.WriteString("DCC add-ons (Blender, Godot, and embedders such as Maya and Rhino) with the ")
	b.WriteString("Blendkit web service.\n\n")
	b.WriteString("Most endpoints are registered twice: once under the bare path (e.g. `/report`) and ")
	fmt.Fprintf(&b, "once under the versioned prefix (e.g. `/%s/report`). Both are equivalent.\n\n", prefix)
	b.WriteString("A machine-readable [OpenAPI 3.1 spec](openapi.json) is generated alongside this file. ")
	b.WriteString("Import it into Postman/Insomnia, render it with Swagger UI/Redoc, or generate client SDKs from it.\n\n")

	// Index.
	b.WriteString("## Endpoints by group\n\n")
	routesByTag := groupByTag()
	for _, tag := range Tags {
		routes := routesByTag[tag]
		if len(routes) == 0 {
			continue
		}
		fmt.Fprintf(&b, "### %s\n\n", tag)
		b.WriteString("| Method | Path | Versioned | Auth | Summary | Request body |\n")
		b.WriteString("|---|---|:---:|:---:|---|---|\n")
		for _, r := range routes {
			versioned := ""
			if r.Versioned {
				versioned = "✓"
			}
			auth := ""
			if r.RequiresAPIKey {
				auth = "🔑"
			}
			req := "—"
			if r.RequestType != "" {
				req = "`" + r.RequestType + "`"
			}
			fmt.Fprintf(&b, "| %s | `%s` | %s | %s | %s | %s |\n",
				strings.Join(r.Methods, ", "), r.Path, versioned, auth, r.Summary, req)
		}
		b.WriteString("\n")
	}

	// Details.
	b.WriteString("## Endpoint details\n\n")
	for _, tag := range Tags {
		routes := routesByTag[tag]
		if len(routes) == 0 {
			continue
		}
		fmt.Fprintf(&b, "### %s\n\n", tag)
		for _, r := range routes {
			fmt.Fprintf(&b, "#### `%s %s`\n\n", strings.Join(r.Methods, " / "), r.Path)
			b.WriteString(r.Description + "\n\n")
			fmt.Fprintf(&b, "- **Handler:** `%s`\n", r.Handler)
			if r.Versioned {
				fmt.Fprintf(&b, "- **Versioned alias:** `/%s%s`\n", prefix, r.Path)
			}
			if r.RequestType != "" {
				fmt.Fprintf(&b, "- **Request body:** JSON `%s` (Go struct in package main)\n", r.RequestType)
			}
			if r.RequestNote != "" {
				fmt.Fprintf(&b, "- **Request notes:** %s\n", r.RequestNote)
			}
			if r.RequiresAPIKey {
				b.WriteString("- **Auth:** requires a logged-in Blendkit API key\n")
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

func groupByTag() map[string][]Route {
	out := map[string][]Route{}
	for _, r := range Routes() {
		out[r.Tag] = append(out[r.Tag], r)
	}
	return out
}
