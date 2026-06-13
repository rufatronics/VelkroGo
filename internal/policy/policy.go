// Package policy is the single chokepoint for side effects. Every tool call is
// evaluated here against the granted scopes and the tool's risk tier before it
// is allowed to run. See ARCHITECTURE.md §5.3.
package policy

import (
	"time"

	"github.com/rufatronics/velkrogo/internal/registry"
)

// Decision is the policy verdict for a proposed tool call.
type Decision int

const (
	Allow Decision = iota // run without prompting
	Ask                   // prompt the user (with a preview) before running
	Deny                  // refuse
)

// Scope narrows a grant (e.g. a path prefix, repo, host, or "*").
type Scope string

// Grant authorises a capability within a scope until it expires. Grants are
// revocable at any time.
type Grant struct {
	Capability string    // tool name or capability group, e.g. "fs.write"
	Scope      Scope     // "~/proj", "github.com/me/repo", "*"
	Expiry     time.Time // zero == session lifetime
}

// Request describes a proposed action evaluated by the engine.
type Request struct {
	Tool      registry.Tool
	Target    string // resolved target (path, url, host) for scope matching
	SaverMode bool
	Preview   string // exact command / diff / HTTP request shown to the user
}

// Engine evaluates requests and manages grants. T2+ actions always surface a
// preview; T4 always asks regardless of grants.
type Engine interface {
	Evaluate(req Request, grants []Grant) Decision
	AddGrant(g Grant)
	Revoke(capability string, scope Scope)
	RevokeAll()
}
