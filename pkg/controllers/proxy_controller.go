/*
Copyright 2022.

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

package controllers

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
	shardingspherev1alpha1 "sphere-ex.com/shardingsphere-operator/api/v1alpha1"
	"sphere-ex.com/shardingsphere-operator/pkg/reconcile"
	"time"
)

const (
	SyncBuildStatusInterval = 5 * time.Second
)

// ProxyReconciler reconciles a Proxy object
type ProxyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=shardingsphere.sphere-ex.com,resources=proxies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=shardingsphere.sphere-ex.com,resources=proxies/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=shardingsphere.sphere-ex.com,resources=proxies/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployment/status,verbs=get;list
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get;list;watch;

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.

func (r *ProxyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logger.FromContext(ctx)
	run := &shardingspherev1alpha1.Proxy{}

	err := r.Get(ctx, req.NamespacedName, run)
	if apierrors.IsNotFound(err) {
		log.Error(err, "Proxy in work queue no longer exists!")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	} else if err != nil {
		return ctrl.Result{}, err
	}
	originStatus := run.Status.DeepCopy()
	if run.Status.Phase == "" || len(run.Status.Conditions) == 0 {
		run.SetInitStatus()
		dp := reconcile.ConstructCascadingDeployment(run)
		err = r.Create(ctx, dp)
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				log.Error(err, "Deployment no longer exists!")
			} else if err != nil {
				run.SetInitFailed()
				_ = r.Status().Update(ctx, run)
				log.Error(err, "Create Resources Deployment Error")
				return ctrl.Result{RequeueAfter: SyncBuildStatusInterval}, err
			}
		}
		svc := reconcile.ConstructCascadingService(run)
		err = r.Create(ctx, svc)
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				log.Error(err, "Service no longer exists!")
			} else {
				run.SetInitFailed()
				_ = r.Status().Update(ctx, run)
				log.Error(err, "Create Resources Service Error")
				return ctrl.Result{RequeueAfter: SyncBuildStatusInterval}, err
			}
		}
		run.Annotations["ResourcesInit"] = "true"
		run.Annotations["UpdateTime"] = metav1.Now().Format(metav1.RFC3339Micro)
	}
	if equality.Semantic.DeepEqual(originStatus, run.Status) {
		log.Info(" status are equal... ", "Status", run.Status)
		return ctrl.Result{RequeueAfter: SyncBuildStatusInterval}, nil
	}
	err = r.Status().Update(ctx, run)
	if err != nil {
		log.Error(err, "Update CRD Status Error")
		return ctrl.Result{}, err
	}
	err = r.Update(ctx, run)
	if err != nil {
		log.Error(err, "Update CRD Resources Error")
		return ctrl.Result{}, err
	}

	log.Info("run spec is ", "spec", run.Spec)
	log.Info("run status is ", "status", run.Status)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProxyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&shardingspherev1alpha1.Proxy{}).
		Owns(&appsv1.Deployment{}).
		Owns(&v1.Service{}).
		Complete(r)
}
