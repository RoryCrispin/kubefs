package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"k8s.io/client-go/kubernetes"
)

var cli *kubernetes.Clientset

type fsNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	name string
}

// Ensure we are implementing the NodeReaddirer interface
var _ = (fs.NodeReaddirer)((*fsNode)(nil))

// // Readdir is part of the NodeReaddirer interface
func (n *fsNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	fmt.Printf("READDIR: %#v\n", ctx)

	pods := getPods(cli)

	entries := make([]fuse.DirEntry, 0, len(pods))
	for i, p := range(pods) {
		if p == "" {
			continue
		}
		entries = append(entries, fuse.DirEntry{
			Name: p,
			Ino: uint64(9900 + i),
			Mode: fuse.S_IFREG,
		})
	}
	return fs.NewListDirStream(entries), 0
}


func (n *fsNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP OF %s \n", name)
	if name != "vpa-recommender-6968b769d9-z9pdz" {
		return nil, syscall.ENOENT
	}

	ch := n.NewInode(
		ctx,
		&timeFile{name: name},
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
}

// timeFile implements Open
var _ = (fs.NodeOpener)((*timeFile)(nil))

func (f *timeFile) Open(ctx context.Context, openFlags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	// disallow writes
	if fuseFlags&(syscall.O_RDWR|syscall.O_WRONLY) != 0 {
		return nil, 0, syscall.EROFS
	}

	podDef, err := getPodDefinition(cli, f.name)
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
	if err != nil {
		panic(err)
	}
	fmt.Printf("\nMOUNT AT : %v\n", mntDir)

	root := &fsNode{}
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
