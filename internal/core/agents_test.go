package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAgentsContext_Missing(t *testing.T) {
	dir := t.TempDir()
	got := LoadAgentsContext(dir)
	if got != "" {
		t.Fatalf("expected empty for missing AGENTS.md, got %q", got)
	}
}

func TestLoadAgentsContext_Reads(t *testing.T) {
	dir := t.TempDir()
	body := "# Project rules\nBe terse."
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := LoadAgentsContext(dir)
	if !strings.Contains(got, body) {
		t.Fatalf("expected file body inside output, got %q", got)
	}
	if !strings.Contains(got, "<agents_md>") || !strings.Contains(got, "</agents_md>") {
		t.Fatalf("expected XML wrapper tags, got %q", got)
	}
}

func TestLoadAgentsContext_EmptyDir(t *testing.T) {
	if got := LoadAgentsContext(""); got != "" {
		t.Fatalf("expected empty for empty dir, got %q", got)
	}
}

func TestLoadAgentsContext_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadAgentsContext(dir); got != "" {
		t.Fatalf("expected empty for empty file, got %q", got)
	}
}

func TestBuildSystemPromptWithAgents_AppendsBoth(t *testing.T) {
	base := "BASE"
	agents := "<agents_md>X</agents_md>"
	out := AppendAgentsContext(base, agents)
	if !strings.Contains(out, "BASE") || !strings.Contains(out, "<agents_md>X</agents_md>") {
		t.Fatalf("expected both base and agents, got %q", out)
	}
}

func TestAppendAgentsContext_EmptyAgents(t *testing.T) {
	if got := AppendAgentsContext("BASE", ""); got != "BASE" {
		t.Fatalf("expected BASE unchanged, got %q", got)
	}
}

func TestAppendAgentsContext_EmptyBase(t *testing.T) {
	if got := AppendAgentsContext("", "X"); got != "X" {
		t.Fatalf("expected X, got %q", got)
	}
}

