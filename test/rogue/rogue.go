/*
Copyright 2026 The Radius Authors.

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

// Package rogue simulates a compromised workload or component inside the cluster for
// authorization functional tests. It runs a pod from the rogue-client image (built from
// ./cmd/rogue-client like magpiego and testrp) and calls internal Radius endpoints from it,
// optionally presenting a TLS certificate from a mounted Secret.
package rogue

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	utilexec "k8s.io/client-go/util/exec"

	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/rogue/rogueclient"
	"github.com/radius-project/radius/test/testutil"
)

const (
	// TLSMountPath is where the Secret named by Options.TLSSecretName is mounted.
	// Requests read tls.crt, tls.key, and ca.crt from this directory.
	TLSMountPath = "/etc/radius-rogue/tls"

	containerName = "rogue"
	podNamePrefix = "radius-rogue-"
	labelKey      = "app.kubernetes.io/name"
	labelValue    = "radius-rogue-client"
	tlsVolumeName = "tls"
	imageName     = "rogue-client"
	clientBinary  = "/rogue-client"
	clientTimeout = "30s"
	readyTimeout  = 2 * time.Minute

	// runAsUser is the distroless nonroot UID. It must be numeric for runAsNonRoot to pass.
	runAsUser = int64(65532)
)

// DefaultImage returns the rogue-client image from DOCKER_REGISTRY and REL_VERSION,
// matching how functional tests locate magpiego.
func DefaultImage() string {
	registry, tag := testutil.SetDefault()
	return registry + "/" + imageName + ":" + tag
}

var fatalWaitingReasons = []string{"ErrImagePull", "ImagePullBackOff", "CreateContainerConfigError"}

// Options configures the rogue pod.
type Options struct {
	// Namespace is the existing namespace to create the pod in.
	Namespace string

	// Image overrides DefaultImage().
	Image string

	// TLSSecretName optionally names a kubernetes.io/tls Secret in Namespace to mount at
	// TLSMountPath, so requests can present its certificate to act as a component.
	TLSSecretName string
}

// Request is an HTTP request sent from the rogue pod.
type Request struct {
	// Method defaults to GET.
	Method  string
	URL     string
	Headers map[string]string
	Body    string

	// ClientCert presents tls.crt and tls.key from the mounted TLS Secret.
	ClientCert bool

	// CACert verifies the server with ca.crt from the mounted TLS Secret.
	CACert bool

	// Insecure skips server certificate verification.
	Insecure bool
}

// Response is the result of a Request.
type Response struct {
	// StatusCode is the HTTP status, or 0 when no HTTP response was received
	// (for example, a refused connection or failed TLS handshake).
	StatusCode int
	Body       string

	// ExitCode is the rogue-client exit code: rogueclient.ExitOK when an HTTP response was
	// received, rogueclient.ExitTransport when none was, and rogueclient.ExitUsage for
	// invalid arguments. Tests that expect a request failure should assert the exact
	// ExitCode or HTTP status and error body, plus a positive control from the same pod
	// when possible.
	ExitCode int
	Stderr   string
}

// Client sends requests from a rogue pod.
type Client struct {
	k8s    kubernetes.Interface
	config *rest.Config
	pod    *corev1.Pod
	hasTLS bool
}

// New creates the rogue pod, waits for it to run, and deletes it when the test finishes.
func New(t *testing.T, k8s kubernetes.Interface, config *rest.Config, options Options) *Client {
	t.Helper()
	require.NotEmpty(t, options.Namespace, "rogue: Namespace is required")

	ctx := t.Context()
	pods := k8s.CoreV1().Pods(options.Namespace)
	pod, err := pods.Create(ctx, newPod(options), metav1.CreateOptions{})
	require.NoError(t, err, "rogue: failed to create pod")
	t.Logf("Created rogue pod %s/%s", pod.Namespace, pod.Name)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) //nolint:usetesting // t.Context() is canceled before cleanup runs.
		defer cancel()
		err := pods.Delete(ctx, pod.Name, metav1.DeleteOptions{GracePeriodSeconds: to.Ptr(int64(0))})
		if err != nil {
			t.Logf("rogue: failed to delete pod %s/%s: %v", pod.Namespace, pod.Name, err)
		}
	})

	err = wait.PollUntilContextTimeout(ctx, time.Second, readyTimeout, true, func(ctx context.Context) (bool, error) {
		current, err := pods.Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		pod = current
		if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
			return false, fmt.Errorf("pod exited with phase %s", pod.Status.Phase)
		}
		if err := podStartFailure(pod); err != nil {
			return false, err
		}
		return pod.Status.Phase == corev1.PodRunning, nil
	})
	if err != nil {
		if waiting := podWaitingState(pod); waiting != "" {
			err = fmt.Errorf("%w: %s", err, waiting)
		}
	}
	require.NoError(t, err, "rogue: pod %s/%s did not start", pod.Namespace, pod.Name)

	return &Client{k8s: k8s, config: config, pod: pod, hasTLS: options.TLSSecretName != ""}
}

// Pod returns the rogue pod.
func (c *Client) Pod() *corev1.Pod {
	return c.pod
}

// Do sends req from the rogue pod. It returns an error only when the request could not be
// run; HTTP errors and transport failures are reported in the Response. Negative tests should
// distinguish expected rejection from broken connectivity by asserting the exact ExitCode
// or HTTP status and error body.
func (c *Client) Do(ctx context.Context, req Request) (Response, error) {
	args, err := clientArgs(req, c.hasTLS)
	if err != nil {
		return Response{}, err
	}

	execReq := c.k8s.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(c.pod.Namespace).
		Name(c.pod.Name).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   args,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(c.config, http.MethodPost, execReq.URL())
	if err != nil {
		return Response{}, fmt.Errorf("rogue: creating exec stream: %w", err)
	}

	var stdout, stderr bytes.Buffer
	resp := Response{}
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: &stdout, Stderr: &stderr})
	resp.Stderr = stderr.String()

	var exitErr utilexec.ExitError
	switch {
	case errors.As(err, &exitErr):
		resp.ExitCode = exitErr.ExitStatus()
	case err != nil:
		return resp, fmt.Errorf("rogue: exec failed: %w: %s", err, strings.TrimSpace(resp.Stderr))
	}

	resp.StatusCode, resp.Body, err = parseOutput(stdout.String())
	if err != nil && resp.ExitCode != rogueclient.ExitOK {
		// rogue-client exits before writing the status on invalid arguments.
		return resp, nil
	}
	return resp, err
}

// newPod builds the rogue pod spec.
func newPod(options Options) *corev1.Pod {
	image := options.Image
	if image == "" {
		image = DefaultImage()
	}

	container := corev1.Container{
		Name:    containerName,
		Image:   image,
		Command: []string{clientBinary, "idle"},
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             to.Ptr(true),
			RunAsUser:                to.Ptr(runAsUser),
			AllowPrivilegeEscalation: to.Ptr(false),
			ReadOnlyRootFilesystem:   to.Ptr(true),
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
			SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
	}

	pod := &corev1.Pod{
		GenerateName: podNamePrefix,
		Namespace:    options.Namespace,
		Labels:       map[string]string{labelKey: labelValue},
		Spec: corev1.PodSpec{
			RestartPolicy:                 corev1.RestartPolicyNever,
			AutomountServiceAccountToken:  to.Ptr(false),
			TerminationGracePeriodSeconds: to.Ptr(int64(0)),
		},
	}

	if options.TLSSecretName != "" {
		pod.Spec.Volumes = []corev1.Volume{{
			Name:   tlsVolumeName,
			Secret: &corev1.SecretVolumeSource{SecretName: options.TLSSecretName},
		}}
		container.VolumeMounts = []corev1.VolumeMount{{Name: tlsVolumeName, MountPath: TLSMountPath, ReadOnly: true}}
	}

	pod.Spec.Containers = []corev1.Container{container}
	return pod
}

func podStartFailure(pod *corev1.Pod) error {
	for _, status := range waitingStatuses(pod) {
		if slices.Contains(fatalWaitingReasons, status.State.Waiting.Reason) {
			return errors.New(formatWaitingState(status))
		}
	}
	return nil
}

func podWaitingState(pod *corev1.Pod) string {
	statuses := waitingStatuses(pod)
	if len(statuses) == 0 {
		return ""
	}
	return formatWaitingState(statuses[0])
}

type waitingStatus struct {
	prefix string
	corev1.ContainerStatus
}

func waitingStatuses(pod *corev1.Pod) []waitingStatus {
	statuses := []waitingStatus{}
	for _, status := range pod.Status.InitContainerStatuses {
		if status.State.Waiting != nil {
			statuses = append(statuses, waitingStatus{prefix: "init container", ContainerStatus: status})
		}
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Waiting != nil {
			statuses = append(statuses, waitingStatus{prefix: "container", ContainerStatus: status})
		}
	}
	return statuses
}

func formatWaitingState(status waitingStatus) string {
	waiting := status.State.Waiting
	if waiting.Message == "" {
		return fmt.Sprintf("%s %q is waiting: %s", status.prefix, status.Name, waiting.Reason)
	}
	return fmt.Sprintf("%s %q is waiting: %s: %s", status.prefix, status.Name, waiting.Reason, waiting.Message)
}

// clientArgs builds the rogue-client command for req. The response body is written to
// stdout, followed by rogueclient.StatusMarker and the HTTP status code.
func clientArgs(req Request, hasTLS bool) ([]string, error) {
	if req.URL == "" {
		return nil, errors.New("rogue: URL is required")
	}
	if (req.ClientCert || req.CACert) && !hasTLS {
		return nil, errors.New("rogue: ClientCert and CACert require Options.TLSSecretName")
	}

	method := req.Method
	if method == "" {
		method = http.MethodGet
	}

	args := []string{clientBinary, "request", "--max-time", clientTimeout, "-X", method}

	keys := make([]string, 0, len(req.Headers))
	for k := range req.Headers {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		args = append(args, "-H", k+": "+req.Headers[k])
	}

	if req.Body != "" {
		args = append(args, "--data", req.Body)
	}
	if req.ClientCert {
		args = append(args, "--cert", path.Join(TLSMountPath, "tls.crt"), "--key", path.Join(TLSMountPath, "tls.key"))
	}
	if req.CACert {
		args = append(args, "--cacert", path.Join(TLSMountPath, "ca.crt"))
	}
	if req.Insecure {
		args = append(args, "-k")
	}

	return append(args, req.URL), nil
}

// parseOutput splits rogue-client's stdout into the response body and HTTP status code.
// A status of 0 means no HTTP response was received.
func parseOutput(stdout string) (int, string, error) {
	i := strings.LastIndex(stdout, rogueclient.StatusMarker)
	if i < 0 {
		return 0, stdout, fmt.Errorf("rogue: status marker not found in client output %q", stdout)
	}

	status, err := strconv.Atoi(strings.TrimSpace(stdout[i+len(rogueclient.StatusMarker):]))
	if err != nil {
		return 0, stdout, fmt.Errorf("rogue: invalid status in client output: %w", err)
	}

	return status, stdout[:i], nil
}
