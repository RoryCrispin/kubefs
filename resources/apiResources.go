package resources

import (
	"context"
	"fmt"
	"strings"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	stateStore *State
	log        *zap.SugaredLogger
}

func NewResourceTypeNode(params genericDirParams) (fs.InodeEmbedder, error) {
	err := checkParams(paramsSpec{
		stateStore: true,
		log: true,
		contextName: true,
	},
		params,
	)
	if err != nil {
		panic(err)
	}
	basePath := fmt.Sprintf("%v/resources", params.contextName)
	return &GenericDir{
		action: &ResourceTypeNode{},
		basePath: basePath,
		params: params,
	}, nil
}

func (n *ResourceTypeNode) Entries(ctx context.Context, params *genericDirParams) (*dirEntries, error) {
	return &dirEntries{
	Directories: []string{"namespaced", "cluster"},
	}, nil
}

func (n *ResourceTypeNode) Entry(name string, params *genericDirParams) (NewNode, FileMode, error) {
	if name != "namespaced" && name != "cluster" {
		return nil, 0, fmt.Errorf("resource type %v not found | %w", name, eNoExists)
	}
	namespaced := name == "namespaced"
	params.namespaced = &namespaced
	return NewRootResourcesNode, syscall.S_IFDIR, nil
}


// RootResourcesNode is a dir containing all of the api resources within a given
// cluster. If the namespaced bool is true, then only namespaced api-resources
// will be returned, and vice-versa.
type RootResourcesNode struct {
	fs.Inode
}

func NewRootResourcesNode(params genericDirParams) (fs.InodeEmbedder, error)  {
	err := checkParams(paramsSpec{
		stateStore: true,
		log: true,
		contextName: true,
		namespaced: true,
	},
		params,
	)
	if err != nil {
		panic(err)
	}
	basePath := fmt.Sprintf("%v/resources", params.contextName)
	return &GenericDir{
		action: &RootResourcesNode{},
		basePath: basePath,
		params: params,
	}, nil
}

func (n *RootResourcesNode) Entries(ctx context.Context, params *genericDirParams) (*dirEntries, error) {
	resources, err := ensureAPIResources(params.log, params.stateStore, params.contextName)
	if err != nil {
		return nil, fmt.Errorf("failure getting api resources | %w", err)
	}
	returnedResources := []string{}
	for _, res := range resources {
		if res.Namespaced != *params.namespaced {
			continue
		}
		returnedResources = append(returnedResources, res.ResourceName)
	}
	return &dirEntries{
		Directories: returnedResources,
	}, nil
}


func (n *RootResourcesNode) Entry(name string, params *genericDirParams) (NewNode, FileMode, error) {
	// TODO RC should be passing the api resource groupVersion here..
	resources, err := ensureAPIResources(params.log, params.stateStore, params.contextName)
	if err != nil {
		return nil, 0, fmt.Errorf("failure getting api resources | %w", err)
	}
	elem, exists := resources[name]
	if !exists {
		fmt.Printf("all resources : %#v", resources)
		return nil, 0, fmt.Errorf("failure looking up resource %v | %w", name, eNoExists)
	}
	params.groupVersion = elem
	if params.namespaced != nil && *params.namespaced {
		return NewListNamespaces, syscall.S_IFDIR, nil
	} else {
		return NewAPIResourceNode, syscall.S_IFDIR, nil
	}
}

type APIResources map[string]*GroupedAPIResource

// GroupdAPIResource is a denormalisation of the metav1.APIResource and GroupVersion
type GroupedAPIResource struct {
	ResourceName string
	ShortNames   []string
	Namespaced   bool
	Group        string
	Version      string
}

func (g *GroupedAPIResource) CLIName() string {
	if g.Group == "" {
		return g.ResourceName
	}
	return fmt.Sprint(g.ResourceName, ".", g.Group)
}

func (g *GroupedAPIResource) GroupVersion() string {
	if g.Group == "" {
		return g.Version
	}
	return fmt.Sprint(g.Group, "/", g.Version)
}

func (g *GroupedAPIResource) GVR() *schema.GroupVersionResource {
	return &schema.GroupVersionResource{
		Group:    g.Group,
		Version:  g.Version,
		Resource: g.ResourceName,
	}
}

func ensureAPIResources(log *zap.SugaredLogger, stateStore *State, contextName string) (APIResources, error) {
	// TODO this should really be in the kube package

	if contextName == "" {
		return nil, fmt.Errorf("context must be set")
	}

	stateKey := fmt.Sprintf("%v/api-resources", contextName)
	rv := make(APIResources)
	elem, exist := stateStore.Get(stateKey)
	if exist {
		var ok bool
		rv, ok = elem.(APIResources)
		if !ok {
			panic("failed type assertion")
		}
	} else {
		cli, err := kube.GetK8sDiscoveryClient(contextName)
		if err != nil {
			return nil, fmt.Errorf("err getting resources | %w", err)
		}
		resp, err := kube.GetApiResources(log, cli)
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
					Group:        group,
					Version:      version,
					ShortNames:   a.ShortNames,
					Namespaced:   a.Namespaced,
				}

				elem, exists := rv[workingResource.CLIName()]
				if exists {
					// TODO DEV this should not get hit, reduce it from a panic later.
					panic(fmt.Errorf("Found collision between %v/%v and %v/%v\n", grp.GroupVersion, a.Name, elem.GroupVersion(), elem.ResourceName))
				}

				rv[workingResource.CLIName()] = workingResource
			}

		}
		// TODO make this TTL configurable
		stateStore.PutTTL(stateKey, rv, 1*time.Minute)
	}
	return rv, nil
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
	stateStore *State
	log        *zap.SugaredLogger
}

