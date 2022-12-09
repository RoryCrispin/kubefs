package resources

import (
	"context"
	"fmt"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"

	kube "rorycrispin.co.uk/kubefs/kubernetes"
)

// ListGenericNamespaceNode returns a list of namespaces. lookup of the
// namespace will reveal the list of Namespaced API resources. Hence, it's
// different from the other, deprecated, namespace list node which reveals
// well-known resources only.
type ListGenericNamespaceNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode
}

func NewListGenericNamespaceNode(
	name string,
	params genericDirParams,
) fs.InodeEmbedder {
	ensureClientSet(&params)
	err := checkParams(paramsSpec{
		contextName: true,
		groupVersion: true,
		cli: true,
		log: true,
		stateStore: true,

	}, params)
	if err != nil {
		// TODO rc dont panic
		panic(err)
	}
	basePath := fmt.Sprintf("%v/resources/%v/%v/namespaces",
		params.contextName, params.groupVersion.GroupVersion(), params.groupVersion.ResourceName,
	)

	return &GenericDir{
		action: &ListGenericNamespaceNode{},
		basePath: basePath,
		params: params,
	}
}

func (n *ListGenericNamespaceNode) Entries(ctx context.Context, params *genericDirParams) (*dirEntries, error) {
	results, err := kube.GetNamespaces(ctx, params.cli)
	if err != nil {
		return nil, err
	}
	return &dirEntries{
		Directories: results,
	}, nil
}

func (n *ListGenericNamespaceNode) Entry(name string) (NewNode, FileMode, error) {
	return NewAPIResourceNode, syscall.S_IFDIR, nil
}
