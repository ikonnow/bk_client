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

// Package blenderkit is the provider descriptor for the built-in BlenderKit
// source. Its actual endpoints (/assets/search, /assets/download, ...) live
// in the client's main package and stay there: moving thousands of lines of
// upstream code into this package would create merge conflicts on every
// upstream sync for zero functional gain. This package exists so the
// provider registry reports BlenderKit uniformly alongside the new
// Assets2Godot providers.
package blenderkit

import "net/http"

// Provider is the BlenderKit provider descriptor.
type Provider struct{}

// New returns the BlenderKit provider descriptor.
func New() *Provider {
	return &Provider{}
}

// Name returns the provider identifier.
func (*Provider) Name() string { return "blenderkit" }

// Kind returns "remote" — assets come from the BlenderKit REST API.
func (*Provider) Kind() string { return "remote" }

// RegisterRoutes mounts nothing: BlenderKit's routes are registered by the
// client's main package.
func (*Provider) RegisterRoutes(*http.ServeMux, string) {}
