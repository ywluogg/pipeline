// +build e2e

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

package test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	knativetest "knative.dev/pkg/test"
)

// TestPipelineRunTimeout is an integration test that will
// verify that pipelinerun timeout works and leads to the the correct TaskRun statuses
// and pod deletions.
func TestPipelineRunTimeout(t *testing.T) {
	c, namespace := setup(t)
	t.Parallel()

	knativetest.CleanupOnInterrupt(func() { tearDown(t, c, namespace) }, t.Logf)
	defer tearDown(t, c, namespace)

	t.Logf("Creating Task in namespace %s", namespace)
	task := &v1beta1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "banana", Namespace: namespace},
		Spec: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{Container: corev1.Container{
				Image:   "busybox",
				Command: []string{"/bin/sh"},
				Args:    []string{"-c", "sleep 10"},
			}}},
		},
	}
	if _, err := c.TaskClient.Create(task); err != nil {
		t.Fatalf("Failed to create Task `%s`: %s", "banana", err)
	}

	pipeline := &v1beta1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "tomatoes", Namespace: namespace},
		Spec: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				Name:    "foo",
				TaskRef: &v1beta1.TaskRef{Name: "banana"},
			}},
		},
	}
	pipelineRun := &v1beta1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{Name: "pear", Namespace: namespace},
		Spec: v1beta1.PipelineRunSpec{
			PipelineRef: &v1beta1.PipelineRef{Name: pipeline.Name},
			Timeout:     &metav1.Duration{Duration: 5 * time.Second},
		},
	}
	if _, err := c.PipelineClient.Create(pipeline); err != nil {
		t.Fatalf("Failed to create Pipeline `%s`: %s", pipeline.Name, err)
	}
	if _, err := c.PipelineRunClient.Create(pipelineRun); err != nil {
		t.Fatalf("Failed to create PipelineRun `%s`: %s", pipelineRun.Name, err)
	}

	t.Logf("Waiting for Pipelinerun %s in namespace %s to be started", pipelineRun.Name, namespace)
	if err := WaitForPipelineRunState(c, pipelineRun.Name, timeout, Running(pipelineRun.Name), "PipelineRunRunning"); err != nil {
		t.Fatalf("Error waiting for PipelineRun %s to be running: %s", pipelineRun.Name, err)
	}

	taskrunList, err := c.TaskRunClient.List(metav1.ListOptions{LabelSelector: fmt.Sprintf("tekton.dev/pipelineRun=%s", pipelineRun.Name)})
	if err != nil {
		t.Fatalf("Error listing TaskRuns for PipelineRun %s: %s", pipelineRun.Name, err)
	}

	t.Logf("Waiting for TaskRuns from PipelineRun %s in namespace %s to be running", pipelineRun.Name, namespace)
	errChan := make(chan error, len(taskrunList.Items))
	defer close(errChan)

	for _, taskrunItem := range taskrunList.Items {
		go func(name string) {
			err := WaitForTaskRunState(c, name, Running(name), "TaskRunRunning")
			errChan <- err
		}(taskrunItem.Name)
	}

	for i := 1; i <= len(taskrunList.Items); i++ {
		if <-errChan != nil {
			t.Errorf("Error waiting for TaskRun %s to be running: %s", taskrunList.Items[i-1].Name, err)
		}
	}

	if _, err := c.PipelineRunClient.Get(pipelineRun.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("Failed to get PipelineRun `%s`: %s", pipelineRun.Name, err)
	}

	t.Logf("Waiting for PipelineRun %s in namespace %s to be timed out", pipelineRun.Name, namespace)
	if err := WaitForPipelineRunState(c, pipelineRun.Name, timeout, FailedWithReason(v1beta1.PipelineRunReasonTimedOut.String(), pipelineRun.Name), "PipelineRunTimedOut"); err != nil {
		t.Errorf("Error waiting for PipelineRun %s to finish: %s", pipelineRun.Name, err)
	}

	t.Logf("Waiting for TaskRuns from PipelineRun %s in namespace %s to time out and be cancelled", pipelineRun.Name, namespace)
	var wg sync.WaitGroup
	for _, taskrunItem := range taskrunList.Items {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			err := WaitForTaskRunState(c, name, FailedWithReason("TaskRunTimeout", name), "TaskRunTimeout")
			if err != nil {
				t.Errorf("Error waiting for TaskRun %s to timeout: %s", name, err)
			}
		}(taskrunItem.Name)
	}
	wg.Wait()

	if _, err := c.PipelineRunClient.Get(pipelineRun.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("Failed to get PipelineRun `%s`: %s", pipelineRun.Name, err)
	}

	// Verify that we can create a second Pipeline using the same Task without a Pipeline-level timeout that will not
	// time out
	secondPipeline := &v1beta1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "peppers", Namespace: namespace},
		Spec: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				Name:    "foo",
				TaskRef: &v1beta1.TaskRef{Name: "banana"},
			}},
		},
	}
	secondPipelineRun := &v1beta1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{Name: "kiwi", Namespace: namespace},
		Spec: v1beta1.PipelineRunSpec{
			PipelineRef: &v1beta1.PipelineRef{Name: "peppers"},
		},
	}
	if _, err := c.PipelineClient.Create(secondPipeline); err != nil {
		t.Fatalf("Failed to create Pipeline `%s`: %s", secondPipeline.Name, err)
	}
	if _, err := c.PipelineRunClient.Create(secondPipelineRun); err != nil {
		t.Fatalf("Failed to create PipelineRun `%s`: %s", secondPipelineRun.Name, err)
	}

	t.Logf("Waiting for PipelineRun %s in namespace %s to complete", secondPipelineRun.Name, namespace)
	if err := WaitForPipelineRunState(c, secondPipelineRun.Name, timeout, PipelineRunSucceed(secondPipelineRun.Name), "PipelineRunSuccess"); err != nil {
		t.Fatalf("Error waiting for PipelineRun %s to finish: %s", secondPipelineRun.Name, err)
	}
}

