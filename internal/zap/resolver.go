// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ResolveOptions struct {
	UpdateAll bool
	Update    map[string]bool
}

type dependencyRequirement struct {
	Name         string
	Parent       string
	Type         string
	URI          string
	Version      string
	Components   []string
	Direct       bool
	ZephyrModule bool
	LegacyCommit string
	SourceRoot   string
}

type resolvedDependency struct {
	Name             string
	Type             string
	URI              string
	Requested        []string
	RootRequested    string
	Resolved         string
	Commit           string
	Hash             string
	Manifest         *PackageManifest
	ManifestText     string
	ManifestSHA256   string
	Components       []string
	RootComponents   []string
	Parents          []string
	Direct           bool
	ZephyrModule     bool
	RootZephyrModule bool
	SourceRoot       string
}

type manifestCacheEntry struct {
	manifest *PackageManifest
	text     string
	digest   string
}

type dependencyResolver struct {
	project  *Project
	config   *Config
	git      string
	existing *Lockfile
	opts     ResolveOptions
	cache    map[string]manifestCacheEntry
}

func (p *Project) ResolveDependencies(c *Config, existing *Lockfile, opts ResolveOptions) (*Lockfile, error) {
	if err := validateConfig(c, false); err != nil {
		return nil, err
	}
	git, _ := findProgram("git")
	r := &dependencyResolver{project: p, config: c, git: git, existing: existing, opts: opts, cache: map[string]manifestCacheEntry{}}
	return r.resolve()
}

func (r *dependencyResolver) resolve() (*Lockfile, error) {
	reqs := r.rootRequirements()
	for iteration := 0; iteration < 64; iteration++ {
		resolved := map[string]*resolvedDependency{}
		names := sortedRequirementNames(reqs)
		for _, name := range names {
			node, err := r.resolveOne(name, reqs[name])
			if err != nil {
				return nil, err
			}
			resolved[name] = node
		}

		next := r.rootRequirements()
		for _, name := range names {
			node := resolved[name]
			if node.Manifest == nil {
				continue
			}
			for _, childName := range node.Manifest.DependencyOrder {
				d := node.Manifest.Dependencies[childName]
				if d == nil {
					continue
				}
				req := dependencyRequirement{
					Name:         childName,
					Parent:       name,
					Type:         strings.ToLower(d.Type),
					URI:          d.URI,
					Version:      d.Version,
					Components:   append([]string{}, d.Components...),
					ZephyrModule: d.ZephyrModule,
				}
				if req.Type == "path" {
					if node.Type != "path" || node.SourceRoot == "" {
						return nil, fmt.Errorf("package %q declares path dependency %q, but transitive path dependencies are only valid from a local path package", name, childName)
					}
					if filepath.IsAbs(req.URI) {
						req.SourceRoot = filepath.Clean(req.URI)
					} else {
						req.SourceRoot = filepath.Clean(filepath.Join(node.SourceRoot, req.URI))
					}
					req.URI = req.SourceRoot
				}
				next[childName] = append(next[childName], req)
			}
		}
		if requirementSetsEqual(reqs, next) {
			return buildLockfile(resolved)
		}
		reqs = next
	}
	return nil, fmt.Errorf("dependency resolution did not converge; check for a package dependency cycle or unstable local package manifests")
}

func (r *dependencyResolver) rootRequirements() map[string][]dependencyRequirement {
	out := map[string][]dependencyRequirement{}
	for _, name := range r.config.DependencyOrder {
		d := r.config.Dependencies[name]
		if d == nil {
			continue
		}
		req := dependencyRequirement{
			Name:         name,
			Parent:       "root",
			Type:         strings.ToLower(d.Type),
			URI:          d.URI,
			Version:      d.Version,
			Components:   append([]string{}, d.Components...),
			Direct:       true,
			ZephyrModule: d.ZephyrModule,
			LegacyCommit: strings.ToLower(strings.TrimSpace(d.Commit)),
		}
		if req.Type == "path" {
			req.SourceRoot = r.project.dependencyPath(r.config, name, d)
		}
		out[name] = append(out[name], req)
	}
	return out
}

