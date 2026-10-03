// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// fileLinkURL escapes spaces, #, %, and other URL metacharacters in filesystem
// paths. Handle drive letters and UNC paths as well as POSIX absolute paths.
func fileLinkURL(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	u := url.URL{Scheme: "file"}
	if strings.HasPrefix(path, "//") {
		parts := strings.SplitN(strings.TrimPrefix(path, "//"), "/", 2)
		u.Host = parts[0]
		if len(parts) == 2 {
			u.Path = "/" + parts[1]
		} else {
			u.Path = "/"
		}
	} else {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		u.Path = path
	}
	return u.String()
}

func uiFileLink(label, path string) {
	absolute, err := filepath.Abs(path)
	if err == nil {
		path = absolute
	}
	target := fileLinkURL(path)
	console := uiConsole(os.Stdout)
	if ansiEnabled(console) && enableANSI(console) {
		// OSC 8 makes the entire label clickable, independent of terminal path guessing.
		link := "\x1b]8;;" + target + "\x1b\\" + auditText(filepath.Base(path)) + "\x1b]8;;\x1b\\"
		uiDetail(label, link)
		uiDetail("Report path", auditText(path))
	} else {
		// A URI contains no unescaped spaces even in terminals without OSC 8 support.
		uiDetail(label, target)
	}
}
