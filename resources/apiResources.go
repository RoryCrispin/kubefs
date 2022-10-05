package resources

import (
	"context"
	"fmt"
	"strings"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"

	kube "rorycrispin.co.uk/kubefs/kubernetes"
)

// ResourceTypeNode is a Dir containing the different resource types within a cluster.
// For example:
// Namespaced <- namespaced resources
// Cluster    <- resources which are not namespaced
type ResourceTypeNode struct {
	fs.Inode
	contextName string

	stateStore map[uint64]any
}

func (n *ResourceTypeNode) Path() string {
	return fmt.Sprintf("%v/resources", n.contextName)
}

func (n *ResourceTypeNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries := []fuse.DirEntry{
		{
			Name: "namespaced",
			Ino:  hash(fmt.Sprintf("%v/namespaces", n.Path())),
			Mode: fuse.S_IFDIR,
		},
		{
			Name: "cluster",
			Ino:  hash(fmt.Sprintf("%v/resources", n.Path())),
			Mode: fuse.S_IFDIR,
		},
	}
	return fs.NewListDirStream(entries), 0
}

func (n *ResourceTypeNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if name != "namespaced" && name != "cluster" {
		return nil, syscall.ENOENT
	}
	ch := n.NewInode(
		ctx,
		&RootResourcesNode{
			namespaced:  name == "namespaced",
			contextName: n.contextName,

			stateStore: n.stateStore,
		},
		fs.StableAttr{
			Mode: syscall.S_IFDIR,
			Ino:  hash(fmt.Sprintf("%v/%v", n.Path(), name)),
		},
	)
	return ch, 0
}

// RootResourcesNode is a dir containing all of the api resources within a given
// cluster. If the namespaced bool is true, then only namespaced api-resources
// will be returned, and vice-versa.
type RootResourcesNode struct {
	fs.Inode

	contextName string
	namespaced  bool

	stateStore map[uint64]interface{}
	lastError        error
}

func (n *RootResourcesNode) Path() string {
	return fmt.Sprintf("%v/resources", n.contextName)
}

type APIResources map[string]*GroupedAPIResource

// GroupdAPIResource is a denormalisation of the metav1.APIResource and GroupVersion
type GroupedAPIResource struct {
	ResourceName string
	GroupVersion_ string
	ShortNames   []string
	Namespaced   bool
	Group string
	Version string
}

func (g *GroupedAPIResource) CLIName() string {
	if g.Version == "" {
		return g.ResourceName
	}
	return fmt.Sprint(g.ResourceName, ".", g.Group)
}

func (g *GroupedAPIResource) GroupVersion() string {
	if g.Version == "" {
		return g.Group
	}
	return fmt.Sprint(g.Group, "/", g.Version)
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
		var workingResource *GroupedAPIResource
		var a *metav1.APIResource
		var i int
		for _, grp := range *resp {
			for i = range grp.APIResources {
				a = &grp.APIResources[i]

				group, version, err := splitGroupVersion(grp.GroupVersion)
				if err != nil {
					return nil, err
				}
				workingResource = &GroupedAPIResource{
					ResourceName: a.Name,
					Group: group,
					Version: version,
					GroupVersion_: grp.GroupVersion,
					ShortNames:   a.ShortNames,
					Namespaced:   a.Namespaced,
				}

				elem, exists := rv[workingResource.CLIName()]
				if exists {
					// TODO DEV this should not get hit, reduce it from a panic later.
					panic(fmt.Errorf("Found collision between %v/%v and %v/%v\n", grp.GroupVersion, a.Name, elem.GroupVersion, elem.ResourceName))
				}

				rv[workingResource.CLIName()] = workingResource
			}

		}
		stateStore[hash(stateKey)] = rv
	}
	return rv, nil
}

func (n *RootResourcesNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	resources, err := ensureAPIResources(n.stateStore, n.contextName)
	if err != nil {
		n.lastError = fmt.Errorf("error while getting API resources | %w", err)
		panic(n.lastError)
		return readDirErrResponse(n.Path())
	}
	entries := make([]fuse.DirEntry, 0, len(resources))
	for _, res := range resources {
		if res.Namespaced != n.namespaced {
			continue
		}
		entries = append(entries, fuse.DirEntry{
			Name: res.CLIName(),
			Ino:  hash(fmt.Sprintf("%v/%v", n.Path(), res.ResourceName)),
			Mode: syscall.S_IFREG,
		})
	}
	return fs.NewListDirStream(entries), 0
}

