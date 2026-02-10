package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStore_CreatesDirectory(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "history")

	// when
	store, err := NewStore(baseDir)

	// then
	r.NoError(err)
	a.NotNil(store)
	a.DirExists(baseDir)
}

func TestStore_SaveAndLoad(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	store, err := NewStore(t.TempDir())
	r.NoError(err)

	session := &Session{
		ID:        "test-session-123",
		WorkDir:   "/tmp/test",
		CreatedAt: time.Now(),
		Messages: []Message{
			{Role: "user", Content: "hello", Timestamp: time.Now()},
			{Role: "assistant", Content: "hi there", Timestamp: time.Now()},
		},
	}

	// when
	err = store.Save(session)
	r.NoError(err)

	// then
	loaded, err := store.Load("test-session-123")
	r.NoError(err)
	a.Equal(session.ID, loaded.ID)
	a.Equal(session.WorkDir, loaded.WorkDir)
	a.Len(loaded.Messages, 2)
	a.Equal("hello", loaded.Messages[0].Content)
	a.Equal("assistant", loaded.Messages[1].Role)
}

func TestStore_Load_NotFound(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	store, err := NewStore(t.TempDir())
	r.NoError(err)

	// when
	_, err = store.Load("nonexistent")

	// then
	a.Error(err)
	a.Contains(err.Error(), "not found")
}

func TestStore_List(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	store, err := NewStore(t.TempDir())
	r.NoError(err)

	// Create sessions with different timestamps
	session1 := &Session{
		ID:        "session-1",
		WorkDir:   "/tmp/1",
		CreatedAt: time.Now().Add(-2 * time.Hour),
		UpdatedAt: time.Now().Add(-1 * time.Hour),
		Messages:  []Message{},
	}
	session2 := &Session{
		ID:        "session-2",
		WorkDir:   "/tmp/2",
		CreatedAt: time.Now().Add(-1 * time.Hour),
		UpdatedAt: time.Now(),
		Messages:  []Message{},
	}

	r.NoError(store.Save(session1))
	r.NoError(store.Save(session2))

	// when
	sessions, err := store.List()

	// then
	r.NoError(err)
	a.Len(sessions, 2)
	// Should be sorted by UpdatedAt desc (most recent first)
	a.Equal("session-2", sessions[0].ID)
	a.Equal("session-1", sessions[1].ID)
}

func TestStore_Delete(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	store, err := NewStore(t.TempDir())
	r.NoError(err)

	session := &Session{
		ID:      "to-delete",
		WorkDir: "/tmp",
	}
	r.NoError(store.Save(session))

	// when
	err = store.Delete("to-delete")

	// then
	r.NoError(err)
	_, err = store.Load("to-delete")
	a.Error(err)
}

func TestSession_Summary(t *testing.T) {
	a := assert.New(t)

	// given
	session := &Session{
		ID:        "abc123def",
		UpdatedAt: time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC),
		Messages:  make([]Message, 5),
	}

	// when
	summary := session.Summary()

	// then
	a.Contains(summary, "abc123de") // first 8 chars of ID
	a.Contains(summary, "5 messages")
	a.Contains(summary, "Jan 15 14:30")
}

func TestStore_List_SkipsInvalidFiles(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	r.NoError(err)

	// Create a valid session
	session := &Session{ID: "valid", WorkDir: "/tmp"}
	r.NoError(store.Save(session))

	// Create invalid files
	os.WriteFile(filepath.Join(tmpDir, "not-json.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "invalid.json"), []byte("not valid json"), 0644)

	// when
	sessions, err := store.List()

	// then
	r.NoError(err)
	a.Len(sessions, 1)
	a.Equal("valid", sessions[0].ID)
}

func TestStore_Save_UpdatesTimestamp(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	store, err := NewStore(t.TempDir())
	r.NoError(err)

	oldTime := time.Now().Add(-1 * time.Hour)
	session := &Session{
		ID:        "test",
		WorkDir:   "/tmp",
		UpdatedAt: oldTime,
	}

	// when
	time.Sleep(10 * time.Millisecond) // ensure time passes
	err = store.Save(session)

	// then
	r.NoError(err)
	a.True(session.UpdatedAt.After(oldTime), "UpdatedAt should be updated to current time")
}
