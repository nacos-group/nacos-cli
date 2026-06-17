//go:build unix

package cmd

import "syscall"

func backgroundSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
