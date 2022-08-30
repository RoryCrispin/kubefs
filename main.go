package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"errors"
	"log"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"k8s.io/client-go/kubernetes"
)

var cli *kubernetes.Clientset

// rootNSNode represents a root dir which will list all namespaces in
// the cluster.
// It is namespaced to rootNS as we may add support for contexts and multiple clusters later,
// so can't call this `rootNode`
type rootNSNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode
}
var _ = (fs.NodeReaddirer)((*rootNSNode)(nil))

// // Readdir is part of the NodeReaddirer interface
func (n *rootNSNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	fmt.Printf("READDIR: %#v\n", ctx)

	results, err := getNamespaces(ctx, cli)
	if err != nil {
		panic(err)
	}

	entries := make([]fuse.DirEntry, 0, len(results))
	for i, p := range(results) {
		if p == "" {
			continue
		}
		entries = append(entries, fuse.DirEntry{
			Name: p,
			Ino: uint64(9900 + rand.Intn(100 + i)),
			Mode: fuse.S_IFDIR,
		})
	}
	return fs.NewListDirStream(entries), 0
}

func (n *rootNSNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP OF rootNSNode %s' \n", name)

	// TODO Rc we need to parse the path here to get the namespace?
	// might work on absolute paths but will it work on relative paths?
	ch := n.NewInode(
		ctx,
		// TODO RC inject a layer here where we expose different resources
		&rootNSObjectsNode{
			namespace: name,
		},
		fs.StableAttr{Mode: syscall.S_IFDIR},
	)
	return ch, 0
}

type rootNSObjectsNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	namespace string
}
// Ensure we are implementing the NodeReaddirer interface
var _ = (fs.NodeReaddirer)((*rootNSObjectsNode)(nil))

// // Readdir is part of the NodeReaddirer interface
func (n *rootNSObjectsNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	fmt.Printf("READDIR rootNSObjectsNode: ns: %s %#v\n", n.namespace, ctx)

	entries := []fuse.DirEntry{
		{
			Name: "pods",
			Ino: uint64(9900+rand.Intn(100)),
			Mode: fuse.S_IFDIR,
		},
	}
	return fs.NewListDirStream(entries), 0
}

func (n *rootNSObjectsNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP OF %s on NSOBJECTSNODE: %s' \n", name, n.namespace)
	if name == "pods" {
		fmt.Printf("LOOKED UP pods: %s:%s", n.namespace, name)
		ch := n.NewInode(
			ctx,
			&rootPodNode{
				namespace: n.namespace,
			},
			fs.StableAttr{Mode: syscall.S_IFDIR},
		)
		return ch, 0
	} else {
		fmt.Printf("rootNSObjects lookup of unrecognised object type %v, %s", name, name)
		return nil, syscall.EROFS
	}
}

type rootPodNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	namespace string
}

// Ensure we are implementing the NodeReaddirer interface
var _ = (fs.NodeReaddirer)((*rootPodNode)(nil))

// // Readdir is part of the NodeReaddirer interface
func (n *rootPodNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	fmt.Printf("READDIR rootPodNode: %#v\n", ctx)

	pods, err := getPods(ctx, cli, n.namespace)
	if err != nil {
		panic(err)
	}

	entries := make([]fuse.DirEntry, 0, len(pods))
	for i, p := range(pods) {
		if p == "" {
			continue
		}
		entries = append(entries, fuse.DirEntry{
			Name: p,
			Ino: uint64(9900 + rand.Intn(100 + i)),
			Mode: fuse.S_IFREG,
		})
	}
	return fs.NewListDirStream(entries), 0
}


func (n *rootPodNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP OF %s' \n", name)

	// TODO Rc we need to parse the path here to get the namespace?
	// might work on absolute paths but will it work on relative paths?
	ch := n.NewInode(
		ctx,
		&timeFile{
			name: name,
			namespace: n.namespace,
		},
		fs.StableAttr{Mode: syscall.S_IFREG},
	)

	return ch, 0
}

// bytesFileHandle is a file handle that carries separate content for
// each Open call
type bytesFileHandle struct {
	content []byte
}

// bytesFileHandle allows reads
var _ = (fs.FileReader)((*bytesFileHandle)(nil))

func (fh *bytesFileHandle) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	end := off + int64(len(dest))
	if end > int64(len(fh.content)) {
		end = int64(len(fh.content))
	}

	// We could copy to the `dest` buffer, but since we have a
	// []byte already, return that.
	return fuse.ReadResultData(fh.content[off:end]), 0
}

// timeFile is a file that contains the wall clock time as ASCII.
type timeFile struct {
	fs.Inode
	name string
	namespace string
}

// timeFile implements Open
var _ = (fs.NodeOpener)((*timeFile)(nil))

func (f *timeFile) Open(ctx context.Context, openFlags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	// disallow writes
	if fuseFlags&(syscall.O_RDWR|syscall.O_WRONLY) != 0 {
		return nil, 0, syscall.EROFS
	}

	podDef, err := getPodDefinition(ctx, cli, f.name, f.namespace)
	if errors.Is(err, ErrNotFound) {
		return nil, 0, syscall.ENOENT
	}
	if err != nil {
		return nil,0, syscall.EROFS
	}

	fh = &bytesFileHandle{
		content: podDef,
	}

	// Return FOPEN_DIRECT_IO so content is not cached.
	return fh, fuse.FOPEN_DIRECT_IO, 0
}


func main() {
	cli = getK8sClient()

	mntDir, err := ioutil.TempDir("", "xoyo")
	mntDir = "/tmp/kubefs"

	if err != nil {
		panic(err)
	}
	fmt.Printf("\nMOUNT AT : %v\n", mntDir)

	root := &rootNSNode{}
	server, err := fs.Mount(mntDir, root, &fs.Options{
		MountOptions: fuse.MountOptions{
			Debug: true,
		},
	})
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Mounted on %s", mntDir)
	log.Printf("Unmount by calling 'fusermount -u %s'", mntDir)

	// Wait until unmount before exiting
	server.Wait()
}
