package main

import (
	"bufio"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

var (
	kernel32      = windows.NewLazySystemDLL("kernel32.dll")
	attachConsole = kernel32.NewProc("AttachConsole")
	allocConsole  = kernel32.NewProc("AllocConsole")

	getConsoleWindow = kernel32.NewProc("GetConsoleWindow")

	user32              = windows.NewLazySystemDLL("user32.dll")
	getAsyncKeyState    = user32.NewProc("GetAsyncKeyState")
	setForegroundWindow = user32.NewProc("SetForegroundWindow")
)

// vkReturn is the Enter key's virtual-key code.
const vkReturn = 0x0D

// attachParentProcess is AttachConsole's ATTACH_PARENT_PROCESS ((DWORD)-1).
const attachParentProcess = ^uintptr(0) & 0xFFFFFFFF

// showIntro shows intro on Windows, where the service has no console of
// its own (a GUI program, so the service manager opens no window): in the
// terminal it was started from, else (opened from Explorer) in a console
// window of its own that waits for Enter.
func showIntro() {
	// Output that goes somewhere already (a pipe, a file, a console build's
	// console) gets it there.
	if _, err := os.Stdout.Stat(); err == nil {
		var mode uint32
		h := windows.Handle(os.Stdout.Fd())
		color := windows.GetConsoleMode(h, &mode) == nil &&
			windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING|windows.ENABLE_PROCESSED_OUTPUT) == nil
		intro(os.Stdout, color)
		return
	}
	own := false
	if r, _, _ := attachConsole.Call(attachParentProcess); r == 0 {
		// One window at a time: should Enter still reach Explorer and open
		// the file again, the second copy shows nothing.
		name, _ := windows.UTF16PtrFromString(`Local\FirefoxVPNTunIntro`)
		mutex, err := windows.CreateMutex(nil, false, name)
		if err == windows.ERROR_ALREADY_EXISTS {
			return
		}
		if mutex != 0 {
			defer windows.CloseHandle(mutex)
		}
		if r, _, _ := allocConsole.Call(); r == 0 {
			return
		}
		own = true
	}
	out, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		return
	}
	defer out.Close()
	color := false
	var mode uint32
	h := windows.Handle(out.Fd())
	if windows.GetConsoleMode(h, &mode) == nil {
		color = windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING|windows.ENABLE_PROCESSED_OUTPUT) == nil
	}
	if own {
		_, _ = out.WriteString("\x1b]0;Firefox VPN TUN service\x07")
		// A new console window opens behind the window that was in front
		// (Explorer), which then gets the Enter meant for it and opens
		// the file again: bring it to the front. Explorer started this
		// program on the user's action, so Windows lets it.
		if w, _, _ := getConsoleWindow.Call(); w != 0 {
			_, _, _ = setForegroundWindow.Call(w)
		}
	}
	intro(out, color)
	if own {
		_, _ = out.WriteString("  Press Enter to close. ")
		if in, err := os.OpenFile("CONIN$", os.O_RDWR, 0); err == nil {
			_, _ = bufio.NewReader(in).ReadString('\n')
			in.Close()
		}
		// Close only once Enter is let go: the window closes on the key's
		// press, and a key still held repeats into Explorer, where the
		// file is still selected, and opens it again.
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
			if r, _, _ := getAsyncKeyState.Call(vkReturn); r&0x8000 == 0 {
				break
			}
		}
	}
}
