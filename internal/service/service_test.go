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
