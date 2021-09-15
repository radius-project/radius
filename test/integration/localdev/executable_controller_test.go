// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	wait "k8s.io/apimachinery/pkg/util/wait"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	radiusv1alpha1 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha1"
	radcontroller "github.com/Azure/radius/pkg/kubernetes/controllers/radius"
	"github.com/Azure/radius/test/testcontext"
)

var (
	testEnv  *envtest.Environment
	executor *TestProcessExecutor
	client   runtimeclient.Client
)

func TestMain(m *testing.M) {
	err := startController()
	if err != nil {
		panic(err)
	}
	code := m.Run()

	err = stopController()
	if err != nil {
		panic(err)
	}

	os.Exit(code)
}

func startController() error {
	assetDir, err := getKubeAssetsDir()
	if err != nil {
		return err
	}

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "deploy", "localdev", "crds")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: assetDir,
	}

	scheme := runtime.NewScheme()

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(radiusv1alpha1.AddToScheme(scheme))

	cfg, err := testEnv.Start()
	if err != nil {
		return fmt.Errorf("failed to initialize environment: %w", err)
	}

	client, err = runtimeclient.New(cfg, runtimeclient.Options{
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme,
		LeaderElection:     false,
		MetricsBindAddress: "0",
	})
	if err != nil {
		return fmt.Errorf("failed to initialize manager: %w", err)
	}

	executor = NewTestProcessExecutor()
	if err = (&radcontroller.ExecutableReconciler{
		Client:          mgr.GetClient(),
		Log:             ctrl.Log.WithName("controllers").WithName("Executable"),
		Scheme:          mgr.GetScheme(),
		ProcessExecutor: executor,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("failed to initialize Executable reconciler: %w", err)
	}

	go func() {
		_ = mgr.Start(ctrl.SetupSignalHandler())
	}()

	return nil
}

func stopController() error {
	return testEnv.Stop()
}

func getKubeAssetsDir() (string, error) {
	assetsDirectory := os.Getenv("KUBEBUILDER_ASSETS")

	if assetsDirectory != "" {
		return assetsDirectory, nil
	}

	// TODO https://github.com/Azure/radius/issues/698, remove hard coded version
	cmd := exec.Command("setup-envtest", "use", "-p", "path", "1.19.x", "--arch", "amd64")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to call setup-envtest to find path: %w", err)
	} else {
		return out.String(), err
	}
}

func find(replicas []radiusv1alpha1.ReplicaStatus, pid int) (radiusv1alpha1.ReplicaStatus, error) {
	for _, rc := range replicas {
		if rc.PID == pid {
			return rc, nil
		}
	}

	return radiusv1alpha1.ReplicaStatus{}, fmt.Errorf("Not found")
}

func ensureNamespace(ctx context.Context, namespace string) error {
	nsObject := v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	return client.Create(ctx, &nsObject, &runtimeclient.CreateOptions{})
}

func ensureReplicasRunning(ctx context.Context, exeName string, n int) {
	waitReplicaStarted := func() (bool, error) {
		runninReplicas := executor.FindAll(exeName, func(pe ProcessExecution) bool {
			return pe.EndedAt.IsZero()
		})

		return len(runninReplicas) == n, nil
	}
	wait.PollUntil(time.Second, waitReplicaStarted, ctx.Done())
}

func updateExecutable(t *testing.T, ctx context.Context, key runtimeclient.ObjectKey, applyChanges func(*radiusv1alpha1.Executable)) error {
	const maxAttempts = 5
	attempt := 0
	var exe radiusv1alpha1.Executable

	for {
		if attempt == maxAttempts {
			return fmt.Errorf("Update failed: too many attempts")
		}

		if err := client.Get(ctx, key, &exe); err != nil {
			return err
		}

		applyChanges(&exe)

		if err := client.Update(ctx, &exe); err != nil {
			if errors.IsConflict(err) {
				t.Log("Conflict detected, retrying Executable update...")
				attempt++
				time.Sleep(time.Second)
				continue
			} else {
				return err
			}
		}

		return nil
	}
}

