package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverExcludesGoalModeFromGenericReflectList(t *testing.T) {
	root := t.TempDir()
	reflectDir := filepath.Join(root, "reflect")
	if err := os.MkdirAll(reflectDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"goal_mode.py", "autonomous.py", "custom_reflect.py"} {
		if err := os.WriteFile(filepath.Join(reflectDir, name), []byte("# test\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	items := NewManager(root, 100).Discover()
	seen := map[string]int{}
	for _, item := range items {
		seen[item.Name]++
	}
	if seen[filepath.ToSlash(filepath.Join("reflect", "goal_mode.py"))] != 1 {
		t.Fatalf("goal_mode.py should appear exactly once as the dedicated lifecycle entry, seen=%d items=%#v", seen[filepath.ToSlash(filepath.Join("reflect", "goal_mode.py"))], items)
	}
	if seen[filepath.ToSlash(filepath.Join("reflect", "autonomous.py"))] != 1 {
		t.Fatalf("autonomous.py should remain discoverable once, seen=%d", seen[filepath.ToSlash(filepath.Join("reflect", "autonomous.py"))])
	}
	if seen[filepath.ToSlash(filepath.Join("reflect", "custom_reflect.py"))] != 1 {
		t.Fatalf("custom reflect should remain discoverable once, seen=%d", seen[filepath.ToSlash(filepath.Join("reflect", "custom_reflect.py"))])
	}
}

func TestCommandLineMatchesServiceRequiresExactScriptPath(t *testing.T) {
	root := filepath.Clean(filepath.Join(t.TempDir(), "ga-root"))
	py := filepath.Join(root, ".venv", "Scripts", "python.exe")
	cmd := []string{py, filepath.ToSlash(filepath.Join("reflect", "custom_reflect.py"))}

	ownAbs := py + " " + filepath.Join(root, "reflect", "custom_reflect.py")
	if !commandLineMatchesService(ownAbs, root, cmd) {
		t.Fatalf("expected absolute GA script command to match")
	}

	ownRel := py + " reflect/custom_reflect.py"
	if !commandLineMatchesService(ownRel, root, cmd) {
		t.Fatalf("expected relative GA script command to match")
	}

	otherSameBase := py + " " + filepath.Join(filepath.Dir(root), "other-root", "reflect", "custom_reflect.py")
	if commandLineMatchesService(otherSameBase, root, cmd) {
		t.Fatalf("same basename under another root must not match")
	}

	otherSimilarRel := py + " other/reflect/custom_reflect.py"
	if commandLineMatchesService(otherSimilarRel, root, cmd) {
		t.Fatalf("relative path with extra prefix must not match")
	}
}
