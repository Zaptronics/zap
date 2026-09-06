// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

func scalar(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if s[0] == '"' && s[len(s)-1] == '"' {
			if v, err := strconv.Unquote(s); err == nil {
				return v
			}
		}
		if s[0] == '\'' && s[len(s)-1] == '\'' {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func splitKV(text string) (string, string, bool) {
	i := strings.Index(text, ":")
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(text[:i]), scalar(text[i+1:]), true
}

func boolScalar(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "1" || s == "true" || s == "yes" || s == "on" || s == "y"
}

func ParseConfig(text string) (*Config, error) {
	cfg := &Config{Schema: ConfigSchema, Dependencies: map[string]*DependencyConfig{}, Upload: UploadConfig{Methods: map[string]*UploadMethodConfig{}}}
	section := ""
	currentDep := ""
	currentList := ""
	currentMethod := ""
	currentFind := ""
	currentOptional := ""
	s := bufio.NewScanner(strings.NewReader(text))
	lineNo := 0
	for s.Scan() {
		lineNo++
		raw := strings.TrimSuffix(s.Text(), "\r")
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		if indent%2 != 0 {
			return nil, fmt.Errorf("zap.yml line %d uses odd indentation; use two spaces per level", lineNo)
		}
		text := strings.TrimSpace(raw)
		if indent == 0 {
			currentDep, currentList, currentMethod, currentFind, currentOptional = "", "", "", "", ""
			switch text {
			case "project:":
				section = "project"
				continue
			case "build:":
				section = "build"
				continue
			case "upload:":
				section = "upload"
				continue
			case "dependencies:":
				section = "dependencies"
				continue
			}
			k, v, ok := splitKV(text)
			if !ok {
				return nil, fmt.Errorf("invalid zap.yml line %d: %s", lineNo, text)
			}
			if k != "schema" {
				return nil, fmt.Errorf("unknown top-level zap.yml key %q on line %d", k, lineNo)
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("invalid schema on line %d", lineNo)
			}
			cfg.Schema = n
			continue
		}

		if section == "project" && indent == 2 {
			k, v, ok := splitKV(text)
			if !ok {
				return nil, fmt.Errorf("invalid project setting on line %d", lineNo)
			}
			switch k {
			case "environment":
				cfg.Project.Environment = v
			case "target":
				cfg.Project.Target = v
			case "board":
				cfg.Project.Board = v
			case "build_dir":
				cfg.Project.BuildDir = v
			case "deps_dir":
				cfg.Project.DepsDir = v
			case "adapters_dir":
				cfg.Project.AdaptersDir = v
			default:
				return nil, fmt.Errorf("unknown project key %q on line %d", k, lineNo)
			}
			continue
		}

		if section == "build" {
			if indent == 2 {
				k, v, ok := splitKV(text)
				if !ok {
					return nil, fmt.Errorf("invalid build setting on line %d", lineNo)
				}
				switch k {
				case "configuration":
					cfg.Build.Configuration = v
					currentList = ""
				case "generator":
					cfg.Build.Generator = v
					currentList = ""
				case "cmake":
					if v != "" {
						return nil, fmt.Errorf("zap.yml line %d: build.cmake must be a mapping", lineNo)
					}
					currentList = "build.cmake"
				default:
					return nil, fmt.Errorf("unknown build key %q on line %d", k, lineNo)
				}
				continue
			}
			if indent == 4 && currentList == "build.cmake" {
				k, v, ok := splitKV(text)
				if !ok || k == "" {
					return nil, fmt.Errorf("invalid build.cmake setting on line %d", lineNo)
				}
				if cfg.Build.CMake == nil {
					cfg.Build.CMake = map[string]string{}
				}
				if _, exists := cfg.Build.CMake[k]; !exists {
					cfg.Build.CMakeOrder = append(cfg.Build.CMakeOrder, k)
				}
				cfg.Build.CMake[k] = v
				continue
			}
		}

		if section == "upload" {
			if indent == 2 {
				k, v, ok := splitKV(text)
				if !ok {
					return nil, fmt.Errorf("invalid upload setting on line %d", lineNo)
				}
				switch k {
				case "default":
					cfg.Upload.Default = v
					currentList = ""
				case "order":
					if v != "" {
						return nil, fmt.Errorf("zap.yml line %d: upload.order must be a list", lineNo)
					}
					currentList = "upload.order"
				case "methods":
					if v != "" {
						return nil, fmt.Errorf("zap.yml line %d: upload.methods must be a mapping", lineNo)
					}
					currentList = "upload.methods"
				default:
					return nil, fmt.Errorf("unknown upload key %q on line %d", k, lineNo)
				}
				continue
			}
			if indent == 4 && currentList == "upload.order" && strings.HasPrefix(text, "- ") {
				cfg.Upload.Order = append(cfg.Upload.Order, scalar(strings.TrimSpace(strings.TrimPrefix(text, "- "))))
				continue
			}
			if indent == 4 && strings.HasSuffix(text, ":") {
				name := strings.TrimSpace(strings.TrimSuffix(text, ":"))
				if name == "" {
					return nil, fmt.Errorf("empty upload method name on line %d", lineNo)
				}
				if cfg.Upload.Methods == nil {
					cfg.Upload.Methods = map[string]*UploadMethodConfig{}
				}
				m := &UploadMethodConfig{Variables: map[string]string{}, Find: map[string]*UploadFindConfig{}, OptionalArgs: map[string][]string{}}
				cfg.Upload.Methods[name] = m
				cfg.Upload.MethodOrder = append(cfg.Upload.MethodOrder, name)
				currentMethod, currentList, currentFind, currentOptional = name, "", "", ""
				continue
			}
			if currentMethod == "" {
				return nil, fmt.Errorf("upload method setting on line %d appears before a method name", lineNo)
			}
			m := cfg.Upload.Methods[currentMethod]
			if indent == 6 {
				k, v, ok := splitKV(text)
				if !ok {
					return nil, fmt.Errorf("invalid upload method setting on line %d", lineNo)
				}
				switch k {
				case "type":
					m.Type = v
					currentList = ""
				case "artifact":
					m.Artifact = v
					currentList = ""
				case "executable":
					m.Executable = v
					currentList = ""
				case "working_dir":
					m.WorkingDir = v
					currentList = ""
				case "volume_labels", "search", "args":
					if v != "" {
						return nil, fmt.Errorf("zap.yml line %d: upload method %s must be a list", lineNo, k)
					}
					currentList = "upload.method." + k
				case "variables", "find", "optional_args":
					if v != "" {
						return nil, fmt.Errorf("zap.yml line %d: upload method %s must be a mapping", lineNo, k)
					}
					currentList = "upload.method." + k
				default:
					return nil, fmt.Errorf("unknown upload method key %q on line %d", k, lineNo)
				}
				currentFind, currentOptional = "", ""
				continue
			}
			if indent == 8 {
				if strings.HasPrefix(text, "- ") {
					item := scalar(strings.TrimSpace(strings.TrimPrefix(text, "- ")))
					switch currentList {
					case "upload.method.volume_labels":
						m.VolumeLabels = append(m.VolumeLabels, item)
					case "upload.method.search":
						m.Search = append(m.Search, item)
					case "upload.method.args":
						m.Args = append(m.Args, item)
					default:
						return nil, fmt.Errorf("upload list item on line %d has no list key", lineNo)
					}
					continue
				}
				if currentList == "upload.method.variables" {
					k, v, ok := splitKV(text)
					if !ok || k == "" {
						return nil, fmt.Errorf("invalid upload variable on line %d", lineNo)
					}
					if _, exists := m.Variables[k]; !exists {
						m.VariableOrder = append(m.VariableOrder, k)
					}
					m.Variables[k] = v
					continue
				}
				if currentList == "upload.method.find" && strings.HasSuffix(text, ":") {
					name := strings.TrimSpace(strings.TrimSuffix(text, ":"))
					m.Find[name] = &UploadFindConfig{}
					m.FindOrder = append(m.FindOrder, name)
					currentFind = name
					continue
				}
				if currentList == "upload.method.optional_args" && strings.HasSuffix(text, ":") {
					name := strings.TrimSpace(strings.TrimSuffix(text, ":"))
					m.OptionalArgs[name] = nil
					m.OptionalArgsOrder = append(m.OptionalArgsOrder, name)
					currentOptional = name
					continue
				}
			}
			if indent == 10 && currentList == "upload.method.find" && currentFind != "" {
				k, v, ok := splitKV(text)
				if !ok {
					return nil, fmt.Errorf("invalid upload find setting on line %d", lineNo)
				}
				f := m.Find[currentFind]
				switch k {
				case "root":
					f.Root = v
				case "file":
					f.File = v
				case "parent":
					n, err := parseInt(v)
					if err != nil {
						return nil, fmt.Errorf("invalid upload find parent on line %d: %w", lineNo, err)
					}
					f.Parent = n
				default:
					return nil, fmt.Errorf("unknown upload find key %q on line %d", k, lineNo)
				}
				continue
			}
			if indent == 10 && currentList == "upload.method.optional_args" && currentOptional != "" && strings.HasPrefix(text, "- ") {
				m.OptionalArgs[currentOptional] = append(m.OptionalArgs[currentOptional], scalar(strings.TrimSpace(strings.TrimPrefix(text, "- "))))
				continue
			}
		}

		if section == "dependencies" {
			if indent == 2 && strings.HasSuffix(text, ":") {
				name := strings.TrimSpace(strings.TrimSuffix(text, ":"))
				if name == "" {
					return nil, fmt.Errorf("empty dependency name on line %d", lineNo)
				}
				dep := &DependencyConfig{Type: "git"}
				cfg.Dependencies[name] = dep
				cfg.DependencyOrder = append(cfg.DependencyOrder, name)
				currentDep, currentList = name, ""
				continue
			}
			if currentDep == "" {
				return nil, fmt.Errorf("dependency setting on line %d appears before a dependency name", lineNo)
			}
			dep := cfg.Dependencies[currentDep]
			if indent == 4 {
				k, v, ok := splitKV(text)
				if !ok {
					return nil, fmt.Errorf("invalid dependency setting on line %d", lineNo)
				}
				if k == "components" || k == "zephyr_config" {
					if v != "" {
						return nil, fmt.Errorf("zap.yml line %d: %q must be a YAML list", lineNo, k)
					}
					currentList = k
					continue
				}
				switch k {
				case "type":
					dep.Type = v
				case "uri":
					dep.URI = v
				case "version":
					dep.Version = v
				case "commit":
					dep.Commit = v
				case "hash":
					dep.Hash = v
				case "override_var":
					dep.OverrideVar = v
				case "zephyr_module":
					dep.ZephyrModule = boolScalar(v)
				default:
					return nil, fmt.Errorf("unknown dependency key %q on line %d", k, lineNo)
				}
				currentList = ""
				continue
			}
			if indent == 6 && strings.HasPrefix(text, "- ") {
				if currentList == "" {
					return nil, fmt.Errorf("list item on line %d has no list key", lineNo)
				}
				item := scalar(strings.TrimSpace(strings.TrimPrefix(text, "- ")))
				if currentList == "components" {
					dep.Components = append(dep.Components, item)
				} else {
					dep.ZephyrConfig = append(dep.ZephyrConfig, item)
				}
				continue
			}
		}
		return nil, fmt.Errorf("unsupported zap.yml structure on line %d: %s", lineNo, text)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if cfg.Schema < 1 || cfg.Schema > ConfigSchema {
		return nil, fmt.Errorf("unsupported zap.yml schema %d; this zap supports schemas 1 through %d", cfg.Schema, ConfigSchema)
	}
	cfg.normalize()
	if err := validateConfig(cfg, false); err != nil {
		return nil, err
	}
	return cfg, nil
}

func yamlQuote(s string) string {
	if s == "" {
		return ""
	}
	if strings.ContainsAny(s, "#{}[],&*!|>'\"%@`\t\r\n") || strings.HasPrefix(s, " ") || strings.HasSuffix(s, " ") {
		return strconv.Quote(s)
	}
	return s
}

func FormatConfig(cfg *Config) string {
	cfg.normalize()
	cfg.Schema = ConfigSchema
	var b strings.Builder
	b.WriteString("# Zap dependency manifest\n")
	fmt.Fprintf(&b, "schema: %d\n\n", cfg.Schema)
	b.WriteString("project:\n")
	fmt.Fprintf(&b, "  environment: %s\n", yamlQuote(cfg.Project.Environment))
	fmt.Fprintf(&b, "  target: %s\n", yamlQuote(cfg.Project.Target))
	if cfg.Project.Board != "" {
		fmt.Fprintf(&b, "  board: %s\n", yamlQuote(cfg.Project.Board))
	}
	fmt.Fprintf(&b, "  build_dir: %s\n", yamlQuote(cfg.Project.BuildDir))
	fmt.Fprintf(&b, "  deps_dir: %s\n", yamlQuote(cfg.Project.DepsDir))
	fmt.Fprintf(&b, "  adapters_dir: %s\n", yamlQuote(cfg.Project.AdaptersDir))
	b.WriteString("\nbuild:\n")
	fmt.Fprintf(&b, "  configuration: %s\n", yamlQuote(cfg.Build.Configuration))
	if cfg.Build.Generator != "" {
		fmt.Fprintf(&b, "  generator: %s\n", yamlQuote(cfg.Build.Generator))
	}
	if len(cfg.Build.CMakeOrder) > 0 {
		b.WriteString("  cmake:\n")
		for _, name := range cfg.Build.CMakeOrder {
			fmt.Fprintf(&b, "    %s: %s\n", name, yamlQuote(cfg.Build.CMake[name]))
		}
	}
	if len(cfg.Upload.MethodOrder) > 0 {
		b.WriteString("\nupload:\n")
		if cfg.Upload.Default != "" {
			fmt.Fprintf(&b, "  default: %s\n", yamlQuote(cfg.Upload.Default))
		}
		if len(cfg.Upload.Order) > 0 {
			b.WriteString("  order:\n")
			for _, name := range cfg.Upload.Order {
				fmt.Fprintf(&b, "    - %s\n", yamlQuote(name))
			}
		}
		b.WriteString("  methods:\n")
		for _, name := range cfg.Upload.MethodOrder {
			m := cfg.Upload.Methods[name]
			if m == nil {
				continue
			}
			fmt.Fprintf(&b, "    %s:\n", name)
			fmt.Fprintf(&b, "      type: %s\n", yamlQuote(m.Type))
			if m.Artifact != "" {
				fmt.Fprintf(&b, "      artifact: %s\n", yamlQuote(m.Artifact))
			}
			if m.Executable != "" {
				fmt.Fprintf(&b, "      executable: %s\n", yamlQuote(m.Executable))
			}
			if m.WorkingDir != "" {
				fmt.Fprintf(&b, "      working_dir: %s\n", yamlQuote(m.WorkingDir))
			}
			if len(m.VolumeLabels) > 0 {
				b.WriteString("      volume_labels:\n")
				for _, v := range m.VolumeLabels {
					fmt.Fprintf(&b, "        - %s\n", yamlQuote(v))
				}
			}
			if len(m.Search) > 0 {
				b.WriteString("      search:\n")
				for _, v := range m.Search {
					fmt.Fprintf(&b, "        - %s\n", yamlQuote(v))
				}
			}
			if len(m.VariableOrder) > 0 {
				b.WriteString("      variables:\n")
				for _, key := range m.VariableOrder {
					fmt.Fprintf(&b, "        %s: %s\n", key, yamlQuote(m.Variables[key]))
				}
			}
			if len(m.FindOrder) > 0 {
				b.WriteString("      find:\n")
				for _, key := range m.FindOrder {
					f := m.Find[key]
					if f == nil {
						continue
					}
					fmt.Fprintf(&b, "        %s:\n", key)
					fmt.Fprintf(&b, "          root: %s\n", yamlQuote(f.Root))
					fmt.Fprintf(&b, "          file: %s\n", yamlQuote(f.File))
					if f.Parent != 0 {
						fmt.Fprintf(&b, "          parent: %d\n", f.Parent)
					}
				}
			}
			if len(m.Args) > 0 {
				b.WriteString("      args:\n")
				for _, v := range m.Args {
					fmt.Fprintf(&b, "        - %s\n", yamlQuote(v))
				}
			}
			if len(m.OptionalArgsOrder) > 0 {
				b.WriteString("      optional_args:\n")
				for _, key := range m.OptionalArgsOrder {
					fmt.Fprintf(&b, "        %s:\n", key)
					for _, v := range m.OptionalArgs[key] {
						fmt.Fprintf(&b, "          - %s\n", yamlQuote(v))
					}
				}
			}
		}
	}
	b.WriteString("\ndependencies:\n")
	for _, name := range cfg.DependencyOrder {
		dep := cfg.Dependencies[name]
		if dep == nil {
			continue
		}
		fmt.Fprintf(&b, "  %s:\n", name)
		fmt.Fprintf(&b, "    type: %s\n", yamlQuote(dep.Type))
		if dep.URI != "" {
			fmt.Fprintf(&b, "    uri: %s\n", yamlQuote(dep.URI))
		}
		if dep.Version != "" {
			fmt.Fprintf(&b, "    version: %s\n", yamlQuote(dep.Version))
		}
		if dep.Commit != "" {
			fmt.Fprintf(&b, "    commit: %s\n", yamlQuote(dep.Commit))
		}
		if dep.Hash != "" {
			fmt.Fprintf(&b, "    hash: %s\n", yamlQuote(dep.Hash))
		}
		if dep.OverrideVar != "" {
			fmt.Fprintf(&b, "    override_var: %s\n", yamlQuote(dep.OverrideVar))
		}
		if dep.ZephyrModule {
			b.WriteString("    zephyr_module: true\n")
		}
		b.WriteString("    components:\n")
		for _, c := range dep.Components {
			fmt.Fprintf(&b, "      - %s\n", yamlQuote(c))
		}
		if len(dep.ZephyrConfig) > 0 {
			b.WriteString("    zephyr_config:\n")
			for _, c := range dep.ZephyrConfig {
				fmt.Fprintf(&b, "      - %s\n", yamlQuote(c))
			}
		}
	}
	return b.String()
}

func ParsePackageManifest(text string) (*PackageManifest, error) {
	m := &PackageManifest{Schema: PackageSchema, Components: map[string]*PackageComponent{}}
	section, currentComponent, currentList := "", "", ""
	s := bufio.NewScanner(strings.NewReader(text))
	lineNo := 0
	for s.Scan() {
		lineNo++
		raw := strings.TrimSuffix(s.Text(), "\r")
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		if indent%2 != 0 {
			return nil, fmt.Errorf("zap-package.yml line %d uses odd indentation", lineNo)
		}
		t := strings.TrimSpace(raw)
		if indent == 0 && t == "package:" {
			section, currentComponent, currentList = "package", "", ""
			continue
		}
		if indent == 0 && t == "components:" {
			section, currentComponent, currentList = "components", "", ""
			continue
		}
		if indent == 0 {
			k, v, ok := splitKV(t)
			if !ok || k != "schema" {
				return nil, fmt.Errorf("invalid top-level package manifest line %d", lineNo)
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("invalid package schema on line %d", lineNo)
			}
			m.Schema = n
			continue
		}
		if section == "package" {
			if indent == 2 {
				k, v, ok := splitKV(t)
				if !ok {
					return nil, fmt.Errorf("invalid package setting on line %d", lineNo)
				}
				if k == "paths" {
					if v != "" {
						return nil, fmt.Errorf("package.paths must be a list on line %d", lineNo)
					}
					currentList = "package.paths"
					continue
				}
				switch k {
				case "name":
					m.Package.Name = v
				case "description":
					m.Package.Description = v
				case "cmake_project":
					m.Package.CMakeProject = v
				default:
					return nil, fmt.Errorf("unknown package key %q on line %d", k, lineNo)
				}
				currentList = ""
				continue
			}
			if indent == 4 && strings.HasPrefix(t, "- ") && currentList == "package.paths" {
				m.Package.Paths = append(m.Package.Paths, scalar(strings.TrimSpace(strings.TrimPrefix(t, "- "))))
				continue
			}
		}
		if section == "components" {
			if indent == 2 && strings.HasSuffix(t, ":") {
				name := strings.TrimSpace(strings.TrimSuffix(t, ":"))
				if name == "" {
					return nil, fmt.Errorf("empty component name on line %d", lineNo)
				}
				m.Components[name] = &PackageComponent{}
				m.ComponentOrder = append(m.ComponentOrder, name)
				currentComponent = name
				currentList = ""
				continue
			}
			if currentComponent == "" {
				return nil, fmt.Errorf("component setting before component name on line %d", lineNo)
			}
			c := m.Components[currentComponent]
			if indent == 4 {
				k, v, ok := splitKV(t)
				if !ok {
					return nil, fmt.Errorf("invalid component setting on line %d", lineNo)
				}
				if k == "depends" || k == "paths" {
					if v != "" {
						return nil, fmt.Errorf("component.%s must be a list on line %d", k, lineNo)
					}
					currentList = k
					continue
				}
				switch k {
				case "kind":
					c.Kind = v
				case "description":
					c.Description = v
				case "pico":
					c.PicoTarget = v
				case "zephyr":
					c.ZephyrTarget = v
				default:
					return nil, fmt.Errorf("unknown component key %q on line %d", k, lineNo)
				}
				currentList = ""
				continue
			}
			if indent == 6 && strings.HasPrefix(t, "- ") {
				item := scalar(strings.TrimSpace(strings.TrimPrefix(t, "- ")))
				if currentList == "depends" {
					c.Depends = append(c.Depends, item)
					continue
				}
				if currentList == "paths" {
					c.Paths = append(c.Paths, item)
					continue
				}
				return nil, fmt.Errorf("component list item has no list key on line %d", lineNo)
			}
		}
		return nil, fmt.Errorf("unsupported zap-package.yml structure on line %d: %s", lineNo, t)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if m.Schema != PackageSchema {
		return nil, fmt.Errorf("unsupported zap-package.yml schema %d", m.Schema)
	}
	m.normalize()
	if err := validatePackageManifest(m); err != nil {
		return nil, err
	}
	return m, nil
}
