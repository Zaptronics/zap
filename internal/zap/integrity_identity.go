// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type signerIdentityPreference struct {
	Mode  string `json:"mode"`
	Label string `json:"label,omitempty"`
}

func signerIdentityMode(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		value = "hashed"
	}
	if value != "hashed" && value != "public" {
		return "", fmt.Errorf("signer identity must be hashed or public")
	}
	return value, nil
}

func saveSignerIdentityPreference(root, mode string, labels ...string) error {
	label := ""
	if len(labels) > 0 {
		label = labels[0]
	}
	mode, err := signerIdentityMode(mode)
	if err != nil {
		return err
	}
	path, err := integrityTrustPath(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(signerIdentityPreference{Mode: mode, Label: label}, "", "  ")
	if err != nil {
		return err
	}
	return integrityAtomicWrite(path+".preferences.json", append(data, '\n'))
}

func loadSignerIdentityPreference(root string) (string, error) {
	path, err := integrityTrustPath(root)
	if err != nil {
		return "", err
	}
	data, err := integrityRead(path + ".preferences.json")
	if os.IsNotExist(err) {
		return "hashed", nil
	}
	if err != nil {
		return "", err
	}
	var p signerIdentityPreference
	if err := json.Unmarshal(data, &p); err != nil {
		return "", err
	}
	return signerIdentityMode(p.Mode)
}

// These hashes bind recorded labels, not an authenticated OS identity or TPM
// attestation. Distinct domains prevent conflating a username with a hostname.
func setSignerIdentity(s *integritySigner, mode, username, host string) {
	s.User, s.Host, s.UserSHA256, s.HostSHA256, s.IdentityHash = "", "", "", "", ""
	if mode == "public" {
		s.User = username
		s.Host = host
		return
	}
	s.IdentityHash = "sha256-zap-identity-v1"
	s.UserSHA256 = historyDigest([]byte("zap-signer-user-v1\x00" + username))
	s.HostSHA256 = historyDigest([]byte("zap-signer-host-v1\x00" + host))
}
