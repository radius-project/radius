// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package apiserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	v1alpha1 "github.com/project-radius/radius/pkg/queue/apiserver/api/ucp.dev/v1alpha1"
	"github.com/project-radius/radius/pkg/queue/client"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

const pollingInterval = time.Duration(100) * time.Millisecond

type testQueueMessage struct {
	ID      string `json:"id"`
	Message string `json:"msg"`
}

func drainMessages(c runtimeclient.Client, namespace string) {
	ctx := context.Background()
	ql := &v1alpha1.QueueMessageList{}
	err := c.List(ctx, ql, runtimeclient.InNamespace(namespace))
	if err != nil {
		return
	}
	for i := range ql.Items {
		_ = c.Delete(ctx, &ql.Items[i])
	}
}

func queueTestMessage(cli *Client, num int) error {
	// Enqueue multiple message and dequeue them
	for i := 0; i < num; i++ {
		msg := &testQueueMessage{ID: fmt.Sprintf("%d", i), Message: fmt.Sprintf("hello world %d", i)}
		data, err := json.Marshal(msg)
		if err != nil {
			return err
		}

		err = cli.Enqueue(context.Background(), &client.Message{Data: data})
		if err != nil {
			return err
		}
	}

	return nil
}

func TestMustParseInt64(t *testing.T) {
	result := mustParseInt64("100")
	require.Equal(t, int64(100), result)

	result = mustParseInt64("abc")
	require.Equal(t, int64(0), result)
}

func TestInt64toa(t *testing.T) {
	result := int64toa(int64(12345))
	require.Equal(t, "12345", result)
}

func TestGetTimeFromString(t *testing.T) {
	now := time.Now().UnixNano()
	unixString := fmt.Sprintf("%d", now)
	result := getTimeFromString(unixString)
	require.Equal(t, now, result.UnixNano())
}

func TestCopyMessage(t *testing.T) {
	msg := &client.Message{
		Metadata: client.Metadata{ID: "testid"},
	}
	now := time.Now()
	queueM := &v1alpha1.QueueMessage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "applications.core.10101010",
			Namespace: "radius-test",
			Labels: map[string]string{
				LabelNextVisibleAt: int64toa(now.UnixNano()),
				LabelQueueName:     "applications.core",
			},
		},
		Spec: v1alpha1.QueueMessageSpec{
			DequeueCount: 2,
			EnqueueAt:    metav1.Time{Time: now.UTC()},
			ExpireAt:     metav1.Time{Time: now.Add(10 * time.Second).UTC()},
			ContentType:  client.JSONContentType, // RawExtension supports only JSON seralized data
			Data:         &runtime.RawExtension{Raw: []byte("hello world")},
		},
	}

	copyMessage(msg, queueM)

	require.Equal(t, queueM.ObjectMeta.Name, msg.ID)
	require.Equal(t, client.JSONContentType, msg.ContentType)
	require.Equal(t, queueM.Spec.DequeueCount, msg.DequeueCount)
	require.Equal(t, queueM.Spec.Data.Raw, msg.Data)
	require.Equal(t, queueM.Spec.ExpireAt.Time, msg.ExpireAt)
	require.Equal(t, queueM.Spec.EnqueueAt.Time, msg.EnqueueAt)
	require.Equal(t, getTimeFromString(queueM.ObjectMeta.Labels[LabelNextVisibleAt]), msg.NextVisibleAt)
}

func TestGenerateID(t *testing.T) {
	cli, err := New(nil, Options{Name: "applications.core", Namespace: "test"})
	require.NoError(t, err)

	id, _ := cli.generateID()
	require.Equal(t, 60, len(id))
}

