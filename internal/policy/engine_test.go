package policy

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/rufatronics/velkrogo/internal/registry"
)

type fakeTool struct {
	name string
	tier registry.Tier
}

func (f fakeTool) Name() string             { return f.name }
func (f fakeTool) Description() string      { return f.name }
func (f fakeTool) Tier() registry.Tier      { return f.tier }
func (f fakeTool) World() registry.World    { return registry.WorldShared }
func (f fakeTool) Schema() json.RawMessage  { return json.RawMessage(`{}`) }
func (f fakeTool) Execute(context.Context, json.RawMessage) (registry.Result, error) {
	return registry.Result{}, nil
}

func TestT0AutoAllows(t *testing.T) {
	e := NewBasic()
	if d := e.Evaluate(Request{Tool: fakeTool{"read_file", registry.TierReadOnly}}, nil); d != Allow {
		t.Fatalf("T0 should auto-allow, got %v", d)
	}
}

func TestUngrantedAsks(t *testing.T) {
	e := NewBasic()
	if d := e.Evaluate(Request{Tool: fakeTool{"write_file", registry.TierReversibleLocal}}, nil); d != Ask {
		t.Fatalf("ungranted T1 should ask, got %v", d)
	}
}

func TestGrantScopeAndRevoke(t *testing.T) {
	e := NewBasic()
	e.AddGrant(Grant{Capability: "write_file", Scope: "/home/u/proj"})

	in := Request{Tool: fakeTool{"write_file", registry.TierReversibleLocal}, Target: "/home/u/proj/a.go"}
	out := Request{Tool: fakeTool{"write_file", registry.TierReversibleLocal}, Target: "/etc/passwd"}

	if e.Evaluate(in, nil) != Allow {
		t.Fatal("in-scope grant should allow")
	}
	if e.Evaluate(out, nil) != Ask {
		t.Fatal("out-of-scope target should still ask")
	}

	e.Revoke("write_file", "/home/u/proj")
	if e.Evaluate(in, nil) != Ask {
		t.Fatal("revoked grant should ask again")
	}
}

func TestExpiredGrantAsks(t *testing.T) {
	e := NewBasic()
	e.AddGrant(Grant{Capability: "write_file", Scope: "*", Expiry: time.Now().Add(-time.Minute)})
	if e.Evaluate(Request{Tool: fakeTool{"write_file", registry.TierReversibleLocal}}, nil) != Ask {
		t.Fatal("expired grant should ask")
	}
}

func TestSelfModifyAlwaysAsks(t *testing.T) {
	e := NewBasic()
	e.AddGrant(Grant{Capability: "*", Scope: "*"})
	if e.Evaluate(Request{Tool: fakeTool{"self_edit", registry.TierSelfModify}}, nil) != Ask {
		t.Fatal("T4 must ask even with a blanket grant")
	}
}
