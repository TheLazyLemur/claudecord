package memory

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestList_EmptyDir(t *testing.T) {
	// given
	// ... an empty memory dir
	dir := t.TempDir()

	// when
	// ... List is called
	got, err := List(dir)

	// then
	// ... it returns an empty slice
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty list, got %v", got)
	}
}

func TestList_FlatFilesAndDailySubdir(t *testing.T) {
	// given
	// ... a memory dir with MEMORY.md and a daily/YYYY-MM-DD.md
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	dailyDir := filepath.Join(dir, "daily")
	if err := os.MkdirAll(dailyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dailyDir, "2026-05-04.md"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}

	// when
	// ... List is called
	got, err := List(dir)

	// then
	// ... it returns both files with relative paths
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	want := []string{"MEMORY.md", "daily/2026-05-04.md"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestRead_ReturnsContent(t *testing.T) {
	// given
	// ... a memory dir with MEMORY.md
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// when
	// ... Read is called
	got, err := Read(dir, "MEMORY.md")

	// then
	// ... it returns the file content
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
}

func TestRead_PathEscapeRejected(t *testing.T) {
	// given
	// ... a memory dir
	dir := t.TempDir()

	// when
	// ... Read is called with a parent traversal
	_, err := Read(dir, "../etc/passwd")

	// then
	// ... it errors
	if err == nil {
		t.Fatal("expected error for path escape")
	}
}

func TestRead_AbsolutePathRejected(t *testing.T) {
	// given
	// ... a memory dir
	dir := t.TempDir()

	// when
	// ... Read is called with an absolute path
	_, err := Read(dir, "/etc/passwd")

	// then
	// ... it errors
	if err == nil {
		t.Fatal("expected error for absolute path")
	}
}

func TestWrite_CreatesNestedFile(t *testing.T) {
	// given
	// ... an empty memory dir
	dir := t.TempDir()

	// when
	// ... Write creates a nested file
	if err := Write(dir, "daily/2026-05-04.md", "today"); err != nil {
		t.Fatal(err)
	}

	// then
	// ... the file exists with the content
	got, err := os.ReadFile(filepath.Join(dir, "daily", "2026-05-04.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "today" {
		t.Fatalf("expected today, got %q", string(got))
	}
}

func TestWrite_PathEscapeRejected(t *testing.T) {
	// given
	// ... a memory dir
	dir := t.TempDir()

	// when
	// ... Write is called with a parent traversal
	err := Write(dir, "../escape.md", "x")

	// then
	// ... it errors
	if err == nil {
		t.Fatal("expected error for path escape")
	}
}

func TestDelete_RemovesFile(t *testing.T) {
	// given
	// ... a memory dir with a file
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}

	// when
	// ... Delete is called
	if err := Delete(dir, "MEMORY.md"); err != nil {
		t.Fatal(err)
	}

	// then
	// ... the file is gone
	if _, err := os.Stat(filepath.Join(dir, "MEMORY.md")); !os.IsNotExist(err) {
		t.Fatalf("expected file to be removed, got err=%v", err)
	}
}

func TestDelete_PathEscapeRejected(t *testing.T) {
	// given
	// ... a memory dir
	dir := t.TempDir()

	// when
	// ... Delete is called with parent traversal
	err := Delete(dir, "../etc/passwd")

	// then
	// ... it errors
	if err == nil {
		t.Fatal("expected error for path escape")
	}
}
