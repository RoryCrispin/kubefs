package resources

import (
	"context"
	"syscall"
	"fmt"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// roBytesFileHandle is a file handle that carries separate content for
// each Open call
type roBytesFileHandle struct {
	content []byte
}

var _ = (fs.FileReader)((*roBytesFileHandle)(nil))

func (fh *roBytesFileHandle) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	fmt.Printf(">> Read roBytesFileHandle")
	end := off + int64(len(dest))
	if end > int64(len(fh.content)) {
		end = int64(len(fh.content))
	}

	return fuse.ReadResultData(fh.content[off:end]), 0
}

type rwBytesFileHandle struct {
	content []byte
}

var _ = (fs.FileReader)((*rwBytesFileHandle)(nil))

func (fh *rwBytesFileHandle) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	fmt.Printf(">> Read rwBytesFileHandle")
	end := off + int64(len(dest))
	if end > int64(len(fh.content)) {
		end = int64(len(fh.content))
	}

	// We could copy to the `dest` buffer, but since we have a
	// []byte already, return that.
	return fuse.ReadResultData(fh.content[off:end]), 0
}

func (fh *rwBytesFileHandle) Write(ctx context.Context, data []byte, off int64) (written uint32, errno syscall.Errno) {
	fmt.Printf("rwBytesFileWrite == %v\n", string(data))
	return 0, 0
}

func (fh *rwBytesFileHandle) Setattr(ctx context.Context, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	fmt.Printf("rwBytesFileSetattr \n")
	return 0
}

func (fh *rwBytesFileHandle) Fsync(ctx context.Context, flags uint32) syscall.Errno {
	fmt.Printf("rwBytesFileFsync, flags: %b\n", flags)
	return 0
}
