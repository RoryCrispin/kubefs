package resources

import (
	"context"
	"fmt"
	"syscall"
	"errors"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	k8s "k8s.io/client-go/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kube "rorycrispin.co.uk/kubefs/kubernetes"
)


type ResourceTypeNode struct {
	fs.Inode
	contextName string

	stateStore map[uint64]any
}

func (n *ResourceTypeNode) Path() string {
	return fmt.Sprintf("%v/resources", n.contextName)
}

func (n *ResourceTypeNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if name != "namespaced" && name != "cluster" {
		fmt.Printf("Lookup of unknwon resource type, %v", name)
		return nil, syscall.ENOENT
	}
	ch := n.NewInode(
		ctx,
		&RootResourcesNode{
			namespaced: name == "namespaced",
			contextName: n.contextName,

			stateStore: n.stateStore,
		},
		fs.StableAttr{
			Mode: syscall.S_IFDIR,
			Ino: hash(fmt.Sprintf("%v/%v", n.Path(), name)),
		},
	)
	return ch, 0
}


// // Readdir is part of the NodeReaddirer interface
func (n *ResourceTypeNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries := []fuse.DirEntry{
		{
			Name: "namespaced",
			Ino: hash(fmt.Sprintf("%v/namespaces", n.Path())),
			Mode: fuse.S_IFDIR,
		},
		{
			Name: "cluster",
			Ino: hash(fmt.Sprintf("%v/resources", n.Path())),
			Mode: fuse.S_IFDIR,
		},
	}
	return fs.NewListDirStream(entries), 0
}


type RootResourcesNode struct {
	fs.Inode

	contextName string
	namespaced bool

	stateStore map[uint64]interface{}
	err error
}


func (n *RootResourcesNode) Path() string {
	return fmt.Sprintf("%v/resources", n.contextName)
}

type APIResources map[string]*GroupedAPIResource

// GroupdAPIResource is a denormalisation of the metav1.APIResource and GroupVersion
type GroupedAPIResource struct {

	ResourceName string
	GroupVersion string
	ShortNames []string
	Namespaced bool
}

func ensureAPIResources(stateStore map[uint64]any, contextName string) (APIResources, error) {
	// TODO RC statestore should just take ints
	//
	stateKey := fmt.Sprintf("%v/api-resources", contextName)
	rv := make(APIResources)
	// TODO RC no cache expiry
	elem, exist := stateStore[hash(stateKey)]
	if exist {
		var ok bool
		rv, ok = elem.(APIResources)
		if !ok {
			panic("failed type assertion")
		}
		// TODO the statestore is shared by contexts,
		// there should be one per context?
		fmt.Printf("Using cached copy of API-Resources\n")
	} else {
		cli, err := kube.GetK8sDiscoveryClient(contextName)

		resp, err := kube.GetApiResources(cli)
		if err != nil {
			return nil, fmt.Errorf("err getting resources | %w", err)
		}
		var a *metav1.APIResource
		var i int
		for _, grp := range *resp {
			for i = range(grp.APIResources) {
				a = &grp.APIResources[i]
				elem, exists := rv[a.Name]
				if exists {
					// TODO RC handle colliding resources
					fmt.Print(fmt.Errorf("Found collision between %v/%v and %v/%v\n", grp.GroupVersion, a.Name, elem.GroupVersion, elem.ResourceName))
				}
				rv[a.Name] = &GroupedAPIResource{
					ResourceName: a.Name,
					GroupVersion: grp.GroupVersion,
					ShortNames: a.ShortNames,
					Namespaced: a.Namespaced,
				}
			}
		}
		stateStore[hash(stateKey)] = rv
	}
	return rv, nil
}

var _ = (fs.NodeReaddirer)((*RootResourcesNode)(nil))

