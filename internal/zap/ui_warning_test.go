// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bytes"
	"strings"
	"testing"
)

func TestIntegrityWarningPresentation(t *testing.T) {
	for _, color := range []bool{false, true} {
		var b bytes.Buffer
		writeIntegrityWarning(&b, color, "after make", "integrity unverified: 2 source file changes")
		text := b.String()
		for _, want := range []string{"ZAP INTEGRITY WARNING", "after make", "2 source file changes", "Action: run zap audit"} {
			if !strings.Contains(text, want) {
				t.Fatalf("missing %q in %q", want, text)
			}
		}
		if strings.Contains(text, ansiYellow) != color {
			t.Fatal("incorrect colour mode")
		}
		if strings.Count(text, "\n") < 4 {
			t.Fatal("warning should stand apart from build output")
		}
	}
}
