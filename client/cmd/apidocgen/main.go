// Command apidocgen renders the Blendkit-Client API documentation from the
// route registry in internal/apispec.
//
// It writes two files into the output directory:
//   - openapi.json: an OpenAPI 3.1 specification (machine-readable).
//   - API.md:       a human-readable Markdown reference.
//
// Usage (run from the client/ directory):
//
//	go run ./cmd/apidocgen -out ./docs
//
// or simply `go generate ./...`.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/blenderkit/blenderkit/client/internal/apispec"
)

func main() {
	out := flag.String("out", "docs", "output directory for generated documentation")
	versionFile := flag.String("version-file", "VERSION", "path to the VERSION file")
	flag.Parse()

	version, err := readVersion(*versionFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "apidocgen: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "apidocgen: creating output dir: %v\n", err)
		os.Exit(1)
	}

	openapi, err := apispec.OpenAPI(version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "apidocgen: building OpenAPI: %v\n", err)
		os.Exit(1)
	}
	openapi = append(openapi, '\n')

	openapiPath := filepath.Join(*out, "openapi.json")
	if err := os.WriteFile(openapiPath, openapi, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "apidocgen: writing %s: %v\n", openapiPath, err)
		os.Exit(1)
	}

	markdownPath := filepath.Join(*out, "API.md")
	if err := os.WriteFile(markdownPath, []byte(apispec.Markdown(version)), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "apidocgen: writing %s: %v\n", markdownPath, err)
		os.Exit(1)
	}

	fmt.Printf("apidocgen: wrote %s and %s (Client v%s)\n", openapiPath, markdownPath, version)
}

func readVersion(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading version file %q: %w", path, err)
	}
	version := strings.TrimSpace(string(data))
	if version == "" {
		return "", fmt.Errorf("version file %q is empty", path)
	}
	return version, nil
}
