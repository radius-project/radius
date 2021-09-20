// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"
	"fmt"
	"sync"
	"time"

	radiusv1alpha1 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha1"
	"github.com/Azure/radius/pkg/process"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	maxReplicaConcurrentStarts = 2
)

const (
	// A valid exit code of a process is a non-negative number
	// We use ExitCodeRunning to indicate that a process is running
	ExitCodeRunning = -1
	// We use ExitCodeAbandoned if we release the process without waiting for it to finish, forfeiting the chance to obtain an exit code. It is also used when replica is killed as result of reducing number of replicas after creation, or when we could not obtain the exit code for some reason.
	ExitCodeAbandoned = -2
	// We use ExitCodeFailedToStart to designate failed replica start attempts
	ExitCodeFailedToStart = -3

	// Invalid PID code is used when replica start fails
	InvalidPID = -1
)

type runningProcessStatus struct {
	ExitCode       int
	OwnerName      string
	OwnerNamespace string
}

type ExecutableReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	// A PID-to-runningProcessStatus map used to track changes to the replicas of an executable.
	processStatus *sync.Map

	ProcessExecutor process.Executor
}

var (
	completedExecutableHarvestDelay, _ = time.ParseDuration("5m")
	reconciliationDelay, _             = time.ParseDuration("1s")
)

/*
At a high level, ExecutableReconciler does the following:

Routine Executable handling:
1. Loads Executable instance data
2. Checks status of running processes
   If we see replicas that were started by different controller instance, they should be marked as "abandoned".
   Eventually we might implement replica "adoption", but as of now we won't be tracking them.
   For processes that belong to us, save exit code for those that finished running.
3. Checks if we have enough replicas started, counting replicas that finished and those that failed to start towards the required number.
   If not enough replicas were started, it will start new replicas, up to MaxReplicaConcurrentStarts at a time, and not more than MaxReplicasPerExecutable.
   If more than necessary replicas have been started, kill unnecessary running replicas.
4. If enough replicas have finished running, stores the FinishTimestamp value in the status struct, marking the Executable as done.

Old Executable harvesting:
1. If an Executable is stale (time.Now() > finishTimestamp + completedExecutableHarvestDelay), delete it.

Deletion handling:
1. If any replicas are still running, kill them.

*/

func (r *ExecutableReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("Executable", req.NamespacedName)

	var executable radiusv1alpha1.Executable
	err := r.Get(ctx, req.NamespacedName, &executable)
	if err != nil && client.IgnoreNotFound(err) == nil {
		// The Executable has been deleted
		r.terminateRemainingReplicas(req.NamespacedName, log)
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "falied to Get() the Executable")
		return ctrl.Result{}, err
	}

	changed := r.updateReplicaState(&executable, log)
	changed = changed || r.manageReplicas(ctx, &executable, log)
	done, markedDone := r.checkDone(&executable, log)
	changed = changed || markedDone

	if !changed {
		if executable.Status.FinishTimestamp == nil {
			// Still running
			log.Info("no changes detected for Executable, continue monitoring...")
			return ctrl.Result{RequeueAfter: reconciliationDelay}, nil
		} else if metav1.Now().After(executable.Status.FinishTimestamp.Add(completedExecutableHarvestDelay)) {
			log.Info("cleaning up old executable", "FinishTimestamp", executable.Status.FinishTimestamp)

			if err = r.Delete(ctx, &executable); err != nil {
				log.Error(err, "old executable cleanup failed")
				return ctrl.Result{}, err
			}

			// We are done with this Executable
			return ctrl.Result{}, nil
		}
	}

	if err = r.Status().Update(ctx, &executable); err != nil {
		log.Error(err, "executable update failed")
		return ctrl.Result{}, err
	}
	if done {
		return ctrl.Result{RequeueAfter: completedExecutableHarvestDelay}, nil
	} else {
		return ctrl.Result{RequeueAfter: reconciliationDelay}, nil
	}
}

