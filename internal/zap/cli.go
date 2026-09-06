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
		fmt.Printf("%s %s\n", paint(ansiEnabled(os.Stdout), ansiBold+ansiCyan, "zap"), Version)
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
		uiBanner("Generate", c.Project.Target)
		return p.Generate(c)
	case "check":
		c, err := p.ReadConfig()
		if err != nil {
			return err
		}
		if err := p.Check(c); err != nil {
			return err
		}
		uiResult("PROJECT CHECK PASSED", uiRow{Label: "Manifest", Value: "zap.yml"}, uiRow{Label: "Generated", Value: "in sync"})
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
		if !*cmakeMode {
			uiBanner("Verify", c.Project.Target)
		}
		return p.Verify(c, VerifyOptions{Offline: *offline, CMake: *cmakeMode})
	case "audit":
		uiBanner("Audit", filepath.Base(p.Root))
		sources, err := scanExternalSources(p.Root)
		if err != nil {
			return err
		}
		if len(sources) == 0 {
			uiSuccess("No literal external dependency/source URLs found")
			return nil
		}
		uiSection("External sources")
		for _, src := range sources {
			uiDetail(src.File, src.URI)
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
	uiBanner("Sync", fmt.Sprintf("%s · %s", c.Project.Target, c.Project.Environment))
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
	uiBanner("Project", c.Project.Target)
	uiDetail("Environment", c.Project.Environment)
	uiDetail("Target", c.Project.Target)
	if c.Project.Board != "" {
		uiDetail("Board", c.Project.Board)
	}
	if len(c.Upload.MethodOrder) > 0 {
		upload := c.Upload.Default
		if strings.EqualFold(c.Upload.Default, "auto") && len(c.Upload.Order) > 0 {
			upload += " · " + strings.Join(c.Upload.Order, " → ")
		}
		uiDetail("Upload", upload)
	}
	uiSection("Dependencies")
	for _, n := range c.DependencyOrder {
		d := c.Dependencies[n]
		uiStep(n, fmt.Sprintf("%s · %s", d.Type, d.Version))
		uiDetail("Source", d.URI)
		if d.Commit != "" {
			uiDetail("Commit", shortCommit(d.Commit))
		}
		if len(d.Components) > 0 {
			uiDetail("Components", strings.Join(d.Components, ", "))
		}
	}
	return nil
}

func status(p *Project) error {
	c, err := p.ReadConfig()
	if err != nil {
		return err
	}
	uiBanner("Status", c.Project.Target)
	git, _ := findProgram("git")
	for _, n := range c.DependencyOrder {
		d := c.Dependencies[n]
		path := p.dependencyPath(c, n, d)
		uiSection(n)
		uiDetail("Configured", fmt.Sprintf("%s %s %s", d.Type, d.URI, d.Version))
		if d.Commit != "" {
			uiDetail("Locked", d.Commit)
		}
		uiDetail("Path", path)
		if strings.ToLower(d.Type) != "git" {
			continue
		}
		if git == "" {
			uiWarning("checkout: git not found")
			continue
		}
		if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
			uiWarning("checkout: not fetched")
			continue
		}
		head, _ := runCapture("", git, "-C", path, "rev-parse", "--short=12", "HEAD")
		origin, _ := runCapture("", git, "-C", path, "remote", "get-url", "origin")
		clean, _, _ := gitClean(git, path)
		uiDetail("Head", head)
		uiDetail("Origin", origin)
		if clean {
			uiSuccess("Working tree clean")
		} else {
			uiWarning("Working tree has local changes")
		}
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
	uiBanner("Releases", name)
	uiDetail("Current", d.Version)
	uiSection("Available")
	for _, tag := range tags {
		if tag == d.Version {
			uiSuccess(tag + " · current")
		} else {
			uiStep("Release", tag)
		}
	}
	uiHint(fmt.Sprintf("use 'zap version %s <tag>' to select a release", name))
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
	uiSuccess(fmt.Sprintf("%s locked to %s", name, v))
	uiDetail("Commit", sha)
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
	uiHelpHeader()
	uiHelpUsage("zap <command> [options]")

	uiHelpSection("Everyday")
	uiHelpCommand("zap", "", "Sync dependencies, generate glue, and configure")
	uiHelpCommand("zap make", "[--clean]", "Sync, configure, and compile the project")
	uiHelpCommand("zap upload", "[--build]", "Program/upload using methods declared in zap.yml")
	uiHelpCommand("zap status", "", "Show configured and checked-out dependency state")

	uiHelpSection("Project")
	uiHelpCommand("zap init", "", "Initialise zap.yml and CMake integration")
	uiHelpCommand("zap sync", "[--no-configure]", "Fetch dependencies and configure the project")
	uiHelpCommand("zap generate", "", "Regenerate CMake/Zephyr glue only")
	uiHelpCommand("zap check", "", "Verify generated files and integration markers")
	uiHelpCommand("zap adapter", "[name]", "Generate a portable platform adapter skeleton")

	uiHelpSection("Dependencies")
	uiHelpCommand("zap list", "", "Show project dependencies and selected targets")
	uiHelpCommand("zap verify", "[--offline]", "Verify local copies and immutable remote locks")
	uiHelpCommand("zap audit", "", "List literal external sources in build manifests")
	uiHelpCommand("zap update", "<dep>", "Show available stable release tags")
	uiHelpCommand("zap version", "<dep> <ref>", "Pin a dependency version/ref in zap.yml")
	uiHelpCommand("zap add", "<dep> <target>", "Add a component and synchronise")
	uiHelpCommand("zap remove", "<dep> <target>", "Remove a component and synchronise")

	uiHelpSection("Tool")
	uiHelpCommand("zap help", "", "Show this help")
	uiHelpCommand("zap version-tool", "", "Show the installed Zap version")

	uiHelpNote("Build and upload settings live in zap.yml; command-line flags override common workflow choices.")
	uiHelpNote("Generated CMake falls back to normal FetchContent when a Zap-managed sparse checkout is absent.")
}
