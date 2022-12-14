package resources

import (
	"context"
	"errors"
	"fmt"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	k8s "k8s.io/client-go/kubernetes"

	kube "rorycrispin.co.uk/kubefs/kubernetes"
)

// ========== Generic JSON file ==========

type GenericJSONFile struct {
	fs.Inode

	name         string
	namespace    string
	contextName  string
	groupVersion *GroupedAPIResource

	lastError  error
	cli        *k8s.Clientset
	stateStore *State
}

// GenericJSONFile implements Open
var _ = (fs.NodeOpener)((*GenericJSONFile)(nil))

func (f *GenericJSONFile) Open(ctx context.Context, openFlags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	if fuseFlags&(syscall.O_RDWR|syscall.O_WRONLY) != 0 {
		// disallow writes
		return nil, 0, syscall.EROFS
	}

	if f.groupVersion == nil {
		fh = &roBytesFileHandle{
			content: []byte(fmt.Sprintf("error while opening genericJSONFile, groupVersion ptr was nil\n")),
		}
		return fh, fuse.FOPEN_DIRECT_IO, 0
	}

	content, err := kube.GetUnstructured(
		ctx, f.contextName, f.name,
		f.groupVersion.Group, f.groupVersion.Version, f.groupVersion.ResourceName, f.namespace,
	)

	if errors.Is(err, kube.ErrNotFound) {
		return nil, 0, syscall.ENOENT
	}

	if err != nil {
		fh = &roBytesFileHandle{
			content: []byte(fmt.Sprintf("%#v", err)),
		}
		return fh, fuse.FOPEN_DIRECT_IO, 0
	}

	fh = &roBytesFileHandle{
		content: content,
	}

	return fh, fuse.FOPEN_DIRECT_IO, 0
}

// ========== GenericEditable JSON file ==========

type GenericEditableJSONFile struct {
	fs.Inode

	name         string
	namespace    string
	contextName  string
	groupVersion *GroupedAPIResource

	lastError  error
	cli        *k8s.Clientset
	stateStore *State
}

// GenericEditableJSONFile implements Open
var _ = (fs.NodeOpener)((*GenericEditableJSONFile)(nil))

func (f *GenericEditableJSONFile) Open(ctx context.Context, openFlags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	if fuseFlags&(syscall.O_RDWR|syscall.O_WRONLY) != 0 {
		// disallow writes
		return nil, 0, syscall.EROFS
	}

	if f.groupVersion == nil {
		fh = &roBytesFileHandle{
			content: []byte(fmt.Sprintf("error while opening genericJSONFile, groupVersion ptr was nil\n")),
		}
		return fh, fuse.FOPEN_DIRECT_IO, 0
	}

	content, err := kube.GetUnstructuredRaw(
		ctx, f.contextName, f.name, f.namespace,
		f.groupVersion.GVR(),
	)

	if errors.Is(err, kube.ErrNotFound) {
		return nil, 0, syscall.ENOENT
	}
	if err != nil {
		fh = &roBytesFileHandle{
			content: []byte(fmt.Sprintf("%#v", err)),
		}
		return fh, fuse.FOPEN_DIRECT_IO, 0
	}

	fh = &editableJSONFileHandle{
		content: content,

		name: f.name,
		namespace: f.namespace,
		contextName: f.contextName,
		groupVersion: f.groupVersion,

		stateStore: f.stateStore,
	}

	return fh, fuse.FOPEN_DIRECT_IO, 0
}

func (fdn *GenericEditableJSONFile) Access(ctx context.Context, mask uint32) syscall.Errno {
	// TODO: parse the mask and return a more correct value instead of always
	// granting permission.
	return syscall.F_OK
}

