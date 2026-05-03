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
