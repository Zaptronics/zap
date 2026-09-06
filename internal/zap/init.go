// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const DefaultZapEEURI = "https://github.com/Zaptronics/zap-ee.git"

type InitOptions struct {
	Environment    string
	Target         string
	Board          string
	DepsDir        string
	AdaptersDir    string
	ZapEEURI       string
	ZapEEVersion   string
	Components     multiFlag
	Force          bool
	AcceptSources  bool
	NonInteractive bool
}
type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func InitFlagSet() (*flag.FlagSet, *InitOptions) {
	o := &InitOptions{DepsDir: "deps", AdaptersDir: "adapters", ZapEEURI: DefaultZapEEURI}
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&o.Environment, "environment", "", "pico-sdk, zephyr, or generic")
	fs.StringVar(&o.Target, "target", "", "application CMake target")
	fs.StringVar(&o.Board, "board", "", "target board")
	fs.StringVar(&o.DepsDir, "deps-dir", "deps", "dependency source directory")
	fs.StringVar(&o.AdaptersDir, "adapters-dir", "adapters", "generated adapter directory")
	fs.StringVar(&o.ZapEEURI, "zapee-uri", DefaultZapEEURI, "ZapEE Git repository")
	fs.StringVar(&o.ZapEEVersion, "zapee-version", "", "ZapEE release/ref (defaults to latest stable)")
	fs.Var(&o.Components, "component", "component target to select; may be repeated")
	fs.BoolVar(&o.Force, "force", false, "overwrite existing zap.yml")
	fs.BoolVar(&o.AcceptSources, "accept-sources", false, "confirm detected external dependency sources (for non-interactive setup)")
	fs.BoolVar(&o.NonInteractive, "non-interactive", false, "do not prompt")
	return fs, o
}

