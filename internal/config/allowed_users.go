package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
)

const configFile = "allowed_users.json"

// LoadAllowedUsers reads and parses allowed_users.json from the current directory.
// Returns a slice of user IDs or an error if the file can't be read or parsed.
func LoadAllowedUsers() ([]string, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cwd, _ := os.Getwd()
			return nil, fmt.Errorf("config file not found: %s (cwd: %s)", configFile, cwd)
		}
		return nil, fmt.Errorf("reading config file %s: %w", configFile, err)
	}

	var users []string
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", configFile, err)
	}

	// Validate user IDs are numeric (Discord snowflake format)
	for i, uid := range users {
		if _, err := strconv.ParseUint(uid, 10, 64); err != nil {
			return nil, fmt.Errorf("invalid user ID at index %d: %q (Discord IDs must be numeric)", i, uid)
		}
	}

	return users, nil
}