// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

func runAdd(p *Project, args []string) error {
	if len(args) == 2 && !strings.HasPrefix(args[0], "-") && !strings.HasPrefix(args[1], "-") && strings.Contains(args[1], "::") {
		uiWarning("'zap add <dependency> <component>' is deprecated")
		uiHint("use 'zap component add <dependency> <component>'")
		return addComponent(p, args[0], args[1], true)
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("usage: zap add <name> (--git <uri> [--version <constraint>] | --path <path>) [--component <target>]")
	}
	name := args[0]
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	gitURI := fs.String("git", "", "Git repository URI")
	pathURI := fs.String("path", "", "local dependency path")
	version := fs.String("version", "", "version constraint, tag/ref, or commit")
	noSync := fs.Bool("no-sync", false, "edit zap.yml without resolving/synchronising")
	var components multiFlag
	fs.Var(&components, "component", "CMake component target to select; may be repeated")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q; usage: zap add <name> (--git <uri> [--version <constraint>] | --path <path>) [--component <target>]", fs.Arg(0))
	}
	if !dependencyNameRE.MatchString(name) {
		return fmt.Errorf("dependency name %q is invalid", name)
	}
	if (*gitURI == "") == (*pathURI == "") {
		return fmt.Errorf("zap add requires exactly one source: --git <uri> or --path <path>; a package registry is not configured yet")
	}
	c, err := p.ReadConfig()
	if err != nil {
		return err
	}
	if c.Dependencies[name] != nil {
		return fmt.Errorf("dependency %q already exists; use 'zap version %s <constraint>' or 'zap component add %s <target>'", name, name, name)
	}
	d := &DependencyConfig{Components: uniqueSorted([]string(components))}
	if *gitURI != "" {
		d.Type = "git"
		d.URI = *gitURI
		d.Version = strings.TrimSpace(*version)
		if d.Version == "" {
			git, err := findProgram("git")
			if err != nil {
				return err
			}
			tags, err := listStableTags(git, d.URI)
			if err != nil {
				return fmt.Errorf("could not discover a default version for %q: %w", name, err)
			}
			if len(tags) == 0 {
				return fmt.Errorf("dependency %q has no stable x.y.z Git tags; specify --version ref:<branch> or --version commit:<sha>", name)
			}
			v, _ := parseSemver(tags[0])
			d.Version = "^" + semverString(v)
			uiHint(fmt.Sprintf("using %s from latest stable release %s", d.Version, tags[0]))
		}
		if _, err := parseVersionSpec(d.Version); err != nil {
			return err
		}
	} else {
		d.Type = "path"
		d.URI = *pathURI
		if *version != "" {
			return fmt.Errorf("--version is not used with --path dependencies")
		}
	}
	c.Dependencies[name] = d
	c.DependencyOrder = append(c.DependencyOrder, name)
	c.Schema = ConfigSchema
	if err := p.WriteConfig(c); err != nil {
		return err
	}
	uiBanner("Add", name)
	uiSuccess(fmt.Sprintf("Added %s dependency", d.Type))
	uiDetail("Source", d.URI)
	if d.Version != "" {
		uiDetail("Requested", d.Version)
	}
	if *noSync {
		uiHint("zap.yml changed; run 'zap sync' to resolve zap.lock")
		return nil
	}
	if err := p.Sync(c); err != nil {
		return err
	}
	return p.Generate(c)
}

func runRemove(p *Project, args []string) error {
	if len(args) == 2 && !strings.HasPrefix(args[0], "-") && !strings.HasPrefix(args[1], "-") {
		uiWarning("'zap remove <dependency> <component>' is deprecated")
		uiHint("use 'zap component remove <dependency> <component>'")
		return removeComponent(p, args[0], args[1], true)
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("usage: zap remove <dependency> [--no-sync]")
	}
	name := args[0]
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	noSync := fs.Bool("no-sync", false, "edit zap.yml without resolving/synchronising")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q; usage: zap remove <dependency> [--no-sync]", fs.Arg(0))
	}
	c, err := p.ReadConfig()
	if err != nil {
		return err
	}
	if c.Dependencies[name] == nil {
		return fmt.Errorf("dependency %q not found", name)
	}
	delete(c.Dependencies, name)
	var order []string
	for _, n := range c.DependencyOrder {
		if n != name {
			order = append(order, n)
		}
	}
	c.DependencyOrder = order
	if err := p.WriteConfig(c); err != nil {
		return err
	}
	uiBanner("Remove", name)
	uiSuccess("Removed direct dependency from zap.yml")
	if *noSync {
		uiHint("run 'zap sync' to update zap.lock and dependency checkouts")
		return nil
	}
	if err := p.Sync(c); err != nil {
		return err
	}
	return p.Generate(c)
}

