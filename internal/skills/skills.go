package skills

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// SkillMetadata contains the minimal info needed for skill discovery.
type SkillMetadata struct {
	Name        string
	Description string
	Path        string
}

// Skill contains full skill data including instructions.
type Skill struct {
	SkillMetadata
	Instructions string
}

// frontmatter represents the YAML frontmatter structure.
type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

var nameRegex = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ParseSkill parses a SKILL.md content string into a Skill.
func ParseSkill(content, path string) (*Skill, error) {
	fm, body, err := parseFrontmatter(content)
	if err != nil {
		return nil, err
	}

	if err := validateFrontmatter(fm); err != nil {
		return nil, err
	}

	return &Skill{
		SkillMetadata: SkillMetadata{
			Name:        fm.Name,
			Description: fm.Description,
			Path:        path,
		},
		Instructions: body,
	}, nil
}

// ParseMetadata parses only the metadata from a SKILL.md, skipping instructions.
func ParseMetadata(content, path string) (*SkillMetadata, error) {
	fm, _, err := parseFrontmatter(content)
	if err != nil {
		return nil, err
	}

	if err := validateFrontmatter(fm); err != nil {
		return nil, err
	}

	return &SkillMetadata{
		Name:        fm.Name,
		Description: fm.Description,
		Path:        path,
	}, nil
}

func parseFrontmatter(content string) (*frontmatter, string, error) {
	if !strings.HasPrefix(content, "---") {
		return nil, "", fmt.Errorf("missing frontmatter: file must start with ---")
	}

	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return nil, "", fmt.Errorf("invalid frontmatter: missing closing ---")
	}

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(parts[1]), &fm); err != nil {
		return nil, "", fmt.Errorf("invalid yaml: %w", err)
	}

	body := strings.TrimPrefix(parts[2], "\n")
	return &fm, body, nil
}

func validateFrontmatter(fm *frontmatter) error {
	if fm.Name == "" {
		return fmt.Errorf("missing required field: name")
	}
	if fm.Description == "" {
		return fmt.Errorf("missing required field: description")
	}
	if !nameRegex.MatchString(fm.Name) {
		return fmt.Errorf("invalid name: must be lowercase alphanumeric with single hyphens, got %q", fm.Name)
	}
	return nil
}
