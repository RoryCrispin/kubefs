package kubernetes

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

)

type void struct{}
var member void

func GetK8sContexts() ([]string, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()

	configOverrides := &clientcmd.ConfigOverrides{}

	config:= clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides).ConfigAccess()
	starting, err := config.GetStartingConfig()
	if err != nil {
		return nil, err
	}

	contexts := make(map[string]void)
	out := []string{}

	for name := range starting.Contexts {
		_, exists := contexts[name]
		if !exists {
			out = append(out, name)
		}
		contexts[name] = member
	}
	return out, nil
}

func GetK8sClientConfig(kCtx string) (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()

	configOverrides := &clientcmd.ConfigOverrides{}
	if kCtx != "" {
		configOverrides.CurrentContext = kCtx
	}

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides).ClientConfig()
	if err != nil {
		return nil, err
	}
	return config, nil
}


func GetK8sClient(kCtx string) (*kubernetes.Clientset, error) {
	config, err := GetK8sClientConfig(kCtx)
	if err != nil {
		return nil, err
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}

func getK8sUnstructuredClient() dynamic.Interface {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return clientset
}

func getResourcesGeneric(cli dynamic.Interface) {
	q := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}
	res, err := cli.Resource(q).List(
		context.TODO(), metav1.ListOptions{
			FieldSelector: "name",
		})
	if err != nil {
		panic(err)
	}

	// b, err := json.Marshal(res.Items)
	// if err != nil {
	// 	panic(err)
	// }
	fmt.Printf("%#v\n", res.Items)
}

func getApiResources(cli *discovery.DiscoveryClient) {

	apiGroups, apiResourceList, err := cli.ServerGroupsAndResources()
	if err != nil {
		panic(err)
	}
	for _, apiGroup := range apiGroups {
		fmt.Printf("Apigroup: %s\n", apiGroup.Name)
	}

	for _, res := range apiResourceList {
		fmt.Printf("GroupVersion: %s\n", res.GroupVersion)
		for _, r := range res.APIResources {
			fmt.Printf("    ApiResource: %s Version %s\n", r.Name, r.Version)
		}
	}
}

func getK8sDiscoveryClient() *discovery.DiscoveryClient {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return clientset
}