// TestStepTimeout is an integration test that will verify a Step can be timed out.
func TestStepTimeout(t *testing.T) {
	c, namespace := setup(t)
	t.Parallel()

	knativetest.CleanupOnInterrupt(func() { tearDown(t, c, namespace) }, t.Logf)
	defer tearDown(t, c, namespace)

	t.Logf("Creating Task with Step step-no-timeout, Step step-timeout, and Step step-canceled in namespace %s", namespace)

	taskrunName := "run-timeout"

	t.Logf("Creating TaskRun %s in namespace %s", taskrunName, namespace)
	taskRun := &v1beta1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{Name: taskrunName, Namespace: namespace},
		Spec: v1beta1.TaskRunSpec{
			TaskSpec: &v1beta1.TaskSpec{
				Steps: []v1beta1.Step{{
					Container: corev1.Container{
						Name:  "no-timeout",
						Image: "busybox",
					},
					Script:  "sleep 1",
					Timeout: "2s",
				}, {
					Container: corev1.Container{
						Name:  "timeout",
						Image: "busybox",
					},
					Script:  "sleep 1",
					Timeout: "1ms",
				}, {
					Container: corev1.Container{
						Name:  "canceled",
						Image: "busybox",
					},
					Script: "sleep 1",
				},
				},
			},
		},
	}
	if _, err := c.TaskRunClient.Create(taskRun); err != nil {
		t.Fatalf("Failed to create TaskRun `%s`: %s", taskrunName, err)
	}

	failMsg := "\"step-timeout\" exited because the step exceeded the specified timeout limit"
	t.Logf("Waiting for %s in namespace %s to time out", "step-timeout", namespace)
	if err := WaitForTaskRunState(c, taskrunName, FailedWithMessage(failMsg, "run-timeout"), "StepTimeout"); err != nil {
		t.Logf("Error in taskRun %s status: %s\n", taskrunName, err)
		t.Errorf("Expected: %s", failMsg)
	}

	tr, err := c.TaskRunClient.Get(taskrunName, metav1.GetOptions{})
	if err != nil {
		t.Errorf("Error getting Taskrun: %v", err)
	}
	if tr.Status.Steps[0].Terminated == nil {
		if tr.Status.Steps[0].Terminated.Reason != "Completed" {
			t.Errorf("step-no-timeout should not have been terminated")
		}
	}
	if tr.Status.Steps[2].Terminated == nil {
		t.Errorf("step-canceled should have been canceled after step-timeout timed out")
	} else if exitcode := tr.Status.Steps[2].Terminated.ExitCode; exitcode != 1 {
		t.Logf("step-canceled exited with exit code %d, expected exit code 1", exitcode)
	}

}

