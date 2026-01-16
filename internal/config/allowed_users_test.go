package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadAllowedUsers(t *testing.T) {
	tests := []struct {
		name           string
		setup          func()
		expectedUsers  []string
		expectError    bool
		errorContains  string
	}{
		{
			name: "valid allowed users file",
			setup: func() {
				data := `["123456789", "987654321", "555666777"]`
				err := os.WriteFile("allowed_users.json", []byte(data), 0644)
				require.NoError(t, err)
			},
			expectedUsers: []string{"123456789", "987654321", "555666777"},
			expectError:   false,
		},
		{
			name: "empty allowed users file",
			setup: func() {
				data := `[]`
				err := os.WriteFile("allowed_users.json", []byte(data), 0644)
				require.NoError(t, err)
			},
			expectedUsers: []string{},
			expectError:   false,
		},
		{
			name: "file not found",
			setup: func() {
				// Ensure file doesn't exist
				os.Remove("allowed_users.json")
			},
			expectError:   true,
			errorContains: "config file not found",
		},
		{
			name: "invalid JSON",
			setup: func() {
				data := `["123", "invalid", json]`
				err := os.WriteFile("allowed_users.json", []byte(data), 0644)
				require.NoError(t, err)
			},
			expectError:   true,
			errorContains: "parsing config file",
		},
		{
			name: "non-numeric user ID",
			setup: func() {
				data := `["123", "abc"]`
				err := os.WriteFile("allowed_users.json", []byte(data), 0644)
				require.NoError(t, err)
			},
			expectError:   true,
			errorContains: "invalid user ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			if tt.setup != nil {
				tt.setup()
				defer os.Remove("allowed_users.json")
			}

			// Test
			users, err := LoadAllowedUsers()

			// Verify
			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					require.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedUsers, users)
			}
		})
	}
}

func TestLoadAllowedUsers_CurrentDir(t *testing.T) {
	// Test that it loads from current working directory
	data := `["123456789"]`
	err := os.WriteFile("allowed_users.json", []byte(data), 0644)
	require.NoError(t, err)
	defer os.Remove("allowed_users.json")

	users, err := LoadAllowedUsers()
	require.NoError(t, err)
	require.Equal(t, []string{"123456789"}, users)
}