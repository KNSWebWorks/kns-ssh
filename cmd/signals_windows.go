//go:build windows

package cmd

// resetInheritedSignals is a no-op on Windows (no POSIX signals).
func resetInheritedSignals() {}
