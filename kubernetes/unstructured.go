package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type metadataOnlyObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

func ListResourceNames(ctx context.Context, groupVersion, resource, contextName, namespace string) ([]string, error) {
	config, err := GetK8sClientConfig(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get k8s config | %w", err)
	}

	cli, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to make client from k8s config | %w", err)
	}

	var pathPrefix string
	if groupVersion == "v1" {
		pathPrefix = "api"
	} else {
		pathPrefix = "apis"
	}

	var req *rest.Request
	if namespace == "" {
		req = cli.CoreV1().RESTClient().
			Get().AbsPath(pathPrefix, groupVersion, resource)
	} else {
		req = cli.CoreV1().RESTClient().
			Get().AbsPath(pathPrefix, groupVersion, "namespaces", namespace, resource)
	}

	req.SetHeader("Accept", fmt.Sprintf("application/json;as=Table;v=%s;g=%s", metav1.SchemeGroupVersion.Version, metav1.GroupName))
	fmt.Printf("Requesting %#v\n", req)

	resp := req.Do(ctx)
	body, err := resp.Raw()
	if err != nil {
		return nil, fmt.Errorf("error returned from api server on %v | %w", req.URL().Path, err)
	}

	resTable := metav1.Table{}
	err = json.Unmarshal(body, &resTable)
	if err != nil {
		return nil, fmt.Errorf("error wile unmarshaling data | %w", err)
	}

	nameColumnIdx, err := getColIndex("Name", &resTable.ColumnDefinitions)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response data | %w", err)
	}

	rv := make([]string, len(resTable.Rows))
	for _, res := range resTable.Rows {
		rv = append(rv, res.Cells[nameColumnIdx].(string))
	}
	return rv, nil
}

func getColIndex(colName string, cols *[]metav1.TableColumnDefinition) (int, error) {
	if cols == nil {
		return 0, fmt.Errorf("passed nil list of columns")
	}

	var col metav1.TableColumnDefinition
	var idx int
	for idx, col = range *cols {
		if col.Name == colName {
			break
		}
	}
	if col.Name != colName {
		return 0, fmt.Errorf("Didn't find column '%v'", colName)

	}
	return idx, nil
}

func GetUnstructured(ctx context.Context, contextName, name, group, version, resource, namespace string) ([]byte, error) {
	cli := getK8sUnstructuredClient()

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	var rv *unstructured.Unstructured
	var err error
	opts := metav1.GetOptions{}
	if namespace != "" {
		rv, err = cli.Resource(gvr).Namespace(namespace).Get(ctx, name, opts)
	} else {
		rv, err = cli.Resource(gvr).Get(ctx, name, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("error encountered while fetching resource definition | %w", err)
	}

	jsonRv, err := json.MarshalIndent(rv, "", "    ")
	if err != nil {
		return nil, fmt.Errorf("error encountered while marshalling resource definition to json | %w", err)
	}
	return jsonRv, nil
}

func GetUnstructuredRaw(ctx context.Context, contextName, name, namespace string, gvr *schema.GroupVersionResource) (*unstructured.Unstructured, error) {
	if gvr == nil {
		return nil, fmt.Errorf("GetUnstructuredRaw passed nil gvr")
	}
	cli := getK8sUnstructuredClient()


	var rv *unstructured.Unstructured
	var err error
	opts := metav1.GetOptions{}
	if namespace != "" {
		rv, err = cli.Resource(*gvr).Namespace(namespace).Get(ctx, name, opts)
	} else {
		rv, err = cli.Resource(*gvr).Get(ctx, name, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("error encountered while fetching resource definition | %w", err)
	}

	return rv, nil
}

func WriteUnstructured(
	ctx context.Context, contextName, name, namespace string,
	gvr *schema.GroupVersionResource,
	obj *unstructured.Unstructured,
) (*unstructured.Unstructured, error) {
	if gvr == nil {
		return nil, fmt.Errorf("WriteUnstructured passed nil gvr")
	}

	cli := getK8sUnstructuredClient()
	opts := metav1.UpdateOptions{
		FieldValidation: "Strict",
	}
	var err error
	var rv *unstructured.Unstructured

	if namespace != "" {
		rv, err = cli.Resource(*gvr).Namespace(namespace).Update(ctx, obj, opts)
	} else {
		rv, err = cli.Resource(*gvr).Update(ctx, obj, opts)
	}

	return rv, err
}
