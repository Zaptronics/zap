// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExternalSourceAuditFindsBuildDependencies(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "CMakeLists.txt"), []byte(`project(Demo)
FetchContent_Declare(foo GIT_REPOSITORY https://github.com/example/foo.git)
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cmake"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cmake", "vendor.cmake"), []byte(`set(REPO git@example.com:team/private.git)
`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := scanExternalSources(root, externalSource{File: "zap.yml (proposed)", URI: "https://github.com/Zaptronics/zap-ee.git"})
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 3 {
		t.Fatalf("expected 3 sources, got %#v", s)
	}
}