func prompt(r *bufio.Reader, label, def string) (string, error) {
	if def != "" {
		fmt.Printf("%s [%s]: ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}
	s, err := r.ReadString('\n')
	if err != nil && len(s) == 0 {
		return "", err
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return def, nil
	}
	return s, nil
}
func promptChoice(r *bufio.Reader, label string, choices []string, def int) (string, error) {
	fmt.Println(label)
	for i, c := range choices {
		fmt.Printf("  %d. %s\n", i+1, c)
	}
	d := strconv.Itoa(def + 1)
	v, err := prompt(r, "Select", d)
	if err != nil {
		return "", err
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > len(choices) {
		return "", fmt.Errorf("invalid selection %q", v)
	}
	return choices[n-1], nil
}

func (p *Project) Init(opts InitOptions) error {
	_, configErr := os.Stat(p.ConfigPath)
	configExists := configErr == nil
	if configExists && !opts.Force {
		return fmt.Errorf("zap.yml already exists; zap init will not overwrite it. Use --force only if you intend to replace it")
	}
	cmakeBytes, err := os.ReadFile(p.CMakePath)
	if err != nil {
		return fmt.Errorf("zap init expects CMakeLists.txt in the project root")
	}
	cmake := string(cmakeBytes)
	interactive := IsInteractive() && !opts.NonInteractive
	r := bufio.NewReader(os.Stdin)
	env := opts.Environment
	if env == "" {
		env = InferEnvironment(cmake)
	}
	target := opts.Target
	if target == "" {
		target = InferTarget(cmake)
	}
	board := opts.Board
	if board == "" && env == "pico-sdk" {
		board = InferPicoBoard(cmake)
	}
	uri := opts.ZapEEURI
	git, gitErr := findProgram("git")
	if interactive {
		uiBanner("Project setup", filepath.Base(p.Root))
		var e error
		env, e = promptChoice(r, "Environment:", []string{"pico-sdk", "zephyr", "generic"}, indexOf([]string{"pico-sdk", "zephyr", "generic"}, env))
		if e != nil {
			return e
		}
		target, e = prompt(r, "Application CMake target", target)
		if e != nil {
			return e
		}
		if env == "pico-sdk" || env == "zephyr" {
			board, e = prompt(r, "Board", board)
			if e != nil {
				return e
			}
		}
		opts.DepsDir, e = prompt(r, "Dependency folder", opts.DepsDir)
		if e != nil {
			return e
		}
		uri, e = prompt(r, "ZapEE repository", uri)
		if e != nil {
			return e
		}
	}
	sources, err := scanExternalSources(p.Root, externalSource{File: "zap.yml (proposed)", URI: uri})
	if err != nil {
		return err
	}
	if err := confirmExternalSources(r, sources, opts.AcceptSources, opts.NonInteractive); err != nil {
		return err
	}
	uiSection("Planned changes")
	uiStep("CREATE", "zap.yml")
	uiStep("CREATE", "cmake/zap_deps.cmake")
	uiStep("CREATE", "cmake/zap_zephyr.conf")
	uiStep("EDIT", "CMakeLists.txt · only ZAP MANAGED blocks")
	uiHint("vendor build files remain unchanged")
	version := opts.ZapEEVersion
	if version == "" {
		if gitErr != nil {
			return fmt.Errorf("git is required during zap init to discover and immutably lock the ZapEE release; offline operation is supported after the dependency lock has been created")
		}
		tags, err := listStableTags(git, uri)
		if err != nil {
			return fmt.Errorf("could not query ZapEE releases: %w", err)
		}
		if len(tags) == 0 {
			return fmt.Errorf("no stable semantic-version tags were found at %s", uri)
		}
		version = tags[0]
		uiSuccess("Latest ZapEE release: " + version)
	}
	components := []string(opts.Components)
	var manifest *PackageManifest
	var lockedCommit string
	if gitErr == nil {
		manifest, lockedCommit, err = loadRemotePackageManifestLocked(git, uri, version)
		if err != nil {
			return err
		}
	}
	if len(components) == 0 {
		if gitErr != nil {
			return fmt.Errorf("git is required to inspect ZapEE components")
		}
		m := manifest
		mods := publicModules(m)
		if interactive && len(mods) > 0 {
			fmt.Println("\nSelect ZapEE modules (comma separated, blank for none):")
			for i, n := range mods {
				c := m.Components[n]
				fmt.Printf("  %d. %s", i+1, n)
				if c.Description != "" {
					fmt.Printf(" - %s", c.Description)
				}
				fmt.Println()
			}
			ans, err := prompt(r, "Modules", "")
			if err != nil {
				return err
			}
			if ans != "" {
				for _, part := range strings.Split(ans, ",") {
					idx, err := strconv.Atoi(strings.TrimSpace(part))
					if err != nil || idx < 1 || idx > len(mods) {
						return fmt.Errorf("invalid module selection %q", part)
					}
					base := mods[idx-1]
					components = append(components, componentForEnvironment(m, base, env))
				}
			}
		}
	}
	build := BuildConfig{Configuration: "Release"}
	if env == "pico-sdk" {
		build.Generator = "Ninja"
	}
	upload := UploadConfig{}
	if env == "pico-sdk" {
		upload = defaultPicoUploadConfig()
	}
	cfg := &Config{Schema: ConfigSchema, Project: ProjectConfig{Environment: env, Target: target, Board: board, BuildDir: "build", DepsDir: opts.DepsDir, AdaptersDir: opts.AdaptersDir}, Build: build, Upload: upload, Dependencies: map[string]*DependencyConfig{}, DependencyOrder: []string{"zapee"}}
	dep := &DependencyConfig{Type: "git", URI: uri, Version: version, Commit: lockedCommit, OverrideVar: "ZAPEE_SOURCE", ZephyrModule: true, Components: components}
	if env == "zephyr" {
		dep.ZephyrConfig = []string{"CONFIG_ZAPEE=y"}
		for _, c := range components {
			if strings.Contains(c, "ztm_pwm") {
				dep.ZephyrConfig = append(dep.ZephyrConfig, "CONFIG_ZAPEE_ZTM_PWM=y", "CONFIG_ZAPEE_ZTM_PWM_ZEPHYR_ADAPTER=y")
			}
			if strings.Contains(c, "ztm_adc") {
				dep.ZephyrConfig = append(dep.ZephyrConfig, "CONFIG_ZAPEE_ZTM_ADC=y", "CONFIG_ZAPEE_ZTM_ADC_ZEPHYR_ADAPTER=y")
			}
		}
	}
	if dep.Commit == "" {
		return fmt.Errorf("zap init requires Git access to resolve %s to an immutable commit; create the project while online, then builds may use offline verification", version)
	}
	cfg.Dependencies["zapee"] = dep
	if err := p.WriteConfig(cfg); err != nil {
		return err
	}
	if err := p.Generate(cfg); err != nil {
		return err
	}
	uiResult("PROJECT CREATED", uiRow{Label: "Manifest", Value: p.ConfigPath})
	uiHint("run 'zap sync' to fetch dependencies and configure the project")
	return nil
}

func indexOf(list []string, v string) int {
	for i, x := range list {
		if x == v {
			return i
		}
	}
	return 0
}
func publicModules(m *PackageManifest) []string {
	var r []string
	for _, n := range m.ComponentOrder {
		if c := m.Components[n]; c != nil && c.Kind == "module" {
			r = append(r, n)
		}
	}
	return r
}
func componentForEnvironment(m *PackageManifest, base, env string) string {
	c := m.Components[base]
	if c == nil {
		return base
	}
	switch env {
	case "pico-sdk":
		if c.PicoTarget != "" {
			return c.PicoTarget
		}
	case "zephyr":
		if c.ZephyrTarget != "" {
			return c.ZephyrTarget
		}
	}
	return base
}

func ensureDir(path string) error {
	if path == "" {
		return nil
	}
	return os.MkdirAll(filepath.Clean(path), 0o755)
}

func defaultPicoUploadConfig() UploadConfig {
	return UploadConfig{
		Default: "auto",
		Order:   []string{"bootsel", "picotool"},
		Methods: map[string]*UploadMethodConfig{
			"bootsel": {
				Type:         "volume-copy",
				Artifact:     "{build_dir}/{target}.uf2",
				VolumeLabels: []string{"RPI-RP2", "RP2350"},
			},
			"picotool": {
				Type:          "command",
				Artifact:      "{build_dir}/{target}.uf2",
				Executable:    "picotool",
				Search:        []string{"PATH", "{pico_home}/picotool"},
				Args:          []string{"load", "-f", "-v", "-x", "{artifact}"},
				Variables:     map[string]string{"serial": "env:ZAP_PICO_SERIAL"},
				VariableOrder: []string{"serial"},
				OptionalArgs: map[string][]string{
					"serial": {"--ser", "{serial}"},
				},
				OptionalArgsOrder: []string{"serial"},
			},
		},
		MethodOrder: []string{"bootsel", "picotool"},
	}
}
