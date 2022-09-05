package kubernetes

import (
	"bytes"
	"context"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	kube_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
)


func GetPods(ctx context.Context, cli *k8s.Clientset, namespace string) ([]string, error) {
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

func GetPodDefinition(ctx context.Context, cli *k8s.Clientset, name, namespace string) ([]byte, error) {
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

func GetLogs(ctx context.Context, cli *k8s.Clientset, name, namespace string) ([]byte, error) {
	return getLogs(ctx, cli, name, namespace, false)
}
func GetPreviousLogs(ctx context.Context, cli *k8s.Clientset, name, namespace string) ([]byte, error) {
	return getLogs(ctx, cli, name, namespace, true)
}

func getLogs(ctx context.Context, cli *k8s.Clientset, name, namespace string, previous bool) ([]byte, error) {

	rc, err := cli.CoreV1().Pods(namespace).GetLogs(
		name,
		 &corev1.PodLogOptions{
			Previous: previous,
		 },
		).Stream(context.TODO())
	if err != nil {
		return nil, err
	}

	defer rc.Close()

	buf := new(bytes.Buffer)
	buf.ReadFrom(rc)
	return buf.Bytes(), nil
}