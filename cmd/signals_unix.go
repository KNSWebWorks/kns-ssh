//go:build !windows

package cmd

import (
	"os"
	"os/signal"
	"syscall"
)

// resetInheritedSignals clears SIG_IGN dispositions inherited from the
// parent process. When the agent is started in the background (nohup, &,
// some service managers), SIGINT/SIGQUIT/SIGTSTP arrive ignored — and the
// ignore disposition survives exec() into bash and its children, making
// Ctrl+C / Ctrl+Z useless in every spawned shell. Taking these signals with
// signal.Notify gives the agent a proper disposition; exec() then resets it
// to default in the child, so shells behave normally.
func resetInheritedSignals() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTSTP)
	go func() {
		for range ch {
			// Swallow: Ctrl+C/Ctrl+Z belong to the PTY shells, not the agent.
		}
	}()
}
