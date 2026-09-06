// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var Version = "dev"

func Run(args []string) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	p := OpenProject(root)
	if len(args) == 0 {
		return runSync(p, nil)
	}
	cmd := strings.ToLower(args[0])
	rest := args[1:]
	switch cmd {
	case "help", "-h", "--help":
		printHelp()
		return nil
	case "--version", "version-tool":
		fmt.Printf("zap %s\n", Version)
		return nil
	case "init", "new":
		fs, o := InitFlagSet()
		if err := fs.Parse(rest); err != nil {
			return err
		}
		return p.Init(*o)
	case "sync":
		return runSync(p, rest)
	case "make", "build":
		fs, o, defineFlags := MakeFlagSet()
		if err := fs.Parse(rest); err != nil {
			return err
		}
		defines, err := parseDefines([]string(*defineFlags))
		if err != nil {
			return err
		}
		o.Defines = defines
		c, err := p.ReadConfig()
		if err != nil {
			return err
		}
		return p.Make(c, *o)
	case "upload", "program", "flash":
		fs, o := UploadFlagSet()
		if err := fs.Parse(rest); err != nil {
			return err
		}
		c, err := p.ReadConfig()
		if err != nil {
			return err
		}
		return p.Upload(c, *o)
	case "generate":
		c, err := p.ReadConfig()
		if err != nil {
			return err
		}
		return p.Generate(c)
	case "check":
		c, err := p.ReadConfig()
		if err != nil {
			return err
		}
		if err := p.Check(c); err != nil {
			return err
		}
		fmt.Println("zap.yml, generated CMake glue, and CMake integration are in sync.")
		return nil
	case "verify":
		fs := flag.NewFlagSet("verify", flag.ContinueOnError)
		offline := fs.Bool("offline", false, "skip remote ls-remote checks but still verify local locked checkouts")
		cmakeMode := fs.Bool("cmake", false, "quiet mode for invocation from generated CMake")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		c, err := p.ReadConfig()
		if err != nil {
			return err
		}
		return p.Verify(c, VerifyOptions{Offline: *offline, CMake: *cmakeMode})
	case "audit":
		sources, err := scanExternalSources(p.Root)
		if err != nil {
			return err
		}
		if len(sources) == 0 {
			fmt.Println("No literal external dependency/source URLs were found in zap.yml or the supported build manifests.")
			return nil
		}
		fmt.Println("External dependency/source locations detected:")
		for _, src := range sources {
			fmt.Printf("  %-28s %s\n", src.File, src.URI)
		}
		return nil
	case "list":
		return listConfig(p)
	case "status":
		return status(p)
	case "update":
		if len(rest) < 1 {
			return fmt.Errorf("usage: zap update <dependency>")
		}
		return update(p, rest[0])
	case "version":
		if len(rest) < 2 {
			return fmt.Errorf("usage: zap version <dependency> <version-or-ref>")
		}
		return setVersion(p, rest[0], rest[1])
	case "add":
		if len(rest) < 2 {
			return fmt.Errorf("usage: zap add <dependency> <cmake-target>")
		}
		return addComponent(p, rest[0], rest[1], true)
	case "remove":
		if len(rest) < 2 {
			return fmt.Errorf("usage: zap remove <dependency> <cmake-target>")
		}
		return removeComponent(p, rest[0], rest[1], true)
	case "adapter":
		name := "platform"
		if len(rest) > 0 {
			name = rest[0]
		}
		c, err := p.ReadConfig()
		if err != nil {
			return err
		}
		return p.GenerateAdapter(c, name)
	default:
		return fmt.Errorf("unknown command %q; run 'zap help'", cmd)
	}
}

func runSync(p *Project, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	noConfigure := fs.Bool("no-configure", false, "fetch/generate but do not run CMake configure")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := p.ReadConfig()
	if err != nil {
		return err
	}
	if err := p.Sync(c); err != nil {
		return err
	}
	if err := p.Generate(c); err != nil {
		return err
	}
	if *noConfigure {
		return nil
	}
	return p.Configure(c)
}

func listConfig(p *Project) error {
	c, err := p.ReadConfig()
	if err != nil {
		return err
	}
	fmt.Printf("Environment : %s\nTarget      : %s\n", c.Project.Environment, c.Project.Target)
	if len(c.Upload.MethodOrder) > 0 {
		fmt.Printf("Upload      : %s", c.Upload.Default)
		if strings.EqualFold(c.Upload.Default, "auto") && len(c.Upload.Order) > 0 {
			fmt.Printf(" (%s)", strings.Join(c.Upload.Order, " -> "))
		}
		fmt.Println()
	}
	for _, n := range c.DependencyOrder {
		d := c.Dependencies[n]
		fmt.Printf("%s  %s  %s  %s", n, d.Type, d.URI, d.Version)
		if d.Commit != "" {
			fmt.Printf("  commit=%s", d.Commit)
		}
		fmt.Println()
		for _, x := range d.Components {
			fmt.Printf("  -> %s\n", x)
		}
	}
	return nil
}

