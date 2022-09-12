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

type RootPodNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	namespace string
	contextName string

	cli *k8s.Clientset
	stateStore map[uint64]interface{}
}

func (n *RootPodNode) Path() uint64 {
	return hash(fmt.Sprintf("%v/%v/pods",
		n.contextName, n.namespace,
	))
}

// Ensure we are implementing the NodeReaddirer interface
var _ = (fs.NodeReaddirer)((*RootPodNode)(nil))

// // Readdir is part of the NodeReaddirer interface
func (n *RootPodNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	fmt.Printf("READDIR RootPodNode: %#v\n", ctx)

	pods, err := kube.GetPods(ctx, n.cli, n.namespace)
	if err != nil {
		panic(err)
	}

	entries := make([]fuse.DirEntry, 0, len(pods))
	for _, p := range pods {
		if p == "" {
			continue
		}
		entries = append(entries, fuse.DirEntry{
			Name: p,
			Ino: hash(fmt.Sprintf("%v/%v", n.Path(), p)),
			Mode: fuse.S_IFREG,
		})
	}
	return fs.NewListDirStream(entries), 0
}

func (n *RootPodNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP OF %s' \n", name)

	ch := n.NewInode(
		ctx,
		&RootPodObjectsNode{
			name: name,
			namespace: n.namespace,
			contextName: n.contextName,
			cli: n.cli,
			stateStore: n.stateStore,
		},
		fs.StableAttr{
			Mode: syscall.S_IFDIR,
			Ino: hash(fmt.Sprintf("%v/%v", n.Path(), name)),
		},
	)
	return ch, 0
}

// ========= Pod Objects Node =======

type RootPodObjectsNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	namespace string
	name string
	contextName string

	cli *k8s.Clientset
	stateStore map[uint64]interface{}
}

func (n *RootPodObjectsNode) Path() uint64 {
	return hash(fmt.Sprintf("%v/%v/pods/%v",
		n.contextName, n.namespace, n.name,
	))
}

// Ensure we are implementing the NodeReaddirer interface
var _ = (fs.NodeReaddirer)((*RootPodObjectsNode)(nil))

func (n *RootPodObjectsNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	fmt.Printf("READDIR RootPodObjectsNode: ns: %s %#v\n", n.namespace, ctx)

	entries := []fuse.DirEntry{
		{
			Name: "containers",
			Ino: hash(fmt.Sprintf("%v/containers", n.Path())),
			Mode: fuse.S_IFDIR,
		},
		{
			Name: "json",
			Ino: hash(fmt.Sprintf("%v/json", n.Path())),
			Mode: fuse.S_IFREG,
		},
	}
	return fs.NewListDirStream(entries), 0
}

func (n *RootPodObjectsNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP OF %s on RootPodObjectsNode: %s' \n", name, n.namespace)
	if name == "json" {
		fmt.Printf("LOOKED UP json: %s:%s\n", n.namespace, n.name)
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
		fmt.Printf("LOOKED UP containers: %s:%s\n", n.namespace, n.name)
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
		fmt.Printf("RootPodObjects lookup of unrecognised object type %v, %s\n", name, name)
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

// PodJSONFile implements Open
var _ = (fs.NodeOpener)((*PodJSONFile)(nil))

func (f *PodJSONFile) Open(ctx context.Context, openFlags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	if fuseFlags&(syscall.O_RDWR|syscall.O_WRONLY) != 0 {
		// disallow writes
		return nil, 0, syscall.EROFS
	}

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

