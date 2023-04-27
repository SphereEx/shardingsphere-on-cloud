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

package controllers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/api/v1alpha1"
	"github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/pkg/kubernetes/configmap"
	"github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/pkg/kubernetes/job"
	sschaos "github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/pkg/kubernetes/shardingspherechaos"
	reconcile "github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/pkg/reconcile/shardingspherechaos"

	"github.com/go-logr/logr"
	batchV1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ShardingSphereChaosControllerName = "shardingsphere-chaos-controller"
	VerifyJobCheck                    = "Verify"
)

type JobCondition string

var (
	CompleteJob JobCondition = "complete"
	FailureJob  JobCondition = "failure"
	SuspendJob  JobCondition = "suspend"
	ActiveJob   JobCondition = "active"

	ErrNoPod = errors.New("no pod in list")
)

// ShardingSphereChaosReconciler is a controller for the ShardingSphereChaos
type ShardingSphereChaosReconciler struct { //
	client.Client

	Scheme    *runtime.Scheme
	Log       logr.Logger
	ClientSet *clientset.Clientset
	Events    record.EventRecorder

	Chaos     sschaos.Chaos
	Job       job.Job
	ConfigMap configmap.ConfigMap
}

// Reconcile handles main function of this controller
func (r *ShardingSphereChaosReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues(ShardingSphereChaosControllerName, req.NamespacedName)

	ssChaos, err := r.getRuntimeChaos(ctx, req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !ssChaos.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	logger.Info("start reconcile chaos")

	//FIXME
	if err := r.reconcileChaos(ctx, ssChaos); err != nil {
		if err == reconcile.ErrChangedSpec {
			errHandle := r.handleChaosChange(ctx, req.NamespacedName)
			return ctrl.Result{}, errHandle
		}

		logger.Error(err, "reconcile shardingspherechaos error")
		//FIXME
		r.Events.Event(ssChaos, "Warning", "shardingspherechaos error", err.Error())
		return ctrl.Result{}, err
	}

	if err := r.reconcileConfigMap(ctx, ssChaos); err != nil {
		logger.Error(err, "reconcile configmap error")
		//FIXME
		r.Events.Event(ssChaos, "Warning", "configmap err", err.Error())

		return ctrl.Result{}, err
	}

	if err := r.reconcileJob(ctx, ssChaos); err != nil {
		logger.Error(err, "reconcile job error")
		//FIXME
		r.Events.Event(ssChaos, "Warning", "job err", err.Error())

		return ctrl.Result{}, err
	}

	if err := r.reconcileStatus(ctx, req.NamespacedName); err != nil {
		//FIXME
		r.Events.Event(ssChaos, "Warning", "update status error", err.Error())
		logger.Error(err, "failed to update status")
	}

	return ctrl.Result{RequeueAfter: defaultRequeueTime}, nil
}

func (r *ShardingSphereChaosReconciler) getRuntimeChaos(ctx context.Context, name types.NamespacedName) (*v1alpha1.ShardingSphereChaos, error) {
	var rt = &v1alpha1.ShardingSphereChaos{}
	err := r.Get(ctx, name, rt)
	return rt, client.IgnoreNotFound(err)
}

func (r *ShardingSphereChaosReconciler) reconcileChaos(ctx context.Context, chaos *v1alpha1.ShardingSphereChaos) error {
	logger := r.Log.WithValues("reconcile shardingspherechaos", fmt.Sprintf("%s/%s", chaos.Namespace, chaos.Name))

	if len(chaos.Status.Phase) == 0 || chaos.Status.Phase == v1alpha1.BeforeExperiment {
		return nil
	}

	namespacedName := types.NamespacedName{
		Namespace: chaos.Namespace,
		Name:      chaos.Name,
	}

	if chaos.Spec.EmbedChaos.PodChaos != nil {
		if err := r.reconcilePodChaos(ctx, chaos, namespacedName); err != nil {
			logger.Error(err, "reconcile pod chaos error")
			return err
		}
	}

	if chaos.Spec.EmbedChaos.NetworkChaos != nil {
		if err := r.reconcileNetworkChaos(ctx, chaos, namespacedName); err != nil {
			logger.Error(err, "reconcile network chaos error")
			return err
		}
	}

	return nil
}

func (r *ShardingSphereChaosReconciler) reconcilePodChaos(ctx context.Context, chaos *v1alpha1.ShardingSphereChaos, namespacedName types.NamespacedName) error {
	pc, err := r.getPodChaosByNamespacedName(ctx, namespacedName)
	if err != nil {
		return err
	}
	if pc != nil {
		return r.updatePodChaos(ctx, chaos, pc)
	}

	return r.createPodChaos(ctx, chaos)
}

func (r *ShardingSphereChaosReconciler) getPodChaosByNamespacedName(ctx context.Context, namespacedName types.NamespacedName) (sschaos.PodChaos, error) {
	pc, err := r.Chaos.GetPodChaosByNamespacedName(ctx, namespacedName)
	if err != nil {
		return nil, err
	}
	return pc, nil
}

func (r *ShardingSphereChaosReconciler) createPodChaos(ctx context.Context, chaos *v1alpha1.ShardingSphereChaos) error {
	podChaos := r.Chaos.NewPodChaos(ctx, chaos)
	err := r.Chaos.CreatePodChaos(ctx, podChaos)
	if err != nil {
		return err
	}
	r.Events.Event(chaos, "Normal", "Created", fmt.Sprintf("PodChaos %s", " is created successfully"))
	return nil
}

func (r *ShardingSphereChaosReconciler) updatePodChaos(ctx context.Context, chaos *v1alpha1.ShardingSphereChaos, podChaos sschaos.PodChaos) error {
	pc := r.Chaos.NewPodChaos(ctx, chaos)
	// TODO: add the update procedure

	err := r.Chaos.UpdatePodChaos(ctx, pc)
	if err != nil {
		/*
			if err == reconcile.ErrNotChanged {
				return nil
			}
		*/
		return err
	}

	r.Events.Event(chaos, "Normal", "applied", fmt.Sprintf("podChaos %s", "new changes updated"))
	return reconcile.ErrChangedSpec
}

func (r *ShardingSphereChaosReconciler) reconcileNetworkChaos(ctx context.Context, chaos *v1alpha1.ShardingSphereChaos, namespacedName types.NamespacedName) error {
	nc, err := r.getNetworkChaosByNamespacedName(ctx, namespacedName)
	if err != nil {
		return err
	}
	if nc != nil {
		return r.updateNetWorkChaos(ctx, chaos, nc)
	}

	return r.createNetworkChaos(ctx, chaos)
}

func (r *ShardingSphereChaosReconciler) reconcileConfigMap(ctx context.Context, ssChaos *v1alpha1.ShardingSphereChaos) error {
	logger := r.Log.WithValues("reconcile configmap", ssChaos.Name)
	namespaceName := types.NamespacedName{Namespace: ssChaos.Namespace, Name: ssChaos.Name}

	rConfigmap, err := r.getConfigMapByNamespacedName(ctx, namespaceName)
	if err != nil {
		logger.Error(err, "get configmap error")
		return err
	}

	if rConfigmap != nil {
		return r.updateConfigMap(ctx, ssChaos, rConfigmap)
	}

	err = r.createConfigMap(ctx, ssChaos)
	if err != nil {
		r.Events.Event(ssChaos, "Warning", "Created", fmt.Sprintf("configmap created fail %s", err))
		return err
	}

	r.Events.Event(ssChaos, "Normal", "Created", "configmap created successfully")
	return nil
}

func (r *ShardingSphereChaosReconciler) reconcileJob(ctx context.Context, ssChaos *v1alpha1.ShardingSphereChaos) error {
	logger := r.Log.WithValues("reconcile job", ssChaos.Name)

	var nowInjectRequirement reconcile.InjectRequirement
	switch {
	case ssChaos.Status.Phase == "" || ssChaos.Status.Phase == v1alpha1.BeforeExperiment || ssChaos.Status.Phase == v1alpha1.AfterExperiment:
		nowInjectRequirement = reconcile.Experimental
	case ssChaos.Status.Phase == v1alpha1.InjectedChaos:
		nowInjectRequirement = reconcile.Pressure
	case ssChaos.Status.Phase == v1alpha1.RecoveredChaos:
		nowInjectRequirement = reconcile.Verify
	}

	namespaceName := types.NamespacedName{Namespace: ssChaos.Namespace, Name: reconcile.SetJobNamespaceName(ssChaos.Name, nowInjectRequirement)}

	rJob, err := r.getJobByNamespacedName(ctx, namespaceName)
	if err != nil {
		logger.Error(err, "get job err")
		return err
	}

	if rJob != nil {
		return r.updateJob(ctx, nowInjectRequirement, ssChaos, rJob)
	}

	err = r.createJob(ctx, nowInjectRequirement, ssChaos)
	if err != nil {
		return err
	}

	r.Events.Event(ssChaos, "Normal", "Created", fmt.Sprintf("%s job created successfully", string(nowInjectRequirement)))
	return nil
}

func (r *ShardingSphereChaosReconciler) reconcileStatus(ctx context.Context, namespacedName types.NamespacedName) error {
	ssChaos, err := r.getRuntimeChaos(ctx, namespacedName)
	if err != nil {
		return err
	}

	setDefault(ssChaos)

	jobName := getRequirement(ssChaos)
	rJob, err := r.getJobByNamespacedName(ctx, types.NamespacedName{Namespace: ssChaos.Namespace, Name: reconcile.SetJobNamespaceName(ssChaos.Name, jobName)})
	if err != nil || rJob == nil {
		return err
	}

	if ssChaos.Status.Phase == v1alpha1.BeforeExperiment && rJob.Status.Succeeded == 1 {
		ssChaos.Status.Phase = v1alpha1.AfterExperiment
	}
	jobConditions := rJob.Status.Conditions
	condition := getJobCondition(jobConditions)

	if condition == FailureJob {
		r.Events.Event(ssChaos, "Warning", "failed", fmt.Sprintf("job: %s", rJob.Name))
	}
	if ssChaos.Status.Phase == v1alpha1.RecoveredChaos {
		if err := r.updateRecoveredJob(ctx, ssChaos, rJob); err != nil {
			r.Events.Event(ssChaos, "Warning", "getPodLog", err.Error())
			return err
		}
	}

	if err := r.updatePhaseStart(ctx, ssChaos); err != nil {
		return err
	}

	rt, err := r.getRuntimeChaos(ctx, namespacedName)
	if err != nil {
		return err
	}
	setRtStatus(rt, ssChaos)
	return r.Status().Update(ctx, rt)
}

func (r *ShardingSphereChaosReconciler) handleChaosChange(ctx context.Context, name types.NamespacedName) error {
	ssChaos, err := r.getRuntimeChaos(ctx, name)
	if err != nil {
		return err
	}

	if ssChaos.Status.Phase != v1alpha1.BeforeExperiment {
		ssChaos.Status.Phase = v1alpha1.AfterExperiment
		if err := r.Status().Update(ctx, ssChaos); err != nil {
			return err
		}
	}
	return nil
}

func getRequirement(ssChaos *v1alpha1.ShardingSphereChaos) reconcile.InjectRequirement {
	var jobName reconcile.InjectRequirement
	if ssChaos.Status.Phase == v1alpha1.BeforeExperiment || ssChaos.Status.Phase == v1alpha1.AfterExperiment {
		jobName = reconcile.Experimental
	}
	if ssChaos.Status.Phase == v1alpha1.InjectedChaos {
		jobName = reconcile.Pressure
	}
	if ssChaos.Status.Phase == v1alpha1.RecoveredChaos {
		jobName = reconcile.Verify
	}
	return jobName
}

func getJobCondition(conditions []batchV1.JobCondition) JobCondition {
	var ret = ActiveJob
	for i := range conditions {
		p := &conditions[i]
		switch {
		case p.Type == batchV1.JobComplete && p.Status == corev1.ConditionTrue:
			ret = CompleteJob
		case p.Type == batchV1.JobFailed && p.Status == corev1.ConditionTrue:
			ret = FailureJob
		case p.Type == batchV1.JobSuspended && p.Status == corev1.ConditionTrue:
			ret = SuspendJob
		case p.Type == batchV1.JobFailureTarget:
			ret = FailureJob
		}
	}
	return ret
}

func setDefault(ssChaos *v1alpha1.ShardingSphereChaos) {
	if ssChaos.Status.Phase == "" {
		ssChaos.Status.Phase = v1alpha1.BeforeExperiment
	}
	if ssChaos.Status.Results == nil {
		ssChaos.Status.Results = []v1alpha1.Result{}
	}
}

func setRtStatus(rt *v1alpha1.ShardingSphereChaos, ssChaos *v1alpha1.ShardingSphereChaos) {
	rt.Status.Results = []v1alpha1.Result{}
	for i := range ssChaos.Status.Results {
		r := &ssChaos.Status.Results[i]
		rt.Status.Results = append(rt.Status.Results, v1alpha1.Result{
			Success: r.Success,
			Detail: v1alpha1.Detail{
				Time:    metav1.Time{Time: time.Now()},
				Message: r.Detail.Message,
			},
		})
	}

	rt.Status.Phase = ssChaos.Status.Phase
	rt.Status.ChaosCondition = ssChaos.Status.ChaosCondition
}

func (r *ShardingSphereChaosReconciler) updateRecoveredJob(ctx context.Context, ssChaos *v1alpha1.ShardingSphereChaos, rJob *batchV1.Job) error {
	if isResultExist(rJob) {
		return nil
	}

	for i := range ssChaos.Status.Results {
		r := &ssChaos.Status.Results[i]
		if strings.HasPrefix(r.Detail.Message, VerifyJobCheck) {
			return nil
		}
	}

	logOpts := &corev1.PodLogOptions{}
	pod, err := r.getPodHaveLog(ctx, rJob)
	if err != nil || pod == nil {
		return err
	}
	podNamespacedName := types.NamespacedName{
		Namespace: pod.Namespace,
		Name:      pod.Name,
	}
	condition := getJobCondition(rJob.Status.Conditions)
	result := &v1alpha1.Result{}

	if condition == CompleteJob {
		log, err := r.getPodLog(ctx, podNamespacedName, logOpts)
		if err != nil {
			return err
		}
		if ssChaos.Spec.Expect.Verify == "" || ssChaos.Spec.Expect.Verify == log {
			result.Success = true
			result.Detail = v1alpha1.Detail{
				Time:    metav1.Time{Time: time.Now()},
				Message: fmt.Sprintf("%s: job succeeded", VerifyJobCheck),
			}
		} else {
			result.Success = false
			result.Detail = v1alpha1.Detail{
				Time:    metav1.Time{Time: time.Now()},
				Message: fmt.Sprintf("%s: %s", VerifyJobCheck, log),
			}
		}
	}

	if condition == FailureJob {
		log, err := r.getPodLog(ctx, podNamespacedName, logOpts)
		if err != nil {
			return err
		}
		result.Success = false
		result.Detail = v1alpha1.Detail{
			Time:    metav1.Time{Time: time.Now()},
			Message: fmt.Sprintf("%s: %s", VerifyJobCheck, log),
		}
	}

	ssChaos.Status.Results = updateResult(ssChaos.Status.Results, *result, VerifyJobCheck)

	return nil
}

func (r *ShardingSphereChaosReconciler) getPodHaveLog(ctx context.Context, rJob *batchV1.Job) (*corev1.Pod, error) {
	pods := &corev1.PodList{}
	if err := r.List(ctx, pods, client.MatchingLabels{"controller-uid": rJob.Spec.Template.Labels["controller-uid"]}); err != nil {
		return nil, err
	}
	if pods.Items == nil {
		return nil, nil
	}
	var pod *corev1.Pod
	for i := range pods.Items {
		pod = &pods.Items[i]
		break
	}
	return pod, nil
}

func isResultExist(rJob *batchV1.Job) bool {
	for _, cmd := range rJob.Spec.Template.Spec.Containers[0].Args {
		if strings.Contains(cmd, string(reconcile.Verify)) {
			return true
		}
	}
	return false
}

func updateResult(results []v1alpha1.Result, r v1alpha1.Result, check string) []v1alpha1.Result {
	for i := range results {
		msg := results[i].Detail.Message
		if strings.HasPrefix(msg, check) && strings.HasPrefix(r.Detail.Message, check) {
			results[i] = r
			return results
		}
	}
	results = append(results, r)
	return results
}

func (r *ShardingSphereChaosReconciler) getPodLog(ctx context.Context, namespacedName types.NamespacedName, options *corev1.PodLogOptions) (string, error) {
	req := r.ClientSet.CoreV1().Pods(namespacedName.Namespace).GetLogs(namespacedName.Name, options)
	res := req.Do(ctx)
	if res.Error() != nil {
		return "", res.Error()
	}
	var ret []byte
	ret, err := res.Raw()
	if err != nil {
		return "", err
	}
	return string(ret), nil
}

func (r *ShardingSphereChaosReconciler) updatePhaseStart(ctx context.Context, ssChaos *v1alpha1.ShardingSphereChaos) error {
	if ssChaos.Status.Phase != v1alpha1.BeforeExperiment {
		if err := r.updateChaosCondition(ctx, ssChaos); err != nil {
			return err
		}

		if ssChaos.Status.ChaosCondition == v1alpha1.AllInjected && ssChaos.Status.Phase == v1alpha1.AfterExperiment {
			ssChaos.Status.Phase = v1alpha1.InjectedChaos
		}

		if ssChaos.Status.ChaosCondition == v1alpha1.AllRecovered && ssChaos.Status.Phase == v1alpha1.InjectedChaos {
			ssChaos.Status.Phase = v1alpha1.RecoveredChaos
		}
	}

	return nil
}

func (r *ShardingSphereChaosReconciler) updateChaosCondition(ctx context.Context, ssChaos *v1alpha1.ShardingSphereChaos) error {
	namespacedName := types.NamespacedName{
		Namespace: ssChaos.Namespace,
		Name:      ssChaos.Name,
	}
	if ssChaos.Spec.EmbedChaos.PodChaos != nil {
		chao, err := r.Chaos.GetPodChaosByNamespacedName(ctx, namespacedName)
		if err != nil {
			return err
		}
		ssChaos.Status.ChaosCondition = sschaos.ConvertChaosStatus(ctx, ssChaos, chao)
	} else if ssChaos.Spec.EmbedChaos.NetworkChaos != nil {
		chao, err := r.Chaos.GetNetworkChaosByNamespacedName(ctx, namespacedName)
		if err != nil {
			return err
		}
		ssChaos.Status.ChaosCondition = sschaos.ConvertChaosStatus(ctx, ssChaos, chao)
	}

	return nil
}

func (r *ShardingSphereChaosReconciler) getNetworkChaosByNamespacedName(ctx context.Context, namespacedName types.NamespacedName) (sschaos.NetworkChaos, error) {
	nc, err := r.Chaos.GetNetworkChaosByNamespacedName(ctx, namespacedName)
	if err != nil {
		return nil, err
	}
	return nc, nil
}

func (r *ShardingSphereChaosReconciler) getConfigMapByNamespacedName(ctx context.Context, namespacedName types.NamespacedName) (*corev1.ConfigMap, error) {
	config, err := r.ConfigMap.GetByNamespacedName(ctx, namespacedName)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func (r *ShardingSphereChaosReconciler) getJobByNamespacedName(ctx context.Context, namespacedName types.NamespacedName) (*batchV1.Job, error) {
	injectJob, err := r.Job.GetByNamespacedName(ctx, namespacedName)
	if err != nil {
		return nil, err
	}
	return injectJob, nil
}

func (r *ShardingSphereChaosReconciler) updateConfigMap(ctx context.Context, chao *v1alpha1.ShardingSphereChaos, cur *corev1.ConfigMap) error {
	exp := reconcile.UpdateConfigMap(chao, cur)
	if exp == nil {
		return nil
	}
	return r.Update(ctx, exp)
}

func (r *ShardingSphereChaosReconciler) createConfigMap(ctx context.Context, chao *v1alpha1.ShardingSphereChaos) error {
	rConfigMap := reconcile.NewSSConfigMap(chao)
	if err := ctrl.SetControllerReference(chao, rConfigMap, r.Scheme); err != nil {
		return err
	}
	err := r.Create(ctx, rConfigMap)
	if err == nil && apierrors.IsAlreadyExists(err) {
		return nil
	}

	return err
}

func (r *ShardingSphereChaosReconciler) updateJob(ctx context.Context, requirement reconcile.InjectRequirement, chao *v1alpha1.ShardingSphereChaos, cur *batchV1.Job) error {
	isEqual, err := reconcile.IsJobChanged(chao, requirement, cur)
	if err != nil {
		return err
	}
	if !isEqual {
		if err := r.Delete(ctx, cur); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		r.Events.Event(chao, "Normal", "Updated", "job Updated")
	}
	return nil
}

func (r *ShardingSphereChaosReconciler) createJob(ctx context.Context, requirement reconcile.InjectRequirement, chao *v1alpha1.ShardingSphereChaos) error {
	injectJob, err := reconcile.NewJob(chao, requirement)
	if err != nil {
		return err
	}
	if err := ctrl.SetControllerReference(chao, injectJob, r.Scheme); err != nil {
		return err
	}

	err = r.Create(ctx, injectJob)
	if err != nil {
		return client.IgnoreAlreadyExists(err)
	}

	rJob := &batchV1.Job{}
	backoff := wait.Backoff{
		Steps:    6,
		Duration: 500 * time.Millisecond,
		Factor:   5.0,
		Jitter:   0.1,
	}

	if err := retry.OnError(backoff, func(e error) bool {
		return true
	}, func() error {
		return r.Get(ctx, types.NamespacedName{Namespace: chao.Namespace, Name: reconcile.SetJobNamespaceName(chao.Name, requirement)}, rJob)
	}); err != nil {
		return err
	}

	podList := &corev1.PodList{}
	if err := retry.OnError(backoff, func(e error) bool {
		return e != nil
	}, func() error {
		if err := r.List(ctx, podList, client.MatchingLabels{"controller-uid": rJob.Spec.Template.Labels["controller-uid"]}); err != nil {
			return err
		}
		if len(podList.Items) == 0 {
			return ErrNoPod
		}
		return nil
	}); err != nil {
		return err
	}

	for i := range podList.Items {
		rPod := &podList.Items[i]
		if err := ctrl.SetControllerReference(rJob, rPod, r.Scheme); err != nil {
			return err
		}

		exp := rPod.DeepCopy()
		updateBackoff := wait.Backoff{
			Steps:    5,
			Duration: 30 * time.Millisecond,
			Factor:   5.0,
			Jitter:   0.1,
		}
		if err := retry.RetryOnConflict(updateBackoff, func() error {
			if err := r.Update(ctx, exp); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *ShardingSphereChaosReconciler) updateNetWorkChaos(ctx context.Context, chaos *v1alpha1.ShardingSphereChaos, netWorkChaos sschaos.NetworkChaos) error {
	networkChaos := r.Chaos.NewNetworkChaos(ctx, chaos)
	//TODO: add update prodecure

	err := r.Chaos.UpdateNetworkChaos(ctx, networkChaos)
	if err != nil {
		/*
			if err == reconcile.ErrNotChanged {
				return nil
			}
		*/
		return err
	}
	r.Events.Event(chaos, "Normal", "applied", fmt.Sprintf("networkChaos %s", "new changes updated"))
	return reconcile.ErrChangedSpec
}

func (r *ShardingSphereChaosReconciler) createNetworkChaos(ctx context.Context, chaos *v1alpha1.ShardingSphereChaos) error {
	networkChaos := r.Chaos.NewNetworkChaos(ctx, chaos)
	err := r.Chaos.CreateNetworkChaos(ctx, networkChaos)
	if err != nil {
		return err
	}

	r.Events.Event(chaos, "Normal", "created", fmt.Sprintf("networkChaos %s", "  is created successfully"))
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ShardingSphereChaosReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ShardingSphereChaos{}).
		// Owns(&chaosv1alpha1.PodChaos{}).
		// Owns(&chaosv1alpha1.NetworkChaos{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&batchV1.Job{}).
		Complete(r)
}
