// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

type UploadOptions struct {
	Method        string
	Build         bool
	Clean         bool
	Configuration string
	BuildDir      string
	Artifact      string
	Offline       bool
}

func UploadFlagSet() (*flag.FlagSet, *UploadOptions) {
	o := &UploadOptions{}
	fs := flag.NewFlagSet("upload", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&o.Method, "method", "", "upload method name from zap.yml (default: upload.default)")
	fs.BoolVar(&o.Build, "build", false, "run 'zap make' before uploading")
	fs.BoolVar(&o.Clean, "clean", false, "with --build, clean the build directory first")
	fs.StringVar(&o.Configuration, "configuration", "", "with --build, override build configuration")
	fs.StringVar(&o.BuildDir, "build-dir", "", "override project build directory")
	fs.StringVar(&o.Artifact, "artifact", "", "override the selected method artifact path")
	fs.BoolVar(&o.Offline, "offline", false, "with --build, avoid dependency network checks")
	return fs, o
}

type uploadUnavailableError struct{ msg string }
type uploadFatalError struct{ msg string }

func (e *uploadUnavailableError) Error() string { return e.msg }
func (e *uploadFatalError) Error() string       { return e.msg }

func unavailablef(format string, args ...any) error {
	return &uploadUnavailableError{msg: fmt.Sprintf(format, args...)}
}
func fatalUploadf(format string, args ...any) error {
	return &uploadFatalError{msg: fmt.Sprintf(format, args...)}
}
func isUnavailable(err error) bool {
	_, ok := err.(*uploadUnavailableError)
	return ok
}
func isUploadFatal(err error) bool {
	_, ok := err.(*uploadFatalError)
	return ok
}

func (p *Project) Upload(c *Config, o UploadOptions) error {
	if len(c.Upload.Methods) == 0 {
		return fmt.Errorf("zap.yml has no upload methods configured; add an upload: section or run 'zap init' for a new project template")
	}
	if o.Build {
		if err := p.Make(c, MakeOptions{
			Configuration: o.Configuration,
			BuildDir:      o.BuildDir,
			Clean:         o.Clean,
			Offline:       o.Offline,
		}); err != nil {
			return err
		}
	}

	uiBanner("Upload", c.Project.Target)
	uiSection("Program")

	selected := strings.TrimSpace(o.Method)
	if selected == "" {
		selected = strings.TrimSpace(c.Upload.Default)
	}
	if selected == "" {
		if len(c.Upload.Order) > 0 {
			selected = "auto"
		} else if len(c.Upload.MethodOrder) == 1 {
			selected = c.Upload.MethodOrder[0]
		}
	}
	if strings.EqualFold(selected, "auto") {
		if len(c.Upload.Order) == 0 {
			return fmt.Errorf("upload.default is auto but upload.order is empty")
		}
		var failures []string
		for _, name := range c.Upload.Order {
			uiStep("Try", name)
			err := p.runUploadMethod(c, name, o)
			if err == nil {
				return nil
			}
			if isUploadFatal(err) {
				return fmt.Errorf("upload method %s stopped auto fallback for safety: %w", name, err)
			}
			failures = append(failures, fmt.Sprintf("%s: %v", name, err))
			if !isUnavailable(err) {
				uiWarning(fmt.Sprintf("%s failed: %v", name, err))
			} else {
				uiHint(fmt.Sprintf("%s unavailable: %v", name, err))
			}
		}
		return fmt.Errorf("no configured upload method succeeded:\n  %s", strings.Join(failures, "\n  "))
	}
	return p.runUploadMethod(c, selected, o)
}

