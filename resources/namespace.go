package resources

import (
	"context"
	"fmt"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"go.uber.org/zap"
	k8s "k8s.io/client-go/kubernetes"

	kube "rorycrispin.co.uk/kubefs/kubernetes"
)

// ListGenericNamespaceNode returns a list of namespaces. lookup of the
// namespace will reveal the list of Namespaced API resources. Hence, it's
// different from the other, deprecated, namespace list node which reveals
// well-known resources only.
type ListGenericNamespaceNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	contextName  string
	groupVersion *GroupedAPIResource

	lastError error

	cli        *k8s.Clientset
	stateStore *State
	log *zap.SugaredLogger
}

func NewListGenericNamespaceNode(
	contextName string,
	groupVersion *GroupedAPIResource,

	cli *k8s.Clientset,
	stateStore *State,
	log *zap.SugaredLogger,
) *ListGenericNamespaceNode {
	if cli == nil {
		var err error
		cli, err = kube.GetK8sClient(contextName)
		if err != nil {
			panic("TODO")
		}
	}
	if stateStore == nil || log == nil {
		return nil
	}
	return &ListGenericNamespaceNode{
		contextName: contextName,
		groupVersion: groupVersion,

		cli: cli,
		stateStore: stateStore,
		log: log,
	}
}

func (n *ListGenericNamespaceNode) Path() string {
	return fmt.Sprintf("%v/resources/%v/%v/namespaces",
		n.contextName, n.groupVersion.GroupVersion, n.groupVersion.ResourceName,
	)
}

func (n *ListGenericNamespaceNode) ensureClientSet() error {
	if n.cli != nil {
		return nil
	}
	cli, err := kube.GetK8sClient(n.contextName)
	if err != nil {
		return err
	}
	n.cli = cli
	return nil
}

// // Readdir is part of the NodeReaddirer interface
func (n *ListGenericNamespaceNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	err := n.ensureClientSet()
	if err != nil {
		panic(err)
	}

	results, err := kube.GetNamespaces(ctx, n.cli)
	if err != nil {
		n.lastError = err
		return readDirErrResponse(n.Path())
	}
	return readdirSubdirResponse(results, n.Path())
}

func (n *ListGenericNamespaceNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if name == "error" {
		// TODO rc return a file whose string contents is the error
		return nil, syscall.ENOENT
	}

	node, err := NewAPIResourceNode(n.contextName, name, n.groupVersion, n.stateStore, n.log)
	if node == nil {
		panic("TODO")
	}
	if err != nil {
		// TODO
		return nil, syscall.ENOENT
	}

	ch := n.NewInode(
		ctx,
		node,
		fs.StableAttr{
			Mode: syscall.S_IFDIR,
			Ino:  hash(fmt.Sprintf("%v/%v", n.Path(), name)),
		},
	)
	return ch, 0
}
