/*
Copyright 2019 The Tekton Authors

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

package v1beta1

import (
	"context"
	"encoding/json"
	"fmt"

	resource "github.com/tektoncd/pipeline/pkg/apis/resource/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/substitution"
	"k8s.io/apimachinery/pkg/util/sets"
	"knative.dev/pkg/apis"
)

// ParamSpec defines arbitrary parameters needed beyond typed inputs (such as
// resources). Parameter values are provided by users as inputs on a TaskRun
// or PipelineRun.
type ParamSpec struct {
	// Name declares the name by which a parameter is referenced.
	Name string `json:"name"`
	// Type is the user-specified type of the parameter. The possible types
	// are currently "string" and "array", and "string" is the default.
	// +optional
	Type ParamType `json:"type,omitempty"`
	// Description is a user-facing description of the parameter that may be
	// used to populate a UI.
	// +optional
	Description string `json:"description,omitempty"`
	// Default is the value a parameter takes if no input value is supplied. If
	// default is set, a Task may be executed without a supplied value for the
	// parameter.
	// +optional
	Default *ArrayOrString `json:"default,omitempty"`
}

// SetDefaults set the default type
func (pp *ParamSpec) SetDefaults(ctx context.Context) {
	if pp != nil && pp.Type == "" {
		if pp.Default != nil {
			// propagate the parsed ArrayOrString's type to the parent ParamSpec's type
			pp.Type = pp.Default.Type
		} else {
			// ParamTypeString is the default value (when no type can be inferred from the default value)
			pp.Type = ParamTypeString
		}
	}
}

// ResourceParam declares a string value to use for the parameter called Name, and is used in
// the specific context of PipelineResources.
type ResourceParam = resource.ResourceParam

// Param declares an ArrayOrString to use for the parameter called name.
type Param struct {
	Name  string        `json:"name"`
	Value ArrayOrString `json:"value"`
}

// ParamType indicates the type of an input parameter;
// Used to distinguish between a single string and an array of strings.
type ParamType string

// Valid ParamTypes:
const (
	ParamTypeString ParamType = "string"
	ParamTypeArray  ParamType = "array"
)

// AllParamTypes can be used for ParamType validation.
var AllParamTypes = []ParamType{ParamTypeString, ParamTypeArray}

// ArrayOrString is modeled after IntOrString in kubernetes/apimachinery:

// ArrayOrString is a type that can hold a single string or string array.
// Used in JSON unmarshalling so that a single JSON field can accept
// either an individual string or an array of strings.
type ArrayOrString struct {
	Type      ParamType `json:"type"` // Represents the stored type of ArrayOrString.
	StringVal string    `json:"stringVal"`
	ArrayVal  []string  `json:"arrayVal"`
}

// UnmarshalJSON implements the json.Unmarshaller interface.
func (arrayOrString *ArrayOrString) UnmarshalJSON(value []byte) error {
	if value[0] == '"' {
		arrayOrString.Type = ParamTypeString
		return json.Unmarshal(value, &arrayOrString.StringVal)
	}
	arrayOrString.Type = ParamTypeArray
	return json.Unmarshal(value, &arrayOrString.ArrayVal)
}

// MarshalJSON implements the json.Marshaller interface.
func (arrayOrString ArrayOrString) MarshalJSON() ([]byte, error) {
	switch arrayOrString.Type {
	case ParamTypeString:
		return json.Marshal(arrayOrString.StringVal)
	case ParamTypeArray:
		return json.Marshal(arrayOrString.ArrayVal)
	default:
		return []byte{}, fmt.Errorf("impossible ArrayOrString.Type: %q", arrayOrString.Type)
	}
}

// ApplyReplacements applyes replacements for ArrayOrString type
func (arrayOrString *ArrayOrString) ApplyReplacements(stringReplacements map[string]string, arrayReplacements map[string][]string) {
	if arrayOrString.Type == ParamTypeString {
		arrayOrString.StringVal = ApplyReplacements(arrayOrString.StringVal, stringReplacements)
	} else {
		var newArrayVal []string
		for _, v := range arrayOrString.ArrayVal {
			newArrayVal = append(newArrayVal, ApplyArrayReplacements(v, stringReplacements, arrayReplacements)...)
		}
		arrayOrString.ArrayVal = newArrayVal
	}
}

// NewArrayOrString creates an ArrayOrString of type ParamTypeString or ParamTypeArray, based on
// how many inputs are given (>1 input will create an array, not string).
func NewArrayOrString(value string, values ...string) *ArrayOrString {
	if len(values) > 0 {
		return &ArrayOrString{
			Type:     ParamTypeArray,
			ArrayVal: append([]string{value}, values...),
		}
	}
	return &ArrayOrString{
		Type:      ParamTypeString,
		StringVal: value,
	}
}

func validatePipelineParametersVariablesInTaskParameters(params []Param, prefix string, paramNames sets.String, arrayParamNames sets.String) *apis.FieldError {
	for _, param := range params {
		if param.Value.Type == ParamTypeString {
			if err := validateStringVariableInTaskParameters(fmt.Sprintf("[%s]", param.Name), param.Value.StringVal, prefix, paramNames, arrayParamNames); err != nil {
				return err
			}
		} else {
			for _, arrayElement := range param.Value.ArrayVal {
				if err := validateArrayVariableInTaskParameters(fmt.Sprintf("[%s]", param.Name), arrayElement, prefix, paramNames, arrayParamNames); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateStringVariableInTaskParameters(name, value, prefix string, stringVars sets.String, arrayVars sets.String) *apis.FieldError {
	if err := substitution.ValidateVariable(name, value, prefix, "task parameter", "pipelinespec.params", stringVars); err != nil {
		return err
	}
	if err := substitution.ValidateVariableProhibited(name, value, prefix, "task parameter", "pipelinespec.params", arrayVars); err != nil {
		return err
	}
	return nil
}

func validateArrayVariableInTaskParameters(name, value, prefix string, stringVars sets.String, arrayVars sets.String) *apis.FieldError {
	if err := substitution.ValidateVariable(name, value, prefix, "task parameter", "pipelinespec.params", stringVars); err != nil {
		return err
	}
	if err := substitution.ValidateVariableIsolated(name, value, prefix, "task parameter", "pipelinespec.params", arrayVars); err != nil {
		return err
	}
	return nil
}
