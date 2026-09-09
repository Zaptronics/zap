// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTransitivePathDependenciesConfigureAndBuildWithCMake(t *testing.T) {
	cmake, err := exec.LookPath("cmake")
	if err != nil {
		t.Skip("cmake not installed")
	}
	if _, err := exec.LookPath("ninja"); err != nil {
		t.Skip("ninja not installed")
	}

	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	aDir := filepath.Join(root, "a")
	bDir := filepath.Join(root, "b")
	for _, dir := range []string{appDir, aDir, bDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	writeTestFile := func(path, text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeTestFile(filepath.Join(bDir, "CMakeLists.txt"), `cmake_minimum_required(VERSION 3.20)
project(dep_b LANGUAGES C)
add_library(dep_b INTERFACE)
add_library(demo::b ALIAS dep_b)
target_compile_definitions(dep_b INTERFACE B_VALUE=42)
`)
	writeTestFile(filepath.Join(bDir, "zap-package.yml"), `schema: 2
package:
  name: dep_b
components:
  demo::b:
    kind: library
`)

	writeTestFile(filepath.Join(aDir, "CMakeLists.txt"), `cmake_minimum_required(VERSION 3.20)
project(dep_a LANGUAGES C)
add_library(dep_a INTERFACE)
add_library(demo::a ALIAS dep_a)
target_link_libraries(dep_a INTERFACE demo::b)
`)
	writeTestFile(filepath.Join(aDir, "zap-package.yml"), `schema: 2
package:
  name: dep_a
dependencies:
  b:
    type: path
    uri: ../b
    components:
      - demo::b
components:
  demo::a:
    kind: library
`)

	writeTestFile(filepath.Join(appDir, "CMakeLists.txt"), `cmake_minimum_required(VERSION 3.20)
project(app LANGUAGES C)
add_executable(app main.c)
`)
	writeTestFile(filepath.Join(appDir, "main.c"), `#ifndef B_VALUE
#error B_VALUE missing through transitive dependency
#endif
int main(void) { return B_VALUE == 42 ? 0 : 1; }
`)

	p := OpenProject(appDir)
	cfg := &Config{
		Schema: ConfigSchema,
		Project: ProjectConfig{
			Environment: "generic",
			Target:      "app",
			BuildDir:    "build",
			DepsDir:     "deps",
			AdaptersDir: "adapters",
		},
		Build: BuildConfig{Configuration: "Release", Generator: "Ninja"},
		Dependencies: map[string]*DependencyConfig{
			"a": {Type: "path", URI: "../a", Components: []string{"demo::a"}},
		},
		DependencyOrder: []string{"a"},
	}
	if err := p.WriteConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := p.Sync(cfg); err != nil {
		t.Fatal(err)
	}
	if err := p.Generate(cfg); err != nil {
		t.Fatal(err)
	}

	lock, err := p.ReadLock()
	if err != nil {
		t.Fatal(err)
	}
	if got := lock.DependencyOrder; len(got) != 2 || got[0] != "b" || got[1] != "a" {
		t.Fatalf("dependency order = %#v, want child before parent", got)
	}
	if got := lock.Dependencies["b"].URI; got != "../b" {
		t.Fatalf("transitive path lock URI = %q, want project-relative ../b", got)
	}

	buildDir := filepath.Join(appDir, "build")
	runTestCommand := func(args ...string) {
		t.Helper()
		cmd := exec.Command(cmake, args...)
		cmd.Dir = appDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("cmake %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	runTestCommand("-S", appDir, "-B", buildDir, "-G", "Ninja", "-DZAP_VERIFY_DEPENDENCIES=OFF")
	runTestCommand("--build", buildDir)

	exe := filepath.Join(buildDir, "app")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	cmd := exec.Command(exe)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("built integration executable failed: %v\n%s", err, out)
	}
}
