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
	v1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	batchv1alpha1 "github.com/mrunesson/at-operator/api/v1alpha1"
)

// AtReconciler reconciles a At object
type AtReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=batch.linuxalert.org,resources=ats,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch.linuxalert.org,resources=ats/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=batch.linuxalert.org,resources=ats/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *AtReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	at := &batchv1alpha1.At{}
	err := r.Get(ctx, req.NamespacedName, at)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			logger.Info("At resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to get At")
		return ctrl.Result{}, err
	}

	// Check if it is time to run?
	currentTime := time.Now()
	expectedStart := time.Date(at.Spec.Year, time.Month(at.Spec.Month), at.Spec.Day, at.Spec.Hour, at.Spec.Minute, 0, 0, currentTime.Location())
	if currentTime.After(expectedStart) && !at.Status.Scheduled {
		at.Status.Scheduled = true
		job := r.deploymentForAt(at)
		logger.Info("Creating a new Job", "Job.Namespace", job.Namespace, "Job.Name", job.Name)
		err = r.Create(ctx, job)
		if err != nil {
			logger.Error(err, "Failed to create Job", "Job.Namespace", job.Namespace, "Job.Name", job.Name)
			return ctrl.Result{}, err
		}
		at.Status.Job = job.Name
		err := r.Status().Update(ctx, at)
		if err != nil {
			logger.Error(err, "Failed to update At status")
			return ctrl.Result{}, err
		}
		// Deployment created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	}

	// TODO: Check if status scheduled and if job is done, then mark as complete
	// TODO: Have some retention time and then delete.

	return ctrl.Result{}, nil
}

// Returns a Job object matching spec in At.
func (r *AtReconciler) deploymentForAt(at *batchv1alpha1.At) *v1.Job {
	// TODO: Set labels. Both copy from at and mark it is managed by conttroller.
	job := &v1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      at.Name,
			Namespace: at.Namespace,
		},
		Spec: at.Spec.JobTemplate,
	}
	ctrl.SetControllerReference(at, job, r.Scheme)
	return job
}

// SetupWithManager sets up the controller with the Manager.
func (r *AtReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&batchv1alpha1.At{}).
		Owns(&v1.Job{}).
		Complete(r)
}
