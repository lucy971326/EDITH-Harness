package permissions

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestPermissionRules(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "project")
	temp := filepath.Join(root, "tmp")
	outside := filepath.Join(root, "notes")
	for _, test := range []struct {
		mode     Mode
		reviewer ReviewerKind
		write    Requirement
		network  Requirement
	}{
		{"", HumanReviewer, Allow, Ask},
		{ReadOnly, HumanReviewer, Ask, Ask},
		{AskForApproval, HumanReviewer, Allow, Ask},
		{ApproveForMe, ModelReviewer, Allow, Ask},
		{FullAccess, NoReviewer, Allow, Allow},
	} {
		t.Run(string(test.mode), func(t *testing.T) {
			policy, reviewer, err := Resolve(test.mode, workspace, []string{temp})
			if err != nil || reviewer != test.reviewer {
				t.Fatalf("resolve: %+v, %s, %v", policy, reviewer, err)
			}
			for _, path := range []string{workspace, temp} {
				got, err := Evaluate(policy, ExtraPermissions{WriteRoots: []string{path}})
				if err != nil || got != test.write {
					t.Fatalf("write %s: %v, %v", path, got, err)
				}
			}
			got, err := Evaluate(policy, ExtraPermissions{Network: true})
			if err != nil || got != test.network {
				t.Fatalf("network: %v, %v", got, err)
			}
		})
	}
	_, _, err := Resolve("unknown", workspace, nil)
	if err == nil {
		t.Fatal("unknown mode accepted")
	}

	base, _, err := Resolve(AskForApproval, workspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path string
		want Requirement
	}{
		{filepath.Join(workspace, "main.go"), Allow},
		{workspace + "-other", Ask},
		{outside, Ask},
		{filepath.Join(workspace, ".git", "config"), Ask},
		{filepath.Join(workspace, ".agents", "skills"), Ask},
		{filepath.Join(workspace, ".harness"), Ask},
		{"relative", Deny},
	} {
		got, err := Evaluate(base, ExtraPermissions{WriteRoots: []string{test.path}})
		if got != test.want || (err != nil) != (test.want == Deny) {
			t.Fatalf("evaluate %s: %v, %v", test.path, got, err)
		}
	}

	// 宽泛授权仍保护元数据；精确批准仅开放目标，不改动基础策略或兄弟路径。
	protected := filepath.Join(workspace, ".agents", "skills")
	for _, test := range []struct {
		name     string
		grant    string
		approved bool
		want     Requirement
	}{
		{"rejected", protected, false, Ask},
		{"broad grant", root, true, Ask},
		{"exact grant", protected, true, Allow},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := ApprovalRequest{Current: base, Requested: ExtraPermissions{
				WriteRoots: []string{test.grant}, Network: true,
			}}
			effective, err := ApplyDecision(request, Decision{Approved: test.approved})
			if !test.approved {
				if err == nil {
					t.Fatal("rejection produced permissions")
				}
				return
			}
			if err != nil || !effective.Network {
				t.Fatalf("approval: %+v, %v", effective, err)
			}
			got, err := Evaluate(effective, ExtraPermissions{WriteRoots: []string{protected}})
			if err != nil || got != test.want {
				t.Fatalf("protected path: %v, %v", got, err)
			}
			got, err = Evaluate(effective, ExtraPermissions{WriteRoots: []string{filepath.Join(workspace, ".agents", "other")}})
			if err != nil || got != Ask {
				t.Fatalf("sibling protection lost: %v, %v", got, err)
			}
			effective.WriteRoots[0] = outside
			if !reflect.DeepEqual(base, Policy{WriteRoots: []string{workspace}}) {
				t.Fatalf("base policy mutated: %+v", base)
			}
		})
	}
}
