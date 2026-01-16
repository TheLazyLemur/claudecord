package main

import (
	"os"
	"testing"

	"github.com/TheLazyLemur/claudecord/internal/config"
	"github.com/stretchr/testify/require"
)

func TestMain_ConfigLoading(t *testing.T) {
	// Test that main.go properly loads allowed users config
	// This is an integration test since it tests the full initialization flow

	// Create test allowed_users.json
	testData := `["123456789", "987654321"]`
	tempDir := t.TempDir()
	oldCwd, _ := os.Getwd()

	// Change to temp directory
	err := os.Chdir(tempDir)
	require.NoError(t, err)
	defer os.Chdir(oldCwd)

	// Create test file
	err = os.WriteFile("allowed_users.json", []byte(testData), 0644)
	require.NoError(t, err)

	// Test config loading directly
	users, err := config.LoadAllowedUsers()
	require.NoError(t, err)
	require.Equal(t, []string{"123456789", "987654321"}, users)
}

func TestMain_ConfigLoading_FileNotFound(t *testing.T) {
	// Test that main.go handles missing config file properly
	tempDir := t.TempDir()
	oldCwd, _ := os.Getwd()

	// Change to temp directory (no allowed_users.json)
	err := os.Chdir(tempDir)
	require.NoError(t, err)
	defer os.Chdir(oldCwd)

	// Verify file doesn't exist
	_, err = os.Stat("allowed_users.json")
	require.True(t, os.IsNotExist(err))

	// Test config loading - should return error
	users, err := config.LoadAllowedUsers()
	require.Error(t, err)
	require.Nil(t, users)
}

func TestMain_ConfigLoading_InvalidJSON(t *testing.T) {
	// Test that main.go handles invalid JSON properly
	tempDir := t.TempDir()
	oldCwd, _ := os.Getwd()

	// Change to temp directory
	err := os.Chdir(tempDir)
	require.NoError(t, err)
	defer os.Chdir(oldCwd)

	// Create invalid JSON file
	invalidData := `["invalid", json]`
	err = os.WriteFile("allowed_users.json", []byte(invalidData), 0644)
	require.NoError(t, err)

	// Test config loading - should return error
	users, err := config.LoadAllowedUsers()
	require.Error(t, err)
	require.Nil(t, users)
}