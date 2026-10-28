package main

import (
	"os"
	"syscall"
)

// Windows builds use the GUI subsystem so no console window appears when the
// app is opened from the Start menu. When run from a terminal with flags,
// attach to that terminal so scripted-mode output is visible.
func init() {
	if len(os.Args) < 2 {
		return
	}
	const attachParentProcess = 0xFFFFFFFF
	r, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("AttachConsole").Call(attachParentProcess)
	if r == 0 {
		return
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout, os.Stderr = f, f
	}
}
