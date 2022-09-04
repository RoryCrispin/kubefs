package kubernetes

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)
func GetNamespaces(ctx context.Context, cli *kubernetes.Clientset) ([]string, error) {
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