package kubernetes

import (
	"context"
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

func GetK8sClientConfig(contextName string) (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()

	configOverrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		configOverrides.CurrentContext = contextName
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
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
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

func GetApiResources(cli *discovery.DiscoveryClient) (*[]*metav1.APIResourceList, error){

	apiResourceList, err := cli.ServerPreferredResources()
	if discovery.IsGroupDiscoveryFailedError(err) {
		fmt.Printf("WARNING: The Kubernetes server has an orphaned API service. Server reports: %s\n", err)
		fmt.Printf("WARNING: To fix this, kubectl delete apiservice <service-name>\n")
	} else {
		return nil, fmt.Errorf("could not get apiVersions from Kubernetes | %w", err)
	}
	return &apiResourceList, nil
}

func GetK8sDiscoveryClient(contextName string) (*discovery.DiscoveryClient, error) {
	config, err := GetK8sClientConfig(contextName)
	if err != nil {
		return nil, err
	}

	// create the clientset
	clientset, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}
