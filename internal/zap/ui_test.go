// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"os"
	"testing"
)

func TestANSIEnvironmentOverrides(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("ZAP_COLOR", "always")
	if !ansiEnabled(os.Stdout) {
		t.Fatal("ZAP_COLOR=always should enable ANSI")
	}

	t.Setenv("ZAP_COLOR", "never")
	if ansiEnabled(os.Stdout) {
		t.Fatal("ZAP_COLOR=never should disable ANSI")
	}
}

func TestNoColorWins(t *testing.T) {
	t.Setenv("ZAP_COLOR", "always")
	t.Setenv("NO_COLOR", "1")
	if ansiEnabled(os.Stdout) {
		t.Fatal("NO_COLOR should disable ANSI even when ZAP_COLOR=always")
	}
}

func TestDisplayWidth(t *testing.T) {
	tests := []struct {
		text string
		want int
	}{
		{"ZAP", 3},
		{"⚡", 2},
		{"⚡ ZAP", 6},
		{"café", 4},
	}
	for _, tc := range tests {
		if got := displayWidth(tc.text); got != tc.want {
			t.Errorf("displayWidth(%q) = %d, want %d", tc.text, got, tc.want)
		}
	}
}

func TestTruncateToDisplayWidth(t *testing.T) {
	if got := truncateToDisplayWidth("⚡ ZAP", 5); got != "⚡ ZA" {
		t.Fatalf("truncateToDisplayWidth = %q, want %q", got, "⚡ ZA")
	}
}
