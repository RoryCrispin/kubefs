package resources

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
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

	cli *k8s.Clientset
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
	for i, p := range pods {
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

func (n *RootDeploymentNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP OF deployment %s' \n", name)

	// TODO Rc we need to parse the path here to get the namespace?
	// might work on absolute paths but will it work on relative paths?
	ch := n.NewInode(
		ctx,
		&deployJSONFile{
			name:      name,
			namespace: n.namespace,

			cli: n.cli,
		},
		fs.StableAttr{Mode: syscall.S_IFREG},
	)

	return ch, 0
}

// deployJSONFile is a file that contains the wall clock time as ASCII.
type deployJSONFile struct {
	fs.Inode
	name      string
	namespace string

	cli *k8s.Clientset
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
		return nil, 0, syscall.EROFS
	}

	fh = &bytesFileHandle{
		content: def,
	}

	// Return FOPEN_DIRECT_IO so content is not cached.
	return fh, fuse.FOPEN_DIRECT_IO, 0
}