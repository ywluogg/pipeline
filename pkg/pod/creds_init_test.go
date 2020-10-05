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

package pod

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/test/diff"
	"github.com/tektoncd/pipeline/test/names"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakek8s "k8s.io/client-go/kubernetes/fake"
)

const (
	serviceAccountName = "my-service-account"
	namespace          = "namespacey-mcnamespace"
)

func TestCredsInit(t *testing.T) {
	customHomeEnvVar := corev1.EnvVar{
		Name:  "HOME",
		Value: "/users/home/my-test-user",
	}

	for _, c := range []struct {
		desc             string
		wantArgs         []string
		wantVolumeMounts []corev1.VolumeMount
		objs             []runtime.Object
		envVars          []corev1.EnvVar
	}{{
		desc: "service account exists with no secrets; nothing to initialize",
		objs: []runtime.Object{
			&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: serviceAccountName, Namespace: namespace}},
		},
		wantArgs:         nil,
		wantVolumeMounts: nil,
	}, {
		desc: "service account has no annotated secrets; nothing to initialize",
		objs: []runtime.Object{
			&corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: serviceAccountName, Namespace: namespace},
				Secrets: []corev1.ObjectReference{{
					Name: "my-creds",
				}},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "my-creds",
					Namespace:   namespace,
					Annotations: map[string]string{
						// No matching annotations.
					},
				},
			},
		},
		wantArgs:         nil,
		wantVolumeMounts: nil,
	}, {
		desc: "service account has annotated secret and no HOME env var passed in; initialize creds in /tekton/creds",
		objs: []runtime.Object{
			&corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: serviceAccountName, Namespace: namespace},
				Secrets: []corev1.ObjectReference{{
					Name: "my-creds",
				}},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-creds",
					Namespace: namespace,
					Annotations: map[string]string{
						"tekton.dev/docker-0": "https://us.gcr.io",
						"tekton.dev/docker-1": "https://docker.io",
						"tekton.dev/git-0":    "github.com",
						"tekton.dev/git-1":    "gitlab.com",
					},
				},
				Type: "kubernetes.io/basic-auth",
				Data: map[string][]byte{
					"username": []byte("foo"),
					"password": []byte("BestEver"),
				},
			},
		},
		envVars: []corev1.EnvVar{},
		wantArgs: []string{
			"-basic-docker=my-creds=https://docker.io",
			"-basic-docker=my-creds=https://us.gcr.io",
			"-basic-git=my-creds=github.com",
			"-basic-git=my-creds=gitlab.com",
		},
		wantVolumeMounts: []corev1.VolumeMount{{
			Name:      "tekton-internal-secret-volume-my-creds-9l9zj",
			MountPath: "/tekton/creds-secrets/my-creds",
		}},
	}, {
		desc: "service account with secret and HOME env var passed in",
		objs: []runtime.Object{
			&corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: serviceAccountName, Namespace: namespace},
				Secrets: []corev1.ObjectReference{{
					Name: "my-creds",
				}},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-creds",
					Namespace: namespace,
					Annotations: map[string]string{
						"tekton.dev/docker-0": "https://us.gcr.io",
						"tekton.dev/docker-1": "https://docker.io",
						"tekton.dev/git-0":    "github.com",
						"tekton.dev/git-1":    "gitlab.com",
					},
				},
				Type: "kubernetes.io/basic-auth",
				Data: map[string][]byte{
					"username": []byte("foo"),
					"password": []byte("BestEver"),
				},
			},
		},
		envVars: []corev1.EnvVar{customHomeEnvVar},
		wantArgs: []string{
			"-basic-docker=my-creds=https://docker.io",
			"-basic-docker=my-creds=https://us.gcr.io",
			"-basic-git=my-creds=github.com",
			"-basic-git=my-creds=gitlab.com",
		},
		wantVolumeMounts: []corev1.VolumeMount{{
			Name:      "tekton-internal-secret-volume-my-creds-9l9zj",
			MountPath: "/tekton/creds-secrets/my-creds",
		}},
	}} {
		t.Run(c.desc, func(t *testing.T) {
			names.TestingSeed()
			kubeclient := fakek8s.NewSimpleClientset(c.objs...)
			args, volumes, volumeMounts, err := credsInit(serviceAccountName, namespace, kubeclient)
			if err != nil {
				t.Fatalf("credsInit: %v", err)
			}
			if len(args) == 0 && len(volumes) != 0 {
				t.Fatalf("credsInit returned secret volumes but no arguments")
			}
			if d := cmp.Diff(c.wantArgs, args); d != "" {
				t.Fatalf("Diff %s", diff.PrintWantGot(d))
			}
			if d := cmp.Diff(c.wantVolumeMounts, volumeMounts); d != "" {
				t.Fatalf("Diff %s", diff.PrintWantGot(d))
			}
		})
	}
}
