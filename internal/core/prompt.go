package core

import "github.com/TheLazyLemur/claudecord/internal/skills"

// BuildSystemPrompt appends skill XML to a base prompt.
func BuildSystemPrompt(base string, store skills.SkillStore) string {
	if store == nil {
		return base
	}
	skillList, _ := store.List()
	if len(skillList) == 0 {
		return base
	}
	xml := skills.GenerateSkillsXML(skillList)
	if base == "" {
		return xml
	}
	return base + "\n\n" + xml
}
