package anthropic

import "github.com/rufatronics/velkrogo/internal/provider"

func init() {
	provider.RegisterFactory("anthropic", func(key, baseURL string) (provider.Provider, error) {
		return New(key, baseURL), nil
	})
}
