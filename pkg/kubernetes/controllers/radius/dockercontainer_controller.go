// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sync"

	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/Azure/radius/pkg/process"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DockerContainerReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	// A PID-to-runningProcessStatus map used to track changes to the replicas of an DockerContainer.
	processStatus   *sync.Map
	ProcessExecutor process.Executor
}

//+kubebuilder:rbac:groups=radius.dev,resources=dockercontainers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=radius.dev,resources=dockercontainers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=radius.dev,resources=dockercontainers/finalizers,verbs=update

func (r *DockerContainerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("DockerContainer", req.NamespacedName)

	var dockerContainer radiusv1alpha3.DockerContainer
	err := r.Get(ctx, req.NamespacedName, &dockerContainer)
	if err != nil && client.IgnoreNotFound(err) == nil {
		// The DockerContainer has been deleted
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "falied to Get() the DockerContainer")
		return ctrl.Result{}, err
	}

	r.startReplica(ctx, &dockerContainer, log)

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

func (r *DockerContainerReconciler) startReplica(ctx context.Context, container *radiusv1alpha3.DockerContainer, log logr.Logger) {
	var err error
	var rs radiusv1alpha3.ReplicaStatus

	log.Info("starting replica...",
		"image", container.Spec.Image,
		"args", fmt.Sprintf("%v", container.Spec.Args),
		"env", fmt.Sprintf("%v", container.Spec.Env))

	logfile, err := os.CreateTemp("", path.Base(container.Spec.Image))
	if err != nil {
		log.Error(err, "failed to start a replica")
		rs.PID = InvalidPID
		rs.ExitCode = ExitCodeFailedToStart
	}

	// ports, err := allocatePorts(ctx, container)
	// if err != nil {
	// 	log.Error(err, "failed to allocate ports for a replica")
	// 	rs.PID = InvalidPID
	// 	rs.ExitCode = ExitCodeFailedToStart
	// }

	// rs.Ports = ports

	cmd := makeDockerCommand(ctx, container, logfile)
	pid, startWaiting, err := r.ProcessExecutor.StartProcess(ctx, cmd, r)
	if err != nil {
		log.Error(err, "failed to start a replica")
		rs.PID = InvalidPID
		rs.ExitCode = ExitCodeFailedToStart
	} else {
		log.Info("replica started", "PID", pid)
		rs.PID = pid
		rs.ExitCode = ExitCodeRunning
	}

	// TODO: nothing currently cleans up these log files.
	rs.LogFile = logfile.Name()

	// container.Status.AddReplica(rs)
	if err == nil {
		r.processStarted(pid, types.NamespacedName{Name: container.Name, Namespace: container.Namespace})
		startWaiting()
	}
}

func (r *DockerContainerReconciler) OnProcessExited(pid int, exitCode int, err error) {
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

func (r *DockerContainerReconciler) processStarted(pid int, owner types.NamespacedName) {
	// status := runningProcessStatus{
	// 	ExitCode:       ExitCodeRunning,
	// 	OwnerName:      owner.Name,
	// 	OwnerNamespace: owner.Namespace,
	// }
	// r.processStatus.Store(pid, status)
}

func (r *DockerContainerReconciler) memorizeExitCode(pid int, exitCode int) bool {
	// status, found := r.getProcesStatus(pid)
	// if found {
	// 	status.ExitCode = exitCode
	// 	r.processStatus.Store(pid, status)
	// }
	return true
}

// func allocateDockerPorts(ctx context.Context, executable *radiusv1alpha3.DockerContainer) ([]radiusv1alpha3.ReplicaPort, error) {
// 	status := []radiusv1alpha3.ReplicaPort{}
// 	for i := range executable.Spec.Ports {
// 		if executable.Spec.Ports[i].Dynamic {
// 			free, err := getFreePort()
// 			if err != nil {
// 				return nil, err
// 			}

// 			executable.Spec.Ports[i].Port = &free

// 			if executable.Spec.Env == nil {
// 				executable.Spec.Env = map[string]string{}
// 			}

// 			for _, e := range executable.Spec.Ports[i].Env {
// 				executable.Spec.Env[e] = fmt.Sprintf("%d", free)
// 			}
// 		}

// 		if executable.Spec.Ports[i].Port == nil {
// 			continue
// 		}

// 		assigned := *executable.Spec.Ports[i].Port
// 		status = append(status, radiusv1alpha3.ReplicaPort{Name: executable.Spec.Ports[i].Name, Port: assigned})
// 	}

// 	return status, nil
// }

// func getFreePort() (int, error) {
// 	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
// 	if err != nil {
// 		return 0, err
// 	}

// 	l, err := net.ListenTCP("tcp", addr)
// 	if err != nil {
// 		return 0, err
// 	}
// 	defer l.Close()
// 	return l.Addr().(*net.TCPAddr).Port, nil
// }

func makeDockerCommand(ctx context.Context, container *radiusv1alpha3.DockerContainer, logfile *os.File) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "docker")
	cmdArgs := []string{"docker", "run", container.Spec.Image, "--name", "test"}
	cmdArgs = append(cmdArgs, container.Spec.Args...)
	cmd.Args = cmdArgs
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// We need to also capture some of the current environment like PATH - otherwise it's
	// pretty shocking.
	//
	// We append anything the user provided so that it will take precedence.
	env := append(os.Environ(), toEnvArray(container.Spec.Env)...)
	cmd.Env = env
	// cmd.Dir = container.Spec.WorkingDirectory
	return cmd
}

func (r *DockerContainerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.processStatus = &sync.Map{}

	if r.ProcessExecutor == nil {
		r.ProcessExecutor = process.NewOSExecutor()
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&radiusv1alpha3.DockerContainer{}).
		Complete(r)
}
