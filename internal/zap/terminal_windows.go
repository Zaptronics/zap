// SPDX-License-Identifier: Apache-2.0
//go:build windows

package zap

import (
	"os"
	"syscall"
	"unsafe"
)

const enableVirtualTerminalProcessing = 0x0004

func enableANSI(f *os.File) bool {
	info, err := f.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return false
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")
	var mode uint32
	ok, _, _ := getConsoleMode.Call(f.Fd(), uintptr(unsafe.Pointer(&mode)))
	if ok == 0 {
		return false
	}
	ok, _, _ = setConsoleMode.Call(f.Fd(), uintptr(mode|enableVirtualTerminalProcessing))
	return ok != 0
}
