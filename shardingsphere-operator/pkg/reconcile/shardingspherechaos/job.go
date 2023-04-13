/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package shardingspherechaos

import (
	"reflect"
	"strconv"

	"github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/api/v1alpha1"
	"github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/pkg/reconcile/common"
	v1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DefaultImageName     = "tools-runtime:1.0"
	DefaultContainerName = "tools-runtime"
)

const (
	completions             = "jobs.batch/completions"
	activeDeadlineSeconds   = "jobs.batch/activeDeadlineSeconds"
	parallelism             = "job.batch/parallelism"
	backoffLimit            = "job.batch/backoffLimit"
	ttlSecondsAfterFinished = "job.batch/ttlSecondsAfterFinished"
	suspend                 = "job.batch/suspend"
)

type InjectRequirement string

var (
	Experimental InjectRequirement = "experimental"
	Pressure     InjectRequirement = "pressure"
)

// todo
func NewJob(ssChaos *v1alpha1.ShardingSphereChaos, requirement InjectRequirement) (*v1.Job, error) {
	jbd := NewJobBuilder()
	jbd.SetNamespace(ssChaos.Namespace).SetLabels(ssChaos.Labels).SetName(ssChaos.Name)

	if v, ok := ssChaos.Annotations[completions]; ok {
		value, err := MustInt32(v)
		if err != nil {
			return nil, err
		}
		jbd.SetCompletions(value)
	}

	if v, ok := ssChaos.Annotations[activeDeadlineSeconds]; ok {
		value, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, err
		}
		jbd.SetActiveDeadlineSeconds(value)
	}

	if v, ok := ssChaos.Annotations[parallelism]; ok {
		value, err := MustInt32(v)
		if err != nil {
			return nil, err
		}
		jbd.SetParallelism(value)
	}

	if v, ok := ssChaos.Annotations[backoffLimit]; ok {
		value, err := MustInt32(v)
		if err != nil {
			return nil, err
		}
		jbd.SetBackoffLimit(value)
	}

	if v, ok := ssChaos.Annotations[ttlSecondsAfterFinished]; ok {
		value, err := MustInt32(v)
		if err != nil {
			return nil, err
		}
		jbd.SetTTLSecondsAfterFinished(value)
	}

	if v, ok := ssChaos.Annotations[suspend]; ok {
		if v == "true" {
			jbd.SetSuspend(true)
		}

		if v == "false" {
			jbd.SetSuspend(false)
		}
	}

	cbd := common.NewContainerBuilder()

	cbd.SetImage("perl:5.34.0")
	cbd.SetName(DefaultContainerName)
	//todo: add cmd line

	cbd.SetCommand([]string{"perl", "-Mbignum=bpi", "-wle", "print bpi(1000)"})
	container := cbd.Build()
	jbd.SetContainers(container)
	rjob := jbd.Build()
	return rjob, nil
}

func MustInt32(s string) (int32, error) {
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(v), nil
}

func UpdateJob(ssChaos *v1alpha1.ShardingSphereChaos, requirement InjectRequirement, cur *v1.Job) (*v1.Job, error) {
	exp := &v1.Job{}
	exp.ObjectMeta = cur.ObjectMeta
	exp.Labels = cur.Labels
	exp.Annotations = cur.Annotations
	now, err := NewJob(ssChaos, requirement)
	if err != nil {
		return nil, err
	}
	if reflect.DeepEqual(now.Spec, cur.Spec) {
		return nil, nil
	}
	exp.Spec = now.Spec
	return exp, nil
}

type JobBuilder interface {
	SetName(string) JobBuilder
	SetNamespace(string) JobBuilder
	SetLabels(map[string]string) JobBuilder
	SetCompletions(int32) JobBuilder
	SetActiveDeadlineSeconds(int64) JobBuilder
	SetParallelism(int32) JobBuilder
	SetBackoffLimit(int32) JobBuilder
	SetContainers(*corev1.Container) JobBuilder
	SetTTLSecondsAfterFinished(int32) JobBuilder
	SetSuspend(bool) JobBuilder
	Build() *v1.Job
}

func NewJobBuilder() JobBuilder {
	return &jobBuilder{
		defaultJob(),
	}
}

type jobBuilder struct {
	job *v1.Job
}

func (j *jobBuilder) SetName(name string) JobBuilder {
	j.job.ObjectMeta.Name = name
	return j
}

func (j *jobBuilder) SetNamespace(namespace string) JobBuilder {
	j.job.ObjectMeta.Namespace = namespace
	return j
}

func (j *jobBuilder) SetLabels(labels map[string]string) JobBuilder {
	j.job.ObjectMeta.Labels = labels
	return j
}

func (j *jobBuilder) SetCompletions(i int32) JobBuilder {
	j.job.Spec.Completions = &i
	return j
}

func (j *jobBuilder) SetActiveDeadlineSeconds(i int64) JobBuilder {
	j.job.Spec.ActiveDeadlineSeconds = &i
	return j
}

func (j *jobBuilder) SetParallelism(i int32) JobBuilder {
	j.job.Spec.Parallelism = &i
	return j
}

func (j *jobBuilder) SetBackoffLimit(i int32) JobBuilder {
	j.job.Spec.BackoffLimit = &i
	return j
}

func (j *jobBuilder) SetContainers(container *corev1.Container) JobBuilder {
	if j.job.Spec.Template.Spec.Containers == nil {
		j.job.Spec.Template.Spec.Containers = []corev1.Container{*container}
	}

	for i := range j.job.Spec.Template.Spec.Containers {
		if j.job.Spec.Template.Spec.Containers[i].Name == DefaultContainerName {
			j.job.Spec.Template.Spec.Containers[i] = *container
			return j
		}
	}

	j.job.Spec.Template.Spec.Containers = append(j.job.Spec.Template.Spec.Containers, *container)
	return j
}

func (j *jobBuilder) SetTTLSecondsAfterFinished(i int32) JobBuilder {
	j.job.Spec.TTLSecondsAfterFinished = &i
	return j
}

func (j *jobBuilder) SetSuspend(b bool) JobBuilder {
	j.job.Spec.Suspend = &b
	return j
}

func (j *jobBuilder) Build() *v1.Job {
	return j.job
}

func defaultJob() *v1.Job {
	return &v1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "shardingsphere-proxy",
		},
		Spec: v1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers:    []corev1.Container{},
					RestartPolicy: corev1.RestartPolicyOnFailure,
				},
			},
		},
	}
}
