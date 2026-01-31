# Worktree Sessions Design

## Overview

Extend `/new-session` to optionally create/enter git worktrees, enabling mobile coding workflows.

## Command Interface

Extend existing slash command in `handler.SlashCommands()`:

```go
{
    Name:        "new-session",
    Description: "Start a new Claude session",
    Options: []*discordgo.ApplicationCommandOption{
        {Name: "directory", Type: discordgo.ApplicationCommandOptionString, ...},
        {Name: "repo", Type: discordgo.ApplicationCommandOptionString, Description: "GitHub repo (owner/repo or URL)"},
        {Name: "branch", Type: discordgo.ApplicationCommandOptionString, Description: "Branch name for worktree"},
    },
}
```

## Behaviour Matrix

| directory | repo | branch | Action |
|-----------|------|--------|--------|
| ✅ | ❌ | ❌ | Current behaviour |
| ❌ | ✅ | ❌ | Clone if needed, create worktree with auto-branch |
| ❌ | ✅ | ✅ | Clone if needed, create/enter worktree for branch |
| ❌ | ❌ | ✅ | Error - branch requires repo |
| ✅ | ✅ | * | Error - directory and repo are mutually exclusive |

## Directory Structure

```
$WORKTREE_BASE/
├── repos/                    # Bare clones
│   └── owner__repo.git/
└── worktrees/               # Active worktrees
    └── owner__repo/
        ├── feature-xyz/
        └── fix-bug-123/
```

## New Package

```
internal/
├── git/
│   ├── manager.go           # GitManager implementation
│   └── manager_test.go
```

## Interface Addition

Add to `internal/core/interfaces.go`:

```go
type GitManager interface {
    EnsureRepo(repoURL string) (bareRepoPath string, err error)
    EnsureWorktree(bareRepoPath, branch string) (worktreePath string, err error)
}
```

## Config Changes

`internal/config/config.go`:

```go
type Config struct {
    DiscordToken  string
    AllowedDirs   []string
    AllowedUsers  []string
    ClaudeCWD     string
    WorktreeBase  string   // new: base dir for repos/worktrees
    GitHubToken   string   // new: for private repos (optional)
}
```

New env vars:
- `WORKTREE_BASE` - defaults to `~/.claudecord`
- `GITHUB_TOKEN` - optional, for private repos

## Handler Changes

`internal/handler/discord.go` in `OnInteractionCreate`:

```go
case "new-session":
    // ... existing user validation ...

    var dir, repo, branch string
    for _, opt := range data.Options {
        switch opt.Name {
        case "directory":
            dir = opt.StringValue()
        case "repo":
            repo = opt.StringValue()
        case "branch":
            branch = opt.StringValue()
        }
    }

    // Validation
    if dir != "" && repo != "" {
        h.respondError(s, i, "Cannot specify both directory and repo")
        return
    }
    if branch != "" && repo == "" {
        h.respondError(s, i, "Branch requires repo")
        return
    }

    // If repo specified, resolve to worktree path
    if repo != "" {
        worktreePath, err := h.gitManager.EnsureWorktree(repo, branch)
        if err != nil {
            h.respondError(s, i, fmt.Sprintf("Git error: %v", err))
            return
        }
        dir = worktreePath
    }

    // ... existing NewSession(dir) call ...
```

## Handler Struct Change

```go
type Handler struct {
    bot          BotInterface
    botID        string
    allowedUsers []string
    passiveBot   PassiveBotInterface
    buffer       *core.DebouncedBuffer
    gitManager   core.GitManager  // new
}
```

## GitManager Implementation

`internal/git/manager.go`:

```go
type Manager struct {
    baseDir     string
    githubToken string
}

func NewManager(baseDir, githubToken string) *Manager

func (m *Manager) EnsureRepo(repoURL string) (string, error) {
    // 1. Normalise URL (owner/repo -> https://github.com/owner/repo)
    // 2. Check if repos/{owner}__{repo}.git exists
    // 3. If not: git clone --bare {url} repos/{owner}__{repo}.git
    // 4. If exists: git fetch --all
    // 5. Return bare repo path
}

func (m *Manager) EnsureWorktree(repoURL, branch string) (string, error) {
    // 1. EnsureRepo(repoURL)
    // 2. Generate branch if empty (e.g., session-{timestamp})
    // 3. Check if worktrees/{owner}__{repo}/{branch} exists
    // 4. If not: git worktree add -b {branch} worktrees/{owner}__{repo}/{branch}
    // 5. Return worktree path
}
```

## ALLOWED_DIRS Handling

Option A (simpler): Require user to add `WORKTREE_BASE` to `ALLOWED_DIRS`.

Option B: Auto-append `WORKTREE_BASE` to allowed dirs in config loading.

Recommend Option B - update `config.LoadFromEnv()`:

```go
func LoadFromEnv() Config {
    // ... existing loading ...
    if cfg.WorktreeBase != "" {
        cfg.AllowedDirs = append(cfg.AllowedDirs, cfg.WorktreeBase)
    }
}
```

## Error Cases

| Case | Response |
|------|----------|
| Invalid repo format | "Invalid repo. Use owner/repo or full URL" |
| Clone fails | "Failed to clone: {error}" |
| Worktree create fails | "Failed to create worktree: {error}" |
| Branch without repo | "Branch requires repo" |
| Both directory and repo | "Cannot specify both directory and repo" |

---

## Speculations & Assumptions

1. **Bare clones** - Using `--bare` avoids wasting space on unused working copy. All work in worktrees.

2. **Branch generation** - If no branch provided, generate `session-{unix-timestamp}`. Simple and sortable.

3. **Fetch on existing repo** - When repo already cloned, do `git fetch --all` to get latest refs before creating worktree.

4. **No cleanup in v1** - Worktrees persist. Future: `/cleanup-worktrees` or TTL-based.

5. **Worktree reuse** - Same repo+branch enters existing worktree. Doesn't "resume" Claude session (that's separate concern).

6. **Auth via env only** - `GITHUB_TOKEN` env var or pre-configured SSH keys. No OAuth flow.

7. **Single remote** - Origin only. Multi-remote out of scope.

8. **Git binary** - Assumes `git` available in PATH. No embedded git library.

9. **Repo URL formats** - Support:
   - `owner/repo` → `https://github.com/owner/repo`
   - `https://github.com/owner/repo`
   - `git@github.com:owner/repo`

10. **Directory naming** - Use `__` as separator since `/` not allowed in paths. `owner/repo` → `owner__repo`.