func TestClient(t *testing.T) {
	rc, env, err := startEnvironment()
	require.NoError(t, err, "If this step is failing for you, run `make test` inside the repository and try again. If you are still stuck then ask for help.")
	defer func() {
		_ = env.Stop()
	}()

	ctx, cancel := testcontext.New(t)
	defer cancel()

	ns := "radius-test"
	err = ensureNamespace(ctx, rc, ns)
	require.NoError(t, err)

	testLockTime := time.Duration(1) * time.Second

	cli, err := New(rc, Options{Name: "applications.core", Namespace: ns, MessageLockDuration: testLockTime})
	require.NoError(t, err)

	// TODO: Move the below subtests to shared test package when inmemory queue impl uses new client interface.
	t.Run("enqueue and dequeue messages", func(t *testing.T) {
		drainMessages(rc, ns)

		num := 10

		err := queueTestMessage(cli, num)
		require.NoError(t, err)

		checked := map[string]*client.Message{}
		for i := 0; i < num; i++ {
			msg, err := cli.Dequeue(ctx)
			require.NoError(t, err)
			result := &testQueueMessage{}
			err = json.Unmarshal(msg.Data, result)
			require.NoError(t, err)
			if _, ok := checked[msg.ID]; ok {
				require.Fail(t, "duplicated message")
			}
			checked[result.ID] = msg
		}

		for _, v := range checked {
			err = cli.FinishMessage(ctx, v)
			require.NoError(t, err)
		}
	})

	t.Run("message lock is expired", func(t *testing.T) {
		drainMessages(rc, ns)

		err := queueTestMessage(cli, 2)
		require.NoError(t, err)

		msg1, err := cli.Dequeue(ctx)
		require.NoError(t, err)
		require.NotNil(t, msg1)

		time.Sleep(10 * time.Millisecond)

		msg2, err := cli.Dequeue(ctx)
		require.NoError(t, err)
		require.NotNil(t, msg2)

		// Ensure that queue doesn't have any valid messages
		_, err = cli.Dequeue(ctx)
		require.ErrorIs(t, err, client.ErrMessageNotFound)

		// Dequeue until message is requeued.
		var msg3 *client.Message
		for {
			msg3, err = cli.Dequeue(ctx)
			if err == nil {
				break
			}
			time.Sleep(pollingInterval)
		}

		require.Equal(t, msg1.ID, msg3.ID)
	})

	t.Run("extend valid message lock", func(t *testing.T) {
		drainMessages(rc, ns)

		err := queueTestMessage(cli, 2)
		require.NoError(t, err)

		msg1, err := cli.Dequeue(ctx)
		require.NoError(t, err)
		t.Logf("%s %v", msg1.ID, msg1.NextVisibleAt)

		msg2, err := cli.Dequeue(ctx)
		require.NoError(t, err)
		t.Logf("%s %v", msg2.ID, msg2.NextVisibleAt)

		// Ensure that queue doesn't have any valid messages
		_, err = cli.Dequeue(ctx)
		require.ErrorIs(t, err, client.ErrMessageNotFound)
		// Extend msg1 after sometime
		time.Sleep(testLockTime / 2)
		err = cli.ExtendMessage(ctx, msg1)
		t.Logf("%s %v", msg1.ID, msg1.NextVisibleAt)
		require.NoError(t, err)

		for {
			// msg2 is requeued. msg3 must be msg2
			msg3, err := cli.Dequeue(ctx)
			if err == nil {
				t.Logf("%s %v", msg3.ID, msg3.NextVisibleAt)
				require.Equal(t, msg2.ID, msg3.ID)
				break
			}
			time.Sleep(pollingInterval)
		}
	})

	t.Run("extend invalid message lock", func(t *testing.T) {
		drainMessages(rc, ns)

		err := queueTestMessage(cli, 2)
		require.NoError(t, err)

		msg1, err := cli.Dequeue(ctx)
		require.NoError(t, err)
		t.Logf("%s %v", msg1.ID, msg1.NextVisibleAt)

		time.Sleep(testLockTime / 2)

		msg2, err := cli.Dequeue(ctx)
		require.NoError(t, err)
		t.Logf("%s %v", msg2.ID, msg2.NextVisibleAt)

		for {
			msg3, err := cli.Dequeue(ctx)
			if err == nil {
				t.Logf("%s %v", msg3.ID, msg3.NextVisibleAt)
				break
			}
			time.Sleep(pollingInterval)
		}

		// Wait until message lock is released.
		time.Sleep(testLockTime)
		err = cli.ExtendMessage(ctx, msg2)
		require.ErrorIs(t, err, client.ErrInvalidMessage)
	})

	t.Run("StartDequeuer dequeues message via channel", func(t *testing.T) {
		drainMessages(rc, ns)
		msgCh, err := cli.StartDequeuer(ctx)
		require.NoError(t, err)

		recvCnt := 0
		done := make(chan struct{})

		msgCount := 10

		// Consumer
		go func(msgCh <-chan *client.Message) {
			for msg := range msgCh {
				require.Equal(t, 1, msg.DequeueCount)
				t.Logf("Dequeued Message ID: %s", msg.ID)
				recvCnt++

				if recvCnt == msgCount {
					done <- struct{}{}
				}
			}
		}(msgCh)

		// Producer
		for i := 0; i < msgCount; i++ {
			msg := &testQueueMessage{ID: fmt.Sprintf("%d", i), Message: fmt.Sprintf("hello world %d", i)}
			data, err := json.Marshal(msg)
			require.NoError(t, err)
			err = cli.Enqueue(ctx, &client.Message{Data: data})
			require.NoError(t, err)
		}

		<-done
		cancel()

		require.Equal(t, msgCount, recvCnt)
	})
}

func startEnvironment() (runtimeclient.Client, *envtest.Environment, error) {
	assetDir, err := getKubeAssetsDir()
	if err != nil {
		return nil, nil, err
	}

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "deploy", "Chart", "crds", "ucpd")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: assetDir,
	}

	scheme := runtime.NewScheme()

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))

	cfg, err := testEnv.Start()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize environment: %w", err)
	}

	client, err := runtimeclient.New(cfg, runtimeclient.Options{
		Scheme: scheme,
	})
	if err != nil {
		_ = testEnv.Stop()
		return nil, nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return client, testEnv, nil
}

func getKubeAssetsDir() (string, error) {
	assetsDirectory := os.Getenv("KUBEBUILDER_ASSETS")
	if assetsDirectory != "" {
		return assetsDirectory, nil
	}

	// We require one or more versions of the test assets to be installed already. This
	// will use whatever's latest of the installed versions.
	cmd := exec.Command("setup-envtest", "use", "-i", "-p", "path", "--arch", "amd64")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to call setup-envtest to find path: %w", err)
	} else {
		return out.String(), err
	}
}

func ensureNamespace(ctx context.Context, client runtimeclient.Client, namespace string) error {
	nsObject := v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	return client.Create(ctx, &nsObject, &runtimeclient.CreateOptions{})
}
