package resources

import (
	"context"
	"fmt"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
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
	stateStore map[uint64]interface{}
}

func (n *ListGenericNamespaceNode) Path() string {
	return fmt.Sprintf("%v/resources/%v/%v/namespaces",
		n.contextName, n.groupVersion.GroupVersion, n.groupVersion.ResourceName,
	)
}

var _ = (fs.NodeReaddirer)((*ListGenericNamespaceNode)(nil))

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

	entries := make([]fuse.DirEntry, 0, len(results))
	for _, p := range results {
		if p == "" {
			continue
		}
		entries = append(entries, fuse.DirEntry{
			Name: p,
			Ino:  hash(fmt.Sprintf("%v/%v", n.Path(), p)),
			Mode: fuse.S_IFDIR,
		})
	}
	return fs.NewListDirStream(entries), 0

}

func (n *ListGenericNamespaceNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if name == "error" {
		// TODO rc return a file whose string contents is the error
		fmt.Printf("Error is %v", n.lastError)
		return nil, syscall.ENOENT
	}

	ch := n.NewInode(
		ctx,
		&APIResourceNode{
			namespace:    name,
			contextName:  n.contextName,
			groupVersion: n.groupVersion,

			stateStore: n.stateStore,
		},
		fs.StableAttr{
			Mode: syscall.S_IFDIR,
			Ino:  hash(fmt.Sprintf("%v/%v", n.Path(), name)),
		},
	)
	return ch, 0
}
