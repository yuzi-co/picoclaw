//go:build ironkvm

package agent

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func writeTestSkill(t *testing.T, workspace, name string) {
	t.Helper()
	dir := filepath.Join(workspace, "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: test skill\n---\n\nDo something.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestApplyBuildSkillsPolicy_IgnoresWorkspaceSkillsByDefault(t *testing.T) {
	workspace := t.TempDir()
	writeTestSkill(t, workspace, "planted")

	cb := NewContextBuilder(workspace)
	applyBuildSkillsPolicy(cb, config.DefaultConfig())

	if names := cb.ListSkillNames(); len(names) != 0 {
		t.Fatalf("ListSkillNames() = %v, want none", names)
	}
	if summary := cb.skillsLoader.BuildSkillsSummary(); summary != "" {
		t.Fatalf("skills summary = %q, want empty", summary)
	}
}

func TestApplyBuildSkillsPolicy_LoadsSkillsWhenEnabled(t *testing.T) {
	workspace := t.TempDir()
	writeTestSkill(t, workspace, "wanted")

	cfg := config.DefaultConfig()
	cfg.Tools.Skills.Enabled = true
	cb := NewContextBuilder(workspace)
	applyBuildSkillsPolicy(cb, cfg)

	if names := cb.ListSkillNames(); !slices.Contains(names, "wanted") {
		t.Fatalf("ListSkillNames() = %v, want it to include wanted", names)
	}
}
