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

// Package index is the shared Assets2Godot asset index: a SQLite store
// (pure-Go modernc.org/sqlite driver, CGO-free) that every local provider
// writes into and queries through.
package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// driver is the database/sql name registered by modernc.org/sqlite.
const driver = "sqlite"

// Store is an open index database.
type Store struct {
	db *sql.DB
}

// Open creates or opens the index at path, creating the parent directory and
// schema as needed.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("index: empty path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open(driver, path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.ensureSchema(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

// Ping reports whether the database is reachable.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

const schema = `
CREATE TABLE IF NOT EXISTS assets (
	provider TEXT NOT NULL,
	path     TEXT NOT NULL,
	name     TEXT NOT NULL,
	kind     TEXT NOT NULL DEFAULT '',
	size     INTEGER NOT NULL DEFAULT 0,
	mtime    INTEGER NOT NULL DEFAULT 0,
	metadata TEXT NOT NULL DEFAULT '{}',
	PRIMARY KEY (provider, path)
);
CREATE INDEX IF NOT EXISTS assets_name ON assets(name);
`

func (s *Store) ensureSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// Asset is one indexed asset record.
type Asset struct {
	Provider string `json:"provider"`
	Path     string `json:"path"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Size     int64  `json:"size"`
	MTime    int64  `json:"mtime"`
	Metadata string `json:"metadata"`
}

// UpsertAsset inserts or replaces the record for (provider, path).
func (s *Store) UpsertAsset(ctx context.Context, a Asset) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO assets (provider, path, name, kind, size, mtime, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (provider, path) DO UPDATE SET
			name=excluded.name, kind=excluded.kind, size=excluded.size,
			mtime=excluded.mtime, metadata=excluded.metadata`,
		a.Provider, a.Path, a.Name, a.Kind, a.Size, a.MTime, a.Metadata)
	return err
}

// Search returns assets whose name contains q, newest first. An empty
// provider searches across all providers; limit <= 0 means 100.
func (s *Store) Search(ctx context.Context, provider, q string, limit int) ([]Asset, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows *sql.Rows
	var err error
	if provider == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT provider, path, name, kind, size, mtime, metadata
			 FROM assets WHERE name LIKE ? ORDER BY mtime DESC LIMIT ?`,
			"%"+q+"%", limit)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT provider, path, name, kind, size, mtime, metadata
			 FROM assets WHERE provider = ? AND name LIKE ? ORDER BY mtime DESC LIMIT ?`,
			provider, "%"+q+"%", limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Asset{}
	for rows.Next() {
		var a Asset
		if err := rows.Scan(&a.Provider, &a.Path, &a.Name, &a.Kind, &a.Size, &a.MTime, &a.Metadata); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteProvider removes every record of a provider — used when its roots
// are re-scanned from scratch.
func (s *Store) DeleteProvider(ctx context.Context, provider string) error {
	if provider == "" {
		return fmt.Errorf("index: provider required")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM assets WHERE provider = ?`, provider)
	return err
}
