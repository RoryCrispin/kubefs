package resources

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	k8s "k8s.io/client-go/kubernetes"
	kube "rorycrispin.co.uk/kubefs/kubernetes"
)

type RootContainerNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	pod       string
	namespace string
	contextName string

	cli *k8s.Clientset
	stateStore map[uint64]interface{}
}

func (n *RootContainerNode) Path() string {
	return fmt.Sprintf("%v/%v/pods/%v",
		n.contextName, n.namespace, n.pod,
	)
}

// Ensure we are implementing the NodeReaddirer interface
var _ = (fs.NodeReaddirer)((*RootContainerNode)(nil))

// // Readdir is part of the NodeReaddirer interface
func (n *RootContainerNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	fmt.Printf("READDIR RootContainerNode: %#v\n", ctx)

	results, err := kube.GetContainers(ctx, n.cli, n.pod, n.namespace)
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
			Ino:  hash(fmt.Sprintf("%v/%v", n.Path(), p)),
			Mode: fuse.S_IFREG,
		})
	}
	return fs.NewListDirStream(entries), 0
}

func (n *RootContainerNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP OF %s' \n", name)

	ch := n.NewInode(
		ctx,
		&RootContainerObjectsNode{
			namespace: n.namespace,
			pod:       n.pod,
			name:      name,
			contextName: n.contextName,

			cli:       n.cli,
			stateStore: n.stateStore,
		},
		fs.StableAttr{
			Mode: syscall.S_IFDIR,
			Ino: hash(fmt.Sprintf("%v/%v", n.Path(), name)),
		},
	)
	return ch, 0
}

// ========== RootContainerObjectsNode ======

type RootContainerObjectsNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	namespace string
	pod       string
	name      string
	contextName string

	cli *k8s.Clientset
	stateStore map[uint64]interface{}
}

// Ensure we are implementing the NodeReaddirer interface
func (n *RootContainerObjectsNode) Path() string {
	return fmt.Sprintf("%v/%v/pods/%v/%v",
		n.contextName, n.namespace, n.pod, n.name,
	)
}


var _ = (fs.NodeReaddirer)((*RootContainerObjectsNode)(nil))
func (n *RootContainerObjectsNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	fmt.Printf("READDIR RootContainerObjectsNode: ns: %s %#v\n", n.namespace, ctx)

	entries := []fuse.DirEntry{
		{
			Name: "logs",
			Ino: hash(fmt.Sprintf("%v/logs", n.Path())),
			Mode: fuse.S_IFREG,
		},
		{
			Name: "logs-previous",
			Ino: hash(fmt.Sprintf("%v/logs-previous", n.Path())),
			Mode: fuse.S_IFDIR,
		},
		{
			Name: "exec",
			Ino: hash(fmt.Sprintf("%v/exec", n.Path())),
			Mode: fuse.S_IFDIR,
		},
	}
	return fs.NewListDirStream(entries), 0
}

func (n *RootContainerObjectsNode) mkContainerExecFile(ctx context.Context) *fs.Inode {
	inode := hash(fmt.Sprintf("%v/exec", n.Path()))

	var node *ContainerExecFile

	elem, exist := n.stateStore[inode]
	if exist {
		var ok bool
		node, ok = elem.(*ContainerExecFile)
		if !ok {
			panic("failed type assertion")
		} else {
			fmt.Printf("><><> reusing container exec file: %v, %v\n", inode, node.name)
		}
	} else  {
		fmt.Printf("<<><><> Creating new container exec file\n")
		node = &ContainerExecFile{
				name: n.name,
				pod:      n.pod,
				namespace: n.namespace,
				contextName: n.contextName,

				cli: n.cli,
				stateStore: n.stateStore,
			}
		n.stateStore[inode] = node

	}

	return n.NewInode(
		ctx, node,
		fs.StableAttr{
			Mode: syscall.S_IFREG,
			Ino: inode,
		},
	)

}

func (n *RootContainerObjectsNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP OF %s on RootContainerObjectsNode: %s' \n", name, n.namespace)
	var previous bool
	if name == "exec" {
		ch := n.mkContainerExecFile(ctx)
		fmt.Printf(">> Assign inode %v to exec file for container %v of pod %v\n", ch.String(), n.name, n.pod)
		return ch, 0
	}
	if name == "logs" {
		previous = false
	} else if name == "logs-previous" {
		previous = true
	} else {
		fmt.Printf("RootContainerObjects lookup of unrecognised object type %v, %s\n", name, name)
		return nil, syscall.ENOENT
	}
	ch := n.NewInode(
		ctx,
		&ContainerLogsFile{
			name: n.name,
			pod:      n.pod,
			namespace: n.namespace,
			previous: previous,
			contextName: n.contextName,

			cli: n.cli,
			stateStore: n.stateStore,
		},
		fs.StableAttr{
			Mode: syscall.S_IFREG,
			Ino: hash(fmt.Sprintf("%v/%v", n.Path(), name)),
		},
	)
	return ch, 0
}

