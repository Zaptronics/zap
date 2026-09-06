//go:build !windows

// SPDX-License-Identifier: Apache-2.0
//
package zap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func findLabeledVolumes(labels []string) ([]string, error) {
	wanted := map[string]bool{}
	for _, l := range labels {
		if strings.TrimSpace(l) != "" {
			wanted[strings.TrimSpace(l)] = true
		}
	}
	if len(wanted) == 0 {
		return nil, fmt.Errorf("volume-copy method has no volume_labels")
	}
	user := os.Getenv("USER")
	roots := []string{"/Volumes", "/mnt"}
	if user != "" {
		roots = append(roots, filepath.Join("/media", user), filepath.Join("/run/media", user))
	}
	seen := map[string]bool{}
	var out []string
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || !wanted[e.Name()] {
				continue
			}
			p := filepath.Join(root, e.Name())
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out, nil
}
