//go:build windows

package zap

import (
	"encoding/binary"
	"testing"
)

func TestHardwareWindowsPublicBlobValidation(t *testing.T) {
	for _, blob := range [][]byte{nil, make([]byte, 24), make([]byte, 300)} {
		if _, err := decodeWindowsRSAPublic(blob); err == nil {
			t.Fatal("invalid CNG blob accepted")
		}
	}
	blob := make([]byte, 24+3+256)
	binary.LittleEndian.PutUint32(blob, 0x31415352)
	binary.LittleEndian.PutUint32(blob[8:], 3)
	binary.LittleEndian.PutUint32(blob[12:], 256)
	copy(blob[24:], []byte{1, 0, 1})
	blob[27] = 0x80
	blob[len(blob)-1] = 1
	public, err := decodeWindowsRSAPublic(blob)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := integrityPublicKey([]byte(public)); err != nil {
		t.Fatal(err)
	}
}

func TestHardwareWindowsProviderProbe(t *testing.T) {
	provider, err := ncProvider()
	if err != nil {
		t.Skipf("TPM provider unavailable: %v", err)
	}
	defer nc("NCryptFreeObject", provider)
	t.Log("Windows TPM provider reports hardware support; no key was created or used")
}
