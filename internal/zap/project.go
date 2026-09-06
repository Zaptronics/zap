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
	return &Project{abs, filepath.Join(abs, "zap.yml"), filepath.Join(abs, "CMakeLists.txt"), filepath.Join(abs, "cmake"), filepath.Join(abs, "cmake", "zap_deps.cmake"), filepath.Join(abs, "cmake", "zap_zephyr.conf")}
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
	cm, err := GenerateCMake(c)
	if err != nil {
		return err
	}
	if err := os.WriteFile(p.GeneratedCMake, []byte(cm), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(p.GeneratedZephyr, []byte(GenerateZephyrConf(c)), 0o644); err != nil {
		return err
	}
	fmt.Printf("Generated %s\n", p.GeneratedCMake)
	return nil
}
func (p *Project) Check(c *Config) error {
	cm, err := GenerateCMake(c)
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
	if err := validateConfig(c, false); err != nil {
		return err
	}
	git, _ := findProgram("git")
	locksChanged := c.Schema != ConfigSchema
	for _, name := range c.DependencyOrder {
		d := c.Dependencies[name]
		if d == nil {
			continue
		}
		path := p.dependencyPath(c, name, d)
		overrideActive := d.OverrideVar != "" && os.Getenv(d.OverrideVar) != ""
		switch strings.ToLower(d.Type) {
		case "git":
			if git == "" {
				return fmt.Errorf("git is required to synchronise dependency %q", name)
			}
			if d.URI == "" || d.Version == "" {
				return fmt.Errorf("dependency %q requires uri and version", name)
			}
			if overrideActive {
				fmt.Printf("%s: local override %s\n", name, path)
				if _, err := os.Stat(path); err != nil {
					return fmt.Errorf("dependency %q local override was not found at %s", name, path)
				}
				sha, err := resolveRemoteRef(git, d.URI, d.Version)
				if err != nil {
					return fmt.Errorf("dependency %q could not lock declared remote ref %q while local override is active: %w", name, d.Version, err)
				}
				sha = strings.ToLower(strings.TrimSpace(sha))
				if d.Commit == "" {
					d.Commit = sha
					locksChanged = true
					fmt.Printf("  locked %s to remote commit %s (local override remains active)\n", d.Version, d.Commit)
				} else if !strings.EqualFold(d.Commit, sha) {
					return fmt.Errorf("dependency %q integrity failure: %s resolved to %s but zap.yml locks %s; review the remote change before updating the lock", name, d.Version, sha, d.Commit)
				}
				continue
			}
			if err := ensureGitRepo(git, name, path, d.URI); err != nil {
				return err
			}
			fmt.Printf("Synchronising %s @ %s\n", name, d.Version)
			sha, err := fetchRevision(git, name, path, d.URI, d.Version)
			if err != nil {
				return err
			}
			sha = strings.ToLower(strings.TrimSpace(sha))
			if d.Commit == "" {
				d.Commit = sha
				locksChanged = true
				fmt.Printf("  locked %s to commit %s\n", d.Version, d.Commit)
			} else if !strings.EqualFold(d.Commit, sha) {
				return fmt.Errorf("dependency %q integrity failure: %s resolved to %s but zap.yml locks %s; review the remote change before updating the lock", name, d.Version, sha, d.Commit)
			}
			manifestText, manifestErr := showFileAt(git, path, sha, "zap-package.yml")
			if manifestErr == nil {
				m, err := ParsePackageManifest(manifestText)
				if err != nil {
					return fmt.Errorf("%s zap-package.yml: %w", name, err)
				}
				paths, err := resolveSparsePaths(m, d.Components)
				if err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
				fmt.Printf("  sparse checkout: %d path(s), %d selected component(s)\n", len(paths), len(d.Components))
				if err := checkoutSparse(git, path, sha, paths); err != nil {
					return err
				}
			} else {
				fmt.Printf("  package manifest not present; using full shallow checkout\n")
				_ = runStreaming("", git, "-C", path, "sparse-checkout", "disable")
				if err := runStreaming("", git, "-C", path, "checkout", "--detach", sha); err != nil {
					return err
				}
			}
		case "path":
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("local dependency %q was not found at %s", name, path)
			}
			fmt.Printf("%s: local path %s\n", name, path)
		case "url":
			fmt.Printf("%s: URL dependency will be populated by CMake\n", name)
		default:
			return fmt.Errorf("unsupported dependency type %q", d.Type)
		}
	}
	if locksChanged {
		c.Schema = ConfigSchema
		if err := p.WriteConfig(c); err != nil {
			return err
		}
		fmt.Printf("Updated %s with immutable dependency commit locks.\n", p.ConfigPath)
	}
	return nil
}
