package resources

import (
	"context"
	"syscall"
	"fmt"
	"encoding/json"
	"bytes"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	kube "rorycrispin.co.uk/kubefs/kubernetes"
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
	writtenSize := uint32(len(data))

	return writtenSize, 0
}

func (fh *rwBytesFileHandle) Setattr(ctx context.Context, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	fmt.Printf("rwBytesFileSetattr \n")
	return 0
}

func (fh *rwBytesFileHandle) Fsync(ctx context.Context, flags uint32) syscall.Errno {
	fmt.Printf("rwBytesFileFsync, flags: %b\n", flags)
	return 0
}

// ========== Error file ==========

type ErrorFile struct {
	fs.Inode

	err error

	stateStore *State
}

func (f *ErrorFile) Open(ctx context.Context, openFlags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	if fuseFlags&(syscall.O_RDWR|syscall.O_WRONLY) != 0 {
		// disallow writes
		return nil, 0, syscall.EROFS
	}

	fh = &roBytesFileHandle{
		content: []byte(fmt.Sprintf("%v", f.err)),
	}
	return fh, fuse.FOPEN_DIRECT_IO, 0
}

// ============= Editable JSON File ==========
//


type editableJSONFileHandle struct {
	content *unstructured.Unstructured

	buf bytes.Buffer

	name         string
	namespace    string
	contextName  string
	groupVersion *GroupedAPIResource

	lastError  error
	cli        *k8s.Clientset
	stateStore *State

}

type safeContent struct {
	UnlockEdit bool `json:"unlockForEdit"`
	Content *unstructured.Unstructured `json:"content"`
}

func (fh *editableJSONFileHandle) GetSafeContent() ([]byte, error) {
	rv := safeContent{
		Content: fh.content,
	}
	return json.MarshalIndent(rv, "", "    ")

}

func (fh *editableJSONFileHandle) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	fmt.Printf(">> Read editableJSONFileHandle")
	content, err := fh.GetSafeContent()
	if err != nil {
		// TODO rc
		panic(err)
	}
	end := off + int64(len(dest))
	if end > int64(len(content)) {
		end = int64(len(content))
	}

	// We could copy to the `dest` buffer, but since we have a
	// []byte already, return that.
	return fuse.ReadResultData(content[off:end]), 0
}

func (fh *editableJSONFileHandle) Write(ctx context.Context, data []byte, off int64) (written uint32, errno syscall.Errno) {
	fmt.Printf("rwBytesFileWrite == %v\n", string(data))
	if off == 0 {
		fh.buf = *bytes.NewBuffer(data)
	} else {
		fh.buf.Write(data)
	}

	complete := &safeContent{}
	err := json.Unmarshal(fh.buf.Bytes(), complete)
	if err != nil {
		fmt.Printf("JSON Marshal Error - assume we haven't finished writing the file yet... %v\n", err)
	} else {
		if !complete.UnlockEdit {
			return 0, syscall.EROFS
		}
		_, err := kube.WriteUnstructured(
			ctx, fh.contextName, fh.name, fh.namespace,
			fh.groupVersion.GVR(),
			complete.Content,
		)
		if err != nil {
			fmt.Printf("Error while writing: %v\n", err)
			return 0, syscall.ESTALE
		}
	}

	writtenSize := uint32(len(data))
	fmt.Printf("WROTE at offset %v = writtenSize was %v\n", off, writtenSize)
	return writtenSize, 0
}

func (fh *editableJSONFileHandle) Setattr(ctx context.Context, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	fmt.Printf("rwBytesFileSetattr \n")
	return 0
}

func (fh *editableJSONFileHandle) Fsync(ctx context.Context, flags uint32) syscall.Errno {
	fmt.Printf("rwBytesFileFsync, flags: %b\n", flags)
	return 0
}
