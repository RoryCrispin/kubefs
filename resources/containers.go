package resources

import (
	"context"
	"fmt"
	"errors"
	"math/rand"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	k8s "k8s.io/client-go/kubernetes"
	kube "rorycrispin.co.uk/kubefs/kubernetes"
)

type RootContainerNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	pod       string
	namespace string

	cli *k8s.Clientset
}

// Ensure we are implementing the NodeReaddirer interface
var _ = (fs.NodeReaddirer)((*RootContainerNode)(nil))

// // Readdir is part of the NodeReaddirer interface
func (n *RootContainerNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	fmt.Printf("READDIR RootContainerNode: %#v\n", ctx)

	results, err := kube.GetContainers(ctx, n.cli, n.pod, n.namespace)
	if err != nil {
		panic(err)
	}

	entries := make([]fuse.DirEntry, 0, len(results))
	for i, p := range results {
		if p == "" {
			continue
		}
		entries = append(entries, fuse.DirEntry{
			Name: p,
			Ino:  uint64(9900 + rand.Intn(100+i)),
			Mode: fuse.S_IFREG,
		})
	}
	return fs.NewListDirStream(entries), 0
}

func (n *RootContainerNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP OF %s' \n", name)

	ch := n.NewInode(
		ctx,
		&RootContainerObjectsNode{
			namespace: n.namespace,
			pod:       n.pod,
			name:      name,
			cli:       n.cli,
		},
		fs.StableAttr{Mode: syscall.S_IFDIR},
	)
	return ch, 0
}

// ========== RootContainerObjectsNode ======

type RootContainerObjectsNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	namespace string
	pod       string
	name      string

	cli *k8s.Clientset
}

// Ensure we are implementing the NodeReaddirer interface
var _ = (fs.NodeReaddirer)((*RootContainerObjectsNode)(nil))

func (n *RootContainerObjectsNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	fmt.Printf("READDIR RootContainerObjectsNode: ns: %s %#v\n", n.namespace, ctx)

	entries := []fuse.DirEntry{
		{
			Name: "logs",
			Ino:  uint64(9900 + rand.Intn(100)),
			Mode: fuse.S_IFREG,
		},
		{
			Name: "logs-previous",
			Ino:  uint64(9900 + rand.Intn(100)),
			Mode: fuse.S_IFDIR,
		},
	}
	return fs.NewListDirStream(entries), 0
}

func (n *RootContainerObjectsNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP OF %s on RootContainerObjectsNode: %s' \n", name, n.namespace)
	var previous bool
	if name == "logs" {
		previous = false
	} else if name == "logs-previous" {
		previous = true
	} else {
		fmt.Printf("RootContainerObjects lookup of unrecognised object type %v, %s", name, name)
		return nil, syscall.ENOENT
	}
	ch := n.NewInode(
		ctx,
		&ContainerLogsFile{
			name: n.name,
			pod:      n.pod,
			namespace: n.namespace,
			previous: previous,

			cli: n.cli,
		},
		fs.StableAttr{Mode: syscall.S_IFREG},
	)
	return ch, 0
}

// ========== Container Logs file ==========

type ContainerLogsFile struct {
	fs.Inode
	name      string
	pod       string
	namespace string
	previous bool

	cli *k8s.Clientset
}

func NewContainerLogsFile(pod, container, namespace string, cli *k8s.Clientset) *ContainerLogsFile {
	return &ContainerLogsFile{
		name:      container,
		pod:       pod,
		namespace: namespace,
		cli:       cli,
	}
}

var _ = (fs.NodeOpener)((*ContainerLogsFile)(nil))

func (f *ContainerLogsFile) Open(ctx context.Context, openFlags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	// disallow writes
	fmt.Printf("Open logs of %#v", *f)
	if fuseFlags&(syscall.O_RDWR|syscall.O_WRONLY) != 0 {
		return nil, 0, syscall.EROFS
	}

	var logs []byte
	var err error
	if f.previous {
		logs, err = kube.GetPreviousLogs(ctx, f.cli, f.pod, f.name, f.namespace)
	} else {
		logs, err = kube.GetLogs(ctx, f.cli, f.pod, f.name, f.namespace)
	}

	if errors.Is(err, kube.ErrNotFound) {
		return nil, 0, syscall.ENOENT
	}
	if err != nil {
		fh = &bytesFileHandle{
			content: []byte(fmt.Sprintf("%v", err)),
		}
		return fh, fuse.FOPEN_DIRECT_IO, 0
	}

	fh = &bytesFileHandle{
		content: logs,
	}

	// Return FOPEN_DIRECT_IO so content is not cached.
	return fh, fuse.FOPEN_DIRECT_IO, 0
}
