package kubernetes

import (
	"context"
	"fmt"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

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

	req := cli.CoreV1().RESTClient().
		Get().AbsPath("apis", groupVersion, "namespaces", namespace, resource)
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
		fmt.Printf("Resource: %v\n", res.Cells[nameColumnIdx])
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

func GetIngressPlaintext()  {
	ctx := context.TODO()
	contextName := "eng-instances"

	config, err := GetK8sClientConfig(contextName)
	if err != nil {
		panic(err)
	}

	cli, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	req := cli.CoreV1().RESTClient().
		Get().AbsPath("/apis/networking.k8s.io/v1/namespaces/eng-dev/ingresses")
	req.SetHeader("Accept", fmt.Sprintf("application/json;as=Table;v=%s;g=%s", metav1.SchemeGroupVersion.Version, metav1.GroupName))

	resp := req.Do(ctx)
	fmt.Printf("RESP: %#v", req.URL())

	body, err := resp.Raw()

	resTable := metav1.Table{}

	err = json.Unmarshal(body, &resTable)
	if err != nil {
		panic(err)
	}

	for _, ing := range resTable.Rows {
		fmt.Printf("Ingress: %v\n", ing.Cells[0])
	}
}

func GetIngressUnstructured() {
	cli := getK8sUnstructuredClient()
	ctx := context.TODO()


	gvr := schema.GroupVersionResource{
		Group: "",
		Version: "networking.k8s.io/v1",
		Resource: "ingresses",
	}
	cli.Resource(gvr).Namespace("eng-dev").List(ctx, metav1.ListOptions{})
}
