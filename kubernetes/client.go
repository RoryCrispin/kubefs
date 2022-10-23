package kubernetes

import (
	"fmt"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"go.uber.org/zap"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
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

	config.Timeout = 3 * time.Second

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
	config.Timeout = 3 * time.Second
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

func GetApiResources(log *zap.SugaredLogger, cli *discovery.DiscoveryClient) (*[]*metav1.APIResourceList, error){

	apiResourceList, err := cli.ServerPreferredResources()
	if discovery.IsGroupDiscoveryFailedError(err) {
		log.Info("the Kubernetes server has an orphaned API service",
			zap.Error(err),
			zap.String("fix", "kubectl delete apiservice <service-name>"),
		)
	} else if err != nil {
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
