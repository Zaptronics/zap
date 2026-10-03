// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type integritySigner struct {
	Label        string    `json:"label,omitempty"`
	IdentityHash string    `json:"identity_hash,omitempty"`
	UserSHA256   string    `json:"user_sha256,omitempty"`
	HostSHA256   string    `json:"host_sha256,omitempty"`
	Fingerprint  string    `json:"fingerprint"`
	PublicKey    string    `json:"public_key"`
	User         string    `json:"user,omitempty"`
	Host         string    `json:"host,omitempty"`
	Created      time.Time `json:"created,omitempty"`
}
type integritySigners struct {
	Schema  int               `json:"schema"`
	Signers []integritySigner `json:"signers"`
}

// The checked-out signer list is the authority. Approval of its changes belongs
// to Git/human review, not a separate local trust-enrollment protocol.
func readIntegritySigners(root string) (*integritySigners, error) {
	if err := integrityStorageSafe(root); err != nil {
		return nil, err
	}
	data, err := integrityRead(filepath.Join(root, ".zap", "signers.json"))
	if err != nil {
		return nil, err
	}
	var list integritySigners
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	if list.Schema != 1 {
		return nil, fmt.Errorf("unsupported signers.json schema")
	}
	seen := map[string]bool{}
	for _, s := range list.Signers {
		_, fp, err := integrityPublicKey([]byte(s.PublicKey))
		if err != nil {
			return nil, err
		}
		if s.Fingerprint != fp || seen[fp] {
			return nil, fmt.Errorf("invalid or duplicate signer fingerprint")
		}
		seen[fp] = true
	}
	return &list, nil
}

func signerPublic(list *integritySigners, fingerprint string) string {
	for _, s := range list.Signers {
		if s.Fingerprint == fingerprint {
			return s.PublicKey
		}
	}
	return ""
}

func publishHardwareIdentity(p *Project, t *integrityTrust) error {
	if _, err := integrityDirs(p.Root); err != nil {
		return err
	}
	list, err := readIntegritySigners(p.Root)
	if os.IsNotExist(err) {
		list = &integritySigners{Schema: 1}
	} else if err != nil {
		return err
	}
	_, fp, err := integrityPublicKey([]byte(t.PublicKey))
	if err != nil {
		return err
	}
	if signerPublic(list, fp) != "" {
		return nil
	}
	pref, err := loadSignerPreferences(p.Root)
	if err != nil {
		return err
	}
	host, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("read signer hostname: %w", err)
	}
	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("read signer account: %w", err)
	}
	entry := integritySigner{Fingerprint: fp, PublicKey: t.PublicKey, Created: time.Now().UTC()}
	setSignerIdentity(&entry, pref.Mode, u.Username, host)
	entry.Label = pref.Label
	list.Signers = append(list.Signers, entry)
	sort.Slice(list.Signers, func(i, j int) bool { return list.Signers[i].Fingerprint < list.Signers[j].Fingerprint })
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return integrityAtomicWrite(filepath.Join(p.Root, ".zap", "signers.json"), append(data, '\n'))
}

