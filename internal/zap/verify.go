// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type VerifyOptions struct {
	Offline bool
	CMake   bool
}

func (p *Project) Verify(c *Config, opts VerifyOptions) error {
	if err := validateConfig(c, false); err != nil {
		return err
	}
	lock, err := p.ReadLock()
	if err != nil {
		return err
	}
	if lock == nil {
		if len(c.DependencyOrder) == 0 {
			if !opts.CMake {
				uiSuccess("Dependency verification passed · no dependencies")
			}
			return nil
		}
		return fmt.Errorf("zap.lock is missing; run 'zap sync' to resolve the project")
	}
	if !lockCompatibleWithConfig(lock, c) {
		return fmt.Errorf("zap.lock does not match zap.yml; run 'zap sync' to resolve the project")
	}
	if err := validateLock(lock); err != nil {
		return err
	}

	git, gitErr := findProgram("git")
	for _, name := range lock.DependencyOrder {
		d := lock.Dependencies[name]
		if d == nil {
			continue
		}
		switch strings.ToLower(d.Type) {
		case "git":
			if gitErr != nil {
				return fmt.Errorf("git is required to verify dependency %q", name)
			}
			overrideActive := false
			if root := c.Dependencies[name]; root != nil && d.Direct && root.OverrideVar != "" && os.Getenv(root.OverrideVar) != "" {
				overrideActive = true
			}
			path := p.dependencyPathLocked(c, name, d)
			if overrideActive {
				if _, err := os.Stat(path); err != nil {
					return fmt.Errorf("dependency %q local override was not found at %s", name, path)
				}
				if !opts.CMake {
					uiWarning(fmt.Sprintf("%s: local override is active", name))
					uiHint("zap.lock still verifies the distributable dependency graph; override contents are developer-controlled")
				}
			} else if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
				origin, err := runCapture("", git, "-C", path, "remote", "get-url", "origin")
				if err != nil {
					return err
				}
				if strings.TrimSpace(origin) != d.URI {
					return fmt.Errorf("dependency %q local origin mismatch: locked %s, checkout uses %s", name, d.URI, strings.TrimSpace(origin))
				}
				head, err := runCapture("", git, "-C", path, "rev-parse", "HEAD")
				if err != nil {
					return err
				}
				if !strings.EqualFold(strings.TrimSpace(head), d.Commit) {
					return fmt.Errorf("dependency %q local checkout mismatch: expected locked commit %s, found %s", name, d.Commit, strings.TrimSpace(head))
				}
				clean, details, err := gitClean(git, path)
				if err != nil {
					return err
				}
				if !clean {
					return fmt.Errorf("dependency %q local checkout has modifications and does not match zap.lock:\n%s", name, details)
				}
				if d.ManifestSHA256 != "" {
					text, err := showFileAt(git, path, d.Commit, "zap-package.yml")
					if err != nil {
						return fmt.Errorf("dependency %q lockfile records zap-package.yml but checkout does not contain it", name)
					}
					if got := packageManifestDigest(text); !strings.EqualFold(got, d.ManifestSHA256) {
						return fmt.Errorf("dependency %q package manifest integrity failure: expected %s, got %s", name, d.ManifestSHA256, got)
					}
				}
			} else {
				return fmt.Errorf("dependency %q has no accessible managed Git checkout at %s; run 'zap sync' before verification or building", name, path)
			}

			if !opts.Offline && lockedRefShouldBeImmutable(d) {
				remote, err := resolveRemoteRef(git, d.URI, d.Resolved)
				if err != nil {
					return fmt.Errorf("dependency %q remote verification failed: %w", name, err)
				}
				if !strings.EqualFold(remote, d.Commit) {
					return fmt.Errorf("dependency %q integrity failure: release %s now resolves to %s but zap.lock records %s; the tag may have moved", name, d.Resolved, remote, d.Commit)
				}
			}
		case "url":
			if !sha256RE.MatchString(d.Hash) {
				return fmt.Errorf("dependency %q has invalid URL integrity hash in zap.lock", name)
			}
			if opts.Offline {
				return fmt.Errorf("dependency %q: offline URL content verification is not implemented; CMake handles archive downloads and hashes online", name)
			}
			if !opts.CMake {
				uiWarning(fmt.Sprintf("%s: URL declaration checked only; archive bytes and extracted source are not verified by Zap", name))
			}
		case "path":
			path := p.dependencyPathLocked(c, name, d)
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("local dependency %q was not found at %s", name, path)
			}
			if d.ManifestSHA256 != "" {
				entry, err := localManifest(path)
				if err != nil {
					return err
				}
				if !strings.EqualFold(entry.digest, d.ManifestSHA256) {
					return fmt.Errorf("local dependency %q package manifest differs from zap.lock; run 'zap sync'", name)
				}
			}
		}
	}
	if !opts.CMake {
		if opts.Offline {
			uiSuccess("Dependency verification passed · offline lockfile")
		} else {
			uiSuccess("Dependency verification passed")
		}
	}
	return nil
}

func lockedRefShouldBeImmutable(d *LockedDependency) bool {
	if d == nil || d.Type != "git" {
		return false
	}
	if _, ok := parseSemver(d.Resolved); ok {
		return true
	}
	for _, raw := range d.Requested {
		spec, err := parseVersionSpec(raw)
		if err == nil && spec.kind == "tag" {
			return true
		}
	}
	return false
}
