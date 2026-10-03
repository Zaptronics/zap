// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func integrityTestKey(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	public, err := hardwarePublic(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	ref := "test:" + public
	previous := hardwareSign
	hardwareSign = func(reference string, digest []byte) ([]byte, error) {
		if reference != ref {
			return previous(reference, digest)
		}
		return ecdsa.SignASN1(rand.Reader, key, digest)
	}
	t.Cleanup(func() { hardwareSign = previous })
	return ref, public
}
func integrityTestTrustDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	previous := integrityUserConfigDir
	integrityUserConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { integrityUserConfigDir = previous })
}
func TestIntegritySignatureRejectsTamperingAndWrongKey(t *testing.T) {
	key, public := integrityTestKey(t)
	data := []byte("{\"approved\":\"source\"}")
	sig, err := integritySign(key, data)
	if err != nil {
		t.Fatal(err)
	}
	if err := integrityVerify(public, data, sig); err != nil {
		t.Fatal(err)
	}
	if err := integrityVerify(public, []byte("{\"approved\":\"tampered\"}"), sig); err == nil {
		t.Fatal("modified manifest accepted")
	}
	_, other := integrityTestKey(t)
	if err := integrityVerify(other, data, sig); err == nil {
		t.Fatal("substituted key accepted")
	}
	bad := append([]byte{}, sig...)
	bad[len(bad)/2] ^= 1
	if err := integrityVerify(public, data, bad); err == nil {
		t.Fatal("modified signature accepted")
	}
}
func TestIntegrityBaselineRoundTripAndRollback(t *testing.T) {
	integrityTestTrustDir(t)
	key, public := integrityTestKey(t)
	root := t.TempDir()
	source := filepath.Join(root, "a.c")
	if err := os.WriteFile(source, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	dir, err := integrityDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	roots := []integrityRoot{{ID: "project", Path: root}}
	first, err := scanIntegrity(roots, filepath.Join(dir, "objects"))
	if err != nil {
		t.Fatal(err)
	}
	trust := &integrityTrust{PublicKey: public, SigningKey: key}
	if err := saveIntegrityBaseline(root, trust, first); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadIntegrityTrust(root)
	if err != nil {
		t.Fatal(err)
	}
	got, original, err := readIntegrityBaseline(root, loaded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(integrityBytes(first), integrityBytes(got)) {
		t.Fatal("manifest round-trip changed")
	}
	if err := os.WriteFile(source, []byte("after"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := scanIntegrity(roots, filepath.Join(dir, "objects"))
	if err != nil {
		t.Fatal(err)
	}
	if err := saveIntegrityBaseline(root, loaded, second); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadDir(filepath.Join(dir, "baselines"))
	if err != nil || len(saved) != 2 {
		t.Fatalf("history not retained: %v %d", err, len(saved))
	}
	if err := os.WriteFile(filepath.Join(root, ".zap", "hash.json"), original, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readIntegrityBaseline(root, loaded); err == nil {
		t.Fatal("older signed baseline replay accepted")
	}
}
func TestIntegrityRefusesSourceChangesBeforeSigning(t *testing.T) {
	integrityTestTrustDir(t)
	key, public := integrityTestKey(t)
	root := t.TempDir()
	path := filepath.Join(root, "source")
	os.WriteFile(path, []byte("reviewed"), 0600)
	m, err := scanIntegrity([]integrityRoot{{ID: "project", Path: root}}, "")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("changed"), 0600)
	err = saveIntegrityBaseline(root, &integrityTrust{PublicKey: public, SigningKey: key}, m)
	if err == nil || !strings.Contains(err.Error(), "changed during signing") {
		t.Fatalf("expected race rejection, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".zap", "hash.json")); !os.IsNotExist(err) {
		t.Fatal("unreviewed baseline saved")
	}
}
func TestIntegrityNonInteractiveNeverEnrolls(t *testing.T) {
	integrityTestTrustDir(t)
	p := OpenProject(t.TempDir())
	c := &Config{Project: ProjectConfig{Environment: "generic", Target: "demo"}, Dependencies: map[string]*DependencyConfig{}}
	c.normalize()
	if err := p.WriteConfig(c); err != nil {
		t.Fatal(err)
	}
	if err := runIntegrityAudit(p, []string{"--non-interactive"}); err == nil {
		t.Fatal("missing baseline passed")
	}
	path, err := integrityTrustPath(p.Root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("non-interactive audit enrolled trust")
	}
}
func TestIntegrityLocalDependencyAndScope(t *testing.T) {
	root := t.TempDir()
	dep := t.TempDir()
	p := OpenProject(root)
	c := &Config{Project: ProjectConfig{Environment: "generic", Target: "demo"}, Dependencies: map[string]*DependencyConfig{
		"local": {Type: "path", URI: dep},
	}, DependencyOrder: []string{"local"}}
	c.normalize()
	if err := p.WriteConfig(c); err != nil {
		t.Fatal(err)
	}
	// Exercise the actual resolver and path mapping.
	os.WriteFile(filepath.Join(dep, "source.c"), []byte("source"), 0600)
	if err := p.Sync(c); err != nil {
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
	found := false
	for _, f := range m.Files {
		if f.Root == "dependency:local" && f.Path == "source.c" {
			found = true
		}
	}
	if !found {
		b, _ := json.Marshal(m)
		t.Fatalf("local source missing: %s", b)
	}
	os.WriteFile(filepath.Join(dep, "source.c"), []byte("modified"), 0600)
	after, err := scanIntegrity(roots, "")
	if err != nil {
		t.Fatal(err)
	}
	changes := integrityChanges(m, after)
	if len(changes) != 1 || changes[0].Path != "dependency:local/source.c" {
		t.Fatalf("local change missing: %+v", changes)
	}
}

func TestIntegrityDownloadedArchiveSourceAndWarning(t *testing.T) {
	integrityTestTrustDir(t)
	p := OpenProject(t.TempDir())
	c := &Config{Project: ProjectConfig{Environment: "generic", Target: "demo"}, Dependencies: map[string]*DependencyConfig{
		"archive": {Type: "url", URI: "https://example.invalid/source.tar.gz", Hash: "SHA256=" + strings.Repeat("a", 64)},
	}, DependencyOrder: []string{"archive"}}
	c.normalize()
	if err := p.WriteConfig(c); err != nil {
		t.Fatal(err)
	}
	if err := p.Sync(c); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(p.Root, "build", "_deps", "archive-src")
	if err := os.MkdirAll(src, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "file.c"), []byte("original"), 0600); err != nil {
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
	found := false
	for _, f := range m.Files {
		if f.Root == "dependency:archive" && f.Path == "file.c" {
			found = true
		}
	}
	if !found {
		t.Fatal("downloaded source excluded with build outputs")
	}
	key, public := integrityTestKey(t)
	trust := &integrityTrust{PublicKey: public, SigningKey: key}
	c.IntegrityPublicKey = public
	if err := p.WriteConfig(c); err != nil {
		t.Fatal(err)
	}
	m, err = scanIntegrity(roots, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := saveIntegrityBaseline(p.Root, trust, m); err != nil {
		t.Fatal(err)
	}
	if err := runIntegrityAudit(p, []string{"--non-interactive"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "file.c"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runIntegrityAudit(p, []string{"--non-interactive"}); err == nil {
		t.Fatal("modified archive source passed")
	}
	stderr, err := os.CreateTemp(t.TempDir(), "warnings")
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = stderr
	p.warnIntegrity("test")
	os.Stderr = old
	stderr.Close()
	output, err := os.ReadFile(stderr.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "ZAP INTEGRITY WARNING") || !strings.Contains(string(output), "1 source file changes") {
		t.Fatalf("warning missing: %s", output)
	}
	loaded, err := loadIntegrityTrust(p.Root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Head != trust.Head {
		t.Fatal("warning changed approved baseline")
	}
}
