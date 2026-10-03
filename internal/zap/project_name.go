// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"net/url"
	"path/filepath"
	"strings"
)

func (p *Project) displayName(c *Config) string {
	if c != nil && strings.TrimSpace(c.Project.Name) != "" {
		return auditText(strings.TrimSpace(c.Project.Name))
	}
	return auditText(filepath.Base(p.Root))
}

// Read only local Git configuration; name suggestions do not contact the remote.
func (p *Project) suggestedName(git string) string {
	if git != "" {
		if remote, err := runCapture(p.Root, git, "config", "--local", "--get", "remote.origin.url"); err == nil {
			if name := repositoryName(remote); name != "" {
				return name
			}
		}
	}
	return filepath.Base(p.Root)
}

func repositoryName(remote string) string {
	remote = strings.TrimSpace(remote)
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil {
			return ""
		}
		remote = u.Path
	}
	remote = strings.TrimRight(remote, "/\\")
	i := strings.LastIndexAny(remote, "/\\:")
	name := strings.TrimSuffix(remote[i+1:], ".git")
	if name == "" || name == "." || name == ".." {
		return ""
	}
	// Remote configuration is untrusted; avoid control characters in the prompt.
	if auditText(name) != name {
		return ""
	}
	return name
}
