// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

type ConfigureOptions struct {
	Configuration string
	BuildDir      string
	Board         string
	Generator     string
	CMake         map[string]string
	Offline       bool
}

func newestMatching(root string, executableParts ...string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	var candidates []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		parts := append([]string{root, e.Name()}, executableParts...)
		p := filepath.Join(parts...)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			candidates = append(candidates, p)
		}
	}
	sort.Strings(candidates)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[len(candidates)-1]
}

func picoSDKRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".pico-sdk")
}

func findCMakeForPico() (string, string) {
	if p, err := exec.LookPath("cmake"); err == nil {
		return p, "PATH"
	}
	root := picoSDKRoot()
	exe := "cmake"
	if runtime.GOOS == "windows" {
		exe = "cmake.exe"
	}
	p := newestMatching(filepath.Join(root, "cmake"), "bin", exe)
	if p != "" {
		return p, "Pico VS Code tools"
	}
	return "", ""
}

func findNinjaForPico() string {
	if p, err := exec.LookPath("ninja"); err == nil {
		return p
	}
	root := picoSDKRoot()
	exe := "ninja"
	if runtime.GOOS == "windows" {
		exe = "ninja.exe"
	}
	return newestMatching(filepath.Join(root, "ninja"), exe)
}

func newestDir(root string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	var aa []string
	for _, e := range entries {
		if e.IsDir() {
			aa = append(aa, filepath.Join(root, e.Name()))
		}
	}
	sort.Strings(aa)
	if len(aa) == 0 {
		return ""
	}
	return aa[len(aa)-1]
}

