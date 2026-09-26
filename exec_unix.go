//go:build aix || android || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package main

import "syscall"

// replaceProcess replaces caph with the configured harness while preserving the
// terminal and standard file descriptors.
func replaceProcess(path string, argv, env []string) error {
	return syscall.Exec(path, argv, env)
}
