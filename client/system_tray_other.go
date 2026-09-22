//go:build !linux

package main

import _ "embed"

//go:embed icons/blendkit.ico
var trayIcon []byte
