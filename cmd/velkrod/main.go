// Command velkrod is the VelkroGo daemon: a long-running local process that
// hosts the agent loop, policy gate, scheduler, and state, and serves the
// HTTP+WS local API that the GUI and TUI connect to. See ARCHITECTURE.md §4.
//
// This is a Phase 0 entry point — it wires nothing yet; it exists so the module
// builds and the layout is real.
package main

import (
	"log"
)

func main() {
	log.Println("velkrod: VelkroGo daemon (Phase 0 skeleton) — not yet wired")
}
