package resources

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"go.uber.org/zap"

	k8s "k8s.io/client-go/kubernetes"
	kube "rorycrispin.co.uk/kubefs/kubernetes"
)

type RootContainerNode struct {
	fs.Inode

	pod       string
	namespace string
	contextName string

	cli *k8s.Clientset
	stateStore *State
	log *zap.SugaredLogger
}

func NewRootContainerNode(
	pod       string,
	namespace string,
	contextName string,

	cli *k8s.Clientset,
	stateStore *State,
	log *zap.SugaredLogger,
) (*RootContainerNode, error) {
	if cli == nil {
		var err error
		cli, err = kube.GetK8sClient(contextName)
		if err != nil {
			return nil, err
		}
	}
	return &RootContainerNode{
		pod: pod,
		namespace:namespace,
		contextName: contextName,

		cli: cli,
		stateStore: stateStore,
		log: log,
	}, nil
}

func (n *RootContainerNode) Path() string {
	return fmt.Sprintf("%v/%v/pods/%v",
		n.contextName, n.namespace, n.pod,
	)
}


func (n *RootContainerNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
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
	node := NewRootContainerObjectsNode(
		n.namespace,
		n.pod, name,
		n.contextName,
		n.cli, n.stateStore,
		n.log)
	if node == nil {
		panic("TODO")
	}
	ch := n.NewInode(
		ctx,
		node,
		fs.StableAttr{
			Mode: syscall.S_IFDIR,
			Ino: hash(fmt.Sprintf("%v/%v", n.Path(), name)),
		},
	)
	return ch, 0
}

// ========== RootContainerObjectsNode ======

type RootContainerObjectsNode struct {
	fs.Inode

	namespace string
	pod       string
	name      string
	contextName string

	cli *k8s.Clientset
	stateStore *State
	log *zap.SugaredLogger
}

func NewRootContainerObjectsNode(
	namespace string,
	pod       string,
	name      string,
	contextName string,

	cli *k8s.Clientset,
	stateStore *State,
	log *zap.SugaredLogger,
) *RootContainerObjectsNode {
	if cli == nil || stateStore == nil || log == nil {
		return nil
	}
	return &RootContainerObjectsNode{
		namespace: namespace,
		pod: pod,
		name: name,
		contextName: contextName,

		cli: cli,
		stateStore: stateStore,
		log: log,
	}
}

// Ensure we are implementing the NodeReaddirer interface
func (n *RootContainerObjectsNode) Path() string {
	return fmt.Sprintf("%v/%v/pods/%v/%v",
		n.contextName, n.namespace, n.pod, n.name,
	)
}

var _ = (fs.NodeReaddirer)((*RootContainerObjectsNode)(nil))
func (n *RootContainerObjectsNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
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
	stateKey := fmt.Sprintf("%v/exec", n.Path())

	var node *ContainerExecFile

	elem, exist := n.stateStore.Get(stateKey)
	if exist {
		var ok bool
		node, ok = elem.(*ContainerExecFile)
		if !ok {
			panic("failed type assertion")
		}
	} else  {
		n.log.Debug("creating new container exec file",
		zap.String("name", n.name),
		)

		node = NewContainerExecFile(
			n.name,
			n.pod,
			n.namespace,
			n.contextName,

			n.cli,
			n.stateStore,
			n.log,
		)
		n.stateStore.Put(stateKey, node)
	}

	return n.NewInode(
		ctx, node,
		fs.StableAttr{
			Mode: syscall.S_IFREG,
			Ino: hash(stateKey),
		},
	)

}

