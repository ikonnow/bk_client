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

package providers

// scan.go holds the shared directory walker every local provider uses:
// bounded, cancellation-aware, tolerant of per-entry errors, and writes
// classified files into the index. Scans only ever run on roots the user
// configured (or, for fab_vault, the launcher-directed cache dirs) — never
// whole drives.

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/blenderkit/blenderkit/client/internal/index"
)

const (
	// ScanMaxDepth bounds how far below a root the walker descends. Vault and
	// library layouts are ~3 levels deep; the cap guards against runaway
	// trees (e.g. a user pointing at a drive root by mistake).
	ScanMaxDepth = 5
	// ScanMaxEntries caps indexed files per root — a circuit breaker, not a
	// target. Hitting it stops that root's walk, other roots continue.
	ScanMaxEntries = 20000
)

var errMaxEntries = errors.New("providers: max scan entries reached")

// kindByExt classifies asset files by extension. Kind buckets let the UI and
// importers filter without re-sniffing files.
var kindByExt = map[string]string{
	".fbx": "model", ".obj": "model", ".gltf": "model", ".glb": "model",
	".blend": "model", ".dae": "model", ".stl": "model", ".ply": "model",
	".usd": "model", ".usda": "model", ".usdc": "model", ".usdz": "model",
	".uasset": "ue_asset", ".umap": "ue_asset", ".ubulk": "ue_asset",
	".uexp": "ue_asset", ".uptnl": "ue_asset",
	".png": "texture", ".jpg": "texture", ".jpeg": "texture", ".tga": "texture",
	".tif": "texture", ".tiff": "texture", ".bmp": "texture", ".webp": "texture",
	".exr": "hdr", ".hdr": "hdr",
	".mtlx": "material",
	".json": "metadata", ".xmp": "metadata",
}

// ClassifyExt returns the asset kind for a filename's extension, or "" when
// the extension is not an indexable asset type.
func ClassifyExt(name string) string {
	return kindByExt[strings.ToLower(filepath.Ext(name))]
}

// RootResult reports one scanned root.
type RootResult struct {
	Path    string `json:"path"`
	Indexed int    `json:"indexed"`
	Skipped bool   `json:"skipped"`
}

// ScanSummary reports a finished ScanRoots run.
type ScanSummary struct {
	Roots []RootResult `json:"roots"`
	Files int          `json:"files"`
}

// ScanRoots walks each root directory and upserts classified asset files
// into idx under provider. A nil idx still counts files (preview mode).
// Unreadable roots and per-entry errors are tolerated, never fatal.
func ScanRoots(ctx context.Context, idx *index.Store, provider string, roots []string) ScanSummary {
	sum := ScanSummary{Roots: []RootResult{}}
	for _, root := range roots {
		rr := RootResult{Path: root}
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			rr.Skipped = true
			sum.Roots = append(sum.Roots, rr)
			continue
		}
		count := 0
		walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // per-entry errors (permissions, races) are skipped
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil // hidden file
			}
			if d.IsDir() {
				// Never follow symlinks/junctions — they can loop the walk
				// and escape the configured root.
				if d.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
					return fs.SkipDir
				}
				if path != root && depth(root, path) >= ScanMaxDepth {
					return fs.SkipDir
				}
				return nil
			}
			kind := ClassifyExt(d.Name())
			if kind == "" {
				return nil
			}
			count++
			if count > ScanMaxEntries {
				return errMaxEntries
			}
			if idx == nil {
				return nil
			}
			fi, err := d.Info()
			if err != nil {
				return nil
			}
			idx.UpsertAsset(ctx, index.Asset{
				Provider: provider,
				Path:     path,
				Name:     strings.TrimSuffix(d.Name(), filepath.Ext(d.Name())),
				Kind:     kind,
				Size:     fi.Size(),
				MTime:    fi.ModTime().Unix(),
				Metadata: `{"ext":"` + strings.ToLower(filepath.Ext(d.Name())) + `"}`,
			})
			return nil
		})
		if walkErr != nil && !errors.Is(walkErr, errMaxEntries) && ctx.Err() == nil {
			// Real walk errors still report what was gathered.
			rr.Skipped = count == 0
		}
		rr.Indexed = count
		sum.Files += count
		sum.Roots = append(sum.Roots, rr)
	}
	return sum
}

// depth returns how many path separators sit between root and path.
func depth(root, path string) int {
	rel := strings.TrimPrefix(path, root)
	return strings.Count(rel, string(filepath.Separator))
}
