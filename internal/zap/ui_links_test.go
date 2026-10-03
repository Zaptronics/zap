// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileLinkURL(t *testing.T) {
	for path, want := range map[string]string{
		"V:\\Firmware\\Hub - Copy\\audit-report.json":      "file:///V:/Firmware/Hub%20-%20Copy/audit-report.json",
		"/tmp/Hub - Copy/audit-report.json":                "file:///tmp/Hub%20-%20Copy/audit-report.json",
		"\\\\server\\share\\Hub - Copy\\audit-report.json": "file://server/share/Hub%20-%20Copy/audit-report.json",
		"/tmp/a#b%20?.json":                                "file:///tmp/a%23b%2520%3F.json",
	} {
		if got := fileLinkURL(path); got != want {
			t.Errorf("%q: got %q want %q", path, got, want)
		}
	}
}

func TestHistoryRemovesSplitHyperlinkControls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stdout.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	h := &operationHistory{}
	w := &historyOutput{h: h, file: f}
	for _, part := range []string{"Full report \x1b]8;;file:///tmp/Hub%20", "-%20Copy/audit-report.json\x1b", "\\audit-report.json\x1b]8;;\x1b", "\\\nReport path /tmp/Hub - Copy/audit-report.json\n"} {
		w.Write([]byte(part))
	}
	w.close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "\x1b") || !strings.Contains(text, "Full report audit-report.json") || !strings.Contains(text, "Hub - Copy/audit-report.json") {
		t.Fatalf("bad recorded link: %q", text)
	}
}
