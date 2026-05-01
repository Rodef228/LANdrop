//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

func enableAnsiSupport() {
	handle := windows.Handle(os.Stdout.Fd())
	var mode uint32
	windows.GetConsoleMode(handle, &mode)
	windows.SetConsoleMode(handle, mode|0x0004) // ENABLE_VIRTUAL_TERMINAL_PROCESSING
}
