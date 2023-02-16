package resources

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"go.uber.org/zap"
	k8s "k8s.io/client-go/kubernetes"

	kube "rorycrispin.co.uk/kubefs/kubernetes"
)

var (
	eNoExists      = fmt.Errorf("does not exist")
	eParamsMissing = fmt.Errorf("params missing")
)

// hash generates a uint64 hash from a given string.
// It's useful for generating stable inode numbers.
func hash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

// readDirErrResponse is a helper for Readdir funcs. It returns a single
// entry which is a regular file named Error.
// Calling functions should store the error and present it as plaintext when
// the user looks up this Error file.
func readDirErrResponse(path string) (fs.DirStream, syscall.Errno) {
	entries := []fuse.DirEntry{
		{
			Name: "error",
			Ino:  hash(fmt.Sprintf("%v/%v", path, "error")),
			Mode: fuse.S_IFREG,
		},
	}
	return fs.NewListDirStream(entries), 0
}

func readdirResponse(e *dirEntries, basePath string) (fs.DirStream, syscall.Errno) {
	if e == nil {
		panic("TODO")
	}
	rv := []fuse.DirEntry{}
	for _, p := range e.Files {
		if p == "" {
			continue
		}
		rv = append(rv, fuse.DirEntry{
			Name: p,
			Ino:  hash(fmt.Sprintf("%v/%v", basePath, p)),
			Mode: fuse.S_IFREG,
		})
	}
	for _, p := range e.Directories {
		if p == "" {
			continue
		}
		rv = append(rv, fuse.DirEntry{
			Name: p,
			Ino:  hash(fmt.Sprintf("%v/%v", basePath, p)),
			Mode: fuse.S_IFDIR,
		})
	}
	return fs.NewListDirStream(rv), 0
}

type dirEntries struct {
	Directories []string
	Files       []string
}

type VirtualDirectory interface {
	Entries(context.Context, *genericDirParams) (*dirEntries, error)
	Entry(string, *genericDirParams) (NewNode, FileMode, error)
}

type VirtualFile interface {
	Access(ctx context.Context, params *genericDirParams, mask uint32) syscall.Errno
	Read(ctx context.Context, params *genericDirParams, fh fs.FileHandle, buf []byte, offset int64) (fuse.ReadResult, syscall.Errno)
	Write(ctx context.Context, params *genericDirParams, fh fs.FileHandle, buf []byte, offset int64) (uint32, syscall.Errno)
	Open(ctx context.Context, params *genericDirParams, flags uint32) (fs.FileHandle, uint32, syscall.Errno)
}

func (n *GenericFile) Access(ctx context.Context, mask uint32) syscall.Errno {
	return n.action.Access(ctx, &n.params, mask)
}

func (n *GenericFile) Read(ctx context.Context, fh fs.FileHandle, buf []byte, offset int64) (fuse.ReadResult, syscall.Errno) {
	return n.action.Read(ctx, &n.params, fh, buf, offset)
}

func (n *GenericFile) Write(ctx context.Context, fh fs.FileHandle, buf []byte, offset int64) (uint32, syscall.Errno) {
	return n.action.Write(ctx, &n.params, fh, buf, offset)
}

func (n *GenericFile) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	return n.action.Open(ctx, &n.params, flags)
}

func getArg(name string, params map[string]string) (string, error) {
	rv, exists := params[name]
	if !exists {
		return "", fmt.Errorf("arg '%v' is required", name)
	}
	return rv, nil
}

type NewNode func(genericDirParams) (fs.InodeEmbedder, error)

type FileMode uint32

type genericDirParams struct {
	contextName  string
	groupVersion *GroupedAPIResource
	name         string
	namespace    string
	namespaced   *bool
	pod          string

	cli        *k8s.Clientset
	stateStore *State
	log        *zap.SugaredLogger
	lastError  error
}

// Identifier returns a stringified identifier for this set of params
func (p *genericDirParams) Identifier() string {
	gvr := ""
	if p.groupVersion != nil {
		gvr = p.groupVersion.GVR().String()
	}
	return fmt.Sprintf("%v/%v/%v/%v/%v",
		p.contextName,
		gvr,
		p.namespace,
		p.pod,
		p.name,
	)
}

type GenericFile struct {
	fs.Inode
	action VirtualFile

	basePath string
	params   genericDirParams

	lastError error
}

type GenericDir struct {
	fs.Inode
	action VirtualDirectory

	basePath string
	params   genericDirParams

	lastError error
}

type paramsSpec struct {
	contextName  bool
	groupVersion bool
	name         bool
	namespace    bool
	namespaced   bool
	pod          bool

	cli        bool
	stateStore bool
	log        bool
	lastError  bool
}

func checkParams(spec paramsSpec, params genericDirParams) error {
	missingValues := []string{}

	if spec.contextName && params.contextName == "" {
		missingValues = append(missingValues, "contextName")
	}
	if spec.groupVersion && params.groupVersion == nil {
		missingValues = append(missingValues, "groupVersion")
	}
	if spec.name && params.name == "" {
		missingValues = append(missingValues, "name")
	}
	if spec.namespace && params.namespace == "" {
		missingValues = append(missingValues, "namespace")
	}
	if spec.pod && params.pod == "" {
		missingValues = append(missingValues, "pod")
	}
	if spec.cli && params.cli == nil {
		missingValues = append(missingValues, "cli")
	}
	if spec.stateStore && params.stateStore == nil {
		missingValues = append(missingValues, "stateStore")
	}
	if spec.log && params.log == nil {
		missingValues = append(missingValues, "log")
	}
	if spec.namespaced && params.namespaced == nil {
		missingValues = append(missingValues, "namespaced")
	}
	if spec.lastError && params.lastError == nil {
		missingValues = append(missingValues, "lastError")
	}
	if len(missingValues) == 0 {
		return nil
	} else {
		return fmt.Errorf("params was missing required values %v | %w", missingValues, eParamsMissing)
	}
}

func (n *GenericDir) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries, err := n.action.Entries(ctx, &n.params)
	if err != nil {
		panic("TODO")
	}
	return readdirResponse(
		entries,
		n.basePath,
	)
}

func (n *GenericDir) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	var node fs.InodeEmbedder
	var err error
	entryConstructor, mode, err := n.action.Entry(name, &n.params)
	if err != nil {
		if errors.Is(err, eNoExists) {
			return nil, syscall.ENOENT
		}
		n.params.lastError = err
		n.params.log.Error(err)

		node, err = NewErrorFile(&n.params)
		if err != nil {
			n.params.log.Error("failure to make error file")
			return nil, syscall.EREMOTEIO
		}
	} else {
		n.params.name = name
		node, err = entryConstructor(n.params)
		if err != nil {
			n.lastError = err
			n.params.log.Error(err)
			node, err = NewErrorFile(&n.params)
			if err != nil {
				n.params.log.Error("failure to make error file")
				return nil, syscall.EREMOTEIO
			}
		}
	}
	ch := n.NewInode(
		ctx,
		node,
		fs.StableAttr{
			Mode: uint32(mode),
			Ino:  hash(fmt.Sprintf("%v/%v", n.basePath, name)),
		},
	)
	return ch, 0
}

func ensureClientSet(params *genericDirParams) error {
	if params.cli != nil {
		return nil
	}
	cli, err := kube.GetK8sClient(params.contextName)
	if err != nil {
		return err
	}
	params.cli = cli
	return nil
}