func sortedRequirementNames(reqs map[string][]dependencyRequirement) []string {
	names := make([]string, 0, len(reqs))
	for name := range reqs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *dependencyResolver) resolveOne(name string, reqs []dependencyRequirement) (*resolvedDependency, error) {
	if len(reqs) == 0 {
		return nil, fmt.Errorf("dependency %q has no requirements", name)
	}
	typ := strings.ToLower(reqs[0].Type)
	uri := reqs[0].URI
	pathRoot := ""
	if typ == "path" {
		pathRoot = filepath.Clean(reqs[0].SourceRoot)
	}
	for _, req := range reqs[1:] {
		if strings.ToLower(req.Type) != typ {
			return nil, dependencyConflict(name, reqs, fmt.Sprintf("requirements disagree on dependency type (%s vs %s)", typ, req.Type))
		}
		if typ == "path" {
			if filepath.Clean(req.SourceRoot) != pathRoot {
				return nil, dependencyConflict(name, reqs, fmt.Sprintf("requirements resolve to different local paths (%s vs %s)", pathRoot, req.SourceRoot))
			}
		} else if req.URI != uri {
			return nil, dependencyConflict(name, reqs, fmt.Sprintf("requirements disagree on source URI (%s vs %s)", uri, req.URI))
		}
	}

	node := &resolvedDependency{Name: name, Type: typ, URI: uri}
	for _, req := range reqs {
		if req.Version != "" {
			node.Requested = append(node.Requested, req.Version)
		}
		node.Components = append(node.Components, req.Components...)
		node.Parents = append(node.Parents, req.Parent)
		node.ZephyrModule = node.ZephyrModule || req.ZephyrModule
		if req.Direct {
			node.Direct = true
			node.RootRequested = req.Version
			node.RootComponents = append([]string{}, req.Components...)
			node.RootZephyrModule = req.ZephyrModule
		}
	}
	node.Requested = uniqueSorted(node.Requested)
	node.Components = uniqueSorted(node.Components)
	node.RootComponents = uniqueSorted(node.RootComponents)
	node.Parents = uniqueSorted(node.Parents)

	switch typ {
	case "git":
		if r.git == "" {
			return nil, fmt.Errorf("git is required to resolve dependency %q", name)
		}
		resolved, commit, err := r.resolveGit(name, uri, reqs)
		if err != nil {
			return nil, err
		}
		node.Resolved, node.Commit = resolved, strings.ToLower(commit)
		entry, err := r.gitManifest(uri, node.Resolved, node.Commit)
		if err != nil {
			return nil, fmt.Errorf("%s zap-package.yml: %w", name, err)
		}
		node.Manifest, node.ManifestText, node.ManifestSHA256 = entry.manifest, entry.text, entry.digest
	case "path":
		node.SourceRoot = pathRoot
		if node.SourceRoot == "" || node.SourceRoot == "." {
			node.SourceRoot = filepath.Clean(uri)
		}
		// Direct path dependencies retain the exact zap.yml URI so lock/config
		// compatibility is stable. Transitive path dependencies are stored
		// relative to the application root when possible, so committed lockfiles
		// do not accidentally capture a developer's absolute filesystem path.
		directURI := ""
		for _, req := range reqs {
			if req.Direct {
				directURI = req.URI
				break
			}
		}
		if directURI != "" {
			node.URI = directURI
		} else {
			node.URI = portableLocalPath(r.project.Root, node.SourceRoot)
		}
		if _, err := os.Stat(node.SourceRoot); err != nil {
			return nil, fmt.Errorf("local dependency %q was not found at %s", name, node.SourceRoot)
		}
		entry, err := localManifest(node.SourceRoot)
		if err != nil {
			return nil, fmt.Errorf("%s zap-package.yml: %w", name, err)
		}
		node.Manifest, node.ManifestText, node.ManifestSHA256 = entry.manifest, entry.text, entry.digest
	case "url":
		for _, req := range reqs {
			if req.Parent != "root" {
				return nil, fmt.Errorf("dependency %q: transitive URL dependencies are not supported", name)
			}
		}
		d := r.config.Dependencies[name]
		if d == nil || !sha256RE.MatchString(d.Hash) {
			return nil, fmt.Errorf("dependency %q requires a SHA256 hash", name)
		}
		node.Hash = d.Hash
	default:
		return nil, fmt.Errorf("dependency %q has unsupported type %q", name, typ)
	}
	return node, nil
}

