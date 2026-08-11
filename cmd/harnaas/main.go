// Command harnaas manages a project's AI-harness assets as a declared,
// versioned dependency of the repository.
package main

import (
	"fmt"
	"os"
)

// main is a placeholder so the module builds while the scaffolding lands.
// The process contract it owes — version loading, the cancellable root
// context, two-stage signal handling, the error switch and the exit-code
// mapping — replaces this body wholesale.
func main() {
	fmt.Fprintln(os.Stderr, "harnaas: no commands are wired up yet")
	os.Exit(1)
}