func (r *ExecutableReconciler) updateReplicaState(executable *radiusv1alpha1.Executable, log logr.Logger) bool {
	replicas := executable.Status.Replicas
	var changed bool

	for _, rs := range replicas {
		if rs.ExitCode != ExitCodeRunning {
			// We are done with this replica
			continue
		}

		// If the process has exited, store exit code
		rps, found := r.getProcesStatus(rs.PID)
		if !found {
			// It could be that the replica was started by another controller and we are not tracking the process.
			// "Attaching to" and tracking processes launched by another controller instance is not implemented as of now.

			log.Info("executable status has a replica process that was started by another controller", "PID", rs.PID)
			executable.Status.SetProcessExitCode(rs.PID, ExitCodeAbandoned)
			changed = true
		} else if rps.ExitCode != ExitCodeRunning {
			log.Info("replica finished", "PID", rs.PID, "ExitCode", rps.ExitCode)
			executable.Status.SetProcessExitCode(rs.PID, rps.ExitCode)
			r.processStatus.Delete(rs.PID)
			changed = true
		}
	}

	return changed
}

func (r *ExecutableReconciler) manageReplicas(ctx context.Context, executable *radiusv1alpha1.Executable, log logr.Logger) bool {
	replicas := executable.Status.Replicas
	count := len(replicas)
	pidsRunning := make([]int, 0)
	for _, rs := range replicas {
		if rs.ExitCode == ExitCodeRunning {
			pidsRunning = append(pidsRunning, rs.PID)
		}
	}

	if count > executable.Spec.Replicas && len(pidsRunning) > 0 {
		cStop := count - executable.Spec.Replicas
		if cStop > len(pidsRunning) {
			cStop = len(pidsRunning)
		}
		log.Info("stopping extra replicas...", "Count", cStop)

		for i, pid := range pidsRunning {
			r.stopReplica(pid, log)
			executable.Status.SetProcessExitCode(pid, ExitCodeAbandoned)
			if cStop--; cStop == 0 {
				// We also want to remove the corresponding Replicas from the ExecutableStatus.
				// If we don't, in the next reconciliation loop it might seem that we are done, that is,
				// we have executed more replicas than the Spec calls for. But this is not the case.
				// We need to differentiate between replicas that finish execution normally,
				// and replicas that were killed as result of a scale-down. The latters "do not count".
				pidsToRemove := pidsRunning[0 : i+1]
				executable.Status.RemoveReplicas(pidsToRemove)

				break
			}
		}

		return true
	}

	if count < executable.Spec.Replicas {
		// We might have thought we are done, but now more replicas are needed.
		// This can happen due to Spec update. We need to clear the finish timestamp.
		executable.Status.FinishTimestamp = nil

		cStart := executable.Spec.Replicas - count
		log.Info("additional replicas needed", "Count", cStart)
		if cStart > maxReplicaConcurrentStarts {
			cStart = maxReplicaConcurrentStarts
		}
		log.Info("starting replicas", "Count", cStart)

		for i := 0; i < cStart; i++ {
			r.startReplica(ctx, executable, log)
		}

		return true
	}

	return false
}

func (r *ExecutableReconciler) startReplica(ctx context.Context, executable *radiusv1alpha1.Executable, log logr.Logger) {
	var err error
	var rs radiusv1alpha1.ReplicaStatus
	env := toEnvArray(executable.Spec.Env)

	log.Info("starting replica...",
		"executable", executable.Spec.Executable,
		"args", fmt.Sprintf("%v", executable.Spec.Args),
		"env", fmt.Sprintf("%v", env))
	pid, startWaiting, err := r.ProcessExecutor.StartProcess(
		ctx,
		executable.Spec.Executable,
		executable.Spec.Args,
		env,
		r)
	if err != nil {
		log.Error(err, "failed to start a replica")
		rs.PID = InvalidPID
		rs.ExitCode = ExitCodeFailedToStart
	} else {
		log.Info("replica started", "PID", pid)
		rs.PID = pid
		rs.ExitCode = ExitCodeRunning
	}

	executable.Status.AddReplica(rs)
	if err == nil {
		r.processStarted(pid, types.NamespacedName{Name: executable.Name, Namespace: executable.Namespace})
		startWaiting()
	}
}

