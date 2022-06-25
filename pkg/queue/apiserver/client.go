// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/project-radius/radius/pkg/queue/client"

	"github.com/go-logr/logr"
	v1alpha1 "github.com/project-radius/radius/pkg/queue/apiserver/api/ucp.dev/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/util/retry"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	LabelQueueName     = "ucp.dev/queuename"
	LabelNextVisibleAt = "ucp.dev/nextvisibleat"

	dequeueInterval     = 5 * time.Millisecond
	messageLockDuration = 5 * time.Minute
)

var _ client.Client = (*Client)(nil)

// Client is the queue client used for dev and test purpose.
type Client struct {
	client    runtimeclient.Client
	namespace string

	name string
}

func int64toa(i int64) string {
	return strconv.FormatInt(int64(i), 10)
}

func getTimeFromString(s string) time.Time {
	nsec, _ := strconv.ParseInt(s, 10, 64)
	return time.Unix(0, nsec)
}

// New creates the queue backed by Kubernetes API server KV store. name is unique name for each service which will consume the queue.
func New(client runtimeclient.Client, namespace string, name string) *Client {
	return &Client{client: client, namespace: namespace, name: name}
}

func (c *Client) Enqueue(ctx context.Context, msg *client.Message, options ...client.EnqueueOptions) error {
	raw, err := json.Marshal(msg.Data)
	if err != nil {
		return err
	}
	id := fmt.Sprintf("%s.%d", c.name, time.Now().UnixNano())
	now := time.Now().UTC()
	resource := &v1alpha1.OperationQueue{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: c.namespace,
			Labels: map[string]string{
				LabelNextVisibleAt: int64toa(now.UnixNano()),
				LabelQueueName:     c.name,
			},
		},
		Spec: v1alpha1.OperationQueueSpec{
			DequeueCount: 0,
			EnqueueAt:    metav1.Time{Time: time.Now().UTC()},
			ExpireAt:     metav1.Time{Time: time.Now().UTC()},
			Data:         &runtime.RawExtension{Raw: raw},
		},
	}

	err = c.client.Create(ctx, resource)
	if err != nil && !(apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)) {
		return err
	}

	return nil
}

func (c *Client) getFirstItem(ctx context.Context) (*v1alpha1.OperationQueue, error) {
	ql := &v1alpha1.OperationQueueList{}

	now := time.Now().UTC()

	selector := labels.NewSelector()
	requirement, err := labels.NewRequirement(LabelNextVisibleAt, selection.LessThan, []string{strconv.Itoa(int(now.UnixNano()))})
	if err != nil {
		return nil, err
	}
	selector = selector.Add(*requirement)

	err = c.client.List(
		ctx, ql,
		runtimeclient.InNamespace(c.namespace), runtimeclient.MatchingLabelsSelector{Selector: selector}, runtimeclient.Limit(1))
	if err != nil {
		return nil, err
	}

	if len(ql.Items) > 0 {
		return &ql.Items[0], nil
	}
	return nil, client.ErrMessageNotFound
}

func (c *Client) extendItem(ctx context.Context, item *v1alpha1.OperationQueue, duration time.Duration) (*v1alpha1.OperationQueue, error) {
	nextVisibleAt := int64toa(time.Now().UTC().Add(duration).UnixNano())
	result := &v1alpha1.OperationQueue{}

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		getErr := c.client.Get(ctx, runtimeclient.ObjectKey{Namespace: c.namespace, Name: item.Name}, result)
		if getErr != nil {
			return getErr
		}

		nsec, _ := strconv.ParseInt(result.Labels[LabelNextVisibleAt], 10, 64)
		if nsec > time.Now().UTC().UnixNano() {
			return client.ErrDeqeueudMessage
		}

		result.Labels[LabelNextVisibleAt] = nextVisibleAt
		return c.client.Update(ctx, result)
	})

	if retryErr != nil {
		return nil, retryErr
	}

	return result, nil
}

func (c *Client) Dequeue(ctx context.Context, opts ...client.DequeueOptions) (*client.Message, error) {
	item, err := c.getFirstItem(ctx)
	if err != nil {
		return nil, err
	}
	result, err := c.extendItem(ctx, item, messageLockDuration)
	if err != nil {
		return nil, err
	}

	return &client.Message{
		Metadata: client.Metadata{
			ID:            result.Name,
			DequeueCount:  result.Spec.DequeueCount,
			EnqueueAt:     result.Spec.EnqueueAt.Time,
			ExpireAt:      result.Spec.ExpireAt.Time,
			NextVisibleAt: getTimeFromString(result.Labels[LabelNextVisibleAt]),
		},
		ContentType: client.JSONContentType,
		Data:        result.Spec.Data.Raw,
	}, nil
}

func (c *Client) StartDequeuer(ctx context.Context, opts ...client.DequeueOptions) (<-chan *client.Message, error) {
	log := logr.FromContextOrDiscard(ctx)
	out := make(chan *client.Message, 1)

	go func() {
		for {
			msg, err := c.Dequeue(ctx, opts...)
			if err == nil {
				out <- msg
			}

			if err != nil && (errors.Is(err, client.ErrDeqeueudMessage) || errors.Is(err, client.ErrMessageNotFound)) {
				log.Error(err, "fails to dequeue the message")
			}

			select {
			case <-ctx.Done():
				close(out)
				return
			case <-time.After(dequeueInterval):
			}
		}
	}()

	return out, nil
}

func (c *Client) FinishMessage(ctx context.Context, msg *client.Message) error {
	result := &v1alpha1.OperationQueue{}
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		getErr := c.client.Get(ctx, runtimeclient.ObjectKey{Namespace: c.namespace, Name: msg.ID}, result)
		if getErr != nil {
			return getErr
		}

		options := &runtimeclient.DeleteOptions{
			Preconditions: &metav1.Preconditions{
				UID:             &result.UID,
				ResourceVersion: &result.ResourceVersion,
			},
		}
		return c.client.Delete(ctx, result, options)
	})

	return retryErr
}

func (c *Client) ExtendMessage(ctx context.Context, msg *client.Message) error {
	result := &v1alpha1.OperationQueue{}
	getErr := c.client.Get(ctx, runtimeclient.ObjectKey{Namespace: c.namespace, Name: msg.ID}, result)
	if getErr != nil {
		return getErr
	}
	_, err := c.extendItem(ctx, result, messageLockDuration)
	return err
}
