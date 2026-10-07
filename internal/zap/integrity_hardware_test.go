// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHardwareCreationFailsWithoutFallback(t *testing.T) {
	previous := hardwareCreate
	calls := 0
	hardwareCreate = func(string) (string, string, error) { calls++; return "", "", fmt.Errorf("device unavailable") }
	t.Cleanup(func() { hardwareCreate = previous })
	_, _, err := newHardwareIdentity()
	if err == nil || !strings.Contains(err.Error(), "no software fallback") || calls != 1 {
		t.Fatalf("unexpected creation: %v, calls=%d", err, calls)
	}
}
func TestHardwareLegacySSHKeyRejected(t *testing.T) {
	if _, _, err := integrityPublicKey([]byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA")); err == nil {
		t.Fatal("software SSH identity accepted")
	}
	if _, err := platformHardwareSign("private-key.pem", make([]byte, 32)); err == nil {
		t.Fatal("software signing path accepted")
	}
}
func TestHardwarePublicIdentityPublication(t *testing.T) {
	integrityTestTrustDir(t)
	p := OpenProject(t.TempDir())
	original := "# Keep my comments\nschema: 5\nproject:\n  environment: generic\n  target: demo\n"
	path := filepath.Join(p.Root, "zap.yml")
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	ref, public := integrityTestKey(t)
	trust := &integrityTrust{PublicKey: public, SigningKey: ref}
	if err := persistHardwareTrust(p.Root, trust); err != nil {
		t.Fatal(err)
	}
	if err := publishHardwareIdentity(p, trust); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatal("publication rewrote user's manifest")
	}
	_, fp, err := integrityPublicKey([]byte(public))
	if err != nil {
		t.Fatal(err)
	}
	list, err := readIntegritySigners(p.Root)
	if err != nil {
		t.Fatal(err)
	}
	if signerPublic(list, fp) != public {
		t.Fatal("public identity not published")
	}
	if err := publishHardwareIdentity(p, trust); err != nil {
		t.Fatal(err)
	}
	if list, err = readIntegritySigners(p.Root); err != nil || len(list.Signers) != 1 {
		t.Fatalf("republication duplicated signer: %v", err)
	}
	if err := persistHardwareTrust(p.Root, trust); err == nil {
		t.Fatal("local trust silently replaced")
	}
	signers, err := os.ReadFile(filepath.Join(p.Root, ".zap", "signers.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(signers), ref) {
		t.Fatal("private-key reference published")
	}
}
func TestHardwareSignedIdentityChangeIsTrustFailure(t *testing.T) {
	integrityTestTrustDir(t)
	p := OpenProject(t.TempDir())
	ref, public := integrityTestKey(t)
	c := &Config{Project: ProjectConfig{Environment: "generic", Target: "demo"}, Dependencies: map[string]*DependencyConfig{}}
	c.normalize()
	if err := p.WriteConfig(c); err != nil {
		t.Fatal(err)
	}
	trust := &integrityTrust{PublicKey: public, SigningKey: ref}
	if err := publishHardwareIdentity(p, trust); err != nil {
		t.Fatal(err)
	}
	roots, err := p.integrityRoots(c)
	if err != nil {
		t.Fatal(err)
	}
	m, err := scanIntegrity(roots, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := saveIntegrityBaseline(p.Root, trust, m); err != nil {
		t.Fatal(err)
	}
	// Replace the reviewed signer list with a different key: the existing
	// baseline's signer is no longer authorised.
	if err := os.Remove(filepath.Join(p.Root, ".zap", "signers.json")); err != nil {
		t.Fatal(err)
	}
	_, replacement := integrityTestKey(t)
	if err := publishHardwareIdentity(p, &integrityTrust{PublicKey: replacement}); err != nil {
		t.Fatal(err)
	}
	err = runIntegrityAudit(p, []string{"--non-interactive"})
	if err == nil || !strings.Contains(err.Error(), "baseline signer is not listed") {
		t.Fatalf("expected trust error: %v", err)
	}
}
