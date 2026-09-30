package source

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjects(t *testing.T) {
	root := t.TempDir()
	committed := filepath.Join(root, "committed")
	empty := filepath.Join(root, "empty")
	for _, path := range []string{committed, empty} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		gitTest(t, path, "init", "--quiet")
	}
	gitTest(t, committed, "-c", "user.name=Fictional", "-c", "user.email=fictional@example.com", "commit", "--quiet", "--allow-empty", "-m", "First change\n\nFull description")
	projects, err := Projects(t.Context(), []string{empty, filepath.Join(root, "missing"), committed})
	if err != nil || len(projects) != 3 {
		t.Fatalf("projects=%+v err=%v", projects, err)
	}
	if projects[0].Name != "committed" || projects[0].Subject != "First change" || !strings.Contains(projects[0].Body, "Full description") || projects[0].Time.IsZero() || len(projects[0].Revision) != 40 {
		t.Fatalf("project=%+v", projects[0])
	}
	for _, p := range projects[1:] {
		if p.Error == "" {
			t.Fatalf("missing error: %+v", p)
		}
	}
}

func gitTest(t *testing.T, path string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", path}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %q: %s: %v", args, data, err)
	}
}