// // Readdir is part of the NodeReaddirer interface
func (n *RootResourcesNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	resources, err := ensureAPIResources(n.stateStore, n.contextName)
	if err != nil {
		n.err = fmt.Errorf("error while getting API resources | %w", err)
		fmt.Println(n.err)
		return fs.NewListDirStream([]fuse.DirEntry{
			{
				Name: "error",
				Ino: hash(fmt.Sprintf("%v/error", n.Path())),
				Mode: fuse.S_IFREG,
			},
		}), 0
	}
	entries := make([]fuse.DirEntry, 0, len(resources))

	for _, res := range resources {
		if res.Namespaced != n.namespaced {
			continue
		}
		entries = append(entries, fuse.DirEntry{
			Name: res.ResourceName,
			Ino: hash(fmt.Sprintf("%v/%v", n.Path(), res.ResourceName)),
			Mode: syscall.S_IFREG,
		})
	}
	return fs.NewListDirStream(entries), 0
}

func (n *RootResourcesNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP RootResourcesNode: %v\n", name)
	resources, err := ensureAPIResources(n.stateStore, n.contextName)
	if err != nil {
		fmt.Printf("Error while looking up Resource %v | %v", name, err)
		return nil, syscall.ENOENT
	}
	elem, exists := resources[name]
	if !exists {
		return nil, syscall.ENOENT
	}
	fmt.Printf("Found resource for lookup, details: %#v\n", elem)
	if elem.Namespaced {
		ch := n.NewInode(ctx, &ListGenericNamespaceNode{
			contextName: n.contextName,
			groupVersion: elem,
			stateStore: n.stateStore,
		},
			fs.StableAttr{
				Mode: syscall.S_IFDIR,
				Ino: hash(fmt.Sprintf("%v/%v", n.Path(), name)),
			},
		)
		return ch, 0
	} else {
		ch := n.NewInode(ctx, &ResourceNode{
			contextName: n.contextName,
			groupVersion: elem,
			stateStore: n.stateStore,
		},
			fs.StableAttr{
				Mode: syscall.S_IFDIR,
				Ino: hash(fmt.Sprintf("%v/%v", n.Path(), name)),
			},
		)
		return ch, 0
	}
}



// ResourceNode represents the root of a specific resource.
type ResourceNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	contextName string
	groupVersion *GroupedAPIResource

	lastError error

	// cli *k8s.Clientset
	stateStore map[uint64]interface{}
}

func (n *ResourceNode) Path() string {
	return fmt.Sprintf("%v/resources/%v/%v",
		n.contextName, n.groupVersion.GroupVersion, n.groupVersion.ResourceName,
	)
}

var _ = (fs.NodeReaddirer)((*ResourceNode)(nil))

// func (n *ResourceNode) ensureClientSet() error {
// 	if n.cli != nil {
// 		return nil
// 	}
// 	cli, err := kube.GetK8sClient(n.contextName)
// 	if err != nil {
// 		return err
// 	}
// 	n.cli = cli
// 	return nil
// }

// // Readdir is part of the NodeReaddirer interface
func (n *ResourceNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {

	results, err := kube.ListResourceNames(ctx, n.groupVersion.GroupVersion,  n.groupVersion.ResourceName, n.contextName, "default")

	if err != nil {
		// The filesystem is our interface with the user, so let
		// errors here be exposed via said interface.
		n.lastError = err
		entries := []fuse.DirEntry{
			{
				Name: "error",
				Ino:  hash(fmt.Sprintf("%v/%v", n.Path(), "error")),
				Mode: fuse.S_IFREG,
			},
		}
		return fs.NewListDirStream(entries), 0
	}

	entries := make([]fuse.DirEntry, 0, len(results))
	for _, p := range results {
		if p == "" {
			continue
		}
		entries = append(entries, fuse.DirEntry{
			Name: p,
			Ino:  hash(fmt.Sprintf("%v/%v", n.Path(), p)),
			Mode: fuse.S_IFDIR,
		})
	}
	return fs.NewListDirStream(entries), 0
}

func (n *ResourceNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP OF ResourceNode %s' \n", name)
	if name == "error" {
		// TODO rc return a file whose string contents is the error
		fmt.Printf("Error is %v", n.lastError)
		return nil, syscall.ENOENT
	}

	if n.groupVersion.Namespaced {
		fmt.Printf("NAMESPACED: %v", name)
		ch := n.NewInode(
			ctx,
			&ListGenericNamespaceNode{
				contextName: n.contextName,
				groupVersion: n.groupVersion,

				stateStore: n.stateStore,
			},
			fs.StableAttr{
				Mode: syscall.S_IFDIR,
				Ino: hash(fmt.Sprintf("%v/%v", n.Path(), name)),
			})
		return ch, 0
	} else {
		fmt.Printf("NOT-NAMESPACED: %v", name)
		ch := n.NewInode(
			ctx,
			&GenericResourceNode{
				contextName: n.contextName,
				groupVersion: n.groupVersion,

				stateStore: n.stateStore,
			},
			fs.StableAttr{
				Mode: syscall.S_IFDIR,
				Ino: hash(fmt.Sprintf("%v/%v", n.Path(), name)),
			})
		return ch, 0
	}
}



///
///
///

// ListGenericNamespaceNode represents the root of a specific resource.
type ListGenericNamespaceNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	contextName string
	groupVersion *GroupedAPIResource

	lastError error

	cli *k8s.Clientset
	stateStore map[uint64]interface{}
}

