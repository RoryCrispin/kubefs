package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	var req *rest.Request
	if namespace == "" {
		req = cli.CoreV1().RESTClient().
			Get().AbsPath("apis", groupVersion, resource)
	} else {
		req = cli.CoreV1().RESTClient().
			Get().AbsPath("apis", groupVersion, "namespaces", namespace, resource)
	}

	req.SetHeader("Accept", fmt.Sprintf("application/json;as=Table;v=%s;g=%s", metav1.SchemeGroupVersion.Version, metav1.GroupName))
	fmt.Printf("Requesting %#v\n", req)

	resp := req.Do(ctx)
	body, err := resp.Raw()
	if err != nil {
		return nil, fmt.Errorf("error returned from api server on %v %v | %w", groupVersion, resource, err)
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

func GetPlaintextREST(ctx context.Context, contextName, name, groupVersion, resource, namespace string) ([]byte, error) {

	config, err := GetK8sClientConfig(contextName)
	if err != nil {
		panic(err)
	}

	cli, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	req := cli.CoreV1().RESTClient().Get()
	if namespace != "" {
		req = req.AbsPath("/apis", groupVersion, "namespaces", namespace, resource, name)
	} else {
		req = req.AbsPath("/apis", groupVersion, resource, name)
	}
	req.SetHeader("Accept", "application/json")
	resp := req.Do(ctx)

	fmt.Printf("RESP: %#v", req.URL())

	return resp.Raw()
}

func GetUnstructured() {
	cli := getK8sUnstructuredClient()
	ctx := context.TODO()


	gvr := schema.GroupVersionResource{
		Group: "",
		Version: "networking.k8s.io/v1",
		Resource: "ingresses",
	}
	cli.Resource(gvr).Namespace("eng-dev").List(ctx, metav1.ListOptions{})
}

func GetIngressUnstructured(ctx context.Context, contextName, name, namespace string, gvr *schema.GroupVersionResource) {
	cli := getK8sUnstructuredClient()

	gvr := schema.GroupVersionResource{
		Group: "",
		Version: "networking.k8s.io/v1",
		Resource: "ingresses",
	}
	cli.Resource(gvr).Namespace("eng-dev").List(ctx, metav1.ListOptions{})
}
