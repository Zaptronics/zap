// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type externalSource struct {
	File string
	URI  string
}

var externalURIRe = regexp.MustCompile(`(?:https?://[^\s"'<>\)\]]+|ssh://[^\s"'<>\)\]]+|git@[A-Za-z0-9._-]+:[^\s"'<>\)\]]+)`)
var dynamicFetchRe = regexp.MustCompile(`(?i)\b(GIT_REPOSITORY|SVN_REPOSITORY|HG_REPOSITORY|URL)\b\s+([^#\r\n]+)`)

func scanExternalSources(root string, proposed ...externalSource) ([]externalSource, error) {
	files := map[string]bool{}
	addFile := func(path string) {
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			files[path] = true
		}
	}
	addFile(filepath.Join(root, "zap.yml"))
	addFile(filepath.Join(root, "CMakeLists.txt"))
	addFile(filepath.Join(root, "west.yml"))
	addFile(filepath.Join(root, "west.yaml"))
	addFile(filepath.Join(root, "zephyr", "module.yml"))

	cmakeDir := filepath.Join(root, "cmake")
	if err := filepath.WalkDir(cmakeDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".cmake") && filepath.Base(path) != "zap_deps.cmake" {
			files[path] = true
		}
		return nil
	}); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("scan cmake directory: %w", err)
	}

	seen := map[string]bool{}
	var out []externalSource
	for path := range files {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("scan external sources: open %s: %w", path, err)
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			foundLiteral := false
			for _, uri := range externalURIRe.FindAllString(line, -1) {
				foundLiteral = true
				uri = strings.TrimRight(uri, ",;.")
				rel, _ := filepath.Rel(root, path)
				key := rel + "\x00" + uri
				if !seen[key] {
					seen[key] = true
					out = append(out, externalSource{File: filepath.ToSlash(rel), URI: uri})
				}
			}
			if !foundLiteral {
				if m := dynamicFetchRe.FindStringSubmatch(line); m != nil {
					rel, _ := filepath.Rel(root, path)
					value := strings.TrimSpace(m[2])
					entry := "<dynamic " + strings.ToUpper(m[1]) + ": " + value + ">"
					key := rel + "\x00" + entry
					if !seen[key] {
						seen[key] = true
						out = append(out, externalSource{File: filepath.ToSlash(rel), URI: entry})
					}
				}
			}
		}
		if err := scanner.Err(); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("scan external sources: read %s: %w", path, err)
		}
		if err := f.Close(); err != nil {
			return nil, fmt.Errorf("scan external sources: close %s: %w", path, err)
		}
	}
	out = append(out, proposed...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].URI < out[j].URI
	})
	return out, nil
}

func confirmExternalSources(r *bufio.Reader, sources []externalSource, accept bool, nonInteractive bool) error {
	if len(sources) == 0 {
		return nil
	}
	uiSection("External sources")
	for _, source := range sources {
		uiDetail(source.File, source.URI)
	}
	uiWarning("Review external sources carefully; URLs are not trusted merely because they appear in a build file")
	if accept {
		uiSuccess("External sources accepted by --accept-sources")
		return nil
	}
	if nonInteractive || !IsInteractive() {
		return fmt.Errorf("external dependency sources require confirmation; review the list and rerun with --accept-sources for non-interactive setup")
	}
	ans, err := prompt(r, "Are all of these external sources expected and trusted? (yes/no)", "no")
	if err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(ans)) != "yes" && strings.ToLower(strings.TrimSpace(ans)) != "y" {
		return fmt.Errorf("zap init cancelled; external sources were not confirmed")
	}
	return nil
}
