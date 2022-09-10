package resources

import (
	"context"
	"fmt"
	"math/rand"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	k8s "k8s.io/client-go/kubernetes"

	kube "rorycrispin.co.uk/kubefs/kubernetes"
)

// RootContextNode represents a root dir which will list all Contexts which are
// discoverable.
type RootContextNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode
}

func NewRootContextNode(cli *k8s.Clientset) *RootContextNode {
	return &RootContextNode{}
}

var _ = (fs.NodeReaddirer)((*RootContextNode)(nil))

// // Readdir is part of the NodeReaddirer interface
func (n *RootContextNode) Readdir(_ context.Context) (fs.DirStream, syscall.Errno) {
	fmt.Printf("READDIR:\n")

	results, err := kube.GetK8sContexts()
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
			Mode: fuse.S_IFDIR,
		})
	}
	return fs.NewListDirStream(entries), 0
}

func (n *RootContextNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP OF RootContextNode %s' \n", name)

	ch := n.NewInode(
		ctx,
		&RootContextObjectsNode{
			name: name,
		},
		fs.StableAttr{Mode: syscall.S_IFDIR},
	)
	return ch, 0
}



type RootContextObjectsNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	name string
}

// Ensure we are implementing the NodeReaddirer interface
var _ = (fs.NodeReaddirer)((*RootContextObjectsNode)(nil))

// // Readdir is part of the NodeReaddirer interface
func (n *RootContextObjectsNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	fmt.Printf("READDIR RootContextObjectsNode: ns: %s %#v\n", n.name, ctx)

	entries := []fuse.DirEntry{
		{
			Name: "namespaces",
			Ino:  uint64(9900 + rand.Intn(100)),
			Mode: fuse.S_IFDIR,
		},
		{
			Name: "config",
			Ino:  uint64(9900 + rand.Intn(100)),
			Mode: fuse.S_IFDIR,
		},
	}
	return fs.NewListDirStream(entries), 0
}

func (n *RootContextObjectsNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if name == "namespaces" {
		fmt.Printf("Looked up NS on context %v", n.name)
		ch := n.NewInode(
			ctx,
			&RootNSNode{
				contextName: n.name,
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		)
		return ch, 0
	} else if name == "config" {
		fmt.Printf("Looked up config on context: %v", n.name)
	}
	fmt.Printf("LOOKUP OF %s on Context objects node: %s' \n", name, n.name)
	fmt.Printf("RootContextObjects lookup of unrecognised object type %v, %s", name, name)
	return nil, syscall.EROFS
}
