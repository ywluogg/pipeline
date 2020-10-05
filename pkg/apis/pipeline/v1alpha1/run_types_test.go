/*
Copyright 2020 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	v1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned/scheme"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

func TestGetParams(t *testing.T) {
	for _, c := range []struct {
		desc string
		spec v1alpha1.RunSpec
		name string
		want *v1beta1.Param
	}{{
		desc: "no params",
		spec: v1alpha1.RunSpec{},
		name: "anything",
		want: nil,
	}, {
		desc: "found",
		spec: v1alpha1.RunSpec{
			Params: []v1beta1.Param{{
				Name:  "first",
				Value: *v1beta1.NewArrayOrString("blah"),
			}, {
				Name:  "foo",
				Value: *v1beta1.NewArrayOrString("bar"),
			}},
		},
		name: "foo",
		want: &v1beta1.Param{
			Name:  "foo",
			Value: *v1beta1.NewArrayOrString("bar"),
		},
	}, {
		desc: "not found",
		spec: v1alpha1.RunSpec{
			Params: []v1beta1.Param{{
				Name:  "first",
				Value: *v1beta1.NewArrayOrString("blah"),
			}, {
				Name:  "foo",
				Value: *v1beta1.NewArrayOrString("bar"),
			}},
		},
		name: "bar",
		want: nil,
	}, {
		// This shouldn't happen since it's invalid, but just in
		// case, GetParams just returns the first param it finds with
		// the specified name.
		desc: "multiple with same name",
		spec: v1alpha1.RunSpec{
			Params: []v1beta1.Param{{
				Name:  "first",
				Value: *v1beta1.NewArrayOrString("blah"),
			}, {
				Name:  "foo",
				Value: *v1beta1.NewArrayOrString("bar"),
			}, {
				Name:  "foo",
				Value: *v1beta1.NewArrayOrString("second bar"),
			}},
		},
		name: "foo",
		want: &v1beta1.Param{
			Name:  "foo",
			Value: *v1beta1.NewArrayOrString("bar"),
		},
	}} {
		t.Run(c.desc, func(t *testing.T) {
			got := c.spec.GetParam(c.name)
			if d := cmp.Diff(c.want, got); d != "" {
				t.Fatalf("Diff(-want,+got): %v", d)
			}
		})
	}
}

// TestRunStatusExtraFields tests that extraFields in a RunStatus can be parsed
// from YAML.
func TestRunStatus(t *testing.T) {
	in := `apiVersion: tekton.dev/v1alpha1
kind: Run
metadata:
  name: run
spec:
  ref:
    apiVersion: example.dev/v0
    kind: Example
status:
  conditions:
  - type: "Succeeded"
    status: "True"
  results:
  - name: foo
    value: bar
  extraFields:
    simple: 'hello'
    complex:
      hello: ['w', 'o', 'r', 'l', 'd']
`
	var r v1alpha1.Run
	if _, _, err := scheme.Codecs.UniversalDeserializer().Decode([]byte(in), nil, &r); err != nil {
		t.Fatalf("Decode YAML: %v", err)
	}

	want := &v1alpha1.Run{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "tekton.dev/v1alpha1",
			Kind:       "Run",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "run",
		},
		Spec: v1alpha1.RunSpec{
			Ref: &v1alpha1.TaskRef{
				APIVersion: "example.dev/v0",
				Kind:       "Example",
			},
		},
		Status: v1alpha1.RunStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionTrue,
				}},
			},
			RunStatusFields: v1alpha1.RunStatusFields{
				// Results are parsed correctly.
				Results: []v1beta1.TaskRunResult{{
					Name:  "foo",
					Value: "bar",
				}},
				// Any extra fields are simply stored as JSON bytes.
				ExtraFields: runtime.RawExtension{
					Raw: []byte(`{"complex":{"hello":["w","o","r","l","d"]},"simple":"hello"}`),
				},
			},
		},
	}
	if d := cmp.Diff(want, &r); d != "" {
		t.Fatalf("Diff(-want,+got): %s", d)
	}
}
