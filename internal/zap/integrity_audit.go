// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var integrityUserConfigDir = os.UserConfigDir

type integrityTrust struct {
	PublicKey  string `json:"public_key"`
	SigningKey string `json:"signing_key"`
	Head       string `json:"latest_baseline_sha256"`
}
type integrityEnvelope struct {
	Signer    string          `json:"signer,omitempty"`
	Manifest  json.RawMessage `json:"manifest"`
	Signature []byte          `json:"hardware_signature"`
}

func integrityRead(path string) ([]byte, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("unsafe integrity file: %s", path)
	}
	return os.ReadFile(path)
}
func integrityTrustPath(root string) (string, error) {
	config, err := integrityUserConfigDir()
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return filepath.Join(config, "zap", "integrity-trust", historyDigest([]byte(abs))+".json"), nil
}
func loadIntegrityTrust(root string) (*integrityTrust, error) {
	path, err := integrityTrustPath(root)
	if err != nil {
		return nil, err
	}
	data, err := integrityRead(path)
	if err != nil {
		return nil, err
	}
	var t integrityTrust
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	if _, _, err := integrityPublicKey([]byte(t.PublicKey)); err != nil {
		return nil, err
	}
	return &t, nil
}
func integrityDirs(root string) (string, error) {
	if _, err := historyDirectory(root, true); err != nil {
		return "", err
	}
	dir := filepath.Join(root, ".zap", "integrity")
	if err := ensureHistoryDirectory(dir); err != nil {
		return "", err
	}
	for _, name := range []string{"objects", "baselines"} {
		if err := ensureHistoryDirectory(filepath.Join(dir, name)); err != nil {
			return "", err
		}
	}
	return dir, nil
}
func integrityStorageSafe(root string) error {
	for _, path := range []string{filepath.Join(root, ".zap"), filepath.Join(root, ".zap", "integrity")} {
		st, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe integrity directory: %s", path)
		}
	}
	return nil
}
func readIntegrityBaseline(root string, _ *integrityTrust) (*integrityManifest, []byte, error) {
	if err := integrityStorageSafe(root); err != nil {
		return nil, nil, err
	}
	list, err := readIntegritySigners(root)
	if err != nil {
		return nil, nil, fmt.Errorf("read .zap/signers.json: %w", err)
	}
	data, err := integrityRead(filepath.Join(root, ".zap", "hash.json"))
	if err != nil {
		return nil, nil, err
	}
	var e integrityEnvelope
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, nil, err
	}
	public := signerPublic(list, e.Signer)
	if e.Signer == "" {
		// Older envelopes lacked a signer ID. Keep their original signature intact
		// during explicit migration and resolve it against the reviewed public keys.
		for _, s := range list.Signers {
			if integrityVerify(s.PublicKey, e.Manifest, e.Signature) == nil {
				public = s.PublicKey
				break
			}
		}
	}
	if public == "" {
		return nil, nil, fmt.Errorf("baseline signer is not listed in .zap/signers.json")
	}
	if err := integrityVerify(public, e.Manifest, e.Signature); err != nil {
		return nil, nil, err
	}
	var m integrityManifest
	if err := json.Unmarshal(e.Manifest, &m); err != nil {
		return nil, nil, err
	}
	if m.Schema != 1 && m.Schema != 2 {
		return nil, nil, fmt.Errorf("unsupported integrity schema")
	}
	return &m, data, nil
}

