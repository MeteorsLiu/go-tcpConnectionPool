//go:build !linux

package connpool

func GetSysMax() int32 {
	return 1024
}
