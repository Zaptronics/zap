// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Project struct {
	Root            string
	ConfigPath      string
	LockPath        string
	CMakePath       string
	GeneratedDir    string
	GeneratedCMake  string
	GeneratedZephyr string
}

func OpenProject(root string) *Project {
	if root == "" {
		root = "."
	}
	abs, _ := filepath.Abs(root)
	return &Project{Root: abs, ConfigPath: filepath.Join(abs, "zap.yml"), LockPath: filepath.Join(abs, "zap.lock"), CMakePath: filepath.Join(abs, "CMakeLists.txt"), GeneratedDir: filepath.Join(abs, "cmake"), GeneratedCMake: filepath.Join(abs, "cmake", "zap_deps.cmake"), GeneratedZephyr: filepath.Join(abs, "cmake", "zap_zephyr.conf")}
}
func (p *Project) ReadConfig() (*Config, error) {
	b, err := os.ReadFile(p.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("zap.yml was not found; run 'zap init' first")
	}
	return ParseConfig(string(b))
}
func (p *Project) WriteConfig(c *Config) error {
	if err := validateConfig(c, false); err != nil {
		return err
	}
	return os.WriteFile(p.ConfigPath, []byte(FormatConfig(c)), 0o644)
}
func (p *Project) Generate(c *Config) error {
	if err := EnsureCMakeIntegration(p.Root, c); err != nil {
		return err
	}
	if err := os.MkdirAll(p.GeneratedDir, 0o755); err != nil {
		return err
	}
	lock, err := p.lockForGeneration(c)
	if err != nil {
		return err
	}
	cm, err := GenerateCMakeWithLock(c, lock)
	if err != nil {
		return err
	}
	if err := os.WriteFile(p.GeneratedCMake, []byte(cm), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(p.GeneratedZephyr, []byte(GenerateZephyrConf(c)), 0o644); err != nil {
		return err
	}
	uiSuccess("Generated CMake/Zephyr integration")
	uiDetail("CMake", p.GeneratedCMake)
	return nil
}
func (p *Project) Check(c *Config) error {
	lock, err := p.lockForGeneration(c)
	if err != nil {
		return err
	}
	cm, err := GenerateCMakeWithLock(c, lock)
	if err != nil {
		return err
	}
	got, err := os.ReadFile(p.GeneratedCMake)
	if err != nil {
		return fmt.Errorf("cmake/zap_deps.cmake is missing; run 'zap generate'")
	}
	if normalize(string(got)) != normalize(cm) {
		return fmt.Errorf("cmake/zap_deps.cmake is out of date; run 'zap generate'")
	}
	zc := GenerateZephyrConf(c)
	got, err = os.ReadFile(p.GeneratedZephyr)
	if err != nil {
		return fmt.Errorf("cmake/zap_zephyr.conf is missing; run 'zap generate'")
	}
	if normalize(string(got)) != normalize(zc) {
		return fmt.Errorf("cmake/zap_zephyr.conf is out of date; run 'zap generate'")
	}
	ct, err := os.ReadFile(p.CMakePath)
	if err != nil {
		return err
	}
	s := string(ct)
	if err := validateAllManagedMarkers(s); err != nil {
		return err
	}
	if !strings.Contains(s, includeBegin) || !strings.Contains(s, applyBegin) {
		return fmt.Errorf("CMakeLists.txt is missing Zap integration markers; run 'zap generate'")
	}
	return nil
}

func (p *Project) lockForGeneration(c *Config) (*Lockfile, error) {
	lock, err := p.ReadLock()
	if err != nil {
		return nil, err
	}
	if len(c.DependencyOrder) == 0 {
		if lock == nil {
			return &Lockfile{Schema: LockSchema, Dependencies: map[string]*LockedDependency{}}, nil
		}
		return lock, nil
	}
	if lock == nil {
		return nil, fmt.Errorf("zap.lock is missing; run 'zap sync' before generating build integration")
	}
	if !lockCompatibleWithConfig(lock, c) {
		return nil, fmt.Errorf("zap.lock does not match zap.yml; run 'zap sync' before generating build integration")
	}
	return lock, nil
}

func shortCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

func normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

func (p *Project) dependencyPath(c *Config, name string, d *DependencyConfig) string {
	if d.OverrideVar != "" {
		if v := os.Getenv(d.OverrideVar); v != "" {
			if filepath.IsAbs(v) {
				return v
			}
			return filepath.Join(p.Root, v)
		}
	}
	if strings.ToLower(d.Type) == "path" && d.URI != "" {
		if filepath.IsAbs(d.URI) {
			return d.URI
		}
		return filepath.Join(p.Root, d.URI)
	}
	base := c.Project.DepsDir
	if base == "" {
		base = "deps"
	}
	if !filepath.IsAbs(base) {
		base = filepath.Join(p.Root, base)
	}
	return filepath.Join(base, safeName(name))
}

func (p *Project) Sync(c *Config) error {
	return p.SyncWithResolveOptions(c, ResolveOptions{})
}

func (p *Project) SyncWithResolveOptions(c *Config, opts ResolveOptions) error {
	if err := validateConfig(c, false); err != nil {
		return err
	}
	uiSection("Dependencies")
	existing, err := p.ReadLock()
	if err != nil {
		return err
	}
	needResolve := opts.UpdateAll || len(opts.Update) > 0 || !lockCompatibleWithConfig(existing, c) || lockContainsMutablePath(existing)
	lock := existing
	if needResolve {
		uiStep("Resolve", "dependency graph")
		lock, err = p.ResolveDependencies(c, existing, opts)
		if err != nil {
			return err
		}
		if err := p.WriteLock(lock); err != nil {
			return err
		}
		uiSuccess("Resolved deterministic dependency graph")
		uiDetail("Lockfile", p.LockPath)
	} else {
		uiSuccess("Lockfile is current")
	}

	if err := p.materializeLock(c, lock); err != nil {
		return err
	}

	legacy := c.Schema != ConfigSchema
	for _, name := range c.DependencyOrder {
		if d := c.Dependencies[name]; d != nil && d.Commit != "" {
			d.Commit = ""
			legacy = true
		}
	}
	if legacy {
		c.Schema = ConfigSchema
		if err := p.WriteConfig(c); err != nil {
			return err
		}
		uiSuccess("Migrated zap.yml to intent-only dependency declarations")
		uiHint("immutable Git commits now live in zap.lock")
	}
	return nil
}

func (p *Project) dependencyPathLocked(c *Config, name string, locked *LockedDependency) string {
	if d := c.Dependencies[name]; d != nil && locked.Direct {
		return p.dependencyPath(c, name, d)
	}
	if locked.Type == "path" {
		if filepath.IsAbs(locked.URI) {
			return filepath.Clean(locked.URI)
		}
		return filepath.Clean(filepath.Join(p.Root, locked.URI))
	}
	base := c.Project.DepsDir
	if base == "" {
		base = "deps"
	}
	if !filepath.IsAbs(base) {
		base = filepath.Join(p.Root, base)
	}
	return filepath.Join(base, safeName(name))
}

func (p *Project) materializeLock(c *Config, lock *Lockfile) error {
	if lock == nil {
		return fmt.Errorf("zap.lock is missing; run 'zap sync' to resolve dependencies")
	}
	if err := validateLock(lock); err != nil {
		return err
	}
	git, _ := findProgram("git")
	for _, name := range lock.DependencyOrder {
		d := lock.Dependencies[name]
		if d == nil {
			continue
		}
		path := p.dependencyPathLocked(c, name, d)
		overrideActive := false
		if root := c.Dependencies[name]; root != nil && d.Direct && root.OverrideVar != "" && os.Getenv(root.OverrideVar) != "" {
			overrideActive = true
		}
		switch strings.ToLower(d.Type) {
		case "git":
			if git == "" {
				return fmt.Errorf("git is required to synchronise dependency %q", name)
			}
			if overrideActive {
				uiStep(name, "local override")
				uiDetail("Path", path)
				if _, err := os.Stat(path); err != nil {
					return fmt.Errorf("dependency %q local override was not found at %s", name, path)
				}
				continue
			}
			if err := ensureGitRepo(git, name, path, d.URI); err != nil {
				return err
			}
			detail := d.Resolved
			if !d.Direct {
				detail += " · transitive"
			}
			uiStep(name, detail)
			if err := fetchCommitForLock(git, name, path, d.URI, d.Resolved, d.Commit); err != nil {
				return err
			}
			manifestText, manifestErr := showFileAt(git, path, d.Commit, "zap-package.yml")
			if manifestErr == nil {
				if d.ManifestSHA256 == "" {
					return fmt.Errorf("dependency %q lockfile is missing zap-package.yml integrity hash; run 'zap update %s'", name, name)
				}
				gotDigest := packageManifestDigest(manifestText)
				if !strings.EqualFold(gotDigest, d.ManifestSHA256) {
					return fmt.Errorf("dependency %q package manifest integrity failure: zap.lock expects %s, got %s", name, d.ManifestSHA256, gotDigest)
				}
				m, err := ParsePackageManifest(manifestText)
				if err != nil {
					return fmt.Errorf("%s zap-package.yml: %w", name, err)
				}
				paths, err := resolveSparsePaths(m, d.Components)
				if err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
				uiDetail("Checkout", fmt.Sprintf("%d paths · %d components", len(paths), len(d.Components)))
				if err := checkoutSparse(git, path, d.Commit, paths); err != nil {
					return err
				}
			} else {
				if d.ManifestSHA256 != "" {
					return fmt.Errorf("dependency %q lockfile records zap-package.yml but the locked commit does not contain it", name)
				}
				uiHint(fmt.Sprintf("%s: package manifest not present; using full shallow checkout", name))
				_ = runStreaming("", git, "-C", path, "sparse-checkout", "disable")
				if err := runStreaming("", git, "-C", path, "checkout", "--detach", d.Commit); err != nil {
					return err
				}
			}
		case "path":
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("local dependency %q was not found at %s", name, path)
			}
			uiStep(name, "local path")
			uiDetail("Path", path)
			if d.ManifestSHA256 != "" {
				entry, err := localManifest(path)
				if err != nil {
					return err
				}
				if !strings.EqualFold(entry.digest, d.ManifestSHA256) {
					return fmt.Errorf("local dependency %q changed after resolution; run 'zap sync' again", name)
				}
			}
		case "url":
			uiStep(name, "CMake URL dependency")
		default:
			return fmt.Errorf("unsupported dependency type %q", d.Type)
		}
	}
	return nil
}
