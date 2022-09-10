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

// RootNSNode represents a root dir which will list all namespaces in
// the cluster.
// It is namespaced to RootNS as we may add support for contexts and multiple clusters later,
// so can't call this `rootNode`
type RootNSNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	cli *k8s.Clientset
	contextName string
}

func NewRootNSNode(cli *k8s.Clientset) *RootNSNode {
	return &RootNSNode{cli:cli}
}

var _ = (fs.NodeReaddirer)((*RootNSNode)(nil))

func (n *RootNSNode) ensureClientSet() error {
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
func (n *RootNSNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	fmt.Printf("READDIR: %#v\n", ctx)
	err := n.ensureClientSet()
	if err != nil {
		panic(err)
	}

	results, err := kube.GetNamespaces(ctx, n.cli)
	if err != nil {
		// The filesystem is our interface with the user, so let
		// errors here be exposed via said interface.
		entries := []fuse.DirEntry{
			{
				Name: "error",
				Ino:  uint64(9900 + rand.Intn(100)),
				Mode: fuse.S_IFREG,
			},
		}
		return fs.NewListDirStream(entries), 0
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

func (n *RootNSNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP OF RootNSNode %s' \n", name)
	err := n.ensureClientSet()
	if err != nil {
		panic(err)
	}

	// TODO Rc we need to parse the path here to get the namespace?
	// might work on absolute paths but will it work on relative paths?
	ch := n.NewInode(
		ctx,
		// TODO RC inject a layer here where we expose different resources
		&RootNSObjectsNode{
			namespace: name,
			cli: n.cli,
		},
		fs.StableAttr{Mode: syscall.S_IFDIR},
	)
	return ch, 0
}



type RootNSObjectsNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	namespace string

	cli *k8s.Clientset
}

// Ensure we are implementing the NodeReaddirer interface
var _ = (fs.NodeReaddirer)((*RootNSObjectsNode)(nil))

// // Readdir is part of the NodeReaddirer interface
func (n *RootNSObjectsNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	fmt.Printf("READDIR RootNSObjectsNode: ns: %s %#v\n", n.namespace, ctx)

	entries := []fuse.DirEntry{
		{
			Name: "pods",
			Ino:  uint64(9900 + rand.Intn(100)),
			Mode: fuse.S_IFDIR,
		},
		{
			Name: "deployments",
			Ino:  uint64(9900 + rand.Intn(100)),
			Mode: fuse.S_IFDIR,
		},
	}
	return fs.NewListDirStream(entries), 0
}

func (n *RootNSObjectsNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP OF %s on NSOBJECTSNODE: %s' \n", name, n.namespace)
	if name == "pods" {
		fmt.Printf("LOOKED UP pods: %s:%s", n.namespace, name)
		ch := n.NewInode(
			ctx,
			&RootPodNode{
				namespace: n.namespace,

				cli: n.cli,
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		)
		return ch, 0
	} else if name == "deployments" {
		fmt.Printf("LOOKED UP pods: %s:%s", n.namespace, name)
		ch := n.NewInode(
			ctx,
			&RootDeploymentNode{
				namespace: n.namespace,

				cli: n.cli, 
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		)
		return ch, 0
	} else {
		fmt.Printf("RootNSObjects lookup of unrecognised object type %v, %s", name, name)
		return nil, syscall.EROFS
	}
}