func (n *ListGenericNamespaceNode) Path() string {
	return fmt.Sprintf("%v/resources/%v/%v/namespaces",
		n.contextName, n.groupVersion.GroupVersion, n.groupVersion.ResourceName,
	)
}

var _ = (fs.NodeReaddirer)((*ListGenericNamespaceNode)(nil))

func (n *ListGenericNamespaceNode) ensureClientSet() error {
	if n.cli != nil {
		return nil
	}
	cli, err := kube.GetK8sClient(n.contextName)
	if err != nil {
		return err
	}
	n.cli = cli
	return nil
}

// // Readdir is part of the NodeReaddirer interface
func (n *ListGenericNamespaceNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	err := n.ensureClientSet()
	if err != nil {
		panic(err)
	}

	results, err := kube.GetNamespaces(ctx, n.cli)
	if err != nil {
		// The filesystem is our interface with the user, so let
		// errors here be exposed via said interface.
		n.lastError = err
		entries := []fuse.DirEntry{
			{
				Name: "error",
				Ino:  hash(fmt.Sprintf("%v/%v", n.Path(), "error")),
				Mode: fuse.S_IFREG,
			},
		}
		return fs.NewListDirStream(entries), 0
	}

	entries := make([]fuse.DirEntry, 0, len(results))
	for _, p := range results {
		if p == "" {
			continue
		}
		entries = append(entries, fuse.DirEntry{
			Name: p,
			Ino:  hash(fmt.Sprintf("%v/%v", n.Path(), p)),
			Mode: fuse.S_IFDIR,
		})
	}
	return fs.NewListDirStream(entries), 0

}

func (n *ListGenericNamespaceNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP OF ListGenericNamespaceNode %s' \n", name)
	if name == "error" {
		// TODO rc return a file whose string contents is the error
		fmt.Printf("Error is %v", n.lastError)
		return nil, syscall.ENOENT
	}

	ch := n.NewInode(
		ctx,
		// TODO rc need to have a new RootObjects for unstructured clients.
		&GenericResourceNode{
			namespace: name,
			contextName: n.contextName,
			groupVersion: n.groupVersion,

			//cli: n.cli,
			stateStore: n.stateStore,
		},
		fs.StableAttr{
			Mode: syscall.S_IFDIR,
			Ino: hash(fmt.Sprintf("%v/%v", n.Path(), name)),
		},
	)
	return ch, 0
}


///
///
///
///
///

// GenericResourceNode represents the root of a specific resource.
type GenericResourceNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	contextName string
	groupVersion *GroupedAPIResource

	// If this the resource is namespaced, then namespace will be set.
	// groupVersion.Namespaced should also be set correctly.
	namespace string

	lastError error

	cli *k8s.Clientset
	stateStore map[uint64]interface{}
}

