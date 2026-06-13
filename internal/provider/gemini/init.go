package gemini

import "github.com/rufatronics/velkrogo/internal/provider"

func init() {
	provider.RegisterFactory("gemini", func(key string) (provider.Provider, error) {
		return New(key), nil
	})
}
