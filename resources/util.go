package resources

import (
	"hash/fnv"
	"syscall"
	"fmt"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// hash generates a uint64 hash from a given string.
// It's useful for generating stable inode numbers.
func hash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

// readDirErrResponse is a helper for Readdir funcs. It returns a single
// entry which is a regular file named Error.
// Calling functions should store the error and present it as plaintext when
// the user looks up this Error file.
func readDirErrResponse(path string) (fs.DirStream, syscall.Errno) {
		entries := []fuse.DirEntry{
			{
				Name: "error",
				Ino:  hash(fmt.Sprintf("%v/%v", path, "error")),
				Mode: fuse.S_IFREG,
			},
		}
		return fs.NewListDirStream(entries), 0
}
