//go:build !linux

package api

import "syscall"

// diskUsage for non-Linux (local builds on macOS): Darwin has no Frsize field,
// and Blocks are already expressed in Bsize units, so Bsize is correct here.
func diskUsage(path string) (total, free uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	bs := uint64(st.Bsize)
	return st.Blocks * bs, st.Bavail * bs, nil
}
