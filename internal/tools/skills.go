package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rufatronics/velkrogo/internal/memory"
	"github.com/rufatronics/velkrogo/internal/registry"
)

// SkillsStore is the shared DB used by skills tools. Set before registering.
var SkillsStore *memory.DB

type SkillsList struct{}

func (SkillsList) Name() string         { return "skills_list" }
func (SkillsList) Description() string  { return "List all saved skills (reusable prompt snippets)." }
func (SkillsList) Tier() registry.Tier  { return registry.TierReadOnly }
func (SkillsList) World() registry.World { return registry.WorldShared }
func (SkillsList) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (SkillsList) Execute(_ context.Context, _ json.RawMessage) (registry.Result, error) {
	if SkillsStore == nil {
		return registry.Result{Content: "(no skills store)"}, nil
	}
	skills, err := SkillsStore.ListSkills()
	if err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	if len(skills) == 0 {
		return registry.Result{Content: "(no skills saved)"}, nil
	}
	var sb strings.Builder
	for _, s := range skills {
		fmt.Fprintf(&sb, "%s — %s\n", s.Name, s.Description)
	}
	return registry.Result{Content: strings.TrimSpace(sb.String())}, nil
}

type SkillsSave struct{}

func (SkillsSave) Name() string         { return "skills_save" }
func (SkillsSave) Description() string  { return "Save a reusable skill (prompt snippet) by name." }
func (SkillsSave) Tier() registry.Tier  { return registry.TierReversibleLocal }
func (SkillsSave) World() registry.World { return registry.WorldShared }
func (SkillsSave) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"description":{"type":"string"},"prompt":{"type":"string","description":"The reusable prompt or instructions"}},"required":["name","description","prompt"]}`)
}
func (SkillsSave) Execute(_ context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Prompt      string `json:"prompt"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if SkillsStore == nil {
		return registry.Result{Content: "skills store not available"}, nil
	}
	id := strings.ReplaceAll(strings.ToLower(in.Name), " ", "_")
	if err := SkillsStore.UpsertSkill(memory.Skill{
		ID:          id,
		Name:        in.Name,
		Description: in.Description,
		Prompt:      in.Prompt,
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	return registry.Result{Content: "skill saved: " + in.Name}, nil
}

type SkillsInvoke struct{}

func (SkillsInvoke) Name() string         { return "invoke_skill" }
func (SkillsInvoke) Description() string  { return "Invoke a saved skill by name — returns its prompt as context." }
func (SkillsInvoke) Tier() registry.Tier  { return registry.TierReadOnly }
func (SkillsInvoke) World() registry.World { return registry.WorldShared }
func (SkillsInvoke) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"Skill name to invoke"}},"required":["name"]}`)
}
func (SkillsInvoke) Execute(_ context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if SkillsStore == nil {
		return registry.Result{Content: "skills store not available"}, nil
	}
	skills, err := SkillsStore.ListSkills()
	if err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	for _, s := range skills {
		if strings.EqualFold(s.Name, in.Name) {
			return registry.Result{Content: s.Prompt}, nil
		}
	}
	return registry.Result{IsError: true, Content: "skill not found: " + in.Name}, nil
}

type SkillsDelete struct{}

func (SkillsDelete) Name() string         { return "skills_delete" }
func (SkillsDelete) Description() string  { return "Delete a saved skill by name." }
func (SkillsDelete) Tier() registry.Tier  { return registry.TierReversibleLocal }
func (SkillsDelete) World() registry.World { return registry.WorldShared }
func (SkillsDelete) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
}
func (SkillsDelete) Execute(_ context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if SkillsStore == nil {
		return registry.Result{Content: "skills store not available"}, nil
	}
	id := strings.ReplaceAll(strings.ToLower(in.Name), " ", "_")
	if err := SkillsStore.DeleteSkill(id); err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	return registry.Result{Content: "deleted skill: " + in.Name}, nil
}
