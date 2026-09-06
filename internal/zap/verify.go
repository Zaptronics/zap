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
	if err := validateConfig(c, true); err != nil {
		return err
	}
	git, gitErr := findProgram("git")
	for _, name := range c.DependencyOrder {
		d := c.Dependencies[name]
		if d == nil {
			continue
		}
		switch strings.ToLower(d.Type) {
		case "git":
			if gitErr != nil {
				return fmt.Errorf("git is required to verify dependency %q", name)
			}
			overrideActive := d.OverrideVar != "" && os.Getenv(d.OverrideVar) != ""
			path := p.dependencyPath(c, name, d)
			if overrideActive {
				if _, err := os.Stat(path); err != nil {
					return fmt.Errorf("dependency %q local override was not found at %s", name, path)
				}
				if !opts.Offline {
					remote, err := resolveRemoteRef(git, d.URI, d.Version)
					if err != nil {
						return fmt.Errorf("dependency %q remote verification failed: %w", name, err)
					}
					if !strings.EqualFold(remote, d.Commit) {
						return fmt.Errorf("dependency %q integrity failure: %s now resolves to %s but zap.yml locks %s. The tag/branch may have moved; review before updating the lock", name, d.Version, remote, d.Commit)
					}
				}
				if !opts.CMake {
					uiWarning(fmt.Sprintf("%s: local override %s is active", name, d.OverrideVar))
					uiHint("remote lock verified; local override contents remain developer-controlled")
				}
				continue
			}
			if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
				origin, err := runCapture("", git, "-C", path, "remote", "get-url", "origin")
				if err != nil {
					return err
				}
				if strings.TrimSpace(origin) != d.URI {
					return fmt.Errorf("dependency %q local origin mismatch: configured %s, checkout uses %s", name, d.URI, strings.TrimSpace(origin))
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
					return fmt.Errorf("dependency %q local checkout has modifications and does not match the locked distribution:\n%s", name, details)
				}
			} else if opts.Offline {
				return fmt.Errorf("dependency %q is not present locally at %s and cannot be verified/fetched in offline mode", name, path)
			}
			if !opts.Offline {
				remote, err := resolveRemoteRef(git, d.URI, d.Version)
				if err != nil {
					return fmt.Errorf("dependency %q remote verification failed: %w", name, err)
				}
				if !strings.EqualFold(remote, d.Commit) {
					return fmt.Errorf("dependency %q integrity failure: %s now resolves to %s but zap.yml locks %s. The tag/branch may have moved; review before updating the lock", name, d.Version, remote, d.Commit)
				}
			}
		case "url":
			// validateConfig already requires a SHA-256 URL hash.
		case "path":
			// Explicit local path dependencies are user-controlled by definition.
		}
	}
	if !opts.CMake {
		if opts.Offline {
			uiSuccess("Dependency verification passed · offline locks only")
		} else {
			uiSuccess("Dependency verification passed")
		}
	}
	return nil
}
