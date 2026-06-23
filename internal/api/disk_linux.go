//go:build linux

package api

import "syscall"

// diskUsage returns the total and free size of the filesystem at path.
// Blocks/Bavail are counted in Frsize units (fundamental block size), NOT Bsize
// (preferred I/O size): on virtiofs/Docker Desktop, Bsize=1 MB while Frsize=4 KB,
// so Blocks*Bsize inflated the result by 256× (231 TB instead of ~926 GB).
func diskUsage(path string) (total, free uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	bs := uint64(st.Frsize)
	if bs == 0 {
		bs = uint64(st.Bsize)
	}
	return st.Blocks * bs, st.Bavail * bs, nil
}
