//go:build darwin

package librarymonitor

import (
	"bytes"
	"fmt"
	"io/fs"
	"strings"
	"syscall"

	"github.com/Silo-Server/silo-server/internal/fspath"

	"golang.org/x/sys/unix"
)

// platformSupported reports whether this build can monitor at all.
const platformSupported = true

// darwinRemoteFilesystems are the f_fstypename values whose changes FSEvents
// does not see: the server writes them, not this host. macOS names its
// filesystems rather than numbering them, so this is the equivalent of the
// statfs f_type table Linux matches on.
var darwinRemoteFilesystems = map[string]fsClass{
	"nfs":     {Name: fsNameNFS, Support: fsUnsupported},
	"smbfs":   {Name: fsNameSMB, Support: fsUnsupported},
	"afpfs":   {Name: "AFP", Support: fsUnsupported},
	"webdav":  {Name: "WebDAV", Support: fsUnsupported},
	"ftp":     {Name: "FTP", Support: fsUnsupported},
	"osxfuse": {Name: "FUSE", Support: fsMonitoredCaveat},
	"macfuse": {Name: "FUSE", Support: fsMonitoredCaveat},
}

// classifyRoot classifies the filesystem holding root by its statfs type name.
func classifyRoot(root string) (fsClass, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(root, &st); err != nil {
		return fsClass{}, fmt.Errorf("statfs %s: %w", root, err)
	}
	name := nullTerminated(st.Fstypename[:])
	if class, ok := darwinRemoteFilesystems[strings.ToLower(name)]; ok {
		return class, nil
	}
	return fsClass{Support: fsMonitored}, nil
}

// nullTerminated turns a fixed-size C string field into a Go string.
func nullTerminated(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}

// linkCount reports a file's hard link count, or 1 when it is unknown.
func linkCount(info fs.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Nlink)
	}
	return 1
}

// identityOf identifies the directory a root path names, so a remounted or
// replaced folder is recorded again.
func identityOf(info fs.FileInfo) fileID {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return fileID{dev: uint64(st.Dev), ino: st.Ino} //nolint:unconvert // Dev is int32 on darwin.
	}
	return fileID{}
}

// readMounts reads the mount table. getmntinfo returns the live table, so
// unlike Linux there is no file to parse.
func readMounts() ([]mountEntry, error) {
	var buf []unix.Statfs_t
	n, err := unix.Getfsstat(nil, unix.MNT_NOWAIT)
	if err != nil {
		return nil, err
	}
	buf = make([]unix.Statfs_t, n)
	n, err = unix.Getfsstat(buf, unix.MNT_NOWAIT)
	if err != nil {
		return nil, err
	}
	out := make([]mountEntry, 0, n)
	for i, st := range buf[:n] {
		out = append(out, mountEntry{
			id:     i,
			dev:    fmt.Sprintf("%d:%d", unix.Major(uint64(st.Fsid.Val[0])), unix.Minor(uint64(st.Fsid.Val[0]))),
			point:  fspath.Encode(nullTerminated(st.Mntonname[:])),
			fsType: nullTerminated(st.Fstypename[:]),
		})
	}
	return out, nil
}

// readMaxUserWatches has no darwin equivalent: FSEvents watches a whole
// subtree with one stream and has no per-user watch limit.
func readMaxUserWatches() int { return 0 }
