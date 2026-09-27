//go:build windows

package main

import "golang.org/x/sys/windows"

func diskFree(path string) (uint64, error) {
	var avail, total, free uint64
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	err = windows.GetDiskFreeSpaceEx(p, &avail, &total, &free)
	return avail, err
}