func (r *dependencyResolver) resolveGit(name, uri string, reqs []dependencyRequirement) (string, string, error) {
	var specs []versionSpec
	legacyCommit := ""
	for _, req := range reqs {
		spec, err := parseVersionSpec(req.Version)
		if err != nil {
			return "", "", fmt.Errorf("dependency %q requirement from %s: %w", name, req.Parent, err)
		}
		specs = append(specs, spec)
		if req.LegacyCommit != "" {
			if legacyCommit != "" && !strings.EqualFold(legacyCommit, req.LegacyCommit) {
				return "", "", dependencyConflict(name, reqs, "legacy zap.yml commit locks disagree")
			}
			legacyCommit = req.LegacyCommit
		}
	}

	update := r.opts.UpdateAll || r.opts.Update[name]
	if !update && r.existing != nil {
		if locked := r.existing.Dependencies[name]; locked != nil && locked.Type == "git" && locked.URI == uri && lockedGitSatisfies(locked, specs) {
			return locked.Resolved, locked.Commit, nil
		}
	}
	if legacyCommit != "" && !update {
		return reqs[0].Version, legacyCommit, nil
	}

	var semSpecs []versionSpec
	var fixed []versionSpec
	for _, spec := range specs {
		if spec.kind == "semver" {
			semSpecs = append(semSpecs, spec)
		} else {
			fixed = append(fixed, spec)
		}
	}
	if len(fixed) > 0 {
		var commit string
		resolved := ""
		for i, spec := range fixed {
			candidate := ""
			label := spec.value
			switch spec.kind {
			case "commit":
				candidate = strings.ToLower(spec.value)
				label = "commit:" + candidate
			case "tag", "ref":
				var err error
				candidate, err = resolveRemoteRef(r.git, uri, spec.value)
				if err != nil {
					return "", "", fmt.Errorf("dependency %q requirement %q could not be resolved: %w", name, spec.raw, err)
				}
				if spec.kind == "tag" {
					label = spec.value
				}
			}
			if i == 0 {
				commit, resolved = candidate, label
			} else if !strings.EqualFold(commit, candidate) {
				return "", "", dependencyConflict(name, reqs, "fixed Git refs resolve to different commits")
			}
		}
		if len(semSpecs) > 0 {
			v, ok := parseSemver(resolved)
			if !ok {
				return "", "", dependencyConflict(name, reqs, "a non-version Git ref cannot be combined with semantic-version constraints")
			}
			for _, spec := range semSpecs {
				if !spec.matchesVersion(v) {
					return "", "", dependencyConflict(name, reqs, fmt.Sprintf("fixed version %s does not satisfy %s", resolved, spec.raw))
				}
			}
		}
		return resolved, commit, nil
	}

	tags, err := listStableTags(r.git, uri)
	if err != nil {
		return "", "", fmt.Errorf("dependency %q could not list releases: %w", name, err)
	}
	for _, tag := range tags {
		if versionSatisfiesAll(tag, semSpecs) {
			commit, err := resolveRemoteRef(r.git, uri, tag)
			if err != nil {
				return "", "", err
			}
			return tag, commit, nil
		}
	}
	return "", "", dependencyConflict(name, reqs, "no stable release satisfies all version constraints")
}