// Ensure that a replica is started when new Executable object appears
func TestExecutableStartsReplicas(t *testing.T) {
	t.Parallel()
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	const namespace = "executable-starts-replicas-ns"
	if err := ensureNamespace(ctx, namespace); err != nil {
		t.Fatalf("Could not create namespace for the test: %v", err)
	}

	exe := radiusv1alpha1.Executable{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "executable-starts-replicas",
		},
		Spec: radiusv1alpha1.ExecutableSpec{
			Executable: "path/to/executable-starts-replicas",
			Replicas:   1,
		},
	}

	t.Logf("Creating Executable '%s'", exe.ObjectMeta.Name)
	if err := client.Create(ctx, &exe, &runtimeclient.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create Executable: %v", err)
	}

	t.Log("Checking if replica has started...")
	ensureReplicasRunning(ctx, exe.Spec.Executable, exe.Spec.Replicas)
}

// Ensure exit code(s) of replicas are captured when replicas exit
func TestExitCodeCaptured(t *testing.T) {
	t.Parallel()
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	const namespace = "exit-code-captured-ns"
	if err := ensureNamespace(ctx, namespace); err != nil {
		t.Fatalf("Could not create namespace for the test: %v", err)
	}

	exe := radiusv1alpha1.Executable{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "exit-code-captured",
		},
		Spec: radiusv1alpha1.ExecutableSpec{
			Executable: "path/to/exit-code-captured",
			Replicas:   2,
		},
	}

	t.Logf("Creating Executable '%s'", exe.ObjectMeta.Name)
	if err := client.Create(ctx, &exe, &runtimeclient.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create Executable: %v", err)
	}

	t.Log("Waiting for replicas to start...")
	ensureReplicasRunning(ctx, exe.Spec.Executable, exe.Spec.Replicas)

	t.Log("Replicas started, shutting them down...")
	var replicas []ProcessExecution
	replicas = executor.FindAll(exe.Spec.Executable, nil)
	require.Equal(t, 2, len(replicas))
	const r0_ec, r1_ec = 12, 14
	executor.SimulateProcessExit(t, replicas[0].PID, r0_ec)
	executor.SimulateProcessExit(t, replicas[1].PID, r1_ec)

	waitExitCodeCaptured := func() (bool, error) {
		t.Log("Checking replica exit codes...")
		var updatedExe radiusv1alpha1.Executable
		if err := client.Get(ctx, runtimeclient.ObjectKeyFromObject(&exe), &updatedExe); err != nil {
			t.Fatalf("Unable to fetch updated Executable: %v", err)
			return false, err
		}

		if len(updatedExe.Status.Replicas) < 2 {
			return false, nil
		}

		rs, err := find(updatedExe.Status.Replicas, replicas[0].PID)
		if err != nil {
			return false, nil
		}
		if rs.ExitCode != r0_ec {
			return false, fmt.Errorf("Unexpected exit code from first replica: expected %d actual %d", r0_ec, rs.ExitCode)
		}

		rs, err = find(updatedExe.Status.Replicas, replicas[1].PID)
		if err != nil {
			return false, nil
		}
		if rs.ExitCode != r1_ec {
			return false, fmt.Errorf("Unexpected exit code from second replica: expected %d actual %d", r1_ec, rs.ExitCode)
		}

		return true, nil
	}
	wait.PollUntil(time.Second, waitExitCodeCaptured, ctx.Done())
}

// Ensure that additional replicas are started if desired replica count is increased
func TestReplicaScaleUp(t *testing.T) {
	t.Parallel()
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	const namespace = "replica-scale-up-ns"
	if err := ensureNamespace(ctx, namespace); err != nil {
		t.Fatalf("Could not create namespace for the test: %v", err)
	}

	exe := radiusv1alpha1.Executable{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "replica-scale-up",
		},
		Spec: radiusv1alpha1.ExecutableSpec{
			Executable: "path/to/replica-scale-up",
			Replicas:   3,
		},
	}

	t.Logf("Creating Executable '%s'", exe.ObjectMeta.Name)
	if err := client.Create(ctx, &exe, &runtimeclient.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create Executable: %v", err)
	}

	t.Log("Waiting for replicas to start...")
	ensureReplicasRunning(ctx, exe.Spec.Executable, exe.Spec.Replicas)

	const desired = 5
	t.Logf("Increasing desired replica count to %d...", desired)
	if err := updateExecutable(t, ctx, runtimeclient.ObjectKeyFromObject(&exe), func(e *radiusv1alpha1.Executable) { e.Spec.Replicas = desired }); err != nil {
		t.Fatalf("Unable to update Executable: %v", err)
	}

	t.Log("Waiting for additional replicas to start...")
	ensureReplicasRunning(ctx, exe.Spec.Executable, desired)
}