func (n *RootContainerObjectsNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	var previous bool
	if name == "exec" {
		ch := n.mkContainerExecFile(ctx)
		n.log.Debug("assign new inode",
			zap.String("inode", ch.String()),
			zap.String("name", n.name),
			zap.String("pod", n.pod),
		)
		return ch, 0
	}
	if name == "logs" {
		previous = false
	} else if name == "logs-previous" {
		previous = true
	} else {
		n.log.Error("RootContainerObjects lookup of unrecognised object type",
			zap.String("name", name),
		)
		return nil, syscall.ENOENT
	}
	node, err := NewContainerLogsFile(
		n.name, n.pod, n.namespace, previous, n.contextName,
		n.cli, n.stateStore, n.log)
	if node == nil {
		panic("TODO")
	}
	if err != nil {
		n.log.Error("RootContainerObjects wrror while constructing RootContainerLogsFile",
			zap.String("name", name), zap.Error(err),
		)
		return nil, syscall.ENOENT
	}
	ch := n.NewInode(
		ctx,
		node,
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
	stateStore *State
	log *zap.SugaredLogger
}

func NewContainerLogsFile(
	name      string,
	pod       string,
	namespace string,
	previous bool,
	contextName string,

	cli *k8s.Clientset,
	stateStore *State,
	log *zap.SugaredLogger,
) (*ContainerLogsFile, error) {
	if cli == nil {
		var err error
		cli, err = kube.GetK8sClient(contextName)
		if err != nil {
			return nil, err
		}
	}
	if log == nil || stateStore == nil {
		return nil, nil
	}
	return &ContainerLogsFile{
		name: name,
		pod: pod,
		namespace: namespace,
		previous: previous,
		contextName: contextName,

		cli: cli,
		stateStore: stateStore,
		log: log,
	}, nil
}

func (f *ContainerLogsFile) Open(ctx context.Context, openFlags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
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
		f.log.Warn("err not found while opening", zap.String("file", f.name))
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

	cli *k8s.Clientset
	stateStore *State
	log *zap.SugaredLogger
}

func NewContainerExecFile(
	name      string,
	pod       string,
	namespace string,
	contextName string,

	cli *k8s.Clientset,
	stateStore *State,
	log *zap.SugaredLogger,
) *ContainerExecFile {
	if cli == nil || stateStore == nil {
		return nil
	}
	return &ContainerExecFile{
		name: name,
		pod: pod,
		namespace: namespace,
		contextName: contextName,
		cli: cli,
		stateStore: stateStore,
		log:log,
	}
}

// Access reports whether a directory can be accessed by the caller.
func (fdn *ContainerExecFile) Access(ctx context.Context, mask uint32) syscall.Errno {
	// TODO: parse the mask and return a more correct value instead of always
	// granting permission.
	return syscall.F_OK
}

func (f *ContainerExecFile) Open(ctx context.Context, openFlags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	fh = &rwBytesFileHandle{}

	return fh, fuse.FOPEN_DIRECT_IO, 0
}

func (bn *ContainerExecFile) Write(ctx context.Context, fh fs.FileHandle, buf []byte, off int64) (uint32, syscall.Errno) {
	if off != 0 {
		panic("TODO support exec large chunks.")
	}

	cmd := strings.Split(strings.TrimSpace(string(buf)), " ")
	stdOut, stdErr, err := kube.ExecCommand(
		ctx,
		bn.contextName,
		bn.pod, bn.name, bn.namespace,
		cmd,
	)
	bn.log.Info(
		zap.ByteString("stdout", stdOut),
		zap.ByteString("stderr", stdErr),
	)
	if err != nil {
		eout := fmt.Errorf("err while executing: %w", err)
		bn.content = []byte(fmt.Sprint(eout))

		//TODO use EREMOTEIO on linux arch
		//return 0, syscall.EREMOTEIO
		return 0, syscall.ENOENT
	}
	bn.mu.Lock()
	defer bn.mu.Unlock()

	sz := int64(len(buf))
	// if off+sz > int64(len(bn.content)) {
	// 	// bn.resize(uint64(off + sz))
	// }
	copy(bn.content[off:], buf)

	bn.content = stdOut

	return uint32(sz), 0
}

func (bn *ContainerExecFile) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	bn.mu.Lock()
	defer bn.mu.Unlock()

	end := off + int64(len(dest))
	if end > int64(len(bn.content)) {
		end = int64(len(bn.content))
	}

	return fuse.ReadResultData(bn.content[off:end]), 0
}
