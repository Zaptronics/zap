// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bytes"
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
	if !bytes.HasPrefix(data, []byte(original)) {
		t.Fatal("publication rewrote user's manifest")
	}
	cfg, err := ParseConfig(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IntegrityPublicKey != public {
		t.Fatal("public identity not published")
	}
	again, err := ParseConfig(FormatConfig(cfg))
	if err != nil || again.IntegrityPublicKey != public {
		t.Fatal("normal config write lost signer")
	}
	if err := publishHardwareIdentity(p, trust); err != nil {
		t.Fatal(err)
	}
	_, other := integrityTestKey(t)
	if err := publishHardwareIdentity(p, &integrityTrust{PublicKey: other}); err == nil {
		t.Fatal("public key silently replaced")
	}
	if err := persistHardwareTrust(p.Root, trust); err == nil {
		t.Fatal("local trust silently replaced")
	}
	if strings.Contains(string(data), ref) {
		t.Fatal("private-key reference published")
	}
}
func TestHardwareSignedIdentityChangeIsTrustFailure(t *testing.T) {
	integrityTestTrustDir(t)
	p := OpenProject(t.TempDir())
	ref, public := integrityTestKey(t)
	c := &Config{IntegrityPublicKey: public, Project: ProjectConfig{Environment: "generic", Target: "demo"}, Dependencies: map[string]*DependencyConfig{}}
	c.normalize()
	if err := p.WriteConfig(c); err != nil {
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
	if err := saveIntegrityBaseline(p.Root, &integrityTrust{PublicKey: public, SigningKey: ref}, m); err != nil {
		t.Fatal(err)
	}
	_, replacement := integrityTestKey(t)
	c.IntegrityPublicKey = replacement
	if err := p.WriteConfig(c); err != nil {
		t.Fatal(err)
	}
	err = runIntegrityAudit(p, []string{"--non-interactive"})
	if err == nil || !strings.Contains(err.Error(), "signing identity changed") {
		t.Fatalf("expected trust error: %v", err)
	}
}
