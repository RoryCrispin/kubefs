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

// RootNSNode represents a root dir which will list all namespaces in
// the cluster.
// It is namespaced to RootNS as we may add support for contexts and multiple clusters later,
// so can't call this `rootNode`
type RootNSNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	contextName string
	lastError error

	cli *k8s.Clientset
	stateStore map[uint64]interface{}
}

func (n *RootNSNode) Path() string {
	return fmt.Sprintf("%v",
		n.contextName,
	)
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
		n.lastError = err
		entries := []fuse.DirEntry{
			{
				Name: "error",
				Ino:  hash(fmt.Sprintf("%v/%v", n.Path(), "error")),
				Mode: fuse.S_IFREG,
			},
		}
		return fs.NewListDirStream(entries), 0
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

func (n *RootNSNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP OF RootNSNode %s' \n", name)
	err := n.ensureClientSet()
	if err != nil {
		// TODO rc return err
		panic(err)
	}

	ch := n.NewInode(
		ctx,
		&RootNSObjectsNode{
			namespace: name,
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



type RootNSObjectsNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	namespace string
	contextName string

	cli *k8s.Clientset
	stateStore map[uint64]interface{}
}

func (n *RootNSObjectsNode) Path() string {
	return fmt.Sprintf("%v/%v",
		n.contextName, n.namespace,
	)
}

func (n *RootNSObjectsNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	fmt.Printf("READDIR RootNSObjectsNode: ns: %s %#v\n", n.namespace, ctx)

	entries := []fuse.DirEntry{
		{
			Name: "pods",
			Ino:  hash(fmt.Sprintf("%v/pods", n.Path())),
			Mode: fuse.S_IFDIR,
		},
		{
			Name: "deployments",
			Ino:  hash(fmt.Sprintf("%v/deployments", n.Path())),
			Mode: fuse.S_IFDIR,
		},
	}
	return fs.NewListDirStream(entries), 0
}

func (n *RootNSObjectsNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP OF %s on NSOBJECTSNODE: %s' \n", name, n.namespace)
	if name == "pods" || name == "po" {
		fmt.Printf("LOOKED UP pods: %s:%s\n", n.namespace, name)
		ch := n.NewInode(
			ctx,
			&RootPodNode{
				namespace: n.namespace,
				contextName: n.contextName,

				cli: n.cli,
				stateStore: n.stateStore,
			},
			fs.StableAttr{
				Mode: syscall.S_IFDIR,
				Ino: hash(fmt.Sprintf("%v/deployments", n.Path())),
			},
		)
		return ch, 0
	} else if name == "deployments" || name == "deploy" {
		fmt.Printf("LOOKED UP pods: %s:%s\n", n.namespace, name)
		ch := n.NewInode(
			ctx,
			&RootDeploymentNode{
				namespace: n.namespace,
				contextName: n.contextName,

				cli: n.cli,
				stateStore: n.stateStore,
			},
			fs.StableAttr{
				Mode: syscall.S_IFDIR,
				Ino: hash(fmt.Sprintf("%v/deployments", n.Path())),
			},
		)
		return ch, 0
	} else {
		fmt.Printf("RootNSObjects lookup of unrecognised object type %v, %s\n", name, name)
		return nil, syscall.ENOENT
	}
}