func lockedGitSatisfies(locked *LockedDependency, specs []versionSpec) bool {
	if locked == nil || !gitCommitRE.MatchString(locked.Commit) {
		return false
	}
	for _, spec := range specs {
		switch spec.kind {
		case "semver":
			v, ok := parseSemver(locked.Resolved)
			if !ok || !spec.matchesVersion(v) {
				return false
			}
		case "commit":
			if !strings.EqualFold(locked.Commit, spec.value) {
				return false
			}
		case "tag":
			if locked.Resolved != spec.value {
				return false
			}
		case "ref":
			if !containsString(locked.Requested, spec.raw) && locked.Resolved != spec.value {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (r *dependencyResolver) gitManifest(uri, resolved, commit string) (manifestCacheEntry, error) {
	key := "git\x00" + uri + "\x00" + strings.ToLower(commit)
	if got, ok := r.cache[key]; ok {
		return got, nil
	}
	dir, err := os.MkdirTemp("", "zap-resolve-")
	if err != nil {
		return manifestCacheEntry{}, err
	}
	defer os.RemoveAll(dir)
	if err := runStreaming("", r.git, "init", "-q", dir); err != nil {
		return manifestCacheEntry{}, err
	}
	if err := runStreaming("", r.git, "-C", dir, "remote", "add", "origin", uri); err != nil {
		return manifestCacheEntry{}, err
	}
	if err := fetchCommitForLock(r.git, "package", dir, uri, resolved, commit); err != nil {
		return manifestCacheEntry{}, err
	}
	text, err := showFileAt(r.git, dir, commit, "zap-package.yml")
	if err != nil {
		entry := manifestCacheEntry{}
		r.cache[key] = entry
		return entry, nil
	}
	manifest, err := ParsePackageManifest(text)
	if err != nil {
		return manifestCacheEntry{}, err
	}
	entry := manifestCacheEntry{manifest: manifest, text: text, digest: packageManifestDigest(text)}
	r.cache[key] = entry
	return entry, nil
}

func portableLocalPath(projectRoot, sourceRoot string) string {
	sourceRoot = filepath.Clean(sourceRoot)
	if projectRoot != "" {
		if rel, err := filepath.Rel(projectRoot, sourceRoot); err == nil {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(sourceRoot)
}

func localManifest(root string) (manifestCacheEntry, error) {
	path := filepath.Join(root, "zap-package.yml")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return manifestCacheEntry{}, nil
		}
		return manifestCacheEntry{}, err
	}
	text := string(b)
	m, err := ParsePackageManifest(text)
	if err != nil {
		return manifestCacheEntry{}, err
	}
	return manifestCacheEntry{manifest: m, text: text, digest: packageManifestDigest(text)}, nil
}

func packageManifestDigest(text string) string {
	canonical := normalize(strings.TrimSpace(text)) + "\n"
	sum := sha256.Sum256([]byte(canonical))
	return fmt.Sprintf("%x", sum[:])
}

func dependencyConflict(name string, reqs []dependencyRequirement, reason string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "dependency conflict: %s: %s", name, reason)
	ordered := append([]dependencyRequirement{}, reqs...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Parent != ordered[j].Parent {
			return ordered[i].Parent < ordered[j].Parent
		}
		return ordered[i].Version < ordered[j].Version
	})
	for _, req := range ordered {
		if req.Version != "" {
			fmt.Fprintf(&b, "\n  %s -> %s requires %s", req.Parent, name, req.Version)
		} else {
			fmt.Fprintf(&b, "\n  %s -> %s", req.Parent, name)
		}
	}
	return fmt.Errorf("%s", b.String())
}

func requirementSetsEqual(a, b map[string][]dependencyRequirement) bool {
	return requirementFingerprint(a) == requirementFingerprint(b)
}

func requirementFingerprint(reqs map[string][]dependencyRequirement) string {
	var rows []string
	for name, rr := range reqs {
		for _, r := range rr {
			rows = append(rows, strings.Join([]string{name, r.Parent, strings.ToLower(r.Type), r.URI, r.Version, strings.Join(uniqueSorted(r.Components), ","), fmt.Sprintf("%t", r.Direct), fmt.Sprintf("%t", r.ZephyrModule)}, "\x1f"))
		}
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n")
}

func buildLockfile(resolved map[string]*resolvedDependency) (*Lockfile, error) {
	graph := map[string][]string{}
	for name, node := range resolved {
		for _, parent := range node.Parents {
			if parent != "root" {
				graph[parent] = append(graph[parent], name)
			}
		}
	}
	order, err := dependencyOrderChildrenFirst(resolved, graph)
	if err != nil {
		return nil, err
	}
	lock := &Lockfile{Schema: LockSchema, Dependencies: map[string]*LockedDependency{}, DependencyOrder: order}
	for _, name := range order {
		node := resolved[name]
		lock.Dependencies[name] = &LockedDependency{
			Type:             node.Type,
			URI:              node.URI,
			Requested:        uniqueSorted(node.Requested),
			RootRequested:    node.RootRequested,
			Resolved:         node.Resolved,
			Commit:           strings.ToLower(node.Commit),
			Hash:             node.Hash,
			ManifestSHA256:   node.ManifestSHA256,
			Components:       uniqueSorted(node.Components),
			RootComponents:   uniqueSorted(node.RootComponents),
			Parents:          uniqueSorted(node.Parents),
			Direct:           node.Direct,
			ZephyrModule:     node.ZephyrModule,
			RootZephyrModule: node.RootZephyrModule,
		}
	}
	if err := validateLock(lock); err != nil {
		return nil, err
	}
	return lock, nil
}

func dependencyOrderChildrenFirst(nodes map[string]*resolvedDependency, graph map[string][]string) ([]string, error) {
	state := map[string]int{}
	var order []string
	var visit func(string, []string) error
	visit = func(name string, stack []string) error {
		switch state[name] {
		case 2:
			return nil
		case 1:
			return fmt.Errorf("package dependency cycle: %s -> %s", strings.Join(stack, " -> "), name)
		}
		state[name] = 1
		children := uniqueSorted(graph[name])
		for _, child := range children {
			if nodes[child] == nil {
				continue
			}
			if err := visit(child, append(stack, name)); err != nil {
				return err
			}
		}
		state[name] = 2
		order = append(order, name)
		return nil
	}
	var roots []string
	for name := range nodes {
		roots = append(roots, name)
	}
	sort.Strings(roots)
	for _, name := range roots {
		if err := visit(name, nil); err != nil {
			return nil, err
		}
	}
	return order, nil
}

func lockCompatibleWithConfig(lock *Lockfile, c *Config) bool {
	if lock == nil || c == nil {
		return false
	}
	lock.normalize()
	c.normalize()
	directCount := 0
	for _, dep := range lock.Dependencies {
		if dep != nil && dep.Direct {
			directCount++
		}
	}
	if directCount != len(c.DependencyOrder) {
		return false
	}
	for _, name := range c.DependencyOrder {
		d := c.Dependencies[name]
		locked := lock.Dependencies[name]
		if d == nil || locked == nil || !locked.Direct {
			return false
		}
		if strings.ToLower(d.Type) != strings.ToLower(locked.Type) {
			return false
		}
		if d.URI != locked.URI || d.Version != locked.RootRequested || !equalStringSet(d.Components, locked.RootComponents) || d.ZephyrModule != locked.RootZephyrModule {
			return false
		}
		if strings.ToLower(d.Type) == "url" && d.Hash != locked.Hash {
			return false
		}
		if d.Commit != "" && !strings.EqualFold(d.Commit, locked.Commit) {
			return false
		}
	}
	return true
}

func lockContainsMutablePath(lock *Lockfile) bool {
	if lock == nil {
		return false
	}
	for _, dep := range lock.Dependencies {
		if dep != nil && strings.EqualFold(dep.Type, "path") {
			return true
		}
	}
	return false
}