func TestBootstrapAgentsMd_CopiesDefaultWhenMissing(t *testing.T) {
	// given
	// ... a workDir without AGENTS.md and a default file with content
	workDir := t.TempDir()
	defaultDir := t.TempDir()
	defaultPath := filepath.Join(defaultDir, "AGENTS.md.default")
	body := "# Default rules\nBe helpful."
	if err := os.WriteFile(defaultPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// when
	// ... BootstrapAgentsMd runs
	if err := BootstrapAgentsMd(workDir, defaultPath); err != nil {
		t.Fatal(err)
	}

	// then
	// ... <workDir>/AGENTS.md contains the default body
	got, err := os.ReadFile(filepath.Join(workDir, AgentsFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Fatalf("expected %q, got %q", body, string(got))
	}
}

func TestBootstrapAgentsMd_LeavesExistingFileUntouched(t *testing.T) {
	// given
	// ... a workDir with an existing AGENTS.md and a default file with different content
	workDir := t.TempDir()
	existing := "# my custom rules"
	if err := os.WriteFile(filepath.Join(workDir, AgentsFileName), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	defaultDir := t.TempDir()
	defaultPath := filepath.Join(defaultDir, "AGENTS.md.default")
	if err := os.WriteFile(defaultPath, []byte("DEFAULT"), 0o644); err != nil {
		t.Fatal(err)
	}

	// when
	// ... BootstrapAgentsMd runs
	if err := BootstrapAgentsMd(workDir, defaultPath); err != nil {
		t.Fatal(err)
	}

	// then
	// ... the existing file is unchanged
	got, err := os.ReadFile(filepath.Join(workDir, AgentsFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != existing {
		t.Fatalf("expected %q, got %q", existing, string(got))
	}
}

func TestBootstrapAgentsMd_NoDefaultIsNoop(t *testing.T) {
	// given
	// ... a workDir without AGENTS.md and a default path that doesn't exist
	workDir := t.TempDir()
	defaultPath := filepath.Join(t.TempDir(), "missing.md")

	// when
	// ... BootstrapAgentsMd runs
	err := BootstrapAgentsMd(workDir, defaultPath)

	// then
	// ... no error and no file is written
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, AgentsFileName)); !os.IsNotExist(err) {
		t.Fatalf("expected AGENTS.md to not exist, got err=%v", err)
	}
}

func TestBootstrapAgentsMd_EmptyDefaultPathIsNoop(t *testing.T) {
	// given
	// ... a workDir without AGENTS.md and an empty default path
	workDir := t.TempDir()

	// when
	// ... BootstrapAgentsMd runs with empty default path
	err := BootstrapAgentsMd(workDir, "")

	// then
	// ... no error and no file is written
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, AgentsFileName)); !os.IsNotExist(err) {
		t.Fatalf("expected AGENTS.md to not exist, got err=%v", err)
	}
}

func TestReadAgentsMd_ReturnsContent(t *testing.T) {
	// given
	// ... a workDir containing an AGENTS.md
	workDir := t.TempDir()
	body := "hello"
	if err := os.WriteFile(filepath.Join(workDir, AgentsFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// when
	// ... ReadAgentsMd is called
	got, err := ReadAgentsMd(workDir)

	// then
	// ... it returns the file body
	if err != nil {
		t.Fatal(err)
	}
	if got != body {
		t.Fatalf("expected %q, got %q", body, got)
	}
}

func TestReadAgentsMd_MissingReturnsEmpty(t *testing.T) {
	// given
	// ... a workDir without AGENTS.md
	workDir := t.TempDir()

	// when
	// ... ReadAgentsMd is called
	got, err := ReadAgentsMd(workDir)

	// then
	// ... it returns empty string, no error
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestWriteAgentsMd_WritesContent(t *testing.T) {
	// given
	// ... a workDir
	workDir := t.TempDir()
	content := "new content"

	// when
	// ... WriteAgentsMd is called
	if err := WriteAgentsMd(workDir, content); err != nil {
		t.Fatal(err)
	}

	// then
	// ... <workDir>/AGENTS.md contains the content
	got, err := os.ReadFile(filepath.Join(workDir, AgentsFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("expected %q, got %q", content, string(got))
	}
}

func TestResetAgentsMd_OverwritesWithDefault(t *testing.T) {
	// given
	// ... a workDir with an existing AGENTS.md and a default file
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, AgentsFileName), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	defaultDir := t.TempDir()
	defaultPath := filepath.Join(defaultDir, "default.md")
	if err := os.WriteFile(defaultPath, []byte("DEFAULT"), 0o644); err != nil {
		t.Fatal(err)
	}

	// when
	// ... ResetAgentsMd is called
	if err := ResetAgentsMd(workDir, defaultPath); err != nil {
		t.Fatal(err)
	}

	// then
	// ... AGENTS.md is replaced with default
	got, err := os.ReadFile(filepath.Join(workDir, AgentsFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "DEFAULT" {
		t.Fatalf("expected DEFAULT, got %q", string(got))
	}
}

func TestResetAgentsMd_MissingDefaultErrors(t *testing.T) {
	// given
	// ... a workDir and a default path that doesn't exist
	workDir := t.TempDir()
	defaultPath := filepath.Join(t.TempDir(), "missing.md")

	// when
	// ... ResetAgentsMd is called
	err := ResetAgentsMd(workDir, defaultPath)

	// then
	// ... it returns an error (caller can surface it to the user)
	if err == nil {
		t.Fatal("expected error for missing default")
	}
}

func TestBootstrapAgentsMd_EmptyWorkDirIsNoop(t *testing.T) {
	// given
	// ... an empty workDir and a valid default path
	defaultDir := t.TempDir()
	defaultPath := filepath.Join(defaultDir, "AGENTS.md.default")
	if err := os.WriteFile(defaultPath, []byte("X"), 0o644); err != nil {
		t.Fatal(err)
	}

	// when
	// ... BootstrapAgentsMd runs with empty workDir
	err := BootstrapAgentsMd("", defaultPath)

	// then
	// ... no error
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
