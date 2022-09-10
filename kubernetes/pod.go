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
func getPod(ctx context.Context, cli *k8s.Clientset, name, namespace string) (*corev1.Pod, error) {
	pod, err := cli.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if kube_errors.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return pod, nil
}

func GetPodDefinition(ctx context.Context, cli *k8s.Clientset, name, namespace string) ([]byte, error) {
	pod, err := getPod(ctx, cli, name, namespace)
	if err != nil {
		return nil, err
	}

	b, err := json.Marshal(pod)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func GetContainers(ctx context.Context, cli *k8s.Clientset, podName, namespace string) ([]string, error) {
	pod, err := getPod(ctx, cli, podName, namespace)
	if err != nil {
		return nil, err
	}
	rv := make([]string, len(pod.Spec.Containers))
	for _, c := range pod.Spec.Containers {
		rv = append(rv, c.Name)
	}
	return rv, nil
}

func GetLogs(ctx context.Context, cli *k8s.Clientset, pod, container, namespace string) ([]byte, error) {
	return getLogs(ctx, cli, pod, container, namespace, false)
}
func GetPreviousLogs(ctx context.Context, cli *k8s.Clientset, pod, container, namespace string) ([]byte, error) {
	return getLogs(ctx, cli, pod, container, namespace, true)
}

func getLogs(ctx context.Context, cli *k8s.Clientset, pod, container, namespace string, previous bool) ([]byte, error) {
	rc, err := cli.CoreV1().Pods(namespace).GetLogs(
		pod,
		 &corev1.PodLogOptions{
			 Container: container,
			 Previous: previous,
		 },
		).Stream(ctx)
	if kube_errors.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	defer rc.Close()

	buf := new(bytes.Buffer)
	buf.ReadFrom(rc)
	return buf.Bytes(), nil
}

