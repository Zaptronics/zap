// SPDX-License-Identifier: Apache-2.0
package zap

import "sort"

const ConfigSchema = 5
const PackageSchema = 2
const LockSchema = 1

type Config struct {
	Schema          int
	Project         ProjectConfig
	Build           BuildConfig
	Upload          UploadConfig
	Dependencies    map[string]*DependencyConfig
	DependencyOrder []string
}

type ProjectConfig struct {
	Environment string
	Target      string
	Board       string
	BuildDir    string
	DepsDir     string
	AdaptersDir string
}

type BuildConfig struct {
	Configuration string
	Generator     string
	CMake         map[string]string
	CMakeOrder    []string
}

type UploadConfig struct {
	Default     string
	Order       []string
	Methods     map[string]*UploadMethodConfig
	MethodOrder []string
}

type UploadMethodConfig struct {
	Type              string
	Artifact          string
	Executable        string
	WorkingDir        string
	VolumeLabels      []string
	Search            []string
	Args              []string
	Variables         map[string]string
	VariableOrder     []string
	Find              map[string]*UploadFindConfig
	FindOrder         []string
	OptionalArgs      map[string][]string
	OptionalArgsOrder []string
}

type UploadFindConfig struct {
	Root   string
	File   string
	Parent int
}

type DependencyConfig struct {
	Type         string
	URI          string
	Version      string
	Commit       string
	Hash         string
	OverrideVar  string
	ZephyrModule bool
	Components   []string
	ZephyrConfig []string
}

func (c *Config) normalize() {
	if c.Schema == 0 {
		c.Schema = ConfigSchema
	}
	if c.Project.Environment == "" {
		c.Project.Environment = "generic"
	}
	if c.Project.BuildDir == "" {
		c.Project.BuildDir = "build"
	}
	if c.Project.DepsDir == "" {
		c.Project.DepsDir = "deps"
	}
	if c.Project.AdaptersDir == "" {
		c.Project.AdaptersDir = "adapters"
	}
	if c.Build.Configuration == "" {
		c.Build.Configuration = "Release"
	}
	if c.Build.CMake == nil {
		c.Build.CMake = map[string]string{}
	}
	if c.Upload.Methods == nil {
		c.Upload.Methods = map[string]*UploadMethodConfig{}
	}
	if len(c.Upload.MethodOrder) == 0 && len(c.Upload.Methods) > 0 {
		for name := range c.Upload.Methods {
			c.Upload.MethodOrder = append(c.Upload.MethodOrder, name)
		}
		sort.Strings(c.Upload.MethodOrder)
	}
	for _, m := range c.Upload.Methods {
		if m == nil {
			continue
		}
		if m.Variables == nil {
			m.Variables = map[string]string{}
		}
		if m.Find == nil {
			m.Find = map[string]*UploadFindConfig{}
		}
		if m.OptionalArgs == nil {
			m.OptionalArgs = map[string][]string{}
		}
	}
	if len(c.Build.CMakeOrder) == 0 && len(c.Build.CMake) > 0 {
		for name := range c.Build.CMake {
			c.Build.CMakeOrder = append(c.Build.CMakeOrder, name)
		}
		sort.Strings(c.Build.CMakeOrder)
	}
	if c.Dependencies == nil {
		c.Dependencies = map[string]*DependencyConfig{}
	}
	if len(c.DependencyOrder) == 0 && len(c.Dependencies) > 0 {
		for name := range c.Dependencies {
			c.DependencyOrder = append(c.DependencyOrder, name)
		}
		sort.Strings(c.DependencyOrder)
	}
	for _, dep := range c.Dependencies {
		if dep.Type == "" {
			dep.Type = "git"
		}
	}
}

type PackageManifest struct {
	Schema          int
	Package         PackageInfo
	Dependencies    map[string]*PackageDependency
	DependencyOrder []string
	Components      map[string]*PackageComponent
	ComponentOrder  []string
}

type PackageDependency struct {
	Type         string
	URI          string
	Version      string
	Components   []string
	ZephyrModule bool
}

type PackageInfo struct {
	Name         string
	Description  string
	CMakeProject string
	Paths        []string
}

type PackageComponent struct {
	Kind         string
	Description  string
	PicoTarget   string
	ZephyrTarget string
	Depends      []string
	Paths        []string
}

func (p *PackageManifest) normalize() {
	if p.Schema == 0 {
		p.Schema = PackageSchema
	}
	if p.Dependencies == nil {
		p.Dependencies = map[string]*PackageDependency{}
	}
	if len(p.DependencyOrder) == 0 && len(p.Dependencies) > 0 {
		for name := range p.Dependencies {
			p.DependencyOrder = append(p.DependencyOrder, name)
		}
		sort.Strings(p.DependencyOrder)
	}
	for _, dep := range p.Dependencies {
		if dep != nil && dep.Type == "" {
			dep.Type = "git"
		}
	}
	if p.Components == nil {
		p.Components = map[string]*PackageComponent{}
	}
}

type Lockfile struct {
	Schema          int
	Dependencies    map[string]*LockedDependency
	DependencyOrder []string
}

type LockedDependency struct {
	Type             string
	URI              string
	Requested        []string
	RootRequested    string
	Resolved         string
	Commit           string
	Hash             string
	ManifestSHA256   string
	Components       []string
	RootComponents   []string
	Parents          []string
	Direct           bool
	ZephyrModule     bool
	RootZephyrModule bool
}

func (l *Lockfile) normalize() {
	if l.Schema == 0 {
		l.Schema = LockSchema
	}
	if l.Dependencies == nil {
		l.Dependencies = map[string]*LockedDependency{}
	}
	if len(l.DependencyOrder) == 0 && len(l.Dependencies) > 0 {
		for name := range l.Dependencies {
			l.DependencyOrder = append(l.DependencyOrder, name)
		}
		sort.Strings(l.DependencyOrder)
	}
}