// Ensure that unnecessary replicas are killed if desired replica count is decreased
func TestReplicaScaleDown(t *testing.T) {
	t.Parallel()
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	const namespace = "replica-scale-down-ns"
	if err := ensureNamespace(ctx, namespace); err != nil {
		t.Fatalf("Could not create namespace for the test: %v", err)
	}

	exe := radiusv1alpha1.Executable{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "replica-scale-down",
		},
		Spec: radiusv1alpha1.ExecutableSpec{
			Executable: "path/to/replica-scale-down",
			Replicas:   5,
		},
	}

	t.Logf("Creating Executable '%s'", exe.ObjectMeta.Name)
	if err := client.Create(ctx, &exe, &runtimeclient.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create Executable: %v", err)
	}

	t.Log("Waiting for replicas to start...")
	ensureReplicasRunning(ctx, exe.Spec.Executable, exe.Spec.Replicas)

	const desired = 1
	t.Logf("Decreasing desired replica count to %d...", desired)
	if err := updateExecutable(t, ctx, runtimeclient.ObjectKeyFromObject(&exe), func(e *radiusv1alpha1.Executable) { e.Spec.Replicas = desired }); err != nil {
		t.Fatalf("Unable to update Executable: %v", err)
	}

	t.Log("Waiting for running replicas to reach desired number...")
	ensureReplicasRunning(ctx, exe.Spec.Executable, desired)
}

// Ensure that Executable is marked as finished (FinishTimestamp is set) if all replicas end execution
func TestExecutableFinishHandling(t *testing.T) {
	t.Parallel()
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	const namespace = "executable-finish-handling-ns"
	if err := ensureNamespace(ctx, namespace); err != nil {
		t.Fatalf("Could not create namespace for the test: %v", err)
	}

	exe := radiusv1alpha1.Executable{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "finish-handling",
		},
		Spec: radiusv1alpha1.ExecutableSpec{
			Executable: "path/to/finish-handling",
			Replicas:   3,
		},
	}

	t.Logf("Creating Executable '%s'", exe.ObjectMeta.Name)
	if err := client.Create(ctx, &exe, &runtimeclient.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create Executable: %v", err)
	}

	t.Log("Waiting for replicas to start...")
	ensureReplicasRunning(ctx, exe.Spec.Executable, exe.Spec.Replicas)

	t.Log("Simulating replica finish...")
	replicas := executor.FindAll(exe.Spec.Executable, nil)
	for _, r := range replicas {
		executor.SimulateProcessExit(t, r.PID, 0)
	}

	waitExecutableFinish := func() (bool, error) {
		t.Log("Checking Executable status...")
		var updatedExe radiusv1alpha1.Executable
		if err := client.Get(ctx, runtimeclient.ObjectKeyFromObject(&exe), &updatedExe); err != nil {
			t.Fatalf("Unable to fetch updated Executable: %v", err)
			return false, err
		}

		if updatedExe.Status.FinishTimestamp.IsZero() {
			return false, nil // Not finished yet, keep waiting
		} else {
			return true, nil
		}
	}
	wait.PollUntil(time.Second, waitExecutableFinish, ctx.Done())
}

