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
type RootDeploymentNode struct {

	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	namespace string
	contextName string

	cli *k8s.Clientset
	stateStore map[uint64]interface{}
}

func (n *RootDeploymentNode) Path() string {
	return fmt.Sprintf("%v/%v/deployments",
		n.contextName, n.namespace,
	)
}

// Ensure we are implementing the NodeReaddirer interface
var _ = (fs.NodeReaddirer)((*RootDeploymentNode)(nil))

// // Readdir is part of the NodeReaddirer interface
func (n *RootDeploymentNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	fmt.Printf("READDIR rootDeploymentNode: %#v\n", ctx)

	pods, err := kube.GetDeployments(ctx, n.cli, n.namespace)
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
			Ino:  hash(fmt.Sprintf("%v/%v", n.Path(), p)),
			Mode: fuse.S_IFREG,
		})
	}
	return fs.NewListDirStream(entries), 0
}

func (n *RootDeploymentNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP OF deployment %s' \n", name)

	// TODO Rc we need to parse the path here to get the namespace?
	// might work on absolute paths but will it work on relative paths?
	ch := n.NewInode(
		ctx,
		&deployJSONFile{
			name:      name,
			namespace: n.namespace,
			contextName: n.contextName,

			cli: n.cli,
			stateStore: n.stateStore,
		},
		fs.StableAttr{
			Mode: syscall.S_IFREG,
			Ino: hash(fmt.Sprintf("%v/name", n.Path())),
		},
	)

	return ch, 0
}

type deployJSONFile struct {
	fs.Inode
	name      string
	namespace string
	contextName string

	cli *k8s.Clientset
	stateStore map[uint64]interface{}
}

// deployJSONFile implements Open
var _ = (fs.NodeOpener)((*deployJSONFile)(nil))

func (f *deployJSONFile) Open(ctx context.Context, openFlags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	// disallow writes
	if fuseFlags&(syscall.O_RDWR|syscall.O_WRONLY) != 0 {
		return nil, 0, syscall.EROFS
	}

	def, err := kube.GetDeploymentDefinition(ctx, f.cli, f.name, f.namespace)
	if errors.Is(err, kube.ErrNotFound) {
		return nil, 0, syscall.ENOENT
	}
	if err != nil {
		fmt.Printf("Encountered error while getting deployment definition: %v", err)
		return nil, 0, syscall.ENOENT
	}

	fh = &roBytesFileHandle{
		content: def,
	}

	// Return FOPEN_DIRECT_IO so content is not cached.
	return fh, fuse.FOPEN_DIRECT_IO, 0
}
