// Package preset lists all built-in provider configurations. Every entry that
// uses the OpenAI-compatible wire format is served by the openaicompat adapter;
// Anthropic and Gemini use their own adapters. Custom providers are stored as
// Kind "openai-compatible" with a user-supplied base URL.
package preset

// Preset describes a known provider that users can enable with just an API key.
type Preset struct {
	ID           string   // machine-readable slug used in config
	Name         string   // display name shown to users
	Kind         string   // "anthropic" | "openai-compatible" | "gemini"
	BaseURL      string   // empty means the adapter's own default
	DefaultModel string   // suggested model for first-time users
	Models       []string // popular models to show in the picker
	NeedsKey     bool     // false for local providers (Ollama, LM Studio)
	KeyEnvHint   string   // env var name to advertise in the UI
	Website      string   // shown in the settings help text
}

// All returns every built-in preset in display order.
func All() []Preset {
	return []Preset{
		{
			ID: "anthropic", Name: "Anthropic (Claude)", Kind: "anthropic",
			DefaultModel: "claude-sonnet-4-6",
			Models:       []string{"claude-fable-5", "claude-opus-4-8", "claude-sonnet-4-6", "claude-haiku-4-5-20251001"},
			NeedsKey: true, KeyEnvHint: "ANTHROPIC_API_KEY",
			Website: "https://console.anthropic.com",
		},
		{
			ID: "openai", Name: "OpenAI (GPT)", Kind: "openai-compatible",
			BaseURL:      "https://api.openai.com/v1",
			DefaultModel: "gpt-4o",
			Models:       []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "o1", "o3-mini"},
			NeedsKey: true, KeyEnvHint: "OPENAI_API_KEY",
			Website: "https://platform.openai.com",
		},
		{
			ID: "gemini", Name: "Google Gemini", Kind: "gemini",
			DefaultModel: "gemini-2.0-flash",
			Models:       []string{"gemini-2.5-pro", "gemini-2.0-flash", "gemini-1.5-pro", "gemini-1.5-flash"},
			NeedsKey: true, KeyEnvHint: "GEMINI_API_KEY",
			Website: "https://aistudio.google.com",
		},
		{
			ID: "deepseek", Name: "DeepSeek", Kind: "openai-compatible",
			BaseURL:      "https://api.deepseek.com/v1",
			DefaultModel: "deepseek-chat",
			Models:       []string{"deepseek-chat", "deepseek-reasoner"},
			NeedsKey: true, KeyEnvHint: "DEEPSEEK_API_KEY",
			Website: "https://platform.deepseek.com",
		},
		{
			ID: "groq", Name: "Groq (ultra-fast)", Kind: "openai-compatible",
			BaseURL:      "https://api.groq.com/openai/v1",
			DefaultModel: "llama-3.3-70b-versatile",
			Models:       []string{"llama-3.3-70b-versatile", "llama-3.1-8b-instant", "mixtral-8x7b-32768", "gemma2-9b-it"},
			NeedsKey: true, KeyEnvHint: "GROQ_API_KEY",
			Website: "https://console.groq.com",
		},
		{
			ID: "mistral", Name: "Mistral AI", Kind: "openai-compatible",
			BaseURL:      "https://api.mistral.ai/v1",
			DefaultModel: "mistral-large-latest",
			Models:       []string{"mistral-large-latest", "mistral-small-latest", "codestral-latest", "open-mixtral-8x22b"},
			NeedsKey: true, KeyEnvHint: "MISTRAL_API_KEY",
			Website: "https://console.mistral.ai",
		},
		{
			ID: "together", Name: "Together AI", Kind: "openai-compatible",
			BaseURL:      "https://api.together.xyz/v1",
			DefaultModel: "meta-llama/Llama-3-70b-chat-hf",
			Models:       []string{"meta-llama/Llama-3-70b-chat-hf", "mistralai/Mixtral-8x22B-Instruct-v0.1", "Qwen/Qwen2.5-72B-Instruct-Turbo"},
			NeedsKey: true, KeyEnvHint: "TOGETHER_API_KEY",
			Website: "https://api.together.ai",
		},
		{
			ID: "perplexity", Name: "Perplexity AI", Kind: "openai-compatible",
			BaseURL:      "https://api.perplexity.ai",
			DefaultModel: "llama-3.1-sonar-large-128k-online",
			Models:       []string{"llama-3.1-sonar-large-128k-online", "llama-3.1-sonar-small-128k-online", "llama-3.1-70b-instruct"},
			NeedsKey: true, KeyEnvHint: "PERPLEXITY_API_KEY",
			Website: "https://www.perplexity.ai/settings/api",
		},
		{
			ID: "xai", Name: "xAI (Grok)", Kind: "openai-compatible",
			BaseURL:      "https://api.x.ai/v1",
			DefaultModel: "grok-3",
			Models:       []string{"grok-3", "grok-3-mini", "grok-2"},
			NeedsKey: true, KeyEnvHint: "XAI_API_KEY",
			Website: "https://console.x.ai",
		},
		{
			ID: "fireworks", Name: "Fireworks AI", Kind: "openai-compatible",
			BaseURL:      "https://api.fireworks.ai/inference/v1",
			DefaultModel: "accounts/fireworks/models/llama-v3p1-70b-instruct",
			Models:       []string{"accounts/fireworks/models/llama-v3p1-70b-instruct", "accounts/fireworks/models/mixtral-8x22b-instruct"},
			NeedsKey: true, KeyEnvHint: "FIREWORKS_API_KEY",
			Website: "https://fireworks.ai",
		},
		{
			ID: "cohere", Name: "Cohere", Kind: "openai-compatible",
			BaseURL:      "https://api.cohere.com/compatibility/v1",
			DefaultModel: "command-r-plus",
			Models:       []string{"command-r-plus", "command-r", "command"},
			NeedsKey: true, KeyEnvHint: "COHERE_API_KEY",
			Website: "https://dashboard.cohere.com",
		},
		{
			ID: "openrouter", Name: "OpenRouter (multi-model)", Kind: "openai-compatible",
			BaseURL:      "https://openrouter.ai/api/v1",
			DefaultModel: "anthropic/claude-sonnet-4-6",
			Models:       []string{"anthropic/claude-sonnet-4-6", "openai/gpt-4o", "google/gemini-2.0-flash", "meta-llama/llama-3.3-70b-instruct"},
			NeedsKey: true, KeyEnvHint: "OPENROUTER_API_KEY",
			Website: "https://openrouter.ai/keys",
		},
		{
			ID: "ollama", Name: "Ollama (local, free)", Kind: "openai-compatible",
			BaseURL:      "http://localhost:11434/v1",
			DefaultModel: "llama3.2",
			Models:       []string{"llama3.2", "llama3.1", "mistral", "codellama", "qwen2.5"},
			NeedsKey: false,
			Website:  "https://ollama.com",
		},
		{
			ID: "lmstudio", Name: "LM Studio (local, free)", Kind: "openai-compatible",
			BaseURL:      "http://localhost:1234/v1",
			DefaultModel: "local-model",
			Models:       []string{"local-model"},
			NeedsKey: false,
			Website:  "https://lmstudio.ai",
		},
		{
			ID: "cerebras", Name: "Cerebras (fast inference)", Kind: "openai-compatible",
			BaseURL:      "https://api.cerebras.ai/v1",
			DefaultModel: "llama3.1-70b",
			Models:       []string{"llama3.1-70b", "llama3.1-8b"},
			NeedsKey: true, KeyEnvHint: "CEREBRAS_API_KEY",
			Website: "https://cloud.cerebras.ai",
		},
		// Sentinel for the custom option shown in the wizard.
		{
			ID: "custom", Name: "Custom (any OpenAI-compatible)", Kind: "openai-compatible",
			NeedsKey: false,
			Website:  "",
		},
	}
}

// ByID returns the preset with the given ID, or nil.
func ByID(id string) *Preset {
	for _, p := range All() {
		if p.ID == id {
			cp := p
			return &cp
		}
	}
	return nil
}
