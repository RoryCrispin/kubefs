package resources

import (
	"context"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"go.uber.org/zap"

	kube "rorycrispin.co.uk/kubefs/kubernetes"
)

// RootContextNode represents a root dir which will list all Contexts which are
// discoverable.
type RootContextNode struct {
	fs.Inode
}

func (n *RootContextNode) Path() string {
	return ""
}

func NewRootContextNode(log *zap.SugaredLogger) fs.InodeEmbedder {
	params := genericDirParams{
		stateStore: NewState(),
		log: log,
	}

	return &GenericDir{
		action: &RootContextNode{},
		params: params,
		basePath: "",
	}
}

func (n *RootContextNode) Entries(ctx context.Context, params *genericDirParams) (*dirEntries, error) {
	results, err := kube.GetK8sContexts()
	if err != nil {
		return nil, err
	}
	return &dirEntries{
		Directories: results,
	}, nil
}
func (n *RootContextNode) Entry(name string, _ *genericDirParams) (NewNode, FileMode, error) {
	return NewRootContextObjectsNode, syscall.S_IFDIR, nil
}


type RootContextObjectsNode struct {
	fs.Inode

	name string
	stateStore *State
	log *zap.SugaredLogger
}

func NewRootContextObjectsNode(params genericDirParams) (fs.InodeEmbedder, error) {
	err := checkParams(paramsSpec{
		log: true,
	stateStore: true,
	}, params)
	if err != nil {
		panic(err)
	}
	params.contextName = params.name
	return &GenericDir{
		action: &RootContextObjectsNode{},
		basePath: params.contextName,
		params: params,
	}, nil
}

func (n *RootContextObjectsNode) Entries(ctx context.Context, params *genericDirParams) (*dirEntries, error) {
	return &dirEntries{
		Directories: []string{"resources"},
	}, nil
}

func (n *RootContextObjectsNode) Entry(name string, _ *genericDirParams) (NewNode, FileMode, error) {
	if name != "resources" {
		// TODO RC should have a shared constant
		return nil, 0, eNoExists
	}
	return NewResourceTypeNode, syscall.S_IFDIR, nil
}
