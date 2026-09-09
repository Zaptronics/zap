// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"strings"
	"testing"
)

func TestDeclaredSourcePolicy(t *testing.T) {
	t.Setenv("ZAP_ALLOW_LOCAL_GIT", "")
	for _, kind := range []string{"git", "url"} {
		for _, uri := range []string{"http://example.invalid/a", "git://example.invalid/a", "ext::echo nope", "file:///tmp/a", "../repo", "C:\\repo", "https:///missing-host", "--upload-pack=payload", "https://example.invalid/a\nextra"} {
			t.Run(kind+"/"+uri, func(t *testing.T) {
				if err := validateSourceURI(kind, uri); err == nil {
					t.Fatal("unsafe declaration accepted")
				}
			})
		}
		if err := validateSourceURI(kind, "https://example.invalid/repo"); err != nil {
			t.Fatal(err)
		}
	}
	for _, uri := range []string{"ssh://git@example.invalid/repo", "git@example.invalid:repo.git"} {
		if err := validateSourceURI("git", uri); err != nil {
			t.Fatal(err)
		}
		if err := validateSourceURI("url", uri); err == nil {
			t.Fatal("SSH archive accepted")
		}
	}
	t.Setenv("ZAP_ALLOW_LOCAL_GIT", "1")
	for _, uri := range []string{"file:///tmp/repo", "../repo"} {
		if err := validateSourceURI("git", uri); err != nil {
			t.Fatal(err)
		}
	}
	for _, uri := range []string{"http://example.invalid/repo", "git://example.invalid/repo", "ext::echo nope"} {
		if err := validateSourceURI("git", uri); err == nil {
			t.Fatal("local escape hatch allowed unsafe remote transport")
		}
	}
}

func TestTransitiveAndLockedSourcePolicy(t *testing.T) {
	t.Setenv("ZAP_ALLOW_LOCAL_GIT", "")
	_, err := ParsePackageManifest("schema: 2\npackage:\n  name: demo\ndependencies:\n  child:\n    type: git\n    uri: http://example.invalid/repo\n    version: v1.0.0\n")
	if err == nil {
		t.Fatal("transitive insecure Git source accepted")
	}
	for _, kind := range []string{"git", "url"} {
		l := &Lockfile{Schema: LockSchema, DependencyOrder: []string{"demo"}, Dependencies: map[string]*LockedDependency{
			"demo": {Type: kind, URI: "http://example.invalid/repo", Commit: strings.Repeat("a", 40), Hash: "SHA256=" + strings.Repeat("a", 64)},
		}}
		if err := validateLock(l); err == nil {
			t.Fatalf("insecure %s source in lock accepted", kind)
		}
	}
}

func TestGitDiscoveryRejectsUnsafeSourceBeforeExecution(t *testing.T) {
	t.Setenv("ZAP_ALLOW_LOCAL_GIT", "")
	// A nonexistent program makes accidental subprocess execution distinguishable
	// from rejection at the source boundary, without contacting any remote.
	_, err := listStableTags("must-not-execute-zap-test", "ext::payload")
	if err == nil || !strings.Contains(err.Error(), "Git sources require") {
		t.Fatalf("policy not applied before discovery: %v", err)
	}
}
