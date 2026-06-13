package openaicompat

import "github.com/rufatronics/velkrogo/internal/provider"

func init() {
	provider.RegisterFactory("openai-compatible", func(name, key, baseURL string) (provider.Provider, error) {
		return New(name, key, baseURL), nil
	})
}
