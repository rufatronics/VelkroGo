package registry

import (
	"fmt"
	"sort"
	"sync"
)

// Memory is the in-process Registry implementation.
type Memory struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewMemory() *Memory {
	return &Memory{tools: map[string]Tool{}}
}

func (m *Memory) Register(t Tool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, dup := m.tools[t.Name()]; dup {
		return fmt.Errorf("registry: duplicate tool %q", t.Name())
	}
	m.tools[t.Name()] = t
	return nil
}

func (m *Memory) Get(name string) (Tool, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tools[name]
	return t, ok
}

// Enabled returns the tools exposed to the model for the given world. In saver
// mode only T0/T1 tools are advertised, trimming the prompt; higher-tier tools
// remain invokable for the user via explicit flows but are not offered to the
// model.
func (m *Memory) Enabled(world World, saverMode bool) []Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []Tool
	for _, t := range m.tools {
		if t.World() != world && t.World() != WorldShared {
			continue
		}
		if saverMode && t.Tier() > TierReversibleLocal {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}