func (p *Project) runUploadMethod(c *Config, name string, o UploadOptions) error {
	m := c.Upload.Methods[name]
	if m == nil {
		return fmt.Errorf("upload method %q is not defined in zap.yml", name)
	}
	buildPath := p.buildPath(c, o.BuildDir)
	vars := map[string]string{
		"project_root": p.Root,
		"build_dir":    buildPath,
		"target":       c.Project.Target,
		"board":        c.Project.Board,
		"pico_home":    picoSDKRoot(),
		"method":       name,
	}
	for _, key := range m.VariableOrder {
		spec := m.Variables[key]
		v, err := resolveUploadVariable(spec)
		if err != nil {
			return fmt.Errorf("upload method %q variable %q: %w", name, key, err)
		}
		vars[key] = v
	}
	artifactSpec := o.Artifact
	if artifactSpec == "" {
		artifactSpec = m.Artifact
	}
	if artifactSpec == "" {
		artifactSpec = "{build_dir}/{target}.uf2"
	}
	artifact, err := expandUploadString(artifactSpec, vars)
	if err != nil {
		return fmt.Errorf("upload method %q artifact: %w", name, err)
	}
	if !filepath.IsAbs(artifact) {
		artifact = filepath.Join(p.Root, artifact)
	}
	artifact = filepath.Clean(artifact)
	vars["artifact"] = artifact

	switch strings.ToLower(strings.TrimSpace(m.Type)) {
	case "volume-copy":
		if _, err := os.Stat(artifact); err != nil {
			return unavailablef("artifact %s was not found", artifact)
		}
		roots, err := findLabeledVolumes(m.VolumeLabels)
		if err != nil {
			return err
		}
		if len(roots) == 0 {
			return unavailablef("no mounted volume matched labels %s", strings.Join(m.VolumeLabels, ", "))
		}
		if len(roots) > 1 {
			return fatalUploadf("more than one matching upload volume is mounted: %s; disconnect the target you do not want to program", strings.Join(roots, ", "))
		}
		dst := filepath.Join(roots[0], filepath.Base(artifact))
		uiStep("Copy", filepath.Base(artifact))
		uiDetail("Destination", dst)
		if err := copyFile(artifact, dst); err != nil {
			return fatalUploadf("copy to selected programming volume failed: %v", err)
		}
		uiResult("UPLOAD COMPLETE", uiRow{Label: "Method", Value: name}, uiRow{Label: "Artifact", Value: artifact})
		uiHint("target should reboot automatically if the volume supports UF2 programming")
		return nil

	case "command":
		if _, err := os.Stat(artifact); err != nil {
			return unavailablef("artifact %s was not found", artifact)
		}
		program, err := resolveUploadExecutable(m, vars)
		if err != nil {
			return unavailablef("%v", err)
		}
		vars["tool"] = program
		vars["tool_dir"] = filepath.Dir(program)
		for _, key := range m.FindOrder {
			spec := m.Find[key]
			found, err := resolveUploadFind(spec, vars)
			if err != nil {
				return unavailablef("upload method %q find %q: %v", name, key, err)
			}
			vars[key] = found
		}
		args, err := expandUploadCommandArgs(m, vars)
		if err != nil {
			return fmt.Errorf("upload method %q arguments: %w", name, err)
		}
		wd := p.Root
		if m.WorkingDir != "" {
			wd, err = expandUploadString(m.WorkingDir, vars)
			if err != nil {
				return err
			}
			if !filepath.IsAbs(wd) {
				wd = filepath.Join(p.Root, wd)
			}
		}
		uiStep(name, program)
		if err := runStreamingEnv(wd, nil, program, args...); err != nil {
			return err
		}
		uiResult("UPLOAD COMPLETE", uiRow{Label: "Method", Value: name}, uiRow{Label: "Artifact", Value: artifact})
		return nil
	default:
		return fmt.Errorf("upload method %q has unsupported type %q", name, m.Type)
	}
}

func resolveUploadVariable(spec string) (string, error) {
	if strings.HasPrefix(spec, "env:") {
		name := strings.TrimPrefix(spec, "env:")
		if !cmakeVarRE.MatchString(name) {
			return "", fmt.Errorf("invalid environment variable name %q", name)
		}
		return os.Getenv(name), nil
	}
	if strings.HasPrefix(spec, "literal:") {
		return strings.TrimPrefix(spec, "literal:"), nil
	}
	if strings.ContainsAny(spec, "\r\n\x00") {
		return "", fmt.Errorf("value contains invalid control characters")
	}
	return spec, nil
}