// TestTaskRunTimeout is an integration test that will verify a TaskRun can be timed out.
func TestTaskRunTimeout(t *testing.T) {
	c, namespace := setup(t)
	t.Parallel()

	knativetest.CleanupOnInterrupt(func() { tearDown(t, c, namespace) }, t.Logf)
	defer tearDown(t, c, namespace)

	t.Logf("Creating Task and TaskRun in namespace %s", namespace)
	task := &v1beta1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "giraffe", Namespace: namespace},
		Spec: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{Container: corev1.Container{
				Image:   "busybox",
				Command: []string{"/bin/sh"},
				Args:    []string{"-c", "sleep 3000"},
			}}},
		},
	}
	if _, err := c.TaskClient.Create(task); err != nil {
		t.Fatalf("Failed to create Task `%s`: %s", "giraffe", err)
	}
	taskRun := &v1beta1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{Name: "run-giraffe", Namespace: namespace},
		Spec: v1beta1.TaskRunSpec{
			TaskRef: &v1beta1.TaskRef{Name: "giraffe"},
			// Do not reduce this timeout. Taskrun e2e test is also verifying
			// if reconcile is triggered from timeout handler and not by pod informers
			Timeout: &metav1.Duration{Duration: 30 * time.Second},
		},
	}
	if _, err := c.TaskRunClient.Create(taskRun); err != nil {
		t.Fatalf("Failed to create TaskRun `%s`: %s", taskRun.Name, err)
	}

	t.Logf("Waiting for TaskRun %s in namespace %s to complete", "run-giraffe", namespace)
	if err := WaitForTaskRunState(c, taskRun.Name, FailedWithReason(v1beta1.TaskRunReasonTimedOut.String(), taskRun.Name), v1beta1.TaskRunReasonTimedOut.String()); err != nil {
		t.Errorf("Error waiting for TaskRun %s to finish: %s", "run-giraffe", err)
	}

	tr, err := c.TaskRunClient.Get(taskRun.Name, metav1.GetOptions{})
	if err != nil {
		t.Errorf("Error retrieving TaskRun %s: %v", taskRun.Name, err)
	}

	for _, step := range tr.Status.Steps {
		if step.Terminated == nil {
			t.Errorf("TaskRun %s step %s does not have a terminated state but should", taskRun.Name, step.Name)
		}
		if d := cmp.Diff(step.Terminated.Reason, v1beta1.TaskRunReasonTimedOut.String()); d != "" {
			t.Fatalf("-got, +want: %v", d)
		}
	}
}