func runComponent(p *Project, args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("usage: zap component <add|remove> <dependency> <cmake-target>")
	}
	switch strings.ToLower(args[0]) {
	case "add":
		return addComponent(p, args[1], args[2], true)
	case "remove":
		return removeComponent(p, args[1], args[2], true)
	default:
		return fmt.Errorf("unknown component command %q; use add or remove", args[0])
	}
}

func runUpdate(p *Project, args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := p.ReadConfig()
	if err != nil {
		return err
	}
	existing, err := p.ReadLock()
	if err != nil {
		return err
	}
	if existing == nil && len(c.DependencyOrder) > 0 {
		return fmt.Errorf("zap.lock is missing; run 'zap sync' before 'zap update'")
	}
	opts := ResolveOptions{Update: map[string]bool{}}
	if fs.NArg() == 0 {
		opts.UpdateAll = true
	} else {
		for _, name := range fs.Args() {
			if c.Dependencies[name] == nil && (existing == nil || existing.Dependencies[name] == nil) {
				return fmt.Errorf("dependency %q not found", name)
			}
			opts.Update[name] = true
		}
	}
	uiBanner("Update", updateLabel(fs.Args()))
	before := lockSnapshot(existing)
	if err := p.SyncWithResolveOptions(c, opts); err != nil {
		return err
	}
	after, err := p.ReadLock()
	if err != nil {
		return err
	}
	printLockChanges(before, after)
	return p.Generate(c)
}

func updateLabel(names []string) string {
	if len(names) == 0 {
		return "all dependencies within declared constraints"
	}
	return strings.Join(names, ", ")
}

func lockSnapshot(lock *Lockfile) map[string]string {
	out := map[string]string{}
	if lock == nil {
		return out
	}
	for name, d := range lock.Dependencies {
		if d == nil {
			continue
		}
		if d.Resolved != "" {
			out[name] = d.Resolved
		} else {
			out[name] = shortCommit(d.Commit)
		}
	}
	return out
}

func printLockChanges(before map[string]string, after *Lockfile) {
	if after == nil {
		return
	}
	changed := 0
	for _, name := range after.DependencyOrder {
		d := after.Dependencies[name]
		if d == nil {
			continue
		}
		value := d.Resolved
		if value == "" {
			value = shortCommit(d.Commit)
		}
		old := before[name]
		if old != "" && old != value {
			uiSuccess(fmt.Sprintf("%s %s -> %s", name, old, value))
			changed++
		}
	}
	if changed == 0 {
		uiSuccess("Already at the newest versions allowed by zap.yml")
	}
}

