package kubernetes

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func GetK8sClient() *kubernetes.Clientset {
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
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return clientset
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
	fmt.Printf("%#v", res.Items)
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
