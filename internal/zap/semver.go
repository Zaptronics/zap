// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type semVersion struct {
	major int
	minor int
	patch int
	raw   string
}

var semverRE = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)

func parseSemver(s string) (semVersion, bool) {
	m := semverRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return semVersion{}, false
	}
	a, err := strconv.Atoi(m[1])
	if err != nil {
		return semVersion{}, false
	}
	b, err := strconv.Atoi(m[2])
	if err != nil {
		return semVersion{}, false
	}
	c, err := strconv.Atoi(m[3])
	if err != nil {
		return semVersion{}, false
	}
	return semVersion{major: a, minor: b, patch: c, raw: strings.TrimSpace(s)}, true
}

func compareSemver(a, b semVersion) int {
	if a.major != b.major {
		if a.major < b.major {
			return -1
		}
		return 1
	}
	if a.minor != b.minor {
		if a.minor < b.minor {
			return -1
		}
		return 1
	}
	if a.patch != b.patch {
		if a.patch < b.patch {
			return -1
		}
		return 1
	}
	return 0
}

func semverString(v semVersion) string {
	return fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)
}

func incrementSemverPart(n int) (int, bool) {
	if n == int(^uint(0)>>1) {
		return 0, false
	}
	return n + 1, true
}

type versionConstraint struct {
	op      string
	version semVersion
}

type versionSpec struct {
	raw         string
	kind        string // semver, tag, ref, commit
	constraints []versionConstraint
	value       string
}

func parseVersionSpec(raw string) (versionSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return versionSpec{}, fmt.Errorf("version constraint is empty")
	}
	lower := strings.ToLower(raw)
	for _, prefix := range []string{"commit:", "tag:", "ref:"} {
		if strings.HasPrefix(lower, prefix) {
			value := strings.TrimSpace(raw[len(prefix):])
			if value == "" {
				return versionSpec{}, fmt.Errorf("%s requires a value", strings.TrimSuffix(prefix, ":"))
			}
			kind := strings.TrimSuffix(prefix, ":")
			if kind == "commit" {
				if !gitCommitRE.MatchString(value) {
					return versionSpec{}, fmt.Errorf("commit constraint requires a 40-character Git SHA")
				}
			} else if !validGitRef(value) {
				return versionSpec{}, fmt.Errorf("%s %q contains unsafe or invalid Git-ref characters", kind, value)
			}
			return versionSpec{raw: raw, kind: kind, value: value}, nil
		}
	}

	if strings.HasPrefix(raw, "^") {
		v, ok := parseSemver(strings.TrimSpace(raw[1:]))
		if !ok {
			return versionSpec{}, fmt.Errorf("invalid compatible-major version constraint %q", raw)
		}
		upper := semVersion{}
		switch {
		case v.major > 0:
			next, ok := incrementSemverPart(v.major)
			if !ok {
				return versionSpec{}, fmt.Errorf("compatible-major version constraint %q overflows supported version range", raw)
			}
			upper = semVersion{major: next}
		case v.minor > 0:
			next, ok := incrementSemverPart(v.minor)
			if !ok {
				return versionSpec{}, fmt.Errorf("compatible-major version constraint %q overflows supported version range", raw)
			}
			upper = semVersion{major: 0, minor: next}
		default:
			next, ok := incrementSemverPart(v.patch)
			if !ok {
				return versionSpec{}, fmt.Errorf("compatible-major version constraint %q overflows supported version range", raw)
			}
			upper = semVersion{major: 0, minor: 0, patch: next}
		}
		return versionSpec{raw: raw, kind: "semver", constraints: []versionConstraint{{op: ">=", version: v}, {op: "<", version: upper}}}, nil
	}
	if strings.HasPrefix(raw, "~") {
		v, ok := parseSemver(strings.TrimSpace(raw[1:]))
		if !ok {
			return versionSpec{}, fmt.Errorf("invalid compatible-minor version constraint %q", raw)
		}
		nextMinor, ok := incrementSemverPart(v.minor)
		if !ok {
			return versionSpec{}, fmt.Errorf("compatible-minor version constraint %q overflows supported version range", raw)
		}
		upper := semVersion{major: v.major, minor: nextMinor}
		return versionSpec{raw: raw, kind: "semver", constraints: []versionConstraint{{op: ">=", version: v}, {op: "<", version: upper}}}, nil
	}
	if v, ok := parseSemver(raw); ok {
		return versionSpec{raw: raw, kind: "semver", constraints: []versionConstraint{{op: "=", version: v}}}, nil
	}

	fields := strings.Fields(raw)
	if len(fields) > 0 {
		allComparisons := true
		var constraints []versionConstraint
		for _, field := range fields {
			op := ""
			value := ""
			for _, candidate := range []string{">=", "<=", ">", "<", "="} {
				if strings.HasPrefix(field, candidate) {
					op = candidate
					value = strings.TrimSpace(field[len(candidate):])
					break
				}
			}
			if op == "" {
				allComparisons = false
				break
			}
			v, ok := parseSemver(value)
			if !ok {
				return versionSpec{}, fmt.Errorf("invalid semantic version %q in constraint %q", value, raw)
			}
			constraints = append(constraints, versionConstraint{op: op, version: v})
		}
		if allComparisons && len(constraints) > 0 {
			return versionSpec{raw: raw, kind: "semver", constraints: constraints}, nil
		}
	}

	// Backward compatibility: legacy values such as "main" or "release/foo"
	// are Git refs. New manifests should prefer the explicit ref: prefix.
	if validGitRef(raw) {
		return versionSpec{raw: raw, kind: "ref", value: raw}, nil
	}
	return versionSpec{}, fmt.Errorf("unsupported version constraint %q", raw)
}

func (s versionSpec) matchesVersion(v semVersion) bool {
	if s.kind != "semver" {
		return false
	}
	for _, c := range s.constraints {
		cmp := compareSemver(v, c.version)
		switch c.op {
		case "=":
			if cmp != 0 {
				return false
			}
		case ">":
			if cmp <= 0 {
				return false
			}
		case ">=":
			if cmp < 0 {
				return false
			}
		case "<":
			if cmp >= 0 {
				return false
			}
		case "<=":
			if cmp > 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func versionSatisfiesAll(tag string, specs []versionSpec) bool {
	v, ok := parseSemver(tag)
	if !ok {
		return false
	}
	for _, spec := range specs {
		if spec.kind != "semver" || !spec.matchesVersion(v) {
			return false
		}
	}
	return true
}

func sortStableTags(tags []string) []string {
	vv := make([]semVersion, 0, len(tags))
	for _, tag := range tags {
		if v, ok := parseSemver(tag); ok {
			vv = append(vv, v)
		}
	}
	sort.Slice(vv, func(i, j int) bool {
		cmp := compareSemver(vv[i], vv[j])
		if cmp != 0 {
			return cmp > 0
		}
		return vv[i].raw > vv[j].raw
	})
	out := make([]string, len(vv))
	for i := range vv {
		out[i] = vv[i].raw
	}
	return out
}

func mustSemver(s string) semVersion {
	v, _ := parseSemver(s)
	return v
}
