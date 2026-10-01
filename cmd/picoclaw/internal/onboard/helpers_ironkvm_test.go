//go:build ironkvm

package onboard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyEmbeddedToTarget_IronKVMCopiesNoSkills(t *testing.T) {
	targetDir := t.TempDir()
	if err := copyEmbeddedToTarget(targetDir); err != nil {
		t.Fatalf("copyEmbeddedToTarget() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "skills")); !os.IsNotExist(err) {
		t.Fatalf("expected no skills directory, got err=%v", err)
	}
	for _, name := range []string{"AGENT.md", "SOUL.md", "USER.md", filepath.Join("memory", "MEMORY.md")} {
		if _, err := os.Stat(filepath.Join(targetDir, name)); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}
}
