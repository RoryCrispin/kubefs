package resources

import (
	"context"
	"fmt"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	kube "rorycrispin.co.uk/kubefs/kubernetes"
)

// RootContextNode represents a root dir which will list all Contexts which are
// discoverable.
type RootContextNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	stateStore *State
}

func (n *RootContextNode) Path() string {
	return ""
}

func NewRootContextNode() *RootContextNode {
	fmt.Printf(">>> Creating new statestore\n")
	s := NewState()
	return &RootContextNode{
		stateStore: s,
	}
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
	for _, p := range results {
		if p == "" {
			continue
		}
		entries = append(entries, fuse.DirEntry{
			Name: p,
			Ino: hash(p),
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
			stateStore: n.stateStore,
		},
		fs.StableAttr{
			Mode: syscall.S_IFDIR,
			Ino: hash(name),
		},
	)
	return ch, 0
}



type RootContextObjectsNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	name string
	stateStore *State
}

func (n *RootContextObjectsNode) Path() string {
	return n.name
}

func (n *RootContextObjectsNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	fmt.Printf("READDIR RootContextObjectsNode: ns: %s %#v\n", n.name, ctx)

	entries := []fuse.DirEntry{
		{
			Name: "resources",
			Ino: hash(fmt.Sprintf("%v/resources", n.Path())),
			Mode: fuse.S_IFDIR,
		},
		{
			Name: "config",
			Ino: hash(fmt.Sprintf("%v/config", n.Path())),
			Mode: fuse.S_IFDIR,
		},
	}
	return fs.NewListDirStream(entries), 0
}

func (n *RootContextObjectsNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if name == "resources" {
		ch := n.NewInode(
			ctx,
			&ResourceTypeNode{
				contextName: n.name,
				stateStore: n.stateStore,
			},
			fs.StableAttr{
				Mode: syscall.S_IFDIR,
				Ino: hash(fmt.Sprintf("%v/resources", n.Path())),
			},
		)
		return ch, 0
	} else if name == "config" {
		fmt.Printf("Looked up config on context: %v", n.name)
	}
	fmt.Printf("LOOKUP OF %s on Context objects node: %s' \n", name, n.name)
	fmt.Printf("RootContextObjects lookup of unrecognised object type %v, %s", name, name)
	return nil, syscall.ENOENT
}