func cmakeSetValue(path, name string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(`(?m)^\s*set\(\s*` + regexp.QuoteMeta(name) + `\s+"?([^"\s\)]+)`)
	m := re.FindStringSubmatch(string(b))
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func exactOrNewestDir(root, version string) string {
	if version != "" {
		p := filepath.Join(root, version)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return newestDir(root)
}

func resolveBuildCache(c *Config, overrides map[string]string) (map[string]string, []string, error) {
	values := map[string]string{}
	var order []string
	seen := map[string]bool{}
	add := func(name, spec string) error {
		if !cmakeVarRE.MatchString(name) {
			return fmt.Errorf("CMake cache variable %q is invalid", name)
		}
		value := spec
		if strings.HasPrefix(spec, "env:") {
			envName := strings.TrimPrefix(spec, "env:")
			if !cmakeVarRE.MatchString(envName) {
				return fmt.Errorf("CMake cache variable %q references invalid environment variable %q", name, envName)
			}
			v, ok := os.LookupEnv(envName)
			if !ok || v == "" {
				return nil // Optional by default, matching the old build.ps1 behaviour.
			}
			value = v
		} else if strings.HasPrefix(spec, "literal:") {
			value = strings.TrimPrefix(spec, "literal:")
		}
		if strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("CMake cache value for %q contains invalid control characters", name)
		}
		if !seen[name] {
			order = append(order, name)
			seen[name] = true
		}
		values[name] = value
		return nil
	}
	for _, name := range c.Build.CMakeOrder {
		if err := add(name, c.Build.CMake[name]); err != nil {
			return nil, nil, err
		}
	}
	var extra []string
	for name := range overrides {
		extra = append(extra, name)
	}
	sort.Strings(extra)
	for _, name := range extra {
		if err := add(name, overrides[name]); err != nil {
			return nil, nil, err
		}
	}
	return values, order, nil
}

func (p *Project) Configure(c *Config) error {
	return p.ConfigureWithOptions(c, ConfigureOptions{})
}

func (p *Project) ConfigureWithOptions(c *Config, o ConfigureOptions) error {
	env := strings.ToLower(c.Project.Environment)
	build := o.BuildDir
	if build == "" {
		build = c.Project.BuildDir
	}
	if build == "" {
		build = "build"
	}
	buildPath := build
	if !filepath.IsAbs(buildPath) {
		buildPath = filepath.Join(p.Root, buildPath)
	}
	configuration := o.Configuration
	if configuration == "" {
		configuration = c.Build.Configuration
	}
	if configuration == "" {
		configuration = "Release"
	}
	board := o.Board
	if board == "" {
		board = c.Project.Board
	}
	generator := o.Generator
	if generator == "" {
		generator = c.Build.Generator
	}
	cache, cacheOrder, err := resolveBuildCache(c, o.CMake)
	if err != nil {
		return err
	}
	var zapEnv []string
	if exe, err := os.Executable(); err == nil && exe != "" {
		zapEnv = append(zapEnv, "ZAP_EXECUTABLE="+exe)
	}
	if o.Offline {
		zapEnv = append(zapEnv, "ZAP_OFFLINE=1")
	}

	appendCommon := func(args []string) []string {
		args = append(args, "-DCMAKE_EXPORT_COMPILE_COMMANDS=ON", "-DCMAKE_BUILD_TYPE="+configuration)
		if o.Offline {
			args = append(args, "-DZAP_OFFLINE=ON")
		}
		for _, name := range cacheOrder {
			args = append(args, "-D"+name+"="+cache[name])
		}
		return args
	}

	switch env {
	case "pico-sdk":
		cmake, source := findCMakeForPico()
		if cmake == "" {
			return fmt.Errorf("cmake was not found on PATH or under ~/.pico-sdk; install CMake/Pico VS Code tools or run 'zap sync --no-configure'")
		}
		root := picoSDKRoot()
		sdkVersion := cmakeSetValue(p.CMakePath, "sdkVersion")
		toolchainVersion := cmakeSetValue(p.CMakePath, "toolchainVersion")
		picotoolVersion := cmakeSetValue(p.CMakePath, "picotoolVersion")
		extra := append([]string{}, zapEnv...)
		sdkPath := os.Getenv("PICO_SDK_PATH")
		if sdkPath == "" {
			sdkPath = exactOrNewestDir(filepath.Join(root, "sdk"), sdkVersion)
			if sdkPath != "" {
				extra = append(extra, "PICO_SDK_PATH="+sdkPath)
			}
		}
		toolchainPath := os.Getenv("PICO_TOOLCHAIN_PATH")
		if toolchainPath == "" {
			toolchainPath = exactOrNewestDir(filepath.Join(root, "toolchain"), toolchainVersion)
			if toolchainPath != "" {
				extra = append(extra, "PICO_TOOLCHAIN_PATH="+toolchainPath)
			}
		}
		if sdkPath == "" || toolchainPath == "" {
			return fmt.Errorf("Pico SDK/toolchain could not be resolved from environment or ~/.pico-sdk")
		}
		args := []string{"-S", p.Root, "-B", buildPath}
		args = appendCommon(args)
		if board != "" {
			args = append(args, "-DPICO_BOARD="+board)
		}
		ninja := ""
		if generator == "" || strings.EqualFold(generator, "Ninja") {
			ninja = findNinjaForPico()
			if ninja != "" {
				generator = "Ninja"
			}
		}
		if generator != "" {
			args = append(args, "-G", generator)
		}
		if strings.EqualFold(generator, "Ninja") && ninja != "" {
			args = append(args, "-DCMAKE_MAKE_PROGRAM="+ninja)
		}
		if picotoolVersion != "" {
			picotoolDir := filepath.Join(root, "picotool", picotoolVersion, "picotool")
			if st, err := os.Stat(picotoolDir); err == nil && st.IsDir() {
				args = append(args, "-Dpicotool_DIR="+picotoolDir)
			}
		}
		fmt.Printf("Configuring %s (%s, %s)\n", c.Project.Target, configuration, board)
		fmt.Printf("  SDK      : %s\n", sdkPath)
		fmt.Printf("  Toolchain: %s\n", toolchainPath)
		fmt.Printf("  CMake    : %s (%s)\n", cmake, source)
		if ninja != "" {
			fmt.Printf("  Ninja    : %s\n", ninja)
		}
		for _, name := range cacheOrder {
			if spec := c.Build.CMake[name]; strings.HasPrefix(spec, "env:") {
				fmt.Printf("  CMake    : %s <- %s\n", name, spec)
			}
		}
		return runStreamingEnv("", extra, cmake, args...)

	case "generic", "cmake":
		cmake, err := findProgram("cmake")
		if err != nil {
			return err
		}
		args := appendCommon([]string{"-S", p.Root, "-B", buildPath})
		if generator != "" {
			args = append(args, "-G", generator)
		}
		return runStreamingEnv("", zapEnv, cmake, args...)

	case "zephyr":
		west, err := findProgram("west")
		if err != nil {
			return err
		}
		if board == "" {
			return fmt.Errorf("Zephyr projects require project.board in zap.yml")
		}
		args := []string{"build", "-b", board, "-d", buildPath, "--cmake-only", p.Root}
		var modulePaths []string
		for _, name := range c.DependencyOrder {
			d := c.Dependencies[name]
			if d != nil && d.ZephyrModule {
				modulePaths = append(modulePaths, p.dependencyPath(c, name, d))
			}
		}
		var ca []string
		if len(modulePaths) > 0 {
			ca = append(ca, "-DZEPHYR_MODULES="+strings.Join(modulePaths, ";"))
		}
		if _, err := os.Stat(p.GeneratedZephyr); err == nil {
			ca = append(ca, "-DEXTRA_CONF_FILE="+p.GeneratedZephyr)
		}
		if configuration != "" {
			ca = append(ca, "-DCMAKE_BUILD_TYPE="+configuration)
		}
		if o.Offline {
			ca = append(ca, "-DZAP_OFFLINE=ON")
		}
		for _, name := range cacheOrder {
			ca = append(ca, "-D"+name+"="+cache[name])
		}
		if len(ca) > 0 {
			args = append(args, "--")
			args = append(args, ca...)
		}
		return runStreamingEnv("", zapEnv, west, args...)
	default:
		return fmt.Errorf("unsupported project.environment %q", c.Project.Environment)
	}
}

func IsInteractive() bool {
	st, err := os.Stdin.Stat()
	return err == nil && (st.Mode()&os.ModeCharDevice) != 0
}
func commandExists(name string) bool { _, err := exec.LookPath(name); return err == nil }
func platformBinaryName() string {
	if runtime.GOOS == "windows" {
		return "zap.exe"
	}
	return "zap"
}