func migrateIntegritySigner(p *Project) error {
	path := filepath.Join(p.Root, "zap.yml")
	data, err := integrityRead(path)
	if err != nil {
		return err
	}
	c, err := ParseConfig(string(data))
	if err != nil {
		return err
	}
	if c.IntegrityPublicKey == "" {
		return fmt.Errorf("no legacy integrity_public_key to migrate")
	}
	// Verify legacy evidence before moving its authority into the reviewed list.
	if raw, err := integrityRead(filepath.Join(p.Root, ".zap", "hash.json")); err == nil {
		var e integrityEnvelope
		if err := json.Unmarshal(raw, &e); err != nil {
			return err
		}
		if err := integrityVerify(c.IntegrityPublicKey, e.Manifest, e.Signature); err != nil {
			return fmt.Errorf("legacy baseline: %w", err)
		}
		dir, err := integrityDirs(p.Root)
		if err != nil {
			return err
		}
		if err := integrityAtomicWrite(filepath.Join(dir, "baselines", historyDigest(raw)+".json"), raw); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := publishHardwareIdentity(p, &integrityTrust{PublicKey: c.IntegrityPublicKey}); err != nil {
		return err
	}
	lines := strings.SplitAfter(string(data), "\n")
	var out strings.Builder
	for _, line := range lines {
		if strings.HasPrefix(line, "integrity_public_key:") {
			continue
		}
		out.WriteString(line)
	}
	next := []byte(out.String())
	parsed, err := ParseConfig(string(next))
	if err != nil {
		return err
	}
	if parsed.IntegrityPublicKey != "" {
		return fmt.Errorf("cannot remove legacy public-key field without rewriting the manifest")
	}
	current, err := integrityRead(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, current) {
		return fmt.Errorf("zap.yml changed during migration")
	}
	return integrityAtomicWrite(path, next)
}

// New signatures bind logical root IDs and relative exclusions rather than a
// developer's absolute checkout path. Resolve physical paths locally on each scan.
func portableIntegrity(m *integrityManifest) *integrityManifest {
	if m == nil {
		return nil
	}
	out := &integrityManifest{Schema: 2, Files: m.Files, Roots: make([]integrityRoot, len(m.Roots))}
	for i, r := range m.Roots {
		next := integrityRoot{ID: r.ID}
		for _, excluded := range r.Exclude {
			rel := excluded
			if r.Path != "" {
				if value, err := filepath.Rel(r.Path, excluded); err == nil {
					rel = value
				}
			}
			next.Exclude = append(next.Exclude, filepath.ToSlash(rel))
		}
		sort.Strings(next.Exclude)
		out.Roots[i] = next
	}
	sort.Slice(out.Roots, func(i, j int) bool { return out.Roots[i].ID < out.Roots[j].ID })
	return out
}
func integrityScope(m *integrityManifest) []byte {
	data, _ := json.Marshal(portableIntegrity(m).Roots)
	return data
}

func acquireIntegrityAudit(root string) (func(), error) {
	dir, err := integrityDirs(root)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "audit.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("audit is locked; check for a running audit before removing %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return func() { os.Remove(path) }, nil
}
func loadSignerPreferences(root string) (signerIdentityPreference, error) {
	path, err := integrityTrustPath(root)
	if err != nil {
		return signerIdentityPreference{}, err
	}
	data, err := integrityRead(path + ".preferences.json")
	if os.IsNotExist(err) {
		return signerIdentityPreference{Mode: "hashed"}, nil
	}
	if err != nil {
		return signerIdentityPreference{}, err
	}
	var pref signerIdentityPreference
	if err := json.Unmarshal(data, &pref); err != nil {
		return pref, err
	}
	pref.Mode, err = signerIdentityMode(pref.Mode)
	return pref, err
}

func proposeSignerLabels(root, mode, label string) (setupWrite, bool, error) {
	t, err := loadIntegrityTrust(root)
	if os.IsNotExist(err) {
		return setupWrite{}, false, nil
	}
	if err != nil {
		return setupWrite{}, false, err
	}
	path := filepath.Join(root, ".zap", "signers.json")
	original, err := integrityRead(path)
	if os.IsNotExist(err) {
		return setupWrite{}, false, nil
	}
	if err != nil {
		return setupWrite{}, false, err
	}
	list, err := readIntegritySigners(root)
	if err != nil {
		return setupWrite{}, false, err
	}
	_, fp, err := integrityPublicKey([]byte(t.PublicKey))
	if err != nil {
		return setupWrite{}, false, err
	}
	for i, entry := range list.Signers {
		if entry.Fingerprint != fp || entry.PublicKey != t.PublicKey {
			continue
		}
		host, err := os.Hostname()
		if err != nil {
			return setupWrite{}, false, err
		}
		u, err := user.Current()
		if err != nil {
			return setupWrite{}, false, err
		}
		updated := entry
		setSignerIdentity(&updated, mode, u.Username, host)
		updated.Label = label
		if updated == entry {
			return setupWrite{}, false, nil
		}
		list.Signers[i] = updated
		data, err := json.MarshalIndent(list, "", "  ")
		if err != nil {
			return setupWrite{}, false, err
		}
		return setupWrite{path: path, before: original, after: append(data, '\n'), exists: true}, true, nil
	}
	return setupWrite{}, false, nil
}
