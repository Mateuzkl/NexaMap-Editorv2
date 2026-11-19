//go:build !windows

package app

import "os"

func openNativeConsole() (bool, error)  { return false, nil }
func closeNativeConsole() (bool, error) { return false, nil }
func writeNativeConsole(line string)    { _, _ = os.Stderr.WriteString(line + "\n") }
func isNativeConsoleOpen() bool         { return false }