func (n *GenericResourceNode) Path() string {
	if n.groupVersion.Namespaced {
		return fmt.Sprintf("%v/resources/%v/%v/namespaces/%v",
			n.contextName, n.groupVersion.GroupVersion, n.groupVersion.ResourceName, n.namespace,
		)
	} else {
		return fmt.Sprintf("%v/resources/%v/%v", //TODO is this shadowed?
			n.contextName, n.groupVersion.GroupVersion, n.groupVersion.ResourceName,
		)
	}
}

var _ = (fs.NodeReaddirer)((*GenericResourceNode)(nil))

func (n *GenericResourceNode) ensureClientSet() error {
	if n.cli != nil {
		return nil
	}
	cli, err := kube.GetK8sClient(n.contextName)
	if err != nil {
		return err
	}
	n.cli = cli
	return nil
}

// // Readdir is part of the NodeReaddirer interface
func (n *GenericResourceNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	err := n.ensureClientSet()
	if err != nil {
		panic(err)
	}
	results, err := kube.ListResourceNames(ctx, n.groupVersion.GroupVersion, n.groupVersion.ResourceName, n.contextName, n.namespace)
	if err != nil {
		// The filesystem is our interface with the user, so let
		// errors here be exposed via said interface.
		n.lastError = err
		entries := []fuse.DirEntry{
			{
				Name: "error",
				Ino:  hash(fmt.Sprintf("%v/%v", n.Path(), "error")),
				Mode: fuse.S_IFREG,
			},
		}
		return fs.NewListDirStream(entries), 0
	}

	entries := make([]fuse.DirEntry, 0, len(results))
	for _, p := range results {
		if p == "" {
			continue
		}
		fmt.Printf("Make reg file : %v\n", p)
		entries = append(entries, fuse.DirEntry{
			Name: p,
			Ino:  hash(fmt.Sprintf("%v/%v", n.Path(), p)),
			Mode: fuse.S_IFREG,
		})
	}
	return fs.NewListDirStream(entries), 0
}

func (n *GenericResourceNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("LOOKUP OF GenericResourceNode %s' \n", name)
	if name == "error" {
		// TODO rc return a file whose string contents is the error
		fmt.Printf("Error is %v", n.lastError)
		return nil, syscall.ENOENT
	}

	ch := n.NewInode(
		ctx,
		&GenericJSONFile{
			name: name,
			contextName: n.contextName,
			namespace: n.namespace,

			stateStore: n.stateStore,
		},
		fs.StableAttr{
			Mode: fuse.S_IFDIR,
			Ino: hash(fmt.Sprintf("%v/%v", n.Path(), name)),
		},
	)
	return ch, 0
}

// ========== Generic JSON file ==========

type GenericJSONFile struct {
	fs.Inode

	name      string
	namespace string
	contextName string
	goupVersion *GroupedAPIResource


	cli *k8s.Clientset
	stateStore map[uint64]interface{}
}

// GenericJSONFile implements Open
var _ = (fs.NodeOpener)((*GenericJSONFile)(nil))

func (f *GenericJSONFile) Open(ctx context.Context, openFlags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	if fuseFlags&(syscall.O_RDWR|syscall.O_WRONLY) != 0 {
		// disallow writes
		return nil, 0, syscall.EROFS
	}

	content, err := kube.GetPlaintext(ctx)
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
		content: content,
	}

	// Return FOPEN_DIRECT_IO so content is not cached.
	return fh, fuse.FOPEN_DIRECT_IO, 0
}

// ========== Error file ==========

type ErrorFile struct {
	fs.Inode

	err error

	stateStore map[uint64]interface{}
}

// GenericJSONFile implements Open
var _ = (fs.NodeOpener)((*GenericJSONFile)(nil))

func (f *ErrorFile) Open(ctx context.Context, openFlags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	if fuseFlags&(syscall.O_RDWR|syscall.O_WRONLY) != 0 {
		// disallow writes
		return nil, 0, syscall.EROFS
	}

	fh = &roBytesFileHandle{
		content: []byte(fmt.Sprintf("%v", f.err)),
	}
	return fh, fuse.FOPEN_DIRECT_IO, 0
}
