package resources

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParamSpec(t *testing.T) {
	type test struct {
		name        string
		expectedErr error
		params      genericDirParams
		spec        paramsSpec
	}

	tests := []test{
		{
			name:        "test accepts zero values",
			params:      genericDirParams{},
			spec:        paramsSpec{},
			expectedErr: nil,
		},
		{
			name: "checks single missing value - matches generic error",
			spec: paramsSpec{
				contextName: true,
			},
			params:      genericDirParams{},
			expectedErr: eParamsMissing,
		},
		{
			name: "checks single missing value - matches specific error",
			spec: paramsSpec{
				contextName: true,
			},
			params:      genericDirParams{},
			expectedErr: fmt.Errorf("params was missing required values [contextName] | %w", eParamsMissing),
		},
		{
			name: "checks multiple missing values",
			spec: paramsSpec{
				contextName: true,
				namespaced:  true,
				pod:         true,
			},
			params: genericDirParams{
				pod: "somePodValue",
			},
			expectedErr: fmt.Errorf("params was missing required values [contextName, namespaced] | %w", eParamsMissing),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkParams(tc.spec, tc.params)
			if tc.expectedErr == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err, tc.expectedErr)
			}
		})
	}
}