func integrityAtomicWrite(path string, data []byte) error {
	if st, err := os.Lstat(path); err == nil && !st.Mode().IsRegular() {
		return fmt.Errorf("unsafe write target %s", path)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".integrity-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func saveIntegrityBaseline(root string, t *integrityTrust, m *integrityManifest) error {
	list, err := readIntegritySigners(root)
	if err != nil {
		return err
	}
	_, fingerprint, err := integrityPublicKey([]byte(t.PublicKey))
	if err != nil {
		return err
	}
	if signerPublic(list, fingerprint) != t.PublicKey {
		return fmt.Errorf("local signer is not listed; use zap audit --enroll to add it")
	}
	data, err := json.Marshal(portableIntegrity(m))
	if err != nil {
		return err
	}
	stop := auditProgress("Signing baseline - complete the system authentication prompt")
	signature, err := integritySign(t.SigningKey, data)
	stop()
	if err != nil {
		return err
	}
	if err := integrityVerify(t.PublicKey, data, signature); err != nil {
		return err
	}
	encoded, err := json.Marshal(integrityEnvelope{Signer: fingerprint, Manifest: data, Signature: signature})
	if err != nil {
		return err
	}
	fresh, err := auditScan(m.Roots, "", "Checking files after signing")
	if err != nil {
		return err
	}
	if !bytes.Equal(integrityBytes(m), integrityBytes(fresh)) {
		return fmt.Errorf("files changed during signing; run zap audit again")
	}
	// Git may have replaced the shared baseline while the authentication UI was open.
	previous, err := integrityRead(filepath.Join(root, ".zap", "hash.json"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	head := ""
	if err == nil {
		head = historyDigest(previous)
	}
	if head != t.Head {
		return fmt.Errorf("shared baseline changed during audit; run zap audit again")
	}
	current, err := loadIntegrityTrust(root)
	if err == nil && (current.PublicKey != t.PublicKey || current.SigningKey != t.SigningKey) {
		return fmt.Errorf("local signing identity changed during audit")
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	dir, err := integrityDirs(root)
	if err != nil {
		return err
	}
	if len(previous) > 0 {
		if err := integrityAtomicWrite(filepath.Join(dir, "baselines", historyDigest(previous)+".json"), previous); err != nil {
			return err
		}
	}
	digest := historyDigest(encoded)
	if err := integrityAtomicWrite(filepath.Join(dir, "baselines", digest+".json"), encoded); err != nil {
		return err
	}
	if err := integrityAtomicWrite(filepath.Join(root, ".zap", "hash.json"), encoded); err != nil {
		return err
	}
	path, err := integrityTrustPath(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	t.Head = digest // Informational last local signature; shared verification uses Git's files.
	trustData, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	if err := integrityAtomicWrite(path, trustData); err != nil {
		return fmt.Errorf("baseline saved but local key record update failed: %w", err)
	}
	return nil
}

func (p *Project) warnIntegrity(phase string) {
	c, err := p.ReadConfig()
	if err == nil && c.IntegrityPublicKey != "" {
		uiIntegrityWarning(phase, "legacy signer: run zap audit --migrate-signers")
		return
	}
	if err == nil {
		var before *integrityManifest
		before, _, err = readIntegrityBaseline(p.Root, nil)
		if err == nil {
			var roots []integrityRoot
			roots, err = p.integrityRoots(c)
			if err == nil {
				var after *integrityManifest
				after, err = auditScan(roots, "", "Checking source integrity "+phase)
				if err == nil {
					if !bytes.Equal(integrityScope(before), integrityScope(after)) {
						err = fmt.Errorf("source coverage changed")
					}
					if n := len(integrityChanges(before, after)); n > 0 {
						err = fmt.Errorf("%d source file changes", n)
					}
				}
			}
		}
	}
	if err != nil {
		if os.IsNotExist(err) {
			uiIntegrityWarning(phase, "no signed source baseline or signer list")
			return
		}
		uiIntegrityWarning(phase, "integrity unverified: "+err.Error())
	}
}

func runIntegrityAudit(p *Project, args []string) error {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	noninteractive := fs.Bool("non-interactive", false, "verify without prompting or accepting changes")
	enroll := fs.Bool("enroll", false, "add this user's hardware signer to the shared list")
	migrate := fs.Bool("migrate-signers", false, "move the legacy zap.yml public key into .zap/signers.json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected audit arguments")
	}
	interactive := IsInteractive() && !*noninteractive
	if (*enroll || *migrate) && !interactive {
		return fmt.Errorf("enrollment and migration require interactive approval")
	}
	release, err := acquireIntegrityAudit(p.Root)
	if err != nil {
		return err
	}
	defer release()
	reader := bufio.NewReader(os.Stdin)
	c, err := p.ReadConfig()
	if err != nil {
		return err
	}
	if c.IntegrityPublicKey != "" {
		if !*migrate {
			return fmt.Errorf("legacy integrity_public_key found; run zap audit --migrate-signers")
		}
		uiWarning("Move the existing public identity into .zap/signers.json; Git/human review governs this list")
		answer, err := auditPrompt(reader, "Migrate the public key and remove integrity_public_key from zap.yml? (yes/no)")
		if err != nil {
			return err
		}
		if !auditYes(answer) {
			return fmt.Errorf("signer migration declined")
		}
		if err := migrateIntegritySigner(p); err != nil {
			return err
		}
		uiSuccess("Public identity migrated; existing private key and previous baseline retained")
		c, err = p.ReadConfig()
		if err != nil {
			return err
		}
	} else if *migrate {
		return fmt.Errorf("no legacy integrity_public_key to migrate")
	}
	stop := auditProgress("Scanning external source references")
	sources, err := scanExternalSources(p.Root)
	stop()
	if err != nil {
		return err
	}
	uiSection("External sources")
	if len(sources) > 0 {
		uiDetail("References", fmt.Sprintf("%d found; details in audit-report.json", len(sources)))
	} else {
		uiSuccess("No literal external dependency/source URLs found")
	}
	roots, err := p.integrityRoots(c)
	if err != nil {
		return err
	}
	uiSection("Source integrity")
	uiHint("Scope: exact file bytes, relative paths and executable bits, including project/.zap/signers.json. Other .zap contents, .git and the project build directory are excluded. Symlinks are refused.")
	after, err := auditScan(roots, "", "Hashing project and dependency files")
	if err != nil {
		return err
	}
	// Check the registry before attempting to verify an older/copied baseline.
	// Enrollment can create a missing list, but must not imply that unverifiable
	// old contents were authenticated by the new identity.
	list, listErr := readIntegritySigners(p.Root)
	if listErr != nil && !os.IsNotExist(listErr) {
		return listErr
	}
	var before *integrityManifest
	var baselineData []byte
	baselinePath := filepath.Join(p.Root, ".zap", "hash.json")
	if _, err := os.Lstat(baselinePath); err == nil {
		if os.IsNotExist(listErr) {
			if !*enroll {
				return fmt.Errorf("signers.json is missing, so the existing baseline cannot be verified; restore .zap/signers.json from Git, or run zap audit --enroll to start a new reviewed baseline")
			}
			uiWarning("Signer list missing: the existing baseline cannot be verified")
			uiHint("Restore .zap/signers.json from Git to retain the original signer authority. Fresh setup will create a new list and preserve the old baseline as unverified evidence.")
			answer, err := auditPrompt(reader, "Start fresh enrollment without trusting the existing baseline? (yes/no)")
			if err != nil {
				return err
			}
			if !auditYes(answer) {
				return fmt.Errorf("fresh enrollment declined; restore .zap/signers.json and retry")
			}
			baselineData, err = integrityRead(baselinePath)
			if err != nil {
				return err
			}
			dir, err := integrityDirs(p.Root)
			if err != nil {
				return err
			}
			archive := filepath.Join(dir, "baselines", "unverified-"+historyDigest(baselineData)+".json")
			if err := integrityAtomicWrite(archive, baselineData); err != nil {
				return err
			}
			uiFileLink("Old baseline", archive)
		} else {
			before, baselineData, err = readIntegrityBaseline(p.Root, nil)
			if err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if os.IsNotExist(listErr) {
		uiHint("No signer list yet. Enrollment will create .zap/signers.json after your hardware identity is available.")
	}
	changes := integrityChanges(before, after)
	sameScope := before != nil && bytes.Equal(integrityScope(before), integrityScope(after))
	if before != nil && !sameScope {
		uiWarning("Source coverage changed; review current scope and the report")
	}
	reportPath, err := auditReport(p.Root, before, after, changes, sources)
	if err != nil {
		return err
	}
	auditSummary(after, changes, before == nil, p.displayName(c))
	uiFileLink("Full report", reportPath)
	if before != nil && sameScope && len(changes) == 0 && !*enroll {
		uiSuccess(fmt.Sprintf("Signed source baseline matches (%d files); signer is listed in .zap/signers.json", len(after.Files)))
		return nil
	}
	if !interactive {
		return fmt.Errorf("source baseline missing or changed; non-interactive audit never accepts changes")
	}
	if before != nil && len(changes) > 0 {
		answer, err := auditPrompt(reader, "Show coloured changes with context? (yes/no)")
		if err != nil {
			return err
		}
		if auditYes(answer) {
			if err := showIntegrityDiffPages(reader, p.Root, after.Roots, changes); err != nil {
				return err
			}
		}
	}
	t, err := loadIntegrityTrust(p.Root)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if t == nil {
		uiSection("Your hardware signing identity")
		uiHint("Your private key stays protected by your hardware. The signer entry records account/host hashes by default, or readable labels if selected during zap init.")
		uiHint("Other developers can verify your baseline without your private key. Each developer signs approved changes with their own key.")
		uiWarning("Every listed key is authorised. Review signer-list changes through Git; Zap adds no separate trust-approval workflow.")
		answer, err := auditPrompt(reader, "Create a non-exportable hardware signing key? (yes/no)")
		if err != nil {
			return err
		}
		if !auditYes(answer) {
			return fmt.Errorf("hardware key creation declined")
		}
		stop := auditProgress("Creating hardware key - complete the system authentication prompt")
		ref, public, err := newHardwareIdentity()
		stop()
		if err != nil {
			return err
		}
		t = &integrityTrust{PublicKey: public, SigningKey: ref}
		if err := persistHardwareTrust(p.Root, t); err != nil {
			return err
		}
		*enroll = true
	}
	_, fingerprint, err := integrityPublicKey([]byte(t.PublicKey))
	if err != nil {
		return err
	}
	listed := list != nil && signerPublic(list, fingerprint) == t.PublicKey
	if !listed {
		if !*enroll && t.Head != "" {
			return fmt.Errorf("local signer is not listed; use zap audit --enroll to add it through your Git review process")
		}
		// Persisted pending enrollments reuse their key, avoiding another hardware key.
		fresh, err := auditScan(roots, "", "Checking files after key enrollment")
		if err != nil {
			return err
		}
		if !bytes.Equal(integrityBytes(after), integrityBytes(fresh)) {
			return fmt.Errorf("source changed during enrollment; run zap audit again")
		}
		if err := publishHardwareIdentity(p, t); err != nil {
			return err
		}
		refreshed, err := auditScan(roots, "", "Hashing the updated signer list")
		if err != nil {
			return err
		}
		for _, ch := range integrityChanges(after, refreshed) {
			if ch.Path != "project/.zap/signers.json" {
				return fmt.Errorf("source changed while publishing signer; run zap audit again")
			}
		}
		after = refreshed
		uiSuccess("Public signer added to .zap/signers.json; commit it through your normal review process")
		if _, err := auditReport(p.Root, before, after, integrityChanges(before, after), sources); err != nil {
			return err
		}
	} else if *enroll {
		uiSuccess("Your existing hardware signer is already listed; no new key created")
	}
	uiDetail("Signing fingerprint", fingerprint)
	answer, err := auditPrompt(reader, "Accept this source snapshot and sign a new baseline? (yes/no)")
	if err != nil {
		return err
	}
	if !auditYes(answer) {
		return fmt.Errorf("source changes not accepted")
	}
	dir, err := integrityDirs(p.Root)
	if err != nil {
		return err
	}
	captured, err := auditScan(roots, filepath.Join(dir, "objects"), "Saving source snapshots and checking reviewed files")
	if err != nil {
		return err
	}
	if !bytes.Equal(integrityBytes(after), integrityBytes(captured)) {
		return fmt.Errorf("files changed during review; run zap audit again")
	}
	t.Head = ""
	if len(baselineData) > 0 {
		t.Head = historyDigest(baselineData)
	}
	if err := saveIntegrityBaseline(p.Root, t, captured); err != nil {
		return err
	}
	uiSuccess("Source baseline signed and saved; previous baselines retained")
	uiHint("Share .zap/signers.json and .zap/hash.json through Git. Local history and source snapshots remain private.")
	return nil
}

func auditYes(answer string) bool {
	return strings.EqualFold(answer, "yes") || strings.EqualFold(answer, "y")
}

func showIntegrityDiffPages(reader *bufio.Reader, root string, roots []integrityRoot, changes []integrityChange) error {
	const pageSize = 5
	total := len(changes)
	for start := 0; start < total; start += pageSize {
		end := start + pageSize
		if end > total {
			end = total
		}
		if total > pageSize {
			uiSection(fmt.Sprintf("Diff page %d of %d", start/pageSize+1, (total+pageSize-1)/pageSize))
			uiDetail("Files", fmt.Sprintf("%d-%d of %d", start+1, end, total))
		}
		if err := showIntegrityDiff(root, roots, changes[start:end]); err != nil {
			return err
		}
		remaining := total - end
		if remaining == 0 {
			if total > pageSize {
				uiHint("All file diffs shown; 0 remaining. Signing approval still defaults to No.")
			}
			return nil
		}
		uiHint(fmt.Sprintf("%d file diffs remaining.", remaining))
		for {
			answer, err := prompt(reader, "Show next page? (yes/no)", "yes")
			if err != nil {
				return err
			}
			if auditYes(answer) {
				break
			}
			if strings.EqualFold(answer, "no") || strings.EqualFold(answer, "n") {
				uiWarning(fmt.Sprintf("%d file diffs not displayed. Any later approval applies to the whole snapshot, including these files.", remaining))
				return nil
			}
			uiWarning("Enter yes or no; press Enter for the next page.")
		}
	}
	return nil
}

func showIntegrityDiff(root string, roots []integrityRoot, changes []integrityChange) error {
	for _, ch := range changes {
		var sides [2][]byte
		missingBefore := false
		for i, file := range []*integrityFile{ch.Before, ch.After} {
			if file == nil {
				continue
			}
			var path string
			if i == 0 {
				if !regexpSHA256Hex.MatchString(file.SHA256) {
					return errors.New("invalid snapshot object digest")
				}
				path = filepath.Join(root, ".zap", "integrity", "objects", file.SHA256)
			} else {
				for _, r := range roots {
					if r.ID == file.Root {
						path = filepath.Join(r.Path, filepath.FromSlash(file.Path))
						if !integrityWithin(r.Path, path) {
							return errors.New("unsafe source path")
						}
						break
					}
				}
				if path == "" {
					return errors.New("source root missing")
				}
			}
			data, err := integrityRead(path)
			if i == 0 && os.IsNotExist(err) {
				missingBefore = true
				continue
			}
			if err != nil {
				return err
			}
			if historyDigest(data) != file.SHA256 {
				return errors.New("source or snapshot changed during diff; run zap audit again")
			}
			sides[i] = data
		}
		if missingBefore {
			uiWarning("Previous file contents are not stored in this checkout: " + auditText(ch.Path))
			uiDetail("Before SHA256", ch.Before.SHA256)
			if ch.After != nil {
				uiDetail("After SHA256", ch.After.SHA256)
			} else {
				uiDetail("After", "deleted")
			}
			uiHint("The shared baseline contains hashes, not source contents; use Git to inspect the old file.")
			continue
		}
		showAuditDiff(ch, sides[0], sides[1])
	}
	return nil
}

func (p *Project) integrityCommandWarnings(args []string) func() {
	name := "sync"
	if len(args) > 0 {
		name = strings.ToLower(args[0])
	}
	switch name {
	case "sync", "make", "build", "update", "upload", "program", "flash", "generate", "check", "version", "add", "remove", "component", "adapter":
		p.warnIntegrity("before " + name)
		return func() { p.warnIntegrity("after " + name) }
	default:
		return func() {}
	}
}

func persistHardwareTrust(root string, t *integrityTrust) error {
	path, err := integrityTrustPath(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("signer already enrolled; refusing replacement")
	} else if !os.IsNotExist(err) {
		return err
	}
	data, _ := json.MarshalIndent(t, "", "  ")
	// Exclusive creation prevents concurrent initial enrollment from replacing trust.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