func (r *ExecutableReconciler) checkDone(executable *radiusv1alpha1.Executable, log logr.Logger) (done bool, changed bool) {
	if !executable.Status.FinishTimestamp.IsZero() {
		return true, false
	}

	if executable.Spec.Replicas > len(executable.Status.Replicas) {
		return false, false
	}

	cDone := 0
	for _, rs := range executable.Status.Replicas {
		if rs.ExitCode != ExitCodeRunning {
			cDone++
		}
	}

	if cDone >= executable.Spec.Replicas {
		now := metav1.Now()
		executable.Status.FinishTimestamp = &now
		log.Info("Marking Executable as finished", "Timestamp", executable.Status.FinishTimestamp)
		return true, true
	} else {
		return false, false
	}
}

func (r *ExecutableReconciler) terminateRemainingReplicas(owner types.NamespacedName, log logr.Logger) {
	processReplica := func(k, v interface{}) bool {
		ps := v.(runningProcessStatus)
		pid := k.(int)
		ours := ps.OwnerName == owner.Name && ps.OwnerNamespace == owner.Namespace
		running := ps.ExitCode == ExitCodeRunning || ps.ExitCode == ExitCodeAbandoned
		if ours && running {
			r.stopReplica(pid, log)
		}

		return true
	}

	r.processStatus.Range(processReplica)
}

func (r *ExecutableReconciler) stopReplica(pid int, log logr.Logger) {
	r.processStatus.Delete(pid)

	err := r.ProcessExecutor.StopProcess(pid)
	if err != nil {
		log.Info("could not terminate replica process", "PID", pid, "Error", err.Error())
	} else {
		log.Info("replica process terminated", "PID", pid)
	}
}

func (r *ExecutableReconciler) OnProcessExited(pid int, exitCode int, err error) {
	if err != nil {
		r.Log.Info("replica process could not be tracked", "PID", pid, "Error", err.Error())

		// Need to keep information about the replica process around for the next run of reconciliation loop.
		// The reconciliation will mark the replica Status accordingly.
		r.memorizeExitCode(pid, ExitCodeAbandoned)
	} else {
		found := r.memorizeExitCode(pid, exitCode)

		// Receiving an exit code update for process that we are not tracking is not necessarily a problem.
		// It can happen when Executable starts a bunch of replicas and then the number of replicas is decreased in the spec.
		// Extra replicas are then killed, but we might still receive exit notifications for them.
		if found {
			r.Log.Info("replica process exited", "PID", pid, "exitCode", exitCode)
		}
	}

}

func (r *ExecutableReconciler) getProcesStatus(pid int) (runningProcessStatus, bool) {
	retval, found := r.processStatus.Load(pid)
	if found {
		return retval.(runningProcessStatus), found
	} else {
		return runningProcessStatus{}, found
	}
}

func (r *ExecutableReconciler) processStarted(pid int, owner types.NamespacedName) {
	status := runningProcessStatus{
		ExitCode:       ExitCodeRunning,
		OwnerName:      owner.Name,
		OwnerNamespace: owner.Namespace,
	}
	r.processStatus.Store(pid, status)
}

func (r *ExecutableReconciler) memorizeExitCode(pid int, exitCode int) bool {
	status, found := r.getProcesStatus(pid)
	if found {
		status.ExitCode = exitCode
		r.processStatus.Store(pid, status)
	}
	return found
}

func toEnvArray(env map[string]string) []string {
	retval := make([]string, len(env))
	i := 0
	for k, v := range env {
		retval[i] = fmt.Sprintf("%s=%s", k, v)
		i++
	}
	return retval
}

func (r *ExecutableReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.processStatus = &sync.Map{}

	if r.ProcessExecutor == nil {
		r.ProcessExecutor = process.NewOSExecutor()
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&radiusv1alpha1.Executable{}).
		Complete(r)
}
