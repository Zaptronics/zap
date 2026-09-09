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
		return runUpdate(p, rest)
	case "outdated":
		return runOutdated(p, rest)
	case "tree":
		return runTree(p, rest)
	case "version":
		if len(rest) < 2 {
			return fmt.Errorf("usage: zap version <dependency> <version-or-ref>")
		}
		return setVersion(p, rest[0], rest[1])
	case "add":
		return runAdd(p, rest)
	case "remove":
		return runRemove(p, rest)
	case "component":
		return runComponent(p, rest)
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
	lock, _ := p.ReadLock()
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
		if lock != nil {
			if l := lock.Dependencies[n]; l != nil {
				if l.Resolved != "" {
					uiDetail("Resolved", l.Resolved)
				}
				if l.Commit != "" {
					uiDetail("Commit", shortCommit(l.Commit))
				}
			}
		}
		if len(d.Components) > 0 {
			uiDetail("Components", strings.Join(d.Components, ", "))
		}
	}
	if lock != nil {
		transitive := 0
		for _, d := range lock.Dependencies {
			if d != nil && !d.Direct {
				transitive++
			}
		}
		if transitive > 0 {
			uiDetail("Transitive", fmt.Sprintf("%d package(s); run 'zap tree'", transitive))
		}
	}
	return nil
}

func status(p *Project) error {
	c, err := p.ReadConfig()
	if err != nil {
		return err
	}
	lock, err := p.ReadLock()
	if err != nil {
		return err
	}
	uiBanner("Status", c.Project.Target)
	if lock == nil {
		uiWarning("zap.lock is missing")
		uiHint("run 'zap sync' to resolve dependencies")
		return nil
	}
	if !lockCompatibleWithConfig(lock, c) {
		uiWarning("zap.lock does not match zap.yml")
		uiHint("run 'zap sync' to resolve the changed manifest")
	}
	git, _ := findProgram("git")
	for _, n := range lock.DependencyOrder {
		d := lock.Dependencies[n]
		if d == nil {
			continue
		}
		path := p.dependencyPathLocked(c, n, d)
		title := n
		if !d.Direct {
			title += " · transitive"
		}
		uiSection(title)
		requested := strings.Join(d.Requested, " & ")
		if requested != "" {
			uiDetail("Requested", requested)
		}
		if d.Resolved != "" {
			uiDetail("Resolved", d.Resolved)
		}
		if d.Commit != "" {
			uiDetail("Locked", d.Commit)
		}
		uiDetail("Source", d.URI)
		uiDetail("Path", path)
		if strings.ToLower(d.Type) != "git" {
			continue
		}
		if git == "" {
			uiWarning("checkout: git not found")
			continue
		}
		if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
			root := c.Dependencies[n]
			if root != nil && d.Direct && root.OverrideVar != "" && os.Getenv(root.OverrideVar) != "" {
				uiSuccess("Local override active")
			} else {
				uiWarning("checkout: not fetched")
			}
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
		return fmt.Errorf("dependency %q is %s; version constraints apply to Git dependencies", name, d.Type)
	}
	if _, err := parseVersionSpec(v); err != nil {
		return fmt.Errorf("dependency %q: %w", name, err)
	}
	d.Version = v
	d.Commit = ""
	c.Schema = ConfigSchema
	if err := p.WriteConfig(c); err != nil {
		return err
	}
	uiBanner("Version", name)
	uiSuccess(fmt.Sprintf("Requested %s", v))
	if err := p.SyncWithResolveOptions(c, ResolveOptions{Update: map[string]bool{name: true}}); err != nil {
		return err
	}
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
	uiHelpCommand("zap add", "<name> --git <uri> [--version <constraint>]", "Add a Git dependency and resolve it")
	uiHelpCommand("zap add", "<name> --path <path>", "Add a local path dependency")
	uiHelpCommand("zap remove", "<dependency>", "Remove a direct dependency")
	uiHelpCommand("zap component", "<add|remove> <dep> <target>", "Change selected package components")
	uiHelpCommand("zap update", "[dependency ...]", "Update locked versions within declared constraints")
	uiHelpCommand("zap outdated", "", "Show compatible and newer dependency releases")
	uiHelpCommand("zap tree", "[--why <dependency>]", "Explain the resolved dependency graph")
	uiHelpCommand("zap list", "", "Show dependency intent and locked resolution")
	uiHelpCommand("zap version", "<dep> <constraint>", "Change a dependency version constraint")
	uiHelpCommand("zap verify", "[--offline]", "Verify zap.lock, checkouts, and immutable sources")
	uiHelpCommand("zap audit", "", "List literal external sources in build manifests")

	uiHelpSection("Tool")
	uiHelpCommand("zap help", "", "Show this help")
	uiHelpCommand("zap version-tool", "", "Show the installed Zap version")

	uiHelpNote("Build and upload settings live in zap.yml; command-line flags override common workflow choices.")
	uiHelpNote("Generated CMake falls back to normal FetchContent when a Zap-managed sparse checkout is absent.")
}
