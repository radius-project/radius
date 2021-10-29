// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"
	"sync"

	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DockerContainerReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	// A PID-to-runningProcessStatus map used to track changes to the replicas of an DockerContainer.
	processStatus *sync.Map
}

//+kubebuilder:rbac:groups=radius.dev,resources=dockercontainers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=radius.dev,resources=dockercontainers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=radius.dev,resources=dockercontainers/finalizers,verbs=update

func (r *DockerContainerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("DockerContainer", req.NamespacedName)

	var DockerContainer radiusv1alpha3.DockerContainer
	err := r.Get(ctx, req.NamespacedName, &DockerContainer)
	if err != nil && client.IgnoreNotFound(err) == nil {
		// The DockerContainer has been deleted
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "falied to Get() the DockerContainer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: reconciliationDelay}, nil
}

// func (r *DockerContainerReconciler) updateReplicaState(dockerContainer *radiusv1alpha3.DockerContainer, log logr.Logger) bool {
// 	replicas := dockerContainer.Status.Replicas
// 	var changed bool

// 	for _, rs := range replicas {
// 		if rs.ExitCode != ExitCodeRunning {
// 			// We are done with this replica
// 			continue
// 		}

// 		// If the process has exited, store exit code
// 		rps, found := r.getProcesStatus(rs.PID)
// 		if !found {
// 			// It could be that the replica was started by another controller and we are not tracking the process.
// 			// "Attaching to" and tracking processes launched by another controller instance is not implemented as of now.

// 			log.Info("executable status has a replica process that was started by another controller", "PID", rs.PID)
// 			dockerContainer.Status.SetProcessExitCode(rs.PID, ExitCodeAbandoned)
// 			changed = true
// 		} else if rps.ExitCode != ExitCodeRunning {
// 			log.Info("replica finished", "PID", rs.PID, "ExitCode", rps.ExitCode)
// 			dockerContainer.Status.SetProcessExitCode(rs.PID, rps.ExitCode)
// 			r.processStatus.Delete(rs.PID)
// 			changed = true
// 		}
// 	}

// 	return changed
// }

// func (r *ExecutableReconciler) manageReplicas(ctx context.Context, executable *radiusv1alpha3.Executable, log logr.Logger) bool {
// 	replicas := executable.Status.Replicas
// 	count := len(replicas)
// 	pidsRunning := make([]int, 0)
// 	for _, rs := range replicas {
// 		if rs.ExitCode == ExitCodeRunning {
// 			pidsRunning = append(pidsRunning, rs.PID)
// 		}
// 	}

// 	if count > executable.Spec.Replicas && len(pidsRunning) > 0 {
// 		cStop := count - executable.Spec.Replicas
// 		if cStop > len(pidsRunning) {
// 			cStop = len(pidsRunning)
// 		}
// 		log.Info("stopping extra replicas...", "Count", cStop)

// 		for i, pid := range pidsRunning {
// 			r.stopReplica(pid, log)
// 			executable.Status.SetProcessExitCode(pid, ExitCodeAbandoned)
// 			if cStop--; cStop == 0 {
// 				// We also want to remove the corresponding Replicas from the ExecutableStatus.
// 				// If we don't, in the next reconciliation loop it might seem that we are done, that is,
// 				// we have executed more replicas than the Spec calls for. But this is not the case.
// 				// We need to differentiate between replicas that finish execution normally,
// 				// and replicas that were killed as result of a scale-down. The latters "do not count".
// 				pidsToRemove := pidsRunning[0 : i+1]
// 				executable.Status.RemoveReplicas(pidsToRemove)

// 				break
// 			}
// 		}

// 		return true
// 	}

// 	if count < executable.Spec.Replicas {
// 		// We might have thought we are done, but now more replicas are needed.
// 		// This can happen due to Spec update. We need to clear the finish timestamp.
// 		executable.Status.FinishTimestamp = nil

// 		cStart := executable.Spec.Replicas - count
// 		log.Info("additional replicas needed", "Count", cStart)
// 		if cStart > maxReplicaConcurrentStarts {
// 			cStart = maxReplicaConcurrentStarts
// 		}
// 		log.Info("starting replicas", "Count", cStart)

// 		for i := 0; i < cStart; i++ {
// 			r.startReplica(ctx, executable, log)
// 		}

// 		return true
// 	}

// 	return false
// }

func (r *DockerContainerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&radiusv1alpha3.DockerContainer{}).
		Complete(r)
}
