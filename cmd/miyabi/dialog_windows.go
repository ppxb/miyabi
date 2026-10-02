//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// messageBox shows a native dialog: a window-less build has no console, and the
// log file may sit in a directory the user cannot find or write.
func messageBox(title, text string) {
	const iconError, setForeground = 0x00000010, 0x00010000

	caption, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	body, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	user32 := syscall.NewLazyDLL("user32.dll")
	_, _, _ = user32.NewProc("MessageBoxW").Call(
		0,
		uintptr(unsafe.Pointer(body)),
		uintptr(unsafe.Pointer(caption)),
		uintptr(iconError|setForeground),
	)
}
