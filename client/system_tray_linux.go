package main

import _ "embed"

// Linux's tray backend decodes PNG images but does not support ICO files.
//
//go:embed icons/blendkit_logo.png
var trayIcon []byte