func status(p *Project) error {
	c, err := p.ReadConfig()
	if err != nil {
		return err
	}
	git, _ := findProgram("git")
	for _, n := range c.DependencyOrder {
		d := c.Dependencies[n]
		path := p.dependencyPath(c, n, d)
		fmt.Printf("%s\n  configured : %s %s %s\n", n, d.Type, d.URI, d.Version)
		if d.Commit != "" {
			fmt.Printf("  locked     : %s\n", d.Commit)
		}
		fmt.Printf("  path       : %s\n", path)
		if strings.ToLower(d.Type) != "git" {
			continue
		}
		if git == "" {
			fmt.Println("  checkout   : git not found")
			continue
		}
		if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
			fmt.Println("  checkout   : not fetched")
			continue
		}
		head, _ := runCapture("", git, "-C", path, "rev-parse", "--short=12", "HEAD")
		origin, _ := runCapture("", git, "-C", path, "remote", "get-url", "origin")
		clean, _, _ := gitClean(git, path)
		fmt.Printf("  head       : %s\n  origin     : %s\n  dirty      : %v\n", head, origin, !clean)
	}
	return nil
}

func update(p *Project, name string) error {
	c, err := p.ReadConfig()
	if err != nil {
		return err
	}
	d := c.Dependencies[name]
	if d == nil {
		return fmt.Errorf("dependency %q not found", name)
	}
	if strings.ToLower(d.Type) != "git" {
		return fmt.Errorf("zap update currently lists releases for git dependencies only")
	}
	git, err := findProgram("git")
	if err != nil {
		return err
	}
	tags, err := listStableTags(git, d.URI)
	if err != nil {
		return err
	}
	fmt.Printf("%s currently uses %s\nAvailable releases (newest first):\n", name, d.Version)
	for _, t := range tags {
		m := " "
		if t == d.Version {
			m = "*"
		}
		fmt.Printf("  %s %s\n", m, t)
	}
	fmt.Printf("Use 'zap version %s <tag>' to select a release.\n", name)
	return nil
}
func setVersion(p *Project, name, v string) error {
	c, err := p.ReadConfig()
	if err != nil {
		return err
	}
	d := c.Dependencies[name]
	if d == nil {
		return fmt.Errorf("dependency %q not found", name)
	}
	if strings.ToLower(d.Type) != "git" {
		d.Version = v
		if err := p.WriteConfig(c); err != nil {
			return err
		}
		return p.Generate(c)
	}
	git, err := findProgram("git")
	if err != nil {
		return fmt.Errorf("git is required to lock dependency %q to a new version: %w", name, err)
	}
	sha, err := resolveRemoteRef(git, d.URI, v)
	if err != nil {
		return err
	}
	d.Version = v
	d.Commit = sha
	c.Schema = ConfigSchema
	if err := p.WriteConfig(c); err != nil {
		return err
	}
	fmt.Printf("%s: %s locked to %s\n", name, v, sha)
	return p.Generate(c)
}

func addComponent(p *Project, name, comp string, syncNow bool) error {
	c, err := p.ReadConfig()
	if err != nil {
		return err
	}
	d := c.Dependencies[name]
	if d == nil {
		return fmt.Errorf("dependency %q not found", name)
	}
	for _, x := range d.Components {
		if x == comp {
			return nil
		}
	}
	d.Components = append(d.Components, comp)
	sort.Strings(d.Components)
	if err := p.WriteConfig(c); err != nil {
		return err
	}
	if syncNow {
		return runSync(p, []string{})
	}
	return p.Generate(c)
}
func removeComponent(p *Project, name, comp string, syncNow bool) error {
	c, err := p.ReadConfig()
	if err != nil {
		return err
	}
	d := c.Dependencies[name]
	if d == nil {
		return fmt.Errorf("dependency %q not found", name)
	}
	var out []string
	for _, x := range d.Components {
		if x != comp {
			out = append(out, x)
		}
	}
	d.Components = out
	if err := p.WriteConfig(c); err != nil {
		return err
	}
	if syncNow {
		return runSync(p, []string{})
	}
	return p.Generate(c)
}

func printHelp() {
	fmt.Println(`zap - friendly dependency and project helper for embedded C/CMake

Usage:
  zap init                 Initialise zap.yml and CMake integration
  zap sync                 Fetch/sparse-checkout dependencies and configure (bare 'zap' is the same)
  zap make [--clean]       Sync, configure, and compile the project
  zap upload [--build]     Program/upload using methods declared in zap.yml
  zap status               Show configured and checked-out dependency state
  zap verify [--offline]   Verify local copies and immutable remote locks
  zap audit                List literal external sources found in project build manifests
  zap list                 Show project dependencies and selected targets
  zap update <dep>         Show available stable release tags
  zap version <dep> <ref>  Pin a dependency version/ref in zap.yml
  zap add <dep> <target>   Add a component and synchronise
  zap remove <dep> <target> Remove a component and synchronise
  zap adapter [name]       Generate a portable platform adapter skeleton
  zap generate             Regenerate CMake/Zephyr glue only
  zap check                Verify generated files and integration markers

Build and upload settings live in zap.yml; command-line flags can override the common workflow choices.

The zap tool is optional. Generated CMake falls back to normal FetchContent
when a zap-managed sparse checkout is not present.`)
}
