package policy

import (
	"strings"
	"sync"
	"time"

	"github.com/rufatronics/velkrogo/internal/registry"
)

// Basic is the Phase 1/2 Engine implementation: T0 auto-allows, T4 always asks,
// and everything in between asks unless a matching unexpired grant exists.
type Basic struct {
	mu     sync.RWMutex
	grants []Grant
}

func NewBasic() *Basic { return &Basic{} }

func (b *Basic) Evaluate(req Request, extra []Grant) Decision {
	tier := req.Tool.Tier()
	if tier == registry.TierReadOnly {
		return Allow
	}
	// Self-modification can never be pre-granted; it always asks.
	if tier == registry.TierSelfModify {
		return Ask
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, g := range append(b.grants, extra...) {
		if g.matches(req.Tool.Name(), req.Target) {
			return Allow
		}
	}
	return Ask
}

func (g Grant) matches(capability, target string) bool {
	if !g.Expiry.IsZero() && time.Now().After(g.Expiry) {
		return false
	}
	if g.Capability != capability && g.Capability != "*" {
		return false
	}
	return g.Scope == "*" || g.Scope == "" || strings.HasPrefix(target, string(g.Scope))
}

func (b *Basic) AddGrant(g Grant) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.grants = append(b.grants, g)
}

func (b *Basic) Revoke(capability string, scope Scope) {
	b.mu.Lock()
	defer b.mu.Unlock()
	kept := b.grants[:0]
	for _, g := range b.grants {
		if g.Capability != capability || (scope != "*" && g.Scope != scope) {
			kept = append(kept, g)
		}
	}
	b.grants = kept
}

func (b *Basic) RevokeAll() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.grants = nil
}
