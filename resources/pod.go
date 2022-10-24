package resources

import (
	"context"
	"errors"
	"fmt"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"go.uber.org/zap"

	k8s "k8s.io/client-go/kubernetes"
	kube "rorycrispin.co.uk/kubefs/kubernetes"
)

// ========= Pod Objects Node =======

// PodObjectsNode lists the objects and actions avaliable from a Pod. for
// instance, def.json, and the containers folder which expands to allow logs and
// exec
type PodObjectsNode struct {
	fs.Inode

	namespace string
	name string
	contextName string

	cli *k8s.Clientset
	stateStore *State
	log *zap.SugaredLogger
}

func NewPodObjectsNode(
	name, namespace, contextName string,
	cli *k8s.Clientset,
	stateStore *State,
	log *zap.SugaredLogger,
) *PodObjectsNode {
	if cli == nil || stateStore == nil || log == nil {
		return nil
	}
	return &PodObjectsNode{
		namespace: namespace,
		name: name,
		contextName: contextName,

		cli: cli,
		stateStore: stateStore,
		log: log,
	}
}

func (n *PodObjectsNode) Path() uint64 {
	return hash(fmt.Sprintf("%v/%v/pods/%v",
		n.contextName, n.namespace, n.name,
	))
}

func (n *PodObjectsNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries := []fuse.DirEntry{
		{
			Name: "containers",
			Ino: hash(fmt.Sprintf("%v/containers", n.Path())),
			Mode: fuse.S_IFDIR,
		},
		{
			Name: "def.json",
			Ino: hash(fmt.Sprintf("%v/def.json", n.Path())),
			Mode: fuse.S_IFREG,
		},
		{
			Name: "def.yaml",
			Ino: hash(fmt.Sprintf("%v/def.yaml", n.Path())),
			Mode: fuse.S_IFREG,
		},
	}
	return fs.NewListDirStream(entries), 0
}

func (n *PodObjectsNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if name == "def.json" {
		node, err := NewPodJSONFile(n.name, n.namespace, n.contextName, n.cli, n.stateStore, n.log)
		if node == nil {
			panic("TODO")
		}
		if err != nil {
			n.log.Error("error while constructing PodJSONFile",
			zap.Error(err))
			return nil, syscall.ENOENT
		}
		ch := n.NewInode(
			ctx,
			node,
			fs.StableAttr{
				Mode: syscall.S_IFREG,
				Ino: hash(fmt.Sprintf("%v/json", n.Path())),
			},
		)
		return ch, 0
	} else if name == "containers" {
		node, err := NewRootContainerNode(
			n.name, n.namespace,
			n.contextName, n.cli,
			n.stateStore,
			n.log,
		)
		if node == nil {
			panic("TODO")
		}
		if err != nil {
			n.log.Error("error while constructing RootContainerNode",
			zap.Error(err))
			return nil, syscall.ENOENT
		}
		ch := n.NewInode(
			ctx,
			node,
			fs.StableAttr{
				Mode: syscall.S_IFDIR,
				Ino: hash(fmt.Sprintf("%v/containers", n.Path())),
			},
		)
		return ch, 0
	} else {
		return nil, syscall.ENOENT
	}
}


// ========== Pod JSON file ==========

type PodJSONFile struct {
	fs.Inode
	name      string
	namespace string
	contextName string

	cli *k8s.Clientset
	stateStore *State
	log *zap.SugaredLogger
}

func NewPodJSONFile(
	name, namespace, contextName string,
	cli *k8s.Clientset,
	stateStore *State,
	log *zap.SugaredLogger,
) (*PodJSONFile, error) {
	if cli == nil {
		var err error
		cli, err = kube.GetK8sClient(contextName)
		if err != nil {
			return nil, err
		}
	}
	if stateStore == nil {
		return nil, nil
	}
	return &PodJSONFile{
		name: name,
		namespace: namespace,
		contextName: contextName,
		cli: cli,
		stateStore: stateStore,
		log:log,
	}, nil
}

func (f *PodJSONFile) Open(ctx context.Context, openFlags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	if fuseFlags&(syscall.O_RDWR|syscall.O_WRONLY) != 0 {
		return nil, 0, syscall.EROFS
	}

	podDef, err := kube.GetPodDefinition(ctx, f.cli, f.name, f.namespace)
	if errors.Is(err, kube.ErrNotFound) {
		return nil, 0, syscall.ENOENT
	}
	if err != nil {
		fh = &roBytesFileHandle{
			content: []byte(fmt.Sprintf("%#v", err)),
		}
		return fh, fuse.FOPEN_DIRECT_IO, 0
	}


	fh = &roBytesFileHandle{
		content: podDef,
	}

	// Return FOPEN_DIRECT_IO so content is not cached.
	return fh, fuse.FOPEN_DIRECT_IO, 0
}
