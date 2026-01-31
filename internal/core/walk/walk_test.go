package walk

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestWalkIncludeExclude(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a.go", "x")
	writeFile(t, root, "a.sql", "x")

	files, err := ListFiles(root, Options{
		IncludeGlobs: []string{"*.go"},
		ExcludeGlobs: []string{"*.sql"},
		ScanAll:      false,
	})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 1 || files[0] != "a.go" {
		t.Fatalf("files=%v", files)
	}
}

func TestWalkSkipsDefaultDirsUnlessScanAll(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a.go", "x")
	writeFile(t, root, ".git/config", "x")
	writeFile(t, root, "node_modules/a.js", "x")
	writeFile(t, root, "dist/a.txt", "x")
	writeFile(t, root, "target/a.txt", "x")

	files, err := ListFiles(root, Options{ScanAll: false})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 1 || files[0] != "a.go" {
		t.Fatalf("files=%v", files)
	}

	files, err = ListFiles(root, Options{ScanAll: true})
	if err != nil {
		t.Fatalf("ListFiles(all): %v", err)
	}
	for _, want := range []string{
		".git/config",
		"node_modules/a.js",
		"dist/a.txt",
		"target/a.txt",
	} {
		if !slices.Contains(files, want) {
			t.Fatalf("missing %q in files=%v", want, files)
		}
	}
}

func TestWalkSkipsHiddenFilesUnlessScanAll(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a.go", "x")
	writeFile(t, root, ".env", "x")
	writeFile(t, root, "sub/.hidden.txt", "x")

	files, err := ListFiles(root, Options{ScanAll: false})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if slices.Contains(files, ".env") || slices.Contains(files, "sub/.hidden.txt") {
		t.Fatalf("hidden files should be skipped by default: %v", files)
	}

	files, err = ListFiles(root, Options{ScanAll: true})
	if err != nil {
		t.Fatalf("ListFiles(all): %v", err)
	}
	for _, want := range []string{".env", "sub/.hidden.txt"} {
		if !slices.Contains(files, want) {
			t.Fatalf("missing %q in files=%v", want, files)
		}
	}
}

func TestWalkRespectsGitIgnore_Basic(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".gitignore", "*.sql\n")
	writeFile(t, root, "a.go", "x")
	writeFile(t, root, "a.sql", "x")

	files, err := ListFiles(root, Options{ScanAll: false})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if slices.Contains(files, "a.sql") {
		t.Fatalf("a.sql should be ignored via .gitignore: %v", files)
	}
	if !slices.Contains(files, "a.go") {
		t.Fatalf("a.go should not be ignored: %v", files)
	}
}

func TestWalkRespectsGitIgnore_Negation(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".gitignore", "*.log\n!keep.log\n")
	writeFile(t, root, "keep.log", "x")
	writeFile(t, root, "drop.log", "x")

	files, err := ListFiles(root, Options{ScanAll: false})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if slices.Contains(files, "drop.log") {
		t.Fatalf("drop.log should be ignored: %v", files)
	}
	if !slices.Contains(files, "keep.log") {
		t.Fatalf("keep.log should be unignored: %v", files)
	}
}

func TestWalkRespectsGitIgnore_Anchored(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".gitignore", "/a.go\n")
	writeFile(t, root, "a.go", "x")
	writeFile(t, root, "sub/a.go", "x")

	files, err := ListFiles(root, Options{ScanAll: false})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if slices.Contains(files, "a.go") {
		t.Fatalf("root a.go should be ignored: %v", files)
	}
	if !slices.Contains(files, "sub/a.go") {
		t.Fatalf("sub/a.go should not be ignored: %v", files)
	}
}

func TestWalkRespectsGitIgnore_DirPattern(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".gitignore", "foo/\n")
	writeFile(t, root, "foo/a.go", "x")
	writeFile(t, root, "bar.go", "x")

	files, err := ListFiles(root, Options{ScanAll: false})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if slices.Contains(files, "foo/a.go") {
		t.Fatalf("foo/ should be ignored: %v", files)
	}
	if !slices.Contains(files, "bar.go") {
		t.Fatalf("bar.go should not be ignored: %v", files)
	}
}

func TestWalkRespectsGitIgnore_NestedGitignore(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".gitignore", "")
	writeFile(t, root, "sub/.gitignore", "*.gen\n")
	writeFile(t, root, "sub/a.gen", "x")
	writeFile(t, root, "sub/a.go", "x")

	files, err := ListFiles(root, Options{ScanAll: false})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if slices.Contains(files, "sub/a.gen") {
		t.Fatalf("sub/a.gen should be ignored by nested .gitignore: %v", files)
	}
	if !slices.Contains(files, "sub/a.go") {
		t.Fatalf("sub/a.go should not be ignored: %v", files)
	}
}

func TestWalkRespectsGitIgnore_CannotUnignoreFileInIgnoredDir(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".gitignore", "foo/\n!foo/keep.txt\n")
	writeFile(t, root, "foo/keep.txt", "x")

	files, err := ListFiles(root, Options{ScanAll: false})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if slices.Contains(files, "foo/keep.txt") {
		t.Fatalf("keep.txt should remain ignored because parent dir is ignored: %v", files)
	}
}

func writeFile(t *testing.T, root string, rel string, content string) {
	t.Helper()

	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", rel, err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", rel, err)
	}
}
