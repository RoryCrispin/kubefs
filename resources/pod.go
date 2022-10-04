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

// ========= Pod Objects Node =======

// PodObjectsNode lists the objects and actions avaliable from a Pod. for
// instance, def.json, and the containers folder which expands to allow logs and
// exec
type PodObjectsNode struct {
	fs.Inode

	namespace string
	name string
	contextName string

	cli *k8s.Clientset
	stateStore map[uint64]any
}

func (n *PodObjectsNode) Path() uint64 {
	return hash(fmt.Sprintf("%v/%v/pods/%v",
		n.contextName, n.namespace, n.name,
	))
}

// Ensure we are implementing the NodeReaddirer interface
var _ = (fs.NodeReaddirer)((*PodObjectsNode)(nil))

func (n *PodObjectsNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries := []fuse.DirEntry{
		{
			Name: "containers",
			Ino: hash(fmt.Sprintf("%v/containers", n.Path())),
			Mode: fuse.S_IFDIR,
		},
		{
			Name: "def.json",
			Ino: hash(fmt.Sprintf("%v/def.json", n.Path())),
			Mode: fuse.S_IFREG,
		},
		{
			Name: "def.yaml",
			Ino: hash(fmt.Sprintf("%v/def.yaml", n.Path())),
			Mode: fuse.S_IFREG,
		},
	}
	return fs.NewListDirStream(entries), 0
}

func (n *PodObjectsNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if name == "def.json" {
		ch := n.NewInode(
			ctx,
			&PodJSONFile{
				name: n.name,
				namespace: n.namespace,
				contextName: n.contextName,

				cli: n.cli,
				stateStore: n.stateStore,
			},
			fs.StableAttr{
				Mode: syscall.S_IFREG,
				Ino: hash(fmt.Sprintf("%v/json", n.Path())),
			},
		)
		return ch, 0
	} else if name == "containers" {
		ch := n.NewInode(
			ctx,
			&RootContainerNode{
				pod: n.name,
				namespace: n.namespace,
				contextName: n.contextName,

				cli: n.cli,
				stateStore: n.stateStore,
			},
			fs.StableAttr{
				Mode: syscall.S_IFDIR,
				Ino: hash(fmt.Sprintf("%v/containers", n.Path())),
			},
		)
		return ch, 0
	} else {
		return nil, syscall.ENOENT
	}
}


// ========== Pod JSON file ==========

type PodJSONFile struct {
	fs.Inode
	name      string
	namespace string
	contextName string

	cli *k8s.Clientset
	stateStore map[uint64]interface{}
}

func (f *PodJSONFile) ensureCLI() error {
	if f.cli != nil {
		return nil
	}
	cli, err := kube.GetK8sClient(f.contextName)
	if err != nil {
		return err
	}
	f.cli = cli
	return nil
}

func (f *PodJSONFile) Open(ctx context.Context, openFlags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	if fuseFlags&(syscall.O_RDWR|syscall.O_WRONLY) != 0 {
		// disallow writes
		return nil, 0, syscall.EROFS
	}

	f.ensureCLI()

	podDef, err := kube.GetPodDefinition(ctx, f.cli, f.name, f.namespace)
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
		content: podDef,
	}

	// Return FOPEN_DIRECT_IO so content is not cached.
	return fh, fuse.FOPEN_DIRECT_IO, 0
}