func runOutdated(p *Project, args []string) error {
	fs := flag.NewFlagSet("outdated", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: zap outdated")
	}
	c, err := p.ReadConfig()
	if err != nil {
		return err
	}
	lock, err := p.ReadLock()
	if err != nil {
		return err
	}
	if lock == nil || !lockCompatibleWithConfig(lock, c) {
		return fmt.Errorf("zap.lock is missing or stale; run 'zap sync' first")
	}
	git, err := findProgram("git")
	if err != nil {
		return err
	}
	uiBanner("Outdated", c.Project.Target)
	updates := 0
	for _, name := range lock.DependencyOrder {
		d := lock.Dependencies[name]
		if d == nil || d.Type != "git" {
			continue
		}
		var semSpecs []versionSpec
		fixedRef := false
		for _, raw := range d.Requested {
			spec, err := parseVersionSpec(raw)
			if err != nil {
				return err
			}
			if spec.kind == "semver" {
				semSpecs = append(semSpecs, spec)
			} else if spec.kind == "ref" {
				fixedRef = true
			}
		}
		uiSection(name)
		uiDetail("Locked", fmt.Sprintf("%s · %s", d.Resolved, shortCommit(d.Commit)))
		if fixedRef && len(semSpecs) == 0 {
			remote, err := resolveRemoteRef(git, d.URI, d.Resolved)
			if err != nil {
				uiWarning(err.Error())
				continue
			}
			if !strings.EqualFold(remote, d.Commit) {
				uiWarning(fmt.Sprintf("ref advanced to %s", shortCommit(remote)))
				updates++
			} else {
				uiSuccess("Ref has not advanced")
			}
			continue
		}
		if len(semSpecs) == 0 {
			uiSuccess("Pinned to fixed tag/commit")
			continue
		}
		tags, err := listStableTags(git, d.URI)
		if err != nil {
			return err
		}
		latest := ""
		compatible := ""
		if len(tags) > 0 {
			latest = tags[0]
		}
		for _, tag := range tags {
			if versionSatisfiesAll(tag, semSpecs) {
				compatible = tag
				break
			}
		}
		uiDetail("Requested", strings.Join(d.Requested, " & "))
		uiDetail("Compatible", compatible)
		uiDetail("Latest", latest)
		if compatible != "" && compatible != d.Resolved {
			uiWarning("compatible update available")
			updates++
		} else if latest != "" && latest != d.Resolved {
			uiHint("newer release exists outside the declared constraint")
		} else {
			uiSuccess("Up to date")
		}
	}
	if updates == 0 {
		uiSuccess("No compatible dependency updates available")
	} else {
		uiHint("run 'zap update' to update within declared constraints")
	}
	return nil
}

func runTree(p *Project, args []string) error {
	fs := flag.NewFlagSet("tree", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	why := fs.String("why", "", "show paths that require a dependency")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: zap tree [--why <dependency>]")
	}
	c, err := p.ReadConfig()
	if err != nil {
		return err
	}
	lock, err := p.ReadLock()
	if err != nil {
		return err
	}
	if lock == nil || !lockCompatibleWithConfig(lock, c) {
		return fmt.Errorf("zap.lock is missing or stale; run 'zap sync' first")
	}
	uiBanner("Dependency Tree", c.Project.Target)
	children := map[string][]string{}
	for name, d := range lock.Dependencies {
		if d == nil {
			continue
		}
		for _, parent := range d.Parents {
			children[parent] = append(children[parent], name)
		}
	}
	for parent := range children {
		sort.Strings(children[parent])
	}
	if *why != "" {
		if lock.Dependencies[*why] == nil {
			return fmt.Errorf("dependency %q not found in zap.lock", *why)
		}
		paths := dependencyPathsTo("root", *why, children, nil)
		uiSection("Why " + *why)
		for _, path := range paths {
			fmt.Printf("  %s\n", strings.Join(path, " -> "))
		}
		return nil
	}
	roots := children["root"]
	if len(roots) == 0 {
		uiSuccess("No dependencies")
		return nil
	}
	for i, name := range roots {
		last := i == len(roots)-1
		printDependencyTree(lock, children, name, "", last, map[string]bool{})
	}
	return nil
}

func printDependencyTree(lock *Lockfile, children map[string][]string, name, prefix string, last bool, stack map[string]bool) {
	branch := "├─"
	nextPrefix := prefix + "│  "
	if last {
		branch = "└─"
		nextPrefix = prefix + "   "
	}
	d := lock.Dependencies[name]
	label := name
	if d != nil {
		if d.Resolved != "" {
			label += " " + d.Resolved
		} else if d.Type == "path" {
			label += " (path)"
		}
	}
	fmt.Printf("  %s%s %s\n", prefix, branch, label)
	if stack[name] {
		fmt.Printf("  %s   (cycle)\n", nextPrefix)
		return
	}
	stack2 := map[string]bool{}
	for k, v := range stack {
		stack2[k] = v
	}
	stack2[name] = true
	cc := children[name]
	for i, child := range cc {
		printDependencyTree(lock, children, child, nextPrefix, i == len(cc)-1, stack2)
	}
}

func dependencyPathsTo(current, target string, children map[string][]string, path []string) [][]string {
	path = append(path, current)
	var out [][]string
	for _, child := range children[current] {
		if child == target {
			p := append(append([]string{}, path...), child)
			out = append(out, p)
			continue
		}
		out = append(out, dependencyPathsTo(child, target, children, path)...)
	}
	return out
}
