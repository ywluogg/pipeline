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

package config

import (
	"fmt"
	"os"
	"strconv"

	corev1 "k8s.io/api/core/v1"
)

const (
	disableHomeEnvOverwriteKey              = "disable-home-env-overwrite"
	disableWorkingDirOverwriteKey           = "disable-working-directory-overwrite"
	disableAffinityAssistantKey             = "disable-affinity-assistant"
	DisableTaskRunStepTimeoutCheckKey       = "disable-taskrun-step-timeout-check"
	runningInEnvWithInjectedSidecarsKey     = "running-in-environment-with-injected-sidecars"
	DefaultDisableHomeEnvOverwrite          = false
	DefaultDisableWorkingDirOverwrite       = false
	DefaultDisableAffinityAssistant         = false
	DefaultDisableTaskRunStepTimeoutCheck   = true
	DefaultRunningInEnvWithInjectedSidecars = true
)

// FeatureFlags holds the features configurations
// +k8s:deepcopy-gen=true
type FeatureFlags struct {
	DisableHomeEnvOverwrite          bool
	DisableWorkingDirOverwrite       bool
	DisableAffinityAssistant         bool
	DisableTaskRunStepTimeoutCheck   bool
	RunningInEnvWithInjectedSidecars bool
}

// GetFeatureFlagsConfigName returns the name of the configmap containing all
// feature flags.
func GetFeatureFlagsConfigName() string {
	if e := os.Getenv("CONFIG_FEATURE_FLAGS_NAME"); e != "" {
		return e
	}
	return "feature-flags"
}

// NewFeatureFlagsFromMap returns a Config given a map corresponding to a ConfigMap
func NewFeatureFlagsFromMap(cfgMap map[string]string) (*FeatureFlags, error) {
	setFeature := func(key string, defaultValue bool, feature *bool) error {
		if cfg, ok := cfgMap[key]; ok {
			value, err := strconv.ParseBool(cfg)
			if err != nil {
				return fmt.Errorf("failed parsing feature flags config %q: %v", cfg, err)
			}
			*feature = value
			return nil
		}
		*feature = defaultValue
		return nil
	}

	tc := FeatureFlags{}
	if err := setFeature(disableHomeEnvOverwriteKey, DefaultDisableHomeEnvOverwrite, &tc.DisableHomeEnvOverwrite); err != nil {
		return nil, err
	}
	if err := setFeature(disableWorkingDirOverwriteKey, DefaultDisableWorkingDirOverwrite, &tc.DisableWorkingDirOverwrite); err != nil {
		return nil, err
	}
	if err := setFeature(disableAffinityAssistantKey, DefaultDisableAffinityAssistant, &tc.DisableAffinityAssistant); err != nil {
		return nil, err
	}
	if err := setFeature(DisableTaskRunStepTimeoutCheckKey, DefaultDisableTaskRunStepTimeoutCheck, &tc.DisableTaskRunStepTimeoutCheck); err != nil {
		return nil, err
	}
	if err := setFeature(runningInEnvWithInjectedSidecarsKey, DefaultRunningInEnvWithInjectedSidecars, &tc.RunningInEnvWithInjectedSidecars); err != nil {
		return nil, err
	}
	return &tc, nil
}

// NewFeatureFlagsFromConfigMap returns a Config for the given configmap
func NewFeatureFlagsFromConfigMap(config *corev1.ConfigMap) (*FeatureFlags, error) {
	return NewFeatureFlagsFromMap(config.Data)
}
