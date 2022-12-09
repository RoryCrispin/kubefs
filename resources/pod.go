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
}

func NewPodObjectsNode(
	params genericDirParams,
) fs.InodeEmbedder {
	err := checkParams(paramsSpec{
		pod: true,
		namespace: true,
		contextName: true,
		cli: true,
		stateStore: true,
		log: true,
	}, params)
	if err != nil {
		// TODO rc dont panic
		panic(err)
	}

	basePath := fmt.Sprintf("%v/%v/pods/%v",
		params.contextName, params.namespace, params.name,
	)

	n := GenericDir{
		action: &PodObjectsNode{},

		basePath: basePath,
		params: params,
	}
	return &n
}

func (n *PodObjectsNode) Entries(_ context.Context, _ *genericDirParams) (*dirEntries, error) {
	return &dirEntries{
		Files: []string{"def.json", "def.yaml"},
		Directories: []string{"containers"},
	}, nil
}

func (n *PodObjectsNode) Entry(name string) (NewNode, FileMode, error) {
	switch name {
	case "def.json":
		return NewPodJSONFile, syscall.S_IFREG, nil
	case "def.yaml":
		// TODO add yaml support
		return NewPodJSONFile, syscall.S_IFREG, nil
	case "containers":
		return NewRootContainerNode, syscall.S_IFDIR, nil
	default:
		return nil, 0, fmt.Errorf("%v not found | %w", name, eNoExists)
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
	params genericDirParams,
) (fs.InodeEmbedder, error) {
// -	pod, err := getArg("pod", params)
// 	if err != nil {
// 		return nil, err
// 	}
// 	namespace, err := getArg("namespace", params)
// 	if err != nil {
// 		return nil, err
// 	}
// 	contextName, err := getArg("contextName", params)
// 	if err != nil {
// 		return nil, err
// 	}

	ensureClientSet(&params)

	return &PodJSONFile{
		name: params.pod,
		namespace: params.namespace,
		contextName: params.contextName,
		cli: params.cli,
		stateStore: params.stateStore,
		log: params.log,
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