func NewAPIResourceNode(
	params genericDirParams,
) (fs.InodeEmbedder, error) {
	err := checkParams(paramsSpec{
		contextName: true,
		groupVersion: true,
		stateStore: true,
		log: true,
	}, params)
	if err != nil {
		// TODO rc dont panic
		panic(err)
	}
	var namespace string
	if params.groupVersion.Namespaced {
		namespace = params.name
	}

	ensureClientSet(&params)

	return &APIResourceNode{
		contextName: params.contextName,

		namespace: namespace,
		groupVersion: params.groupVersion,

		stateStore: params.stateStore,
		log:        params.log,
		cli: params.cli,
	}, nil
}

func (n *APIResourceNode) Path() string {
	if n.groupVersion.Namespaced {
		return fmt.Sprintf("%v/resources/%v/%v/namespaces/%v",
			n.contextName, n.groupVersion.GroupVersion(), n.groupVersion.ResourceName, n.namespace,
		)
	} else {
		return fmt.Sprintf("%v/resources/%v/%v", //TODO is this shadowed?
			n.contextName, n.groupVersion.GroupVersion(), n.groupVersion.ResourceName,
		)
	}
}

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
	results, err := kube.ListResourceNames(
		ctx,
		n.log,
		n.groupVersion.GroupVersion(),
		n.groupVersion.ResourceName,
		n.contextName,
		n.namespace,
	)
	if err != nil {
		n.lastError = err
		return readDirErrResponse(n.Path())
	}

	return readdirResponse(&dirEntries{Directories: results}, n.Path())
}

func getAPIResourceStruct(
	name, contextName, namespace string,
	groupVersion *GroupedAPIResource,
	cli *k8s.Clientset,
	stateStore *State, log *zap.SugaredLogger,
) fs.InodeEmbedder {
	if groupVersion.GroupVersion() == "v1" && groupVersion.ResourceName == "pods" {
		params := genericDirParams{
			name: name,
			pod: name,
			namespace: namespace,
			contextName: contextName,
			cli: cli,
			stateStore: stateStore,
			log: log,
		}
		node := NewPodObjectsNode(params)
		if node == nil {
			panic("TODO")
		}
		return node
	} else {
		node := NewAPIResourceActions(name, namespace, contextName, groupVersion, cli, stateStore, log)
		if node == nil {
			panic("TODO")
		}
		return node
	}

}

func (n *APIResourceNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if name == "error" {
		// TODO rc return a file whose string contents is the error
		n.log.Error("error is", zap.Error(n.lastError))
		return nil, syscall.ENOENT
	}

	node := getAPIResourceStruct(name, n.contextName, n.namespace, n.groupVersion, n.cli, n.stateStore, n.log)
	if node == nil {
		panic("TODO")
	}

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

	contextName string

	name         string
	namespace    string
	groupVersion *GroupedAPIResource

	lastError  error
	cli        *k8s.Clientset
	stateStore *State
	log        *zap.SugaredLogger
}

func NewAPIResourceActions(
	name, namespace, contextName string,
	groupVersion *GroupedAPIResource,
	cli *k8s.Clientset,
	stateStore *State,
	log *zap.SugaredLogger,
) *APIResourceActions {
	if groupVersion == nil || cli == nil || log == nil {
		return nil
	}
	return &APIResourceActions{
		contextName: contextName,

		name:         name,
		namespace:    namespace,
		groupVersion: groupVersion,

		cli:        cli,
		stateStore: stateStore,
		log:        log,
	}
}

func (n *APIResourceActions) Path() string {
	if n.groupVersion.Namespaced {
		return fmt.Sprintf("%v/resources/%v/%v/namespaces/%v/%v",
			n.contextName, n.groupVersion.GroupVersion(), n.groupVersion.ResourceName, n.namespace, n.name,
		)
	} else {
		return fmt.Sprintf("%v/resources/%v/%v/%v", //TODO is this shadowed?
			n.contextName, n.groupVersion.GroupVersion(), n.groupVersion.ResourceName, n.name,
		)
	}
}

func (n *APIResourceActions) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	return readdirResponse(
		&dirEntries{Directories: []string{"def.json","edit.json"}},
		n.Path())
}

func (n *APIResourceActions) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if name == "def.json" {
		node := NewGenericJSONFile(n.name, n.namespace, n.contextName, n.groupVersion, n.cli, n.stateStore, n.log)
		if node == nil {
			panic("TODO")
		}
		ch := n.NewInode(
			ctx, node,
			fs.StableAttr{
				Mode: syscall.S_IFREG,
				Ino:  hash(fmt.Sprintf("%v/%v", n.Path(), name)),
			},
		)
		return ch, 0
	} else if name == "edit.json" {
		node := NewGenericEditableJSONFile(
			n.name,
			n.namespace,
			n.contextName,
			n.groupVersion,
			n.cli,
			n.stateStore,
			n.log,
		)
		ch := n.NewInode(
			ctx, node,
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
		return "", "v1", nil
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