func expandUploadArgs(in []string, vars map[string]string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, a := range in {
		x, err := expandUploadString(a, vars)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, nil
}

func expandUploadCommandArgs(m *UploadMethodConfig, vars map[string]string) ([]string, error) {
	var out []string
	used := map[string]bool{}
	for _, a := range m.Args {
		if strings.HasPrefix(a, "{?") && strings.HasSuffix(a, "}") {
			key := strings.TrimSuffix(strings.TrimPrefix(a, "{?"), "}")
			if !cmakeVarRE.MatchString(key) {
				return nil, fmt.Errorf("invalid optional argument placeholder %q", a)
			}
			group, ok := m.OptionalArgs[key]
			if !ok {
				return nil, fmt.Errorf("optional argument placeholder %q has no optional_args definition", key)
			}
			used[key] = true
			if vars[key] == "" {
				continue
			}
			more, err := expandUploadArgs(group, vars)
			if err != nil {
				return nil, err
			}
			out = append(out, more...)
			continue
		}
		x, err := expandUploadString(a, vars)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	for _, key := range m.OptionalArgsOrder {
		if used[key] || vars[key] == "" {
			continue
		}
		more, err := expandUploadArgs(m.OptionalArgs[key], vars)
		if err != nil {
			return nil, err
		}
		out = append(out, more...)
	}
	return out, nil
}

func expandUploadString(s string, vars map[string]string) (string, error) {
	if strings.ContainsAny(s, "\r\n\x00") {
		return "", fmt.Errorf("template contains invalid control characters")
	}
	var out strings.Builder
	for i := 0; i < len(s); {
		open := strings.IndexByte(s[i:], '{')
		if open < 0 {
			out.WriteString(s[i:])
			break
		}
		open += i
		out.WriteString(s[i:open])
		closeRel := strings.IndexByte(s[open+1:], '}')
		if closeRel < 0 {
			return "", fmt.Errorf("unclosed placeholder in %q", s)
		}
		close := open + 1 + closeRel
		key := s[open+1 : close]
		if !cmakeVarRE.MatchString(key) {
			return "", fmt.Errorf("invalid placeholder {%s}", key)
		}
		v, ok := vars[key]
		if !ok {
			return "", fmt.Errorf("unknown placeholder {%s}", key)
		}
		out.WriteString(v)
		i = close + 1
	}
	return out.String(), nil
}

func resolveUploadExecutable(m *UploadMethodConfig, vars map[string]string) (string, error) {
	exe, err := expandUploadString(m.Executable, vars)
	if err != nil {
		return "", err
	}
	exe = strings.TrimSpace(exe)
	if exe == "" {
		return "", fmt.Errorf("no executable configured")
	}
	if filepath.IsAbs(exe) || strings.ContainsAny(exe, `/\\`) {
		if !filepath.IsAbs(exe) {
			exe = filepath.Join(vars["project_root"], exe)
		}
		if st, err := os.Stat(exe); err == nil && !st.IsDir() {
			return filepath.Clean(exe), nil
		}
		return "", fmt.Errorf("configured executable %s was not found", exe)
	}
	search := m.Search
	if len(search) == 0 {
		search = []string{"PATH"}
	}
	for _, entry := range search {
		if strings.EqualFold(strings.TrimSpace(entry), "PATH") {
			if p, err := exec.LookPath(exe); err == nil {
				return p, nil
			}
			continue
		}
		root, err := expandUploadString(entry, vars)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(root) {
			root = filepath.Join(vars["project_root"], root)
		}
		if p := findExecutableRecursive(root, exe); p != "" {
			return p, nil
		}
	}
	return "", fmt.Errorf("executable %q was not found in configured search locations", exe)
}

func executableNames(name string) map[string]bool {
	out := map[string]bool{name: true}
	if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
		out[name+".exe"] = true
		out[name+".cmd"] = true
		out[name+".bat"] = true
	}
	return out
}

func findExecutableRecursive(root, name string) string {
	st, err := os.Stat(root)
	if err != nil {
		return ""
	}
	if !st.IsDir() {
		if executableNames(name)[filepath.Base(root)] {
			return root
		}
		return ""
	}
	names := executableNames(name)
	var found []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && names[d.Name()] {
			found = append(found, path)
		}
		return nil
	})
	sort.Strings(found)
	if len(found) == 0 {
		return ""
	}
	return found[len(found)-1]
}

func resolveUploadFind(spec *UploadFindConfig, vars map[string]string) (string, error) {
	if spec == nil || strings.TrimSpace(spec.Root) == "" || strings.TrimSpace(spec.File) == "" {
		return "", fmt.Errorf("root and file are required")
	}
	root, err := expandUploadString(spec.Root, vars)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(root) {
		root = filepath.Join(vars["project_root"], root)
	}
	var found []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == spec.File {
			found = append(found, path)
		}
		return nil
	})
	sort.Strings(found)
	if len(found) == 0 {
		return "", fmt.Errorf("%s was not found under %s", spec.File, root)
	}
	p := found[len(found)-1]
	for i := 0; i < spec.Parent; i++ {
		p = filepath.Dir(p)
	}
	return p, nil
}

func copyFile(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if dstInfo, err := os.Stat(dst); err == nil {
		if os.SameFile(srcInfo, dstInfo) {
			return nil
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(dst)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

func parseInt(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return 0, fmt.Errorf("expected non-negative integer, got %q", s)
	}
	return n, nil
}
