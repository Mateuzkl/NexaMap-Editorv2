//go:build windows

package app

import (
	"os"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32DLL         = windows.NewLazySystemDLL("kernel32.dll")
	procAllocConsole    = kernel32DLL.NewProc("AllocConsole")
	procFreeConsole     = kernel32DLL.NewProc("FreeConsole")
	procSetConsoleOutCP = kernel32DLL.NewProc("SetConsoleOutputCP")
	procSetConsoleTitle = kernel32DLL.NewProc("SetConsoleTitleW")

	nativeConsoleMu   sync.Mutex
	nativeConsoleOpen bool
	nativeConsoleFile *os.File
)

func openNativeConsole() (bool, error) {
	nativeConsoleMu.Lock()
	defer nativeConsoleMu.Unlock()
	if nativeConsoleOpen {
		return true, nil
	}
	ret, _, callErr := procAllocConsole.Call()
	if ret == 0 {
		// ERROR_ACCESS_DENIED (5) means the process already has a console attached.
		if errno, ok := callErr.(syscall.Errno); !ok || errno != 5 {
			return false, callErr
		}
	}
	_, _, _ = procSetConsoleOutCP.Call(65001) // UTF-8
	if titlePtr, err := windows.UTF16PtrFromString("NexaMap Debug Console"); err == nil {
		_, _, _ = procSetConsoleTitle.Call(uintptr(unsafe.Pointer(titlePtr)))
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		nativeConsoleFile = f
		os.Stdout = f
		os.Stderr = f
	}
	nativeConsoleOpen = true
	return true, nil
}

func closeNativeConsole() (bool, error) {
	nativeConsoleMu.Lock()
	defer nativeConsoleMu.Unlock()
	if !nativeConsoleOpen {
		return false, nil
	}
	if nativeConsoleFile != nil {
		_ = nativeConsoleFile.Close()
		nativeConsoleFile = nil
	}
	_, _, _ = procFreeConsole.Call()
	nativeConsoleOpen = false
	return false, nil
}

func writeNativeConsole(line string) {
	nativeConsoleMu.Lock()
	f := nativeConsoleFile
	nativeConsoleMu.Unlock()
	if f != nil {
		_, _ = f.WriteString(line + "\n")
	}
}

func isNativeConsoleOpen() bool {
	nativeConsoleMu.Lock()
	defer nativeConsoleMu.Unlock()
	return nativeConsoleOpen
}
