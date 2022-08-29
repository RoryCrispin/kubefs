package main

import (
	"context"
	"encoding/json"
	"flag"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func getPods(cli *kubernetes.Clientset) []string {
	pods, err := cli.CoreV1().Pods("default").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	rv := make([]string, len(pods.Items))

	for _, p := range pods.Items {
		rv = append(rv, p.Name)
	}
	return rv
}

func getPodDefinition(cli *kubernetes.Clientset, name string) ([]byte, error) {
	pod, err := cli.CoreV1().Pods("default").Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	b, err := json.Marshal(pod)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func getK8sClient() *kubernetes.Clientset {
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