// Ensure that Executable is marked as finished (FinishTimestamp is set) if all replicas are terminated as a result of scale-down
func TestExecutableFinishAfterScaleDown(t *testing.T) {
	t.Parallel()
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	const namespace = "executable-finish-after-scale-down-ns"
	if err := ensureNamespace(ctx, namespace); err != nil {
		t.Fatalf("Could not create namespace for the test: %v", err)
	}

	exe := radiusv1alpha1.Executable{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "finish-after-scale-down",
		},
		Spec: radiusv1alpha1.ExecutableSpec{
			Executable: "path/to/finish-after-scale-down",
			Replicas:   5,
		},
	}

	t.Logf("Creating Executable '%s'", exe.ObjectMeta.Name)
	if err := client.Create(ctx, &exe, &runtimeclient.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create Executable: %v", err)
	}

	t.Log("Waiting for replicas to start...")
	ensureReplicasRunning(ctx, exe.Spec.Executable, exe.Spec.Replicas)

	t.Log("Simulating two replica finish...")
	replicas := executor.FindAll(exe.Spec.Executable, nil)
	require.Equal(t, exe.Spec.Replicas, len(replicas))
	executor.SimulateProcessExit(t, replicas[0].PID, 0)
	executor.SimulateProcessExit(t, replicas[1].PID, 0)

	// With two replicas finished normally, after scaling the Spec down to two replicas, the Executable should:
	// 1. be marked as finished
	// 2. remaining replicas should be killed
	const desired = 2
	t.Logf("Decreasing desired replica count to %d...", desired)
	if err := updateExecutable(t, ctx, runtimeclient.ObjectKeyFromObject(&exe), func(e *radiusv1alpha1.Executable) { e.Spec.Replicas = desired }); err != nil {
		t.Fatalf("Unable to update Executable: %v", err)
	}

	waitExecutableFinish := func() (bool, error) {
		t.Log("Checking Executable status...")
		var updatedExe radiusv1alpha1.Executable
		if err := client.Get(ctx, runtimeclient.ObjectKeyFromObject(&exe), &updatedExe); err != nil {
			t.Fatalf("Unable to fetch updated Executable: %v", err)
			return false, err
		}

		if updatedExe.Status.FinishTimestamp.IsZero() {
			return false, nil // Not finished yet, keep waiting
		} else {
			return true, nil
		}
	}
	wait.PollUntil(time.Second, waitExecutableFinish, ctx.Done())

	replicas = executor.FindAll(exe.Spec.Executable, func(pe ProcessExecution) bool {
		return !pe.EndedAt.IsZero() && pe.ExitCode == 0
	})
	require.Equal(t, 2, len(replicas), "Expected two normally finished replicas")

	replicas = executor.FindAll(exe.Spec.Executable, func(pe ProcessExecution) bool {
		return !pe.EndedAt.IsZero() && pe.ExitCode == KilledProcessExitCode
	})
	require.Equal(t, 3, len(replicas), "Expected three killed replicas")
}

// Ensure all replicas are killed if Executable is deleted
func TestReplicasTerminatedUponExecutableDeletion(t *testing.T) {
	t.Parallel()
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	const namespace = "executable-deletion-ns"
	if err := ensureNamespace(ctx, namespace); err != nil {
		t.Fatalf("Could not create namespace for the test: %v", err)
	}

	exe := radiusv1alpha1.Executable{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "executable-deletion",
		},
		Spec: radiusv1alpha1.ExecutableSpec{
			Executable: "path/to/executable-deletion",
			Replicas:   2,
		},
	}

	t.Logf("Creating Executable '%s'", exe.ObjectMeta.Name)
	if err := client.Create(ctx, &exe, &runtimeclient.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create Executable: %v", err)
	}

	t.Log("Waiting for replicas to start...")
	ensureReplicasRunning(ctx, exe.Spec.Executable, exe.Spec.Replicas)

	t.Log("Deleting executable...")
	if err := client.Delete(ctx, &exe); err != nil {
		t.Fatalf("Unable to create Executable: %v", err)
	}

	t.Log("Waiting for all replicas to be killed...")
	waitReplicasKilled := func() (bool, error) {
		killedReplicas := executor.FindAll(exe.Spec.Executable, func(pe ProcessExecution) bool {
			return !pe.EndedAt.IsZero() && pe.ExitCode == KilledProcessExitCode
		})

		return len(killedReplicas) == exe.Spec.Replicas, nil
	}
	wait.PollUntil(time.Second, waitReplicasKilled, ctx.Done())
}
