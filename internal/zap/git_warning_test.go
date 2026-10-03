// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestGitSelectorWarning(t *testing.T) {
	names := []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES"}
	if mode := os.Getenv("ZAP_TEST_SELECTOR_WARNING"); mode != "" {
		env := []string{"PATH=ordinary-path"}
		if mode == "selectors" {
			for _, name := range names {
				env = append(env, name+"=PRIVATE_VALUE", strings.ToLower(name)+"=PRIVATE_VALUE")
			}
		}
		for i := 0; i < 2; i++ {
			filtered := sanitizedGitEnv(env)
			if strings.Contains(strings.Join(filtered, "\n"), "PRIVATE_VALUE") {
				t.Fatal("warning did not retain protection")
			}
		}
		return
	}
	for _, mode := range []string{"selectors", "clean"} {
		t.Run(mode, func(t *testing.T) {
			// A fresh process verifies per-invocation warning deduplication without
			// resetting production state or interfering with other tests.
			cmd := exec.Command(os.Args[0], "-test.run=^TestGitSelectorWarning$")
			cmd.Env = append(os.Environ(), "ZAP_TEST_SELECTOR_WARNING="+mode)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("warning fixture: %v\n%s\n%s", err, &stdout, &stderr)
			}
			warning := stderr.String()
			if mode == "clean" {
				if warning != "" {
					t.Fatalf("warned with no selectors: %s", warning)
				}
				return
			}
			if strings.Count(warning, "Warning:") != 1 {
				t.Fatalf("expected one warning: %s", warning)
			}
			for _, name := range names {
				if strings.Count(warning, name) != 1 {
					t.Errorf("expected %s once: %s", name, warning)
				}
			}
			if strings.Contains(warning, "PRIVATE_VALUE") {
				t.Error("warning disclosed variable values")
			}
			if strings.Contains(stdout.String(), "Warning:") {
				t.Error("warning polluted stdout")
			}
			if !strings.Contains(warning, "cannot be disabled") {
				t.Error("missing protection explanation")
			}
		})
	}
}
