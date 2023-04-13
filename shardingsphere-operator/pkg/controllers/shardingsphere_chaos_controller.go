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
	"time"

	"github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/pkg/kubernetes/configmap"
	v1 "k8s.io/api/core/v1"

	sschaosv1alpha1 "github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/api/v1alpha1"
	"github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/pkg/kubernetes/chaos"
	"github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/pkg/kubernetes/job"
	reconcile "github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/pkg/reconcile/shardingspherechaos"
	chaosv1alpha1 "github.com/chaos-mesh/chaos-mesh/api/v1alpha1"
	"github.com/go-logr/logr"
	batchV1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ShardingSphereChaosControllerName = "shardingsphere-chaos-controller"
	ssChaosDefaultEnqueueTime         = 5 * time.Second
)

// ShardingSphereChaosReconciler is a controller for the ShardingSphereChaos
type ShardingSphereChaosReconciler struct { //
	client.Client
	Scheme    *runtime.Scheme
	Log       logr.Logger
	Chaos     chaos.Chaos
	Job       job.Job
	ConfigMap configmap.ConfigMap
}

// Reconcile handles main function of this controller
func (r *ShardingSphereChaosReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues(ShardingSphereChaosControllerName, req.NamespacedName)

	var ssChaos sschaosv1alpha1.ShardingSphereChaos
	if err := r.Get(ctx, req.NamespacedName, &ssChaos); err != nil {
		logger.Error(err, "unable to fetch ShardingSphereChaos source")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !ssChaos.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	logger.Info("start reconcile chaos")
	if err := r.reconcileChaos(ctx, &ssChaos); err != nil {
		logger.Error(err, " unable to reconcile chaos")
		return ctrl.Result{}, err
	}
	if err := r.reconcileConfigMap(ctx, &ssChaos); err != nil {
		logger.Error(err, "unable to reconcile configmap")
		return ctrl.Result{}, err
	}
	if err := r.reconcileJob(ctx, &ssChaos); err != nil {
		logger.Error(err, "unable to reconcile job")
		return ctrl.Result{}, err
	}
	if err := r.reconcileStatus(ctx, &ssChaos); err != nil {
		logger.Error(err, "failed to update status")
	}

	return ctrl.Result{RequeueAfter: ssChaosDefaultEnqueueTime}, nil
}

func (r *ShardingSphereChaosReconciler) reconcileChaos(ctx context.Context, ssChao *sschaosv1alpha1.ShardingSphereChaos) error {
	logger := r.Log.WithValues("reconcile chaos", ssChao.Name)
	namespaceName := types.NamespacedName{Namespace: ssChao.Namespace, Name: ssChao.Name}
	if ssChao.Spec.EmbedChaos.PodChaos != nil {
		chao, isExist, err := r.getPodChaosByNamespacedName(ctx, namespaceName)
		if err != nil {
			logger.Error(err, "pod chaos err")
			return err
		}
		if isExist {
			return r.updatePodChaos(ctx, ssChao, chao)
		}
		return r.CreatePodChaos(ctx, ssChao)
	} else if ssChao.Spec.EmbedChaos.NetworkChaos != nil {
		chao, isExist, err := r.getNetworkChaosByNamespacedName(ctx, namespaceName)
		if err != nil {
			logger.Error(err, "network chao err")
			return err
		}
		if isExist {
			return r.updateNetWorkChaos(ctx, ssChao, chao)
		}
		return r.CreateNetworkChaos(ctx, ssChao)
	}
	return nil
}

func (r *ShardingSphereChaosReconciler) reconcileConfigMap(ctx context.Context, ssChaos *sschaosv1alpha1.ShardingSphereChaos) error {
	logger := r.Log.WithValues("reconcile configmap", ssChaos.Name)
	namespaceName := types.NamespacedName{Namespace: ssChaos.Namespace, Name: ssChaos.Name}
	rConfigmap, isExist, err := r.getConfigMapByNamespacedName(ctx, namespaceName)
	if err != nil {
		logger.Error(err, "get configmap error")
		return err
	}

	if isExist {
		return r.updateConfigMap(ctx, ssChaos, rConfigmap)
	}

	return r.CreateConfigMap(ctx, ssChaos)
}

func (r *ShardingSphereChaosReconciler) reconcileJob(ctx context.Context, ssChaos *sschaosv1alpha1.ShardingSphereChaos) error {
	logger := r.Log.WithValues("reconcile job", ssChaos.Name)
	namespaceName := types.NamespacedName{Namespace: ssChaos.Namespace, Name: ssChaos.Name}

	rJob, isExist, err := r.getJobByNamespacedName(ctx, namespaceName)
	if err != nil {
		logger.Error(err, "get job err")
		return err
	}
	//todo:update InjectRequirement by chaos status
	if isExist {
		return r.updateJob(ctx, reconcile.Experimental, ssChaos, rJob)
	}

	return r.createJob(ctx, reconcile.Experimental, ssChaos)
}

func (r *ShardingSphereChaosReconciler) reconcileStatus(ctx context.Context, ssChaos *sschaosv1alpha1.ShardingSphereChaos) error {
	var (
		chaoCondition  sschaosv1alpha1.ChaosCondition
		namespacedName = types.NamespacedName{
			Namespace: ssChaos.Namespace,
			Name:      ssChaos.Name,
		}
	)
	if ssChaos.Spec.EmbedChaos.PodChaos != nil {
		chao, err := r.Chaos.GetPodChaosByNamespacedName(ctx, namespacedName)
		if err != nil {
			return err
		}
		chaoCondition = r.Chaos.ConvertChaosStatus(ctx, ssChaos, chao)
	} else if ssChaos.Spec.EmbedChaos.NetworkChaos != nil {
		chao, err := r.Chaos.GetNetworkChaosByNamespacedName(ctx, namespacedName)
		if err != nil {
			return err
		}
		chaoCondition = r.Chaos.ConvertChaosStatus(ctx, ssChaos, chao)
	}

	var rt sschaosv1alpha1.ShardingSphereChaos
	if err := r.Get(ctx, namespacedName, &rt); err != nil {
		return err
	}
	ssChaos.Status.ChaosCondition = chaoCondition
	rt.Status = ssChaos.Status
	return r.Status().Update(ctx, &rt)
}

func (r *ShardingSphereChaosReconciler) getNetworkChaosByNamespacedName(ctx context.Context, namespacedName types.NamespacedName) (reconcile.NetworkChaos, bool, error) {
	nc, err := r.Chaos.GetNetworkChaosByNamespacedName(ctx, namespacedName)
	if err != nil {
		return nil, false, err
	}
	if nc == nil {
		return nil, false, nil
	}
	return nc, true, nil
}

func (r *ShardingSphereChaosReconciler) getPodChaosByNamespacedName(ctx context.Context, namespacedName types.NamespacedName) (reconcile.PodChaos, bool, error) {
	pc, err := r.Chaos.GetPodChaosByNamespacedName(ctx, namespacedName)
	if err != nil {
		return nil, false, err
	}
	if pc == nil {
		return nil, false, nil
	}
	return pc, true, nil
}

func (r *ShardingSphereChaosReconciler) getConfigMapByNamespacedName(ctx context.Context, namespacedName types.NamespacedName) (*v1.ConfigMap, bool, error) {
	config, err := r.ConfigMap.GetByNamespacedName(ctx, namespacedName)
	if err != nil {
		return nil, false, err
	}
	if config == nil {
		return nil, false, nil
	}

	return config, true, nil
}

func (r *ShardingSphereChaosReconciler) getJobByNamespacedName(ctx context.Context, namespacedName types.NamespacedName) (*batchV1.Job, bool, error) {
	injectJob, err := r.Job.GetByNamespacedName(ctx, namespacedName)
	if err != nil {
		return nil, false, err
	}
	if injectJob == nil {
		return nil, false, nil
	}

	return injectJob, true, nil
}

func (r *ShardingSphereChaosReconciler) updateConfigMap(ctx context.Context, chao *sschaosv1alpha1.ShardingSphereChaos, cur *v1.ConfigMap) error {
	exp := reconcile.UpdateConfigMap(chao, cur)
	if exp == nil {
		return nil
	}
	return r.Update(ctx, exp)
}

func (r *ShardingSphereChaosReconciler) CreateConfigMap(ctx context.Context, chao *sschaosv1alpha1.ShardingSphereChaos) error {
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

func (r *ShardingSphereChaosReconciler) updateJob(ctx context.Context, requirement reconcile.InjectRequirement, chao *sschaosv1alpha1.ShardingSphereChaos, cur *batchV1.Job) error {
	exp, err := reconcile.UpdateJob(chao, requirement, cur)
	if err != nil {
		return err
	}
	if exp != nil {
		if err := r.Delete(ctx, cur); err != nil {
			return err
		}
		if err := ctrl.SetControllerReference(chao, exp, r.Scheme); err != nil {
			return err
		}
		if err := r.Create(ctx, exp); err != nil {
			return err
		}
	}
	return nil
}

// todo:
func (r *ShardingSphereChaosReconciler) createJob(ctx context.Context, requirement reconcile.InjectRequirement, chao *sschaosv1alpha1.ShardingSphereChaos) error {
	injectJob, err := reconcile.NewJob(chao, requirement)
	if err := ctrl.SetControllerReference(chao, injectJob, r.Scheme); err != nil {
		return err
	}
	if err != nil {
		return err
	}
	err = r.Create(ctx, injectJob)
	if err == nil && apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func (r *ShardingSphereChaosReconciler) updatePodChaos(ctx context.Context, chao *sschaosv1alpha1.ShardingSphereChaos, podChaos reconcile.PodChaos) error {
	return r.Chaos.UpdatePodChaos(ctx, chao, podChaos)
}

func (r *ShardingSphereChaosReconciler) CreatePodChaos(ctx context.Context, chao *sschaosv1alpha1.ShardingSphereChaos) error {
	podChaos, err := r.Chaos.NewPodChaos(chao)
	if err != nil {
		return err
	}
	return r.Chaos.CreatePodChaos(ctx, podChaos)
}

func (r *ShardingSphereChaosReconciler) updateNetWorkChaos(ctx context.Context, chao *sschaosv1alpha1.ShardingSphereChaos, netWorkChaos reconcile.NetworkChaos) error {
	return r.Chaos.UpdateNetworkChaos(ctx, chao, netWorkChaos)
}

func (r *ShardingSphereChaosReconciler) CreateNetworkChaos(ctx context.Context, chao *sschaosv1alpha1.ShardingSphereChaos) error {
	networkChaos, err := r.Chaos.NewNetworkPodChaos(chao)
	if err != nil {
		return err
	}
	return r.Chaos.CreateNetworkChaos(ctx, networkChaos)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ShardingSphereChaosReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&sschaosv1alpha1.ShardingSphereChaos{}).
		Owns(&chaosv1alpha1.PodChaos{}).
		Owns(&chaosv1alpha1.NetworkChaos{}).
		Owns(&batchV1.Job{}).
		Complete(r)
}
