package main

import "syscall"

// detachedProcAttr returns process attributes for a detached child process on macOS.
func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