func TestPipelineTaskTimeout(t *testing.T) {
	c, namespace := setup(t)
	t.Parallel()

	knativetest.CleanupOnInterrupt(func() { tearDown(t, c, namespace) }, t.Logf)
	defer tearDown(t, c, namespace)

	t.Logf("Creating Tasks in namespace %s", namespace)
	task1 := &v1beta1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "success", Namespace: namespace},
		Spec: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{Container: corev1.Container{
				Image:   "busybox",
				Command: []string{"sleep"},
				Args:    []string{"1s"},
			}}},
		},
	}
	task2 := &v1beta1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "timeout", Namespace: namespace},
		Spec: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{Container: corev1.Container{
				Image:   "busybox",
				Command: []string{"sleep"},
				Args:    []string{"10s"},
			}}},
		},
	}

	if _, err := c.TaskClient.Create(task1); err != nil {
		t.Fatalf("Failed to create Task `%s`: %s", task1.Name, err)
	}
	if _, err := c.TaskClient.Create(task2); err != nil {
		t.Fatalf("Failed to create Task `%s`: %s", task2.Name, err)
	}

	pipeline := &v1beta1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "pipelinetasktimeout", Namespace: namespace},
		Spec: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				Name:    "pipelinetask1",
				TaskRef: &v1beta1.TaskRef{Name: task1.Name},
				Timeout: &metav1.Duration{Duration: 60 * time.Second},
			}, {
				Name:    "pipelinetask2",
				TaskRef: &v1beta1.TaskRef{Name: task2.Name},
				Timeout: &metav1.Duration{Duration: 5 * time.Second},
			}},
		},
	}
	pipelineRun := &v1beta1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{Name: "prtasktimeout", Namespace: namespace},
		Spec: v1beta1.PipelineRunSpec{
			PipelineRef: &v1beta1.PipelineRef{Name: pipeline.Name},
		},
	}

	if _, err := c.PipelineClient.Create(pipeline); err != nil {
		t.Fatalf("Failed to create Pipeline `%s`: %s", pipeline.Name, err)
	}
	if _, err := c.PipelineRunClient.Create(pipelineRun); err != nil {
		t.Fatalf("Failed to create PipelineRun `%s`: %s", pipelineRun.Name, err)
	}

	t.Logf("Waiting for Pipelinerun %s in namespace %s to be started", pipelineRun.Name, namespace)
	if err := WaitForPipelineRunState(c, pipelineRun.Name, timeout, Running(pipelineRun.Name), "PipelineRunRunning"); err != nil {
		t.Fatalf("Error waiting for PipelineRun %s to be running: %s", pipelineRun.Name, err)
	}

	taskrunList, err := c.TaskRunClient.List(metav1.ListOptions{LabelSelector: fmt.Sprintf("tekton.dev/pipelineRun=%s", pipelineRun.Name)})
	if err != nil {
		t.Fatalf("Error listing TaskRuns for PipelineRun %s: %s", pipelineRun.Name, err)
	}

	t.Logf("Waiting for TaskRuns from PipelineRun %s in namespace %s to be running", pipelineRun.Name, namespace)
	errChan := make(chan error, len(taskrunList.Items))
	defer close(errChan)

	for _, taskrunItem := range taskrunList.Items {
		go func(name string) {
			err := WaitForTaskRunState(c, name, Running(name), "TaskRunRunning")
			errChan <- err
		}(taskrunItem.Name)
	}

	for i := 1; i <= len(taskrunList.Items); i++ {
		if <-errChan != nil {
			t.Errorf("Error waiting for TaskRun %s to be running: %s", taskrunList.Items[i-1].Name, err)
		}
	}

	if _, err := c.PipelineRunClient.Get(pipelineRun.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("Failed to get PipelineRun `%s`: %s", pipelineRun.Name, err)
	}

	t.Logf("Waiting for PipelineRun %s with PipelineTask timeout in namespace %s to fail", pipelineRun.Name, namespace)
	if err := WaitForPipelineRunState(c, pipelineRun.Name, timeout, FailedWithReason(v1beta1.PipelineRunReasonFailed.String(), pipelineRun.Name), "PipelineRunTimedOut"); err != nil {
		t.Fatalf("Error waiting for PipelineRun %s to finish: %s", pipelineRun.Name, err)
	}

	t.Logf("Waiting for TaskRun from PipelineRun %s in namespace %s to be timed out", pipelineRun.Name, namespace)
	var wg sync.WaitGroup
	for _, taskrunItem := range taskrunList.Items {
		wg.Add(1)
		go func(tr v1beta1.TaskRun) {
			defer wg.Done()
			name := tr.Name
			err := WaitForTaskRunState(c, name, func(ca apis.ConditionAccessor) (bool, error) {
				cond := ca.GetCondition(apis.ConditionSucceeded)
				if cond != nil {
					if tr.Spec.TaskRef.Name == task1.Name && cond.Status == corev1.ConditionTrue {
						if cond.Reason == "Succeeded" {
							return true, nil
						}
						return true, fmt.Errorf("taskRun %q completed with the wrong reason: %s", task1.Name, cond.Reason)
					} else if tr.Spec.TaskRef.Name == task1.Name && cond.Status == corev1.ConditionFalse {
						return true, fmt.Errorf("taskRun %q failed, but should have been Succeeded", name)
					}

					if tr.Spec.TaskRef.Name == task2.Name && cond.Status == corev1.ConditionFalse {
						if cond.Reason == "TaskRunTimeout" {
							return true, nil
						}
						return true, fmt.Errorf("taskRun %q completed with the wrong reason: %s", task2.Name, cond.Reason)
					} else if tr.Spec.TaskRef.Name == task2.Name && cond.Status == corev1.ConditionTrue {
						return true, fmt.Errorf("taskRun %q should have timed out", name)
					}
				}
				return false, nil
			}, "TaskRunTimeout")
			if err != nil {
				t.Errorf("Error waiting for TaskRun %s to timeout: %s", name, err)
			}
		}(taskrunItem)
	}
	wg.Wait()
}
