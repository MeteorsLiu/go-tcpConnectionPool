//go:build linux

package pool

import (
	"syscall"
)

func GetSysMax() int32 {
	var SIZE syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &SIZE)
	if err != nil {
		return 1024
	}
	return int32(SIZE.Max)
}
