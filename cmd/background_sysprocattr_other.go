//go:build !unix && !windows

package cmd

import "syscall"

func backgroundSysProcAttr() *syscall.SysProcAttr {
	return nil
}
