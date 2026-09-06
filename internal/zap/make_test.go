// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMakeGenericProject(t *testing.T) {
	if _, err := findProgram("cmake"); err != nil {
		t.Skip("cmake not installed")
	}
	root := t.TempDir()
	cmake := `cmake_minimum_required(VERSION 3.20)
project(Demo C)
if(NOT DEMO_VALUE STREQUAL "from-env")
  message(FATAL_ERROR "DEMO_VALUE was not supplied by zap make")
endif()
add_executable(app main.c)
`
	if err := os.WriteFile(filepath.Join(root, "CMakeLists.txt"), []byte(cmake), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.c"), []byte("int main(void){return 0;}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &Config{
		Schema:       ConfigSchema,
		Project:      ProjectConfig{Environment: "generic", Target: "app", BuildDir: "build", DepsDir: "deps", AdaptersDir: "adapters"},
		Build:        BuildConfig{Configuration: "Release", CMake: map[string]string{"DEMO_VALUE": "env:ZAP_TEST_DEMO_VALUE"}, CMakeOrder: []string{"DEMO_VALUE"}},
		Dependencies: map[string]*DependencyConfig{},
	}
	p := OpenProject(root)
	if err := p.WriteConfig(c); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZAP_TEST_DEMO_VALUE", "from-env")
	if err := p.Make(c, MakeOptions{Clean: true, NoSync: true, Defines: map[string]string{"ZAP_VERIFY_DEPENDENCIES": "OFF"}}); err != nil {
		t.Fatal(err)
	}
	exe := "app"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	candidates := []string{
		filepath.Join(root, "build", exe),
		filepath.Join(root, "build", c.Build.Configuration, exe),
	}
	found := false
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("built executable missing; checked %v", candidates)
	}
}

func TestSafeRemoveBuildDirRefusesProjectRoot(t *testing.T) {
	root := t.TempDir()
	if err := safeRemoveBuildDir(root, root); err == nil {
		t.Fatal("expected project-root clean refusal")
	}
}
