// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

var hardwareCreate = platformHardwareCreate
var hardwareSign = platformHardwareSign

func newHardwareIdentity() (string, string, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return "", "", err
	}
	ref, public, err := hardwareCreate("ZapIntegrity-" + hex.EncodeToString(id))
	if err != nil {
		return "", "", fmt.Errorf("hardware signing setup unavailable (no software fallback): %w", err)
	}
	if _, _, err := integrityPublicKey([]byte(public)); err != nil {
		return "", "", err
	}
	return ref, public, nil
}
func hardwarePublic(public any) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		return "", err
	}
	return "zap-hw-v1:" + base64.StdEncoding.EncodeToString(der), nil
}
func parseHardwarePublic(public string) (any, []byte, error) {
	if !strings.HasPrefix(public, "zap-hw-v1:") {
		return nil, nil, fmt.Errorf("legacy SSH signer is not hardware enrollment; automatic migration is refused")
	}
	der, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(public, "zap-hw-v1:"))
	if err != nil {
		return nil, nil, err
	}
	key, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, nil, err
	}
	switch k := key.(type) {
	case *rsa.PublicKey:
		if k.N.BitLen() < 2048 {
			return nil, nil, fmt.Errorf("RSA signing key is too small")
		}
	case *ecdsa.PublicKey:
		if k.Curve != elliptic.P256() {
			return nil, nil, fmt.Errorf("expected P-256 public key")
		}
	default:
		return nil, nil, fmt.Errorf("unsupported hardware public-key algorithm")
	}
	return key, der, nil
}
func integrityPublicKey(data []byte) (string, string, error) {
	public := strings.TrimSpace(string(data))
	_, der, err := parseHardwarePublic(public)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(der)
	return public, "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:]), nil
}
func hardwareDigest(data []byte) []byte {
	sum := sha256.Sum256(append([]byte("zap-source-baseline-v2\x00"), data...))
	return sum[:]
}
func integrityVerify(public string, data, signature []byte) error {
	key, _, err := parseHardwarePublic(public)
	if err != nil {
		return err
	}
	digest := hardwareDigest(data)
	switch k := key.(type) {
	case *rsa.PublicKey:
		err = rsa.VerifyPKCS1v15(k, crypto.SHA256, digest, signature)
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(k, digest, signature) {
			err = fmt.Errorf("invalid ECDSA signature")
		}
	}
	if err != nil {
		return fmt.Errorf("baseline signature verification failed: %w", err)
	}
	return nil
}
func integritySign(reference string, data []byte) ([]byte, error) {
	return hardwareSign(reference, hardwareDigest(data))
}
