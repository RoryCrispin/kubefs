package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"path/filepath"

	kube_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

var (
	ErrNotFound = fmt.Errorf("object not found")
)

func getPods(ctx context.Context, cli *kubernetes.Clientset, namespace string) ([]string, error) {
	pods, err := cli.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	rv := make([]string, len(pods.Items))

	for i, p := range pods.Items {
		rv[i] = p.Name
	}
	return rv, nil
}

func getDeployments(ctx context.Context, cli *kubernetes.Clientset, namespace string) ([]string, error) {
	results, err := cli.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	rv := make([]string, len(results.Items))

	for i, p := range results.Items {
		rv[i] = p.Name
	}
	return rv, nil
}

func getPodDefinition(ctx context.Context, cli *kubernetes.Clientset, name, namespace string) ([]byte, error) {
	pod, err := cli.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if kube_errors.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	b, err := json.Marshal(pod)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func getDeploymentDefinition(ctx context.Context, cli *kubernetes.Clientset, name, namespace string) ([]byte, error) {
	pod, err := cli.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if kube_errors.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	b, err := json.Marshal(pod)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func getNamespaces(ctx context.Context, cli *kubernetes.Clientset) ([]string, error) {
	namespaces, err := cli.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	rv := make([]string, len(namespaces.Items))

	for i, ns := range namespaces.Items {
		rv[i] = ns.Name
	}
	return rv, nil
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
