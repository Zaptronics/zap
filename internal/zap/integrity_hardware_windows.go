//go:build windows

// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"crypto/rsa"
	"encoding/binary"
	"fmt"
	"math/big"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

var ncrypt, ncryptInitErr = windowsCryptoLibrary()

func windowsCryptoLibrary() (*syscall.LazyDLL, error) {
	var buffer [32768]uint16
	r, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetSystemDirectoryW").Call(uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	if r == 0 || r >= uintptr(len(buffer)) {
		return nil, fmt.Errorf("cannot locate Windows system crypto library")
	}
	return syscall.NewLazyDLL(syscall.UTF16ToString(buffer[:r]) + "\\ncrypt.dll"), nil
}

//go:uintptrescapes
//go:noinline
func nc(name string, args ...uintptr) error {
	if ncryptInitErr != nil {
		return ncryptInitErr
	}
	r, _, _ := ncrypt.NewProc(name).Call(args...)
	if uint32(r) != 0 {
		return fmt.Errorf("%s: Windows CNG status 0x%08x", name, uint32(r))
	}
	return nil
}
func ncString(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }
func ncProperty(h uintptr, name string) ([]byte, error) {
	var size uint32
	if err := nc("NCryptGetProperty", h, uintptr(unsafe.Pointer(ncString(name))), 0, 0, uintptr(unsafe.Pointer(&size)), 0); err != nil {
		return nil, err
	}
	if size == 0 || size > 65536 {
		return nil, fmt.Errorf("invalid CNG property size")
	}
	data := make([]byte, size)
	err := nc("NCryptGetProperty", h, uintptr(unsafe.Pointer(ncString(name))), uintptr(unsafe.Pointer(&data[0])), uintptr(size), uintptr(unsafe.Pointer(&size)), 0)
	return data, err
}
func ncDWORD(h uintptr, name string, value uint32) error {
	return nc("NCryptSetProperty", h, uintptr(unsafe.Pointer(ncString(name))), uintptr(unsafe.Pointer(&value)), 4, 0)
}
func ncProvider() (uintptr, error) {
	var p uintptr
	if err := nc("NCryptOpenStorageProvider", uintptr(unsafe.Pointer(&p)), uintptr(unsafe.Pointer(ncString("Microsoft Platform Crypto Provider"))), 0); err != nil {
		return 0, err
	}
	data, err := ncProperty(p, "Impl Type")
	if err != nil || len(data) < 4 || binary.LittleEndian.Uint32(data)&1 == 0 {
		nc("NCryptFreeObject", p)
		return 0, fmt.Errorf("TPM hardware provider unavailable")
	}
	return p, nil
}
func ncProtected(key uintptr) error {
	data, err := ncProperty(key, "Export Policy")
	if err != nil || len(data) < 4 || binary.LittleEndian.Uint32(data) != 0 {
		return fmt.Errorf("key is not non-exportable")
	}
	data, err = ncProperty(key, "UI Policy")
	if err != nil || len(data) < 8 || binary.LittleEndian.Uint32(data[4:])&2 == 0 {
		return fmt.Errorf("provider does not enforce high-protection approval")
	}
	return nil
}
func decodeWindowsRSAPublic(blob []byte) (string, error) {
	if len(blob) < 24 || binary.LittleEndian.Uint32(blob) != 0x31415352 {
		return "", fmt.Errorf("invalid TPM RSA public blob")
	}
	eLen, nLen := int(binary.LittleEndian.Uint32(blob[8:])), int(binary.LittleEndian.Uint32(blob[12:]))
	if eLen < 1 || eLen > 4 || nLen < 256 || nLen > 1024 || len(blob) != 24+eLen+nLen {
		return "", fmt.Errorf("invalid RSA public blob lengths")
	}
	e := new(big.Int).SetBytes(blob[24 : 24+eLen]).Int64()
	if e < 3 || e > 2147483647 || e%2 == 0 {
		return "", fmt.Errorf("invalid RSA exponent")
	}
	return hardwarePublic(&rsa.PublicKey{N: new(big.Int).SetBytes(blob[24+eLen:]), E: int(e)})
}
func ncPublic(key uintptr) (string, error) {
	var size uint32
	kind := ncString("RSAPUBLICBLOB")
	if err := nc("NCryptExportKey", key, 0, uintptr(unsafe.Pointer(kind)), 0, 0, 0, uintptr(unsafe.Pointer(&size)), 0); err != nil {
		return "", err
	}
	if size < 24 || size > 4096 {
		return "", fmt.Errorf("invalid RSA public blob size")
	}
	data := make([]byte, size)
	if err := nc("NCryptExportKey", key, 0, uintptr(unsafe.Pointer(kind)), 0, uintptr(unsafe.Pointer(&data[0])), uintptr(size), uintptr(unsafe.Pointer(&size)), 0); err != nil {
		return "", err
	}
	return decodeWindowsRSAPublic(data[:size])
}
func platformHardwareCreate(id string) (string, string, error) {
	provider, err := ncProvider()
	if err != nil {
		return "", "", err
	}
	defer nc("NCryptFreeObject", provider)
	var key uintptr
	if err := nc("NCryptCreatePersistedKey", provider, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(ncString("RSA"))), uintptr(unsafe.Pointer(ncString(id))), 0, 0); err != nil {
		return "", "", err
	}
	success := false
	defer func() {
		if !success {
			nc("NCryptDeleteKey", key, 0)
		} else {
			nc("NCryptFreeObject", key)
		}
	}()
	for name, value := range map[string]uint32{"Length": 2048, "Export Policy": 0, "Key Usage": 2} {
		if err := ncDWORD(key, name, value); err != nil {
			return "", "", err
		}
	}
	policy := struct {
		Version, Flags           uint32
		Title, Name, Description *uint16
	}{
		1, 3, ncString("Create Zap source signing key"), ncString(id), ncString("Approve signing of a reviewed Zap source baseline"),
	}
	if err := nc("NCryptSetProperty", key, uintptr(unsafe.Pointer(ncString("UI Policy"))), uintptr(unsafe.Pointer(&policy)), unsafe.Sizeof(policy), 0); err != nil {
		return "", "", fmt.Errorf("TPM approval policy unsupported: %w", err)
	}
	runtime.KeepAlive(policy)
	if err := nc("NCryptFinalizeKey", key, 0); err != nil {
		return "", "", err
	}
	if err := ncProtected(key); err != nil {
		return "", "", err
	}
	public, err := ncPublic(key)
	if err != nil {
		return "", "", err
	}
	success = true
	return "windows-tpm:" + id, public, nil
}
func platformHardwareSign(reference string, digest []byte) ([]byte, error) {
	if !strings.HasPrefix(reference, "windows-tpm:ZapIntegrity-") || len(digest) != 32 {
		return nil, fmt.Errorf("expected Windows TPM key reference")
	}
	provider, err := ncProvider()
	if err != nil {
		return nil, err
	}
	defer nc("NCryptFreeObject", provider)
	var key uintptr
	if err := nc("NCryptOpenKey", provider, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(ncString(strings.TrimPrefix(reference, "windows-tpm:")))), 0, 0); err != nil {
		return nil, err
	}
	defer nc("NCryptFreeObject", key)
	if err := ncProtected(key); err != nil {
		return nil, err
	}
	padding := struct{ Algorithm *uint16 }{ncString("SHA256")}
	var size uint32
	if err := nc("NCryptSignHash", key, uintptr(unsafe.Pointer(&padding)), uintptr(unsafe.Pointer(&digest[0])), 32, 0, 0, uintptr(unsafe.Pointer(&size)), 2); err != nil {
		return nil, err
	}
	if size < 256 || size > 1024 {
		return nil, fmt.Errorf("unexpected RSA signature size")
	}
	signature := make([]byte, size)
	err = nc("NCryptSignHash", key, uintptr(unsafe.Pointer(&padding)), uintptr(unsafe.Pointer(&digest[0])), 32, uintptr(unsafe.Pointer(&signature[0])), uintptr(size), uintptr(unsafe.Pointer(&size)), 2)
	runtime.KeepAlive(padding)
	return signature[:size], err
}
