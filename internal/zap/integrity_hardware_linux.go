//go:build linux

// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const tpmDevice = "device:/dev/tpmrm0"

func linuxSystemTool(tool string) (string, error) {
	for _, base := range []string{"/usr/bin", "/bin"} {
		path := filepath.Join(base, tool)
		st, err := os.Stat(path)
		if err != nil {
			continue
		}
		stat, ok := st.Sys().(*syscall.Stat_t)
		if ok && stat.Uid == 0 && st.Mode().IsRegular() && st.Mode().Perm()&0022 == 0 && st.Mode().Perm()&0111 != 0 {
			return path, nil
		}
	}
	return "", fmt.Errorf("trusted system tool %s is unavailable; install it through your distribution", tool)
}
func linuxHardwareCommand(tool string, args ...string) (*exec.Cmd, error) {
	path, err := linuxSystemTool(tool)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(path, args...)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if name == "PATH" || strings.HasPrefix(name, "LD_") || strings.HasPrefix(name, "TPM2TOOLS_") || strings.HasPrefix(name, "TSS2_") {
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	cmd.Env = append(cmd.Env, "PATH=/usr/bin:/bin")
	return cmd, nil
}
func linuxTPMReady() error {
	st, err := os.Stat("/dev/tpmrm0")
	if err != nil || st.Mode()&os.ModeDevice == 0 {
		return fmt.Errorf("TPM resource-manager device /dev/tpmrm0 unavailable")
	}
	for _, tool := range []string{"tpm2_createprimary", "tpm2_create", "tpm2_load", "tpm2_readpublic", "tpm2_sign", "tpm2_flushcontext", "systemd-ask-password"} {
		if _, err := linuxSystemTool(tool); err != nil {
			return fmt.Errorf("required hardware signing tool %s is unavailable", tool)
		}
	}
	return nil
}
func linuxTPM(tool string, auth []byte, args ...string) error {
	args = append([]string{"-T", tpmDevice}, args...)
	cmd, err := linuxHardwareCommand(tool, args...)
	if err != nil {
		return err
	}
	if auth != nil {
		cmd.Stdin = bytes.NewReader(auth)
	}
	// Do not expose authorization material through diagnostic command output.
	if _, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s failed: %w (check TPM permissions and provisioning)", tool, err)
	}
	return nil
}
func linuxAuth(message string) ([]byte, error) {
	cmd, err := linuxHardwareCommand("systemd-ask-password", "--timeout=120", message)
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	secret, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("TPM approval cancelled or unavailable: %w", err)
	}
	defer clear(secret)
	secret = bytes.TrimSuffix(secret, []byte("\n"))
	if len(secret) < 8 {
		return nil, fmt.Errorf("TPM key passphrase must contain at least 8 bytes")
	}
	sum := sha256.Sum256(secret)
	return sum[:], nil
}
func linuxPrimary(dir string) error {
	return linuxTPM("tpm2_createprimary", nil, "-C", "o", "-G", "rsa2048:aes128cfb", "-g", "sha256", "-c", filepath.Join(dir, "primary.ctx"))
}
func linuxFlush(dir string) {
	for _, name := range []string{"key.ctx", "primary.ctx"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			_ = linuxTPM("tpm2_flushcontext", nil, filepath.Join(dir, name))
		}
	}
}
func platformHardwareCreate(id string) (string, string, error) {
	if err := linuxTPMReady(); err != nil {
		return "", "", err
	}
	auth, err := linuxAuth("Create Zap TPM key passphrase (not your account password):")
	if err != nil {
		return "", "", err
	}
	defer clear(auth)
	confirm, err := linuxAuth("Confirm Zap TPM key passphrase:")
	if err != nil {
		return "", "", err
	}
	defer clear(confirm)
	if !bytes.Equal(auth, confirm) {
		return "", "", fmt.Errorf("passphrases differ")
	}
	config, err := integrityUserConfigDir()
	if err != nil {
		return "", "", err
	}
	parent := filepath.Join(config, "zap", "hardware-keys")
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", "", err
	}
	dir := filepath.Join(parent, id)
	if err := os.Mkdir(dir, 0700); err != nil {
		return "", "", err
	}
	success := false
	defer func() {
		linuxFlush(dir)
		if !success {
			os.RemoveAll(dir)
		}
	}()
	if err := linuxPrimary(dir); err != nil {
		return "", "", err
	}
	if err := linuxTPM("tpm2_create", auth, "-C", filepath.Join(dir, "primary.ctx"), "-G", "rsa2048:rsassa-sha256", "-g", "sha256",
		"-a", "fixedtpm|fixedparent|sensitivedataorigin|userwithauth|sign",
		"-p", "file:-", "-u", filepath.Join(dir, "key.pub"), "-r", filepath.Join(dir, "key.priv")); err != nil {
		return "", "", err
	}
	if err := linuxTPM("tpm2_load", nil, "-C", filepath.Join(dir, "primary.ctx"), "-u", filepath.Join(dir, "key.pub"), "-r", filepath.Join(dir, "key.priv"), "-c", filepath.Join(dir, "key.ctx")); err != nil {
		return "", "", err
	}
	if err := linuxTPM("tpm2_readpublic", nil, "-c", filepath.Join(dir, "key.ctx"), "-f", "pem", "-o", filepath.Join(dir, "public.pem")); err != nil {
		return "", "", err
	}
	data, err := integrityRead(filepath.Join(dir, "public.pem"))
	if err != nil {
		return "", "", err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return "", "", fmt.Errorf("TPM returned invalid public key")
	}
	public, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", "", err
	}
	encoded, err := hardwarePublic(public)
	if err != nil {
		return "", "", err
	}
	success = true
	return "linux-tpm:" + id, encoded, nil
}
func platformHardwareSign(reference string, digest []byte) ([]byte, error) {
	if !strings.HasPrefix(reference, "linux-tpm:ZapIntegrity-") || len(digest) != 32 {
		return nil, fmt.Errorf("expected Linux TPM key reference")
	}
	id := strings.TrimPrefix(reference, "linux-tpm:")
	if filepath.Base(id) != id {
		return nil, fmt.Errorf("unsafe TPM key reference")
	}
	if err := linuxTPMReady(); err != nil {
		return nil, err
	}
	auth, err := linuxAuth("Approve Zap baseline: enter TPM key passphrase:")
	if err != nil {
		return nil, err
	}
	defer clear(auth)
	config, err := integrityUserConfigDir()
	if err != nil {
		return nil, err
	}
	keys := filepath.Join(config, "zap", "hardware-keys", id)
	dir, err := os.MkdirTemp("", "zap-tpm-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	defer linuxFlush(dir)
	if err := linuxPrimary(dir); err != nil {
		return nil, err
	}
	if err := linuxTPM("tpm2_load", nil, "-C", filepath.Join(dir, "primary.ctx"), "-u", filepath.Join(keys, "key.pub"), "-r", filepath.Join(keys, "key.priv"), "-c", filepath.Join(dir, "key.ctx")); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "digest"), digest, 0600); err != nil {
		return nil, err
	}
	if err := linuxTPM("tpm2_sign", auth, "-c", filepath.Join(dir, "key.ctx"), "-p", "file:-", "-g", "sha256", "-s", "rsassa", "-d", "-f", "plain", "-o", filepath.Join(dir, "signature"), filepath.Join(dir, "digest")); err != nil {
		return nil, err
	}
	return integrityRead(filepath.Join(dir, "signature"))
}
