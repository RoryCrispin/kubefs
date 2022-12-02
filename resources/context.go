package resources

import (
	"context"
	"fmt"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"go.uber.org/zap"

	kube "rorycrispin.co.uk/kubefs/kubernetes"
)

// RootContextNode represents a root dir which will list all Contexts which are
// discoverable.
type RootContextNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	stateStore *State
	log *zap.SugaredLogger
}

func (n *RootContextNode) Path() string {
	return ""
}

func NewRootContextNode(log *zap.SugaredLogger) *RootContextNode {
	log.Debug("Creating new statestore")
	s := NewState()
	return &RootContextNode{
		stateStore: s,
		log: log,
	}
}

func (n *RootContextNode) Readdir(_ context.Context) (fs.DirStream, syscall.Errno) {
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
	node := NewRootContextObjectsNode(
			name,

			n.stateStore,
			n.log,
		)
	if node == nil {
		panic("TODO")
	}
	ch := n.NewInode(
		ctx,
		node,
		fs.StableAttr{
			Mode: syscall.S_IFDIR,
			Ino: hash(name),
		},
	)
	return ch, 0
}


type RootContextObjectsNode struct {
	fs.Inode

	name string
	stateStore *State
	log *zap.SugaredLogger
}

func NewRootContextObjectsNode(name string, stateStore *State, log *zap.SugaredLogger) *RootContextObjectsNode{
	if stateStore == nil || log == nil {
		return nil
	}
	return &RootContextObjectsNode{
		name: name,
		stateStore: stateStore,
		log: log,
	}
}

func (n *RootContextObjectsNode) Path() string {
	return n.name
}

func (n *RootContextObjectsNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
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
		node := NewResourceTypeNode(
				n.name,
				n.stateStore,
				n.log,
			)
		if node == nil {
			panic("TODO")
		}
		ch := n.NewInode(
			ctx,
			node,
			fs.StableAttr{
				Mode: syscall.S_IFDIR,
				Ino: hash(fmt.Sprintf("%v/resources", n.Path())),
			},
		)
		return ch, 0
	} else if name == "config" {
		n.log.Info("config is not yet supported", zap.String("contextName", n.name))
		return nil, syscall.ENOENT
	}
	n.log.Error("lookup of unrecognised object type",
		zap.String("type", name),
		zap.String("contextName", n.name))

	return nil, syscall.ENOENT
}
