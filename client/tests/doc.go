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

// Package tests holds black-box, end-to-end integration tests for the whole
// Blendkit-Client.
//
// Unlike the unit tests that live next to the code they cover (Go requires a
// package's *_test.go files to sit in that package's own directory, so they can
// reach unexported symbols), these tests treat the Client as a black box: they
// build the real binary, start it as a separate process, and exercise it purely
// over its public HTTP API (/report, /settings/*, /bkclientjs/status, ...).
//
// They are gated behind the "integration" build tag so the normal, fast
// `go test ./...` run does not build and spawn the binary. Run them explicitly:
//
//	go test -tags=integration ./tests/...
//
// This file (untagged) exists only so the package is valid when the tag is
// absent; the actual tests are in the tag-gated *_test.go files.
package tests
