//go:build !linux

package pool

func GetSysMax() int32 {
	return 1024
}
