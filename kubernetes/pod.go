package kubernetes

import (
	"fmt"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	kube_errors "k8s.io/apimachinery/pkg/api/errors"
	spdyStream "k8s.io/apimachinery/pkg/util/httpstream/spdy"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/kubernetes"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/kubernetes/scheme"

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

	b, err := json.MarshalIndent(pod, "", "    ")
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

func ExecCommand(ctx context.Context, contextName, pod, container, namespace string, cmd []string) ([]byte, []byte, error) {
	if contextName != "microk8s" && contextName != "rancher-desktop" {
		panic("disabling exec on real cluster!")
	}
	config, err := GetK8sClientConfig(contextName)
	if err != nil {
		return nil, nil, err
	}

	cli, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}

	req := cli.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec").
		Param("container", container).
		VersionedParams(&corev1.PodExecOptions{
			Command: cmd,
			Container: container,
			Stdin: false,
			Stdout: true,
			Stderr: true,
			TTY: false,
		}, scheme.ParameterCodec)

	upgrader := spdyStream.NewRoundTripper(&tls.Config{InsecureSkipVerify: true})
	wrapper, err := rest.HTTPWrappersForConfig(config, upgrader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed creating SPDY upgrade wrapper: %w", err)
	}

	exec, err := remotecommand.NewSPDYExecutorForTransports(wrapper, upgrader, "POST", req.URL())
	if err != nil {
		return nil, nil, err
	}
	var stdoutBuf, stderrBuf bytes.Buffer
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin: nil,
		Stdout: &stdoutBuf,
		Stderr: &stderrBuf,
		Tty: false,
	})
	if err != nil {
		return nil, nil, err
	}
	return stdoutBuf.Bytes(), stderrBuf.Bytes(), err
}
