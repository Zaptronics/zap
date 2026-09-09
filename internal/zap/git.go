// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func isGitProgram(program string) bool {
	base := strings.ToLower(filepath.Base(program))
	return base == "git" || base == "git.exe"
}

func sanitizedGitEnv(env []string) []string {
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		key := entry
		if i := strings.IndexByte(entry, '='); i >= 0 {
			key = entry[:i]
		}
		upper := strings.ToUpper(key)
		if upper == "GIT_CONFIG_COUNT" || upper == "GIT_CONFIG_PARAMETERS" || strings.HasPrefix(upper, "GIT_CONFIG_KEY_") || strings.HasPrefix(upper, "GIT_CONFIG_VALUE_") {
			continue
		}
		out = append(out, entry)
	}
	// Prevent process-injected config pairs (for example url.*.insteadOf) from
	// silently changing the transport selected by a validated source URI.
	out = append(out, "GIT_CONFIG_COUNT=0")
	return out
}

var credentialURLRE = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)([^/@\s]+)@`)

func redactSensitiveText(text string) string {
	return credentialURLRE.ReplaceAllString(text, `${1}<redacted>@`)
}

func runStreaming(dir, program string, args ...string) error {
	return runStreamingEnv(dir, nil, program, args...)
}

func runStreamingEnv(dir string, extraEnv []string, program string, args ...string) error {
	cmd := exec.Command(program, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if len(extraEnv) > 0 || isGitProgram(program) {
		env := append(os.Environ(), extraEnv...)
		if isGitProgram(program) {
			env = sanitizedGitEnv(env)
		}
		cmd.Env = env
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", program, err)
	}
	return nil
}

func runCapture(dir, program string, args ...string) (string, error) {
	cmd := exec.Command(program, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if isGitProgram(program) {
		cmd.Env = sanitizedGitEnv(os.Environ())
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := redactSensitiveText(strings.TrimSpace(errb.String()))
		safeArgs := redactSensitiveText(strings.Join(args, " "))
		if msg != "" {
			return "", fmt.Errorf("%s %s failed: %s", program, safeArgs, msg)
		}
		return "", fmt.Errorf("%s %s failed: %w", program, safeArgs, err)
	}
	return strings.TrimSpace(out.String()), nil
}

func findProgram(name string) (string, error) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s was not found on PATH", name)
	}
	return p, nil
}

func gitClean(git, path string) (bool, string, error) {
	out, err := runCapture("", git, "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-C", path, "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return false, "", err
	}
	return strings.TrimSpace(out) == "", out, nil
}

func ensureClean(git, name, path string) error {
	clean, out, err := gitClean(git, path)
	if err != nil {
		return err
	}
	if clean {
		return nil
	}
	return fmt.Errorf("dependency %q has local changes in %s; commit/stash/revert them before zap changes the checkout:\n%s", name, path, out)
}

func ensureGitRepo(git, name, path, uri string) error {
	gitDir := filepath.Join(path, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		if err := ensureClean(git, name, path); err != nil {
			return err
		}
		current, _ := runCapture("", git, "-C", path, "remote", "get-url", "origin")
		if strings.TrimSpace(current) != uri {
			if err := runStreaming("", git, "-C", path, "remote", "set-url", "origin", uri); err != nil {
				return err
			}
		}
		return nil
	}
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		entries, _ := os.ReadDir(path)
		if len(entries) > 0 {
			return fmt.Errorf("dependency directory %s exists but is not an empty Git checkout", path)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	if err := runStreaming("", git, "init", "-q", path); err != nil {
		return err
	}
	return runStreaming("", git, "-C", path, "remote", "add", "origin", uri)
}

func fetchRevision(git, name, path, uri, revision string) (string, error) {
	if err := validateSourceURI("git", uri); err != nil {
		return "", err
	}
	if revision == "" {
		return "", fmt.Errorf("dependency %q has no version/ref", name)
	}
	// Fetch only the requested ref/commit. This keeps history shallow even before sparse checkout.
	_, err := runCapture("", git, "-C", path, "fetch", "--depth=1", "--force", "origin", revision)
	if err != nil {
		return "", fmt.Errorf("dependency %q requested Git ref %q, but it could not be fetched from %s; check zap.yml and make sure the tag/branch has been pushed", name, revision, uri)
	}
	sha, err := runCapture("", git, "-C", path, "rev-parse", "FETCH_HEAD^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sha), nil
}

func showFileAt(git, path, revision, file string) (string, error) {
	return runCapture("", git, "-C", path, "show", revision+":"+file)
}

func resolveSparsePaths(m *PackageManifest, selected []string) ([]string, error) {
	set := map[string]bool{}
	for _, p := range m.Package.Paths {
		if p != "" {
			set[filepath.ToSlash(filepath.Clean(p))] = true
		}
	}
	visiting := map[string]bool{}
	done := map[string]bool{}
	var visit func(string) error
	visit = func(name string) error {
		if done[name] {
			return nil
		}
		if visiting[name] {
			return fmt.Errorf("package component dependency cycle at %s", name)
		}
		c, ok := m.Components[name]
		if !ok {
			return fmt.Errorf("package does not declare component %q", name)
		}
		visiting[name] = true
		for _, d := range c.Depends {
			if err := visit(d); err != nil {
				return err
			}
		}
		for _, p := range c.Paths {
			if p != "" {
				set[filepath.ToSlash(filepath.Clean(p))] = true
			}
		}
		delete(visiting, name)
		done[name] = true
		return nil
	}
	for _, s := range selected {
		if err := visit(s); err != nil {
			return nil, err
		}
	}
	paths := make([]string, 0, len(set))
	for p := range set {
		if p != "." && p != "" {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func checkoutSparse(git, path, sha string, paths []string) error {
	if len(paths) == 0 {
		_ = runStreaming("", git, "-C", path, "sparse-checkout", "disable")
		return runStreaming("", git, "-C", path, "checkout", "--detach", sha)
	}
	if err := runStreaming("", git, "-C", path, "sparse-checkout", "init", "--cone"); err != nil {
		return err
	}
	args := []string{"-C", path, "sparse-checkout", "set", "--cone"}
	args = append(args, paths...)
	if err := runStreaming("", git, args...); err != nil {
		return err
	}
	return runStreaming("", git, "-C", path, "checkout", "--detach", sha)
}

func loadRemotePackageManifest(git, uri, revision string) (*PackageManifest, error) {
	m, _, err := loadRemotePackageManifestLocked(git, uri, revision)
	return m, err
}

func loadRemotePackageManifestLocked(git, uri, revision string) (*PackageManifest, string, error) {
	dir, err := os.MkdirTemp("", "zap-package-")
	if err != nil {
		return nil, "", err
	}
	defer os.RemoveAll(dir)
	if err := runStreaming("", git, "init", "-q", dir); err != nil {
		return nil, "", err
	}
	if err := runStreaming("", git, "-C", dir, "remote", "add", "origin", uri); err != nil {
		return nil, "", err
	}
	sha, err := fetchRevision(git, "package", dir, uri, revision)
	if err != nil {
		return nil, "", err
	}
	text, err := showFileAt(git, dir, sha, "zap-package.yml")
	if err != nil {
		return nil, "", fmt.Errorf("%s @ %s does not provide zap-package.yml", uri, revision)
	}
	m, err := ParsePackageManifest(text)
	if err != nil {
		return nil, "", err
	}
	return m, strings.ToLower(strings.TrimSpace(sha)), nil
}

func listStableTags(git, uri string) ([]string, error) {
	if err := validateSourceURI("git", uri); err != nil {
		return nil, err
	}
	out, err := runCapture("", git, "ls-remote", "--tags", "--refs", uri)
	if err != nil {
		return nil, err
	}
	var tags []string
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		tag := strings.TrimPrefix(f[1], "refs/tags/")
		if _, ok := parseSemver(tag); ok {
			tags = append(tags, tag)
		}
	}
	return sortStableTags(tags), nil
}

func resolveRemoteRef(git, uri, revision string) (string, error) {
	if err := validateSourceURI("git", uri); err != nil {
		return "", err
	}
	if gitCommitRE.MatchString(revision) {
		return strings.ToLower(revision), nil
	}
	out, err := runCapture("", git, "ls-remote", "--tags", "--heads", uri)
	if err != nil {
		return "", err
	}
	refs := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 {
			refs[f[1]] = strings.ToLower(f[0])
		}
	}
	candidates := []string{}
	if strings.HasPrefix(revision, "refs/") {
		candidates = append(candidates, revision+"^{}", revision)
	} else {
		candidates = append(candidates,
			"refs/tags/"+revision+"^{}",
			"refs/tags/"+revision,
			"refs/heads/"+revision,
		)
	}
	for _, ref := range candidates {
		if sha := refs[ref]; gitCommitRE.MatchString(sha) {
			return sha, nil
		}
	}
	return "", fmt.Errorf("Git ref %q was not found at %s", revision, uri)
}

func fetchCommitForLock(git, name, path, uri, resolved, commit string) error {
	if err := validateSourceURI("git", uri); err != nil {
		return err
	}
	commit = strings.ToLower(strings.TrimSpace(commit))
	if !gitCommitRE.MatchString(commit) {
		return fmt.Errorf("dependency %q has invalid locked commit %q", name, commit)
	}
	if _, err := runCapture("", git, "-C", path, "fetch", "--depth=1", "--force", "origin", commit); err == nil {
		return nil
	}
	if resolved != "" && !strings.HasPrefix(strings.ToLower(resolved), "commit:") {
		if _, err := runCapture("", git, "-C", path, "fetch", "--depth=64", "--force", "origin", resolved); err == nil {
			if _, err := runCapture("", git, "-C", path, "cat-file", "-e", commit+"^{commit}"); err == nil {
				return nil
			}
		}
	}
	return fmt.Errorf("dependency %q locked commit %s could not be fetched from %s", name, commit, uri)
}
