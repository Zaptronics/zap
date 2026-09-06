// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package zap

import "os"

func enableANSI(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