func (n *RootResourcesNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	resources, err := ensureAPIResources(n.stateStore, n.contextName)
	if err != nil {
		fmt.Printf("Error while looking up Resource %v | %v", name, err)
		return nil, syscall.ENOENT
	}
	elem, exists := resources[name]
	if !exists {
		return nil, syscall.ENOENT
	}
	if elem.Namespaced {
		ch := n.NewInode(ctx, &ListGenericNamespaceNode{
			contextName:  n.contextName,
			groupVersion: elem,
			stateStore:   n.stateStore,
		},
			fs.StableAttr{
				Mode: syscall.S_IFDIR,
				Ino:  hash(fmt.Sprintf("%v/%v", n.Path(), name)),
			},
		)
		return ch, 0
	} else {
		ch := n.NewInode(ctx, &APIResourceNode{
			contextName:  n.contextName,
			groupVersion: elem,
			stateStore:   n.stateStore,
		},
			fs.StableAttr{
				Mode: syscall.S_IFDIR,
				Ino:  hash(fmt.Sprintf("%v/%v", n.Path(), name)),
			},
		)
		return ch, 0
	}
}

// APIResourceNode is a dir containing the list of resources for an API
// resource. It may be Namespaced or Clustered. If the resource is clustered,
// then the namespace field will be the empty string.
type APIResourceNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	contextName  string
	groupVersion *GroupedAPIResource

	// If this the resource is namespaced, then namespace will be set.
	// groupVersion.Namespaced should also be set correctly.
	namespace string

	lastError error

	cli        *k8s.Clientset
	stateStore map[uint64]interface{}
}

func (n *APIResourceNode) Path() string {
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

var _ = (fs.NodeReaddirer)((*APIResourceNode)(nil))

func (n *APIResourceNode) ensureClientSet() error {
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

func (n *APIResourceNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	err := n.ensureClientSet()
	if err != nil {
		panic(err)
	}
	results, err := kube.ListResourceNames(ctx, n.groupVersion.GroupVersion(), n.groupVersion.ResourceName, n.contextName, n.namespace)
	if err != nil {
		// The filesystem is our interface with the user, so let
		// errors here be exposed via said interface.
		n.lastError = err
		return readDirErrResponse(n.Path())
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

func getAPIResourceStruct(name, contextName, namespace string, groupVersion *GroupedAPIResource, stateStore map[uint64]any) fs.InodeEmbedder {
	if groupVersion.GroupVersion() == "v1" && groupVersion.ResourceName == "pods" {
		return &PodObjectsNode{
			name: name,
			contextName: contextName,
			namespace: namespace,

			stateStore: stateStore,
		}
	} else {
		return &APIResourceActions{
			name: name,
			contextName: contextName,
			namespace: namespace,
			groupVersion: groupVersion,

			stateStore: stateStore,
		}
	}

}

func (n *APIResourceNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if name == "error" {
		// TODO rc return a file whose string contents is the error
		fmt.Printf("Error is %v", n.lastError)
		return nil, syscall.ENOENT
	}

	node := getAPIResourceStruct(name, n.contextName, n.namespace, n.groupVersion, n.stateStore)

	ch := n.NewInode(
		ctx,
		node,
		fs.StableAttr{
			Mode: fuse.S_IFDIR,
			Ino:  hash(fmt.Sprintf("%v/%v", n.Path(), name)),
		},
	)
	return ch, 0
}


type APIResourceActions struct {
	fs.Inode

	name         string
	namespace    string
	contextName  string
	groupVersion *GroupedAPIResource

	lastError  error
	cli        *k8s.Clientset
	stateStore map[uint64]interface{}
}

func (n *APIResourceActions) Path() string {
	if n.groupVersion.Namespaced {
		return fmt.Sprintf("%v/resources/%v/%v/namespaces/%v/%v",
			n.contextName, n.groupVersion.GroupVersion, n.groupVersion.ResourceName, n.namespace, n.name,
		)
	} else {
		return fmt.Sprintf("%v/resources/%v/%v/%v", //TODO is this shadowed?
			n.contextName, n.groupVersion.GroupVersion, n.groupVersion.ResourceName, n.name,
		)
	}
}

func (n *APIResourceActions) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries := []fuse.DirEntry{
		{
			Name: "def.json",
			Ino:  hash(fmt.Sprintf("%v/json", n.Path())),
			Mode: fuse.S_IFDIR,
		},
	}
	return fs.NewListDirStream(entries), 0
}

func (n *APIResourceActions) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if name == "def.json" {
	ch := n.NewInode(
		ctx,
		&GenericJSONFile{
			name: n.name,
			namespace: n.namespace,
			contextName: n.contextName,
			groupVersion: n.groupVersion,

			cli: n.cli,
			stateStore: n.stateStore,
		},
		fs.StableAttr{
			Mode: syscall.S_IFREG,
			Ino:  hash(fmt.Sprintf("%v/%v", n.Path(), name)),
		},
	)
	return ch, 0
	}
	return nil, syscall.ENOENT
}


func splitGroupVersion(groupVersion string) (string, string, error) {
	if groupVersion == "v1" {
		// The core api is a special case
		return "v1", "", nil
	}
	splat := strings.Split(groupVersion, "/")
	if len(splat) != 2 {
		return "", "",
			fmt.Errorf(
				"failed to split groupVersion, resource (%v) contained an unexpected number of '/' chars.",
				groupVersion,
			)
	}
	return splat[0], splat[1], nil
}