// ========== Container Logs file ==========

type ContainerLogsFile struct {
	fs.Inode
	name      string
	pod       string
	namespace string
	previous bool
	contextName string

	cli *k8s.Clientset
	stateStore map[uint64]interface{}
}

var _ = (fs.NodeOpener)((*ContainerLogsFile)(nil))

func (f *ContainerLogsFile) Open(ctx context.Context, openFlags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	fmt.Printf("Open logs\n")
	// disallow writes
	if fuseFlags&(syscall.O_RDWR|syscall.O_WRONLY) != 0 {
		return nil, 0, syscall.EROFS
	}

	var logs []byte
	var err error
	if f.previous {
		logs, err = kube.GetPreviousLogs(ctx, f.cli, f.pod, f.name, f.namespace)
	} else {
		logs, err = kube.GetLogs(ctx, f.cli, f.pod, f.name, f.namespace)
	}

	if errors.Is(err, kube.ErrNotFound) {
		return nil, 0, syscall.ENOENT
	}
	if err != nil {
		fh = &roBytesFileHandle{
			content: []byte(fmt.Sprintf("%v", err)),
		}
		return fh, fuse.FOPEN_DIRECT_IO, 0
	}

	fh = &roBytesFileHandle{
		content: logs,
	}

	// Return FOPEN_DIRECT_IO so content is not cached.
	return fh, fuse.FOPEN_DIRECT_IO, 0
}

// ========== Container Exec file ==========

type ContainerExecFile struct {
	fs.Inode
	name      string
	pod       string
	namespace string
	contextName string

	// When file systems are mutable, all access must use
	// synchronization.
	mu      sync.Mutex
	content []byte
	mtime   time.Time

	cli *k8s.Clientset
	stateStore map[uint64]interface{}
}

var _ = (fs.NodeAccesser)((*ContainerExecFile)(nil))
// Access reports whether a directory can be accessed by the caller.
func (fdn *ContainerExecFile) Access(ctx context.Context, mask uint32) syscall.Errno {
	// TODO: parse the mask and return a more correct value instead of always
	// granting permission.
	return syscall.F_OK
}

var _ = (fs.NodeOpener)((*ContainerExecFile)(nil))

func (f *ContainerExecFile) Open(ctx context.Context, openFlags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	fh = &rwBytesFileHandle{}
	fmt.Printf("OPEN exec file for container %v on pod == %v'\n", f.name, f.pod)

	return fh, fuse.FOPEN_DIRECT_IO, 0
}

// Implement handleless write.
var _ = (fs.NodeWriter)((*ContainerExecFile)(nil))

func (bn *ContainerExecFile) Write(ctx context.Context, fh fs.FileHandle, buf []byte, off int64) (uint32, syscall.Errno) {
	if off != 0 {
		fmt.Printf("Write with offset neq 0 (was %v)\n", off)
	}

	cmd := strings.Split(strings.TrimSpace(string(buf)), " ")
	stdOut, stdErr, err := kube.ExecCommand(
		ctx,
		bn.contextName,
		bn.pod, bn.name, bn.namespace,
		cmd,
	)
	fmt.Printf("stdout: %v, stderr: %v\n", stdOut, stdErr)
	if err != nil {
		eout := fmt.Errorf("err while executing: %w", err)
		fmt.Print(eout, "\n")
		bn.content = []byte(fmt.Sprint(eout))
		return 0, syscall.EREMOTEIO
	}
	bn.mu.Lock()
	defer bn.mu.Unlock()

	sz := int64(len(buf))
	// if off+sz > int64(len(bn.content)) {
	// 	// bn.resize(uint64(off + sz))
	// }
	copy(bn.content[off:], buf)
	bn.mtime = time.Now()

	bn.content = stdOut

	// We report back to the filesytem that the number of bytes sent to us were
	// written, even though we stored the response in the file.
	return uint32(sz), 0
}

var _ = (fs.NodeReader)((*ContainerExecFile)(nil))

func (bn *ContainerExecFile) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	fmt.Printf(">> execfile READ offset=%v : %v\n", off, string(bn.content))
	bn.mu.Lock()
	defer bn.mu.Unlock()

	end := off + int64(len(dest))
	if end > int64(len(bn.content)) {
		end = int64(len(bn.content))
	}

	return fuse.ReadResultData(bn.content[off:end]), 0
}
