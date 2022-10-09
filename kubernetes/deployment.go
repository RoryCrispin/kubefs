package kubernetes

import (
	"context"
	"encoding/json"

	kube_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)


func GetDeployments(ctx context.Context, cli *kubernetes.Clientset, namespace string) ([]string, error) {
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

func GetDeploymentDefinition(ctx context.Context, cli *kubernetes.Clientset, name, namespace string) ([]byte, error) {
	rv, err := cli.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if kube_errors.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	b, err := json.MarshalIndent(rv, "", "    ")
	if err != nil {
		return nil, err
	}
	return b, nil
}
