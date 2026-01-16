package config

import (
	"encoding/json"
	"os"
)

// LoadAllowedUsers reads and parses allowed_users.json from the project root.
// Returns a slice of user IDs or an error if the file can't be read or parsed.
func LoadAllowedUsers() ([]string, error) {
	data, err := os.ReadFile("allowed_users.json")
	if err != nil {
		return nil, err
	}

	var users []string
	err = json.Unmarshal(data, &users)
	if err != nil {
		return nil, err
	}

	return users, nil
}