package preset

import "testing"

func TestAllHaveIDs(t *testing.T) {
	for _, p := range All() {
		if p.ID == "" {
			t.Errorf("preset with empty ID: %+v", p)
		}
		if p.Kind == "" {
			t.Errorf("preset %q has no kind", p.ID)
		}
	}
}

func TestByID(t *testing.T) {
	for _, want := range []string{"anthropic", "gemini", "deepseek", "groq", "mistral", "ollama", "custom"} {
		if ByID(want) == nil {
			t.Errorf("ByID(%q) returned nil", want)
		}
	}
}

func TestNoDuplicateIDs(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range All() {
		if seen[p.ID] {
			t.Errorf("duplicate preset ID: %q", p.ID)
		}
		seen[p.ID] = true
	}
}
