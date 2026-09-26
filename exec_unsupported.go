//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package main

import (
	"errors"
	"runtime"
)

func replaceProcess(_ string, _, _ []string) error {
	return errors.New("process replacement is not supported on " + runtime.GOOS)
}
