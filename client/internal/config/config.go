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

// Package config defines the on-disk configuration for the Blendkit-Client
// when it runs as a standalone app.
//
// Historically these options were stored in the host DCC's preferences (Blender,
// Maya). Storing them next to the Client lets it run standalone and keep its own
// settings, while DCCs can still override them via CLI flags at startup.
//
// SECURITY: the API key (and any other secret/token) is deliberately NOT part of
// this configuration and must never be written to this file. Secrets must be kept
// with the strongest storage available on the platform (OS keychain/credential
// store). See the APIKey note on Config.
//
// MULTI-VERSION: several Client versions/instances may exist and run at once. By
// default the config file lives next to the running executable, so each installed
// version keeps its own config and there is no cross-version interference. The
// location can be overridden with the BLENDKIT_CLIENT_CONFIG environment
// variable (e.g. to share one config across versions).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// EnvConfigPath is the environment variable that, when set, overrides the
// default configuration file location.
const EnvConfigPath = "BLENDKIT_CLIENT_CONFIG"

// FileName is the default configuration file name used when the config is
// stored next to the executable.
const FileName = "blendkit-client-config.json"

// Default values mirror the Client's CLI flag defaults in main.go so that a
// freshly written config reproduces the current out-of-the-box behaviour.
const (
	DefaultServer        = "https://www.blendkit.com"
	DefaultPort          = "62485"
	DefaultProxyWhich    = "SYSTEM"
	DefaultSSLContext    = ""
	DefaultProxyAddress  = ""
	DefaultTrustedCACert = ""
	DefaultAutoShutdown  = true
)

// Config holds the persisted, non-secret settings of the Client.
//
// Every field corresponds to a Client CLI flag or standalone-app behaviour. A
// DCC starting the Client may still override any of these via CLI flags; the
// config provides the defaults used when a flag is not supplied (and the values
// shown/edited by the standalone settings UI).
type Config struct {
	// Server is the Blendkit server address to connect to.
	Server string `json:"server"`
	// PreferredPort is the port the Client tries to listen on first.
	PreferredPort string `json:"preferred_port"`
	// SSLContext controls TLS verification: "" (default), "ENABLED" or "DISABLED".
	SSLContext string `json:"ssl_context"`
	// ProxyWhich selects the proxy source: "SYSTEM", "ENVIRONMENT", "CUSTOM" or "NONE".
	ProxyWhich string `json:"proxy_which"`
	// ProxyAddress is the proxy URL used when ProxyWhich is "CUSTOM".
	ProxyAddress string `json:"proxy_address"`
	// TrustedCACerts is a path to additional trusted CA certificates (PEM).
	TrustedCACerts string `json:"trusted_ca_certs"`
	// AutoShutdown controls whether the Client exits after a period of
	// inactivity (no /report polling). Standalone users may want this off.
	AutoShutdown bool `json:"auto_shutdown"`

	// APIKey is intentionally absent. Do NOT add it here. Secrets must be stored
	// in the platform's secure credential store, never in this plaintext file.
}

// Default returns a Config populated with the built-in default values.
//
// Returns:
//
//	A Config equivalent to the Client's out-of-the-box CLI defaults.
func Default() Config {
	return Config{
		Server:         DefaultServer,
		PreferredPort:  DefaultPort,
		SSLContext:     DefaultSSLContext,
		ProxyWhich:     DefaultProxyWhich,
		ProxyAddress:   DefaultProxyAddress,
		TrustedCACerts: DefaultTrustedCACert,
		AutoShutdown:   DefaultAutoShutdown,
	}
}

// DefaultPathFor returns the default config file path for a given executable
// path: the FileName located in the same directory as the executable.
//
// Args:
//
//	executablePath: Absolute or relative path to the Client executable.
//
// Returns:
//
//	The config file path next to the executable.
func DefaultPathFor(executablePath string) string {
	return filepath.Join(filepath.Dir(executablePath), FileName)
}

// Path resolves the configuration file path.
//
// It returns the value of the BLENDKIT_CLIENT_CONFIG environment variable when
// set, otherwise the FileName located next to the running executable.
//
// Returns:
//
//	The resolved config file path, or an error if the executable path cannot
//	be determined.
func Path() (string, error) {
	if p := os.Getenv(EnvConfigPath); p != "" {
		return p, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("config: cannot determine executable path: %w", err)
	}
	return DefaultPathFor(exe), nil
}

// Load reads and parses the configuration from the given path.
//
// Missing files are not an error: Load returns the default configuration and a
// false "existed" flag so the caller can decide whether to write defaults out.
// Unknown JSON keys are ignored, and any field absent from the file keeps its
// default value (Load starts from Default()).
//
// Args:
//
//	path: Path to the JSON configuration file.
//
// Returns:
//
//	The loaded (or default) Config, a bool reporting whether the file existed,
//	and an error if the file existed but could not be read or parsed.
func Load(path string) (Config, bool, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, false, nil
		}
		return cfg, false, fmt.Errorf("config: reading %q: %w", path, err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), true, fmt.Errorf("config: parsing %q: %w", path, err)
	}
	return cfg, true, nil
}

// Save atomically writes the configuration to the given path as indented JSON.
//
// The file is written to a temporary file in the same directory and then renamed
// into place, so a crash mid-write cannot corrupt an existing config. The parent
// directory is created if necessary.
//
// Args:
//
//	path: Destination path for the JSON configuration file.
//
// Returns:
//
//	An error if the config could not be marshalled or written.
func (c Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshalling: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("config: creating dir %q: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, FileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("config: creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("config: writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("config: closing temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("config: renaming temp file into place: %w", err)
	}
	return nil
}
