// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type MakeOptions struct {
	Configuration string
	BuildDir      string
	Board         string
	Generator     string
	Defines       map[string]string
	Clean         bool
	ConfigureOnly bool
	Offline       bool
	NoSync        bool
}

func MakeFlagSet() (*flag.FlagSet, *MakeOptions, *multiFlag) {
	o := &MakeOptions{Defines: map[string]string{}}
	var defines multiFlag
	fs := flag.NewFlagSet("make", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&o.Configuration, "configuration", "", "Debug, Release, RelWithDebInfo, or MinSizeRel")
	fs.StringVar(&o.BuildDir, "build-dir", "", "override project build directory")
	fs.StringVar(&o.Board, "board", "", "override project board")
	fs.StringVar(&o.Generator, "generator", "", "override CMake generator")
	fs.Var(&defines, "define", "CMake cache value NAME=VALUE; may be repeated")
	fs.BoolVar(&o.Clean, "clean", false, "remove the build directory before configuring")
	fs.BoolVar(&o.ConfigureOnly, "configure-only", false, "configure but do not compile")
	fs.BoolVar(&o.Offline, "offline", false, "do not access dependency remotes; verify local locked copies only")
	fs.BoolVar(&o.NoSync, "no-sync", false, "do not change dependency checkouts before building")
	return fs, o, &defines
}

func parseDefines(values []string) (map[string]string, error) {
	out := map[string]string{}
	for _, item := range values {
		i := strings.Index(item, "=")
		if i <= 0 {
			return nil, fmt.Errorf("--define requires NAME=VALUE, got %q", item)
		}
		name := strings.TrimSpace(item[:i])
		value := item[i+1:]
		if !cmakeVarRE.MatchString(name) {
			return nil, fmt.Errorf("--define name %q is not a valid CMake cache variable", name)
		}
		if strings.ContainsAny(value, "\r\n\x00") {
			return nil, fmt.Errorf("--define value for %q contains invalid control characters", name)
		}
		out[name] = value
	}
	return out, nil
}

func (p *Project) buildPath(c *Config, override string) string {
	build := override
	if build == "" {
		build = c.Project.BuildDir
	}
	if build == "" {
		build = "build"
	}
	if filepath.IsAbs(build) {
		return filepath.Clean(build)
	}
	return filepath.Clean(filepath.Join(p.Root, build))
}

func safeRemoveBuildDir(projectRoot, buildPath string) error {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return err
	}
	build, err := filepath.Abs(buildPath)
	if err != nil {
		return err
	}
	root = filepath.Clean(root)
	build = filepath.Clean(build)
	if build == root {
		return fmt.Errorf("refusing to clean project root %s", root)
	}
	vol := filepath.VolumeName(build)
	if build == filepath.Clean(vol+string(os.PathSeparator)) {
		return fmt.Errorf("refusing to clean filesystem root %s", build)
	}
	if _, err := os.Stat(build); os.IsNotExist(err) {
		return nil
	}
	fmt.Printf("Removing %s\n", build)
	return os.RemoveAll(build)
}

func (p *Project) Make(c *Config, o MakeOptions) error {
	if o.Configuration == "" {
		o.Configuration = c.Build.Configuration
	}
	if o.Configuration == "" {
		o.Configuration = "Release"
	}
	switch o.Configuration {
	case "Debug", "Release", "RelWithDebInfo", "MinSizeRel":
	default:
		return fmt.Errorf("invalid configuration %q; use Debug, Release, RelWithDebInfo, or MinSizeRel", o.Configuration)
	}

	if o.Offline {
		if err := p.Verify(c, VerifyOptions{Offline: true}); err != nil {
			return err
		}
	} else if !o.NoSync {
		if err := p.Sync(c); err != nil {
			return err
		}
	} else {
		if err := p.Verify(c, VerifyOptions{}); err != nil {
			return err
		}
	}
	if err := p.Generate(c); err != nil {
		return err
	}
	if !o.Offline && !o.NoSync {
		if err := p.Verify(c, VerifyOptions{}); err != nil {
			return err
		}
	}

	buildPath := p.buildPath(c, o.BuildDir)
	if o.Clean {
		if err := safeRemoveBuildDir(p.Root, buildPath); err != nil {
			return err
		}
	}

	if err := p.ConfigureWithOptions(c, ConfigureOptions{
		Configuration: o.Configuration,
		BuildDir:      o.BuildDir,
		Board:         o.Board,
		Generator:     o.Generator,
		CMake:         o.Defines,
		Offline:       o.Offline,
	}); err != nil {
		return err
	}
	if o.ConfigureOnly {
		fmt.Println("Configuration complete; compilation was skipped.")
		return nil
	}

	fmt.Printf("Building %s\n", c.Project.Target)
	switch strings.ToLower(c.Project.Environment) {
	case "zephyr":
		west, err := findProgram("west")
		if err != nil {
			return err
		}
		if err := runStreamingEnv("", zapExecutableEnv(o.Offline), west, "build", "-d", buildPath); err != nil {
			return err
		}
	default:
		var cmake string
		var err error
		if strings.ToLower(c.Project.Environment) == "pico-sdk" {
			cmake, _ = findCMakeForPico()
			if cmake == "" {
				err = fmt.Errorf("cmake was not found")
			}
		} else {
			cmake, err = findProgram("cmake")
		}
		if err != nil {
			return err
		}
		args := []string{"--build", buildPath, "--parallel"}
		if o.Configuration != "" {
			args = append(args, "--config", o.Configuration)
		}
		if err := runStreamingEnv("", zapExecutableEnv(o.Offline), cmake, args...); err != nil {
			return err
		}
	}

	return p.reportBuildOutputs(c, buildPath)
}

func zapExecutableEnv(offline bool) []string {
	var env []string
	if exe, err := os.Executable(); err == nil && exe != "" {
		env = append(env, "ZAP_EXECUTABLE="+exe)
	}
	if offline {
		env = append(env, "ZAP_OFFLINE=1")
	}
	return env
}

func (p *Project) reportBuildOutputs(c *Config, buildPath string) error {
	fmt.Println()
	fmt.Println("Build complete.")
	if strings.ToLower(c.Project.Environment) != "pico-sdk" {
		fmt.Printf("  Build directory: %s\n", buildPath)
		return nil
	}
	target := c.Project.Target
	uf2 := filepath.Join(buildPath, target+".uf2")
	elf := filepath.Join(buildPath, target+".elf")
	if _, err := os.Stat(uf2); err != nil {
		return fmt.Errorf("build completed but %s was not produced", uf2)
	}
	fmt.Printf("  UF2: %s\n", uf2)
	if _, err := os.Stat(elf); err == nil {
		fmt.Printf("  ELF: %s\n", elf)
	}
	return nil
}
