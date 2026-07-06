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

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofrs/flock"
)

// instanceLock holds the per-version single-instance file lock for the lifetime
// of the process. It is kept at package scope so the underlying OS handle is not
// closed (which would release the lock) before the Client exits.
var instanceLock *flock.Flock

// instanceLockPath returns the lock file path for this Client version. The path
// is keyed by ClientVersion so that different versions can run concurrently while
// two instances of the *same* version cannot.
func instanceLockPath() string {
	return filepath.Join(os.TempDir(), "blenderkit-client", "v"+ClientVersion, "client.lock")
}

// instancePIDPath returns the sidecar file in which the lock holder records its
// PID, so a duplicate instance can report who is already running.
func instancePIDPath() string {
	return filepath.Join(filepath.Dir(instanceLockPath()), "client.pid")
}

// runningInstanceInfo reads the holder's PID from the sidecar file and returns a
// human-readable description for logging. It best-effort only: on any error it
// returns a generic string.
func runningInstanceInfo() string {
	data, err := os.ReadFile(instancePIDPath())
	if err != nil {
		return fmt.Sprintf("v%s (PID unknown)", ClientVersion)
	}
	pid := strings.TrimSpace(string(data))
	if pid == "" {
		return fmt.Sprintf("v%s (PID unknown)", ClientVersion)
	}
	return fmt.Sprintf("v%s (PID %s)", ClientVersion, pid)
}

// ensureSingleInstance guarantees that only one Client of this exact version runs
// at a time. If another same-version instance already holds the lock, this
// process exits gracefully (code 0), leaving the original running. Different
// versions use different lock files and are unaffected.
//
// The lock is advisory and held by an open file handle, so the OS releases it
// automatically if the holding process crashes — no stale PID files to clean up.
//
// On any unexpected lock error the Client logs a warning and continues without
// the guard, so single-instance enforcement can never prevent a legitimate start.
func ensureSingleInstance() {
	lockPath := instanceLockPath()
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		BKLog.Printf("%s Single-instance: cannot create lock dir for %q: %v (continuing)", EmoWarning, lockPath, err)
		return
	}

	lock := flock.New(lockPath)
	locked, err := lock.TryLock()
	if err != nil {
		BKLog.Printf("%s Single-instance: cannot acquire lock %q: %v (continuing)", EmoWarning, lockPath, err)
		return
	}
	if !locked {
		BKLog.Printf("%s Blendkit-Client %s is already running — exiting this duplicate instance.", EmoOK, runningInstanceInfo())
		os.Exit(0)
	}

	// We hold the lock: record our PID so any duplicate can report who is running.
	if err := os.WriteFile(instancePIDPath(), []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		BKLog.Printf("%s Single-instance: cannot write PID file %q: %v (continuing)", EmoWarning, instancePIDPath(), err)
	}

	instanceLock = lock
}
