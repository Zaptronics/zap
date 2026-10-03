//go:build !windows && !linux && !darwin

package zap

import "fmt"

func platformHardwareCreate(string) (string, string, error) {
	return "", "", fmt.Errorf("hardware signing is unsupported on this operating system")
}
func platformHardwareSign(string, []byte) ([]byte, error) {
	return nil, fmt.Errorf("hardware signing is unsupported on this operating system")
}
