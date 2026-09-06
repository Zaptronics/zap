//go:build windows

// SPDX-License-Identifier: Apache-2.0
//
package zap

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procGetLogicalDrives = kernel32.NewProc("GetLogicalDrives")
	procGetVolumeInfoW   = kernel32.NewProc("GetVolumeInformationW")
)

func findLabeledVolumes(labels []string) ([]string, error) {
	wanted := map[string]bool{}
	for _, l := range labels {
		if strings.TrimSpace(l) != "" {
			wanted[strings.ToUpper(strings.TrimSpace(l))] = true
		}
	}
	if len(wanted) == 0 {
		return nil, fmt.Errorf("volume-copy method has no volume_labels")
	}
	mask, _, e := procGetLogicalDrives.Call()
	if mask == 0 {
		return nil, e
	}
	var out []string
	for i := 0; i < 26; i++ {
		if mask&(1<<uintptr(i)) == 0 {
			continue
		}
		root := fmt.Sprintf("%c:\\", 'A'+i)
		rootp, _ := syscall.UTF16PtrFromString(root)
		name := make([]uint16, 261)
		r, _, _ := procGetVolumeInfoW.Call(
			uintptr(unsafe.Pointer(rootp)),
			uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)),
			0, 0, 0, 0, 0,
		)
		if r == 0 {
			continue
		}
		label := strings.ToUpper(syscall.UTF16ToString(name))
		if wanted[label] {
			out = append(out, filepath.Clean(root))
		}
	}
	return out, nil
}
