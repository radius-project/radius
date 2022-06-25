// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package apiserverstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/project-radius/radius/pkg/queue"
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
	LabelNextVisibleAt = "ucp.dev/nextvisibleat"
	LabelExpireAt      = "ucp.dev/expireat"

	dequeueInterval     = 5 * time.Millisecond
	messageLockDuration = 5 * time.Minute
)

var _ queue.Client = (*Client)(nil)

// Client is the queue client used for dev and test purpose.
type Client struct {
	client    runtimeclient.Client
	namespace string
}

// New creates the in-memory queue Client instance. Client will use the default global queue if queue is nil.
func New(client runtimeclient.Client, namespace string) *Client {
	return &Client{client: client, namespace: namespace}
}

func Int64toa(i int64) string {
	return strconv.FormatInt(int64(i), 10)
}

// Enqueue enqueues message to the in-memory queue.
func (c *Client) Enqueue(ctx context.Context, msg *queue.Message, options ...queue.EnqueueOptions) error {
	raw, err := json.Marshal(msg.Data)
	if err != nil {
		return err
	}
	resourceName := fmt.Sprintf("operationqueue.applicationscore.%d", time.Now().UnixMicro())
	now := time.Now().UTC()
	resource := &v1alpha1.OperationQueue{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: c.namespace,
			Labels: map[string]string{
				LabelExpireAt:      Int64toa(now.Add(time.Duration(5) * time.Hour).UnixNano()),
				LabelNextVisibleAt: Int64toa(now.UnixNano()),
			},
		},
		Spec: v1alpha1.OperationQueueSpec{
			DequeueCount: 0,
			EnqueueAt:    metav1.Time{time.Now().UTC()},
			ExpireAt:     metav1.Time{time.Now().UTC()},
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
	return nil, errors.New("not found")
}

func (c *Client) extendItem(ctx context.Context, item *v1alpha1.OperationQueue) (*v1alpha1.OperationQueue, error) {
	nextVisibleAt := Int64toa(time.Now().UTC().Add(messageLockDuration).UnixNano())
	result := &v1alpha1.OperationQueue{}

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		getErr := c.client.Get(ctx, runtimeclient.ObjectKey{Namespace: c.namespace, Name: item.Name}, result)
		if getErr != nil {
			return getErr
		}

		result.Labels[LabelNextVisibleAt] = nextVisibleAt

		updateErr := c.client.Update(ctx, result)
		return updateErr
	})

	if retryErr != nil {
		return nil, retryErr
	}

	return result, nil
}

func (c *Client) dequeueItem(ctx context.Context) (*v1alpha1.OperationQueue, error) {
	item, err := c.getFirstItem(ctx)
	if err != nil {
		return nil, err
	}

	return c.extendItem(ctx, item)
}

// Dequeue dequeues message from the in-memory queue.
func (c *Client) Dequeue(ctx context.Context, options ...queue.DequeueOptions) (<-chan *queue.Message, error) {
	out := make(chan *queue.Message, 1)

	go func() {
		for {
			q, err := c.dequeueItem(ctx)
			if err != nil || q != nil {
				msg := &queue.Message{
					Metadata: queue.Metadata{
						ID:            q.Name,
						DequeueCount:  q.Spec.DequeueCount,
						EnqueueAt:     q.Spec.EnqueueAt.Time,
						ExpireAt:      q.Spec.ExpireAt.Time,
						NextVisibleAt: time.Now().UTC().Add(messageLockDuration),
					},
					Data: q.Spec.Data.Raw,
				}

				msg.WithFinish(func(err error) error {
					result := &v1alpha1.OperationQueue{}
					getErr := c.client.Get(context.TODO(), runtimeclient.ObjectKey{Namespace: c.namespace, Name: q.Name}, result)
					if getErr != nil {
						return getErr
					}

					options := &runtimeclient.DeleteOptions{
						Preconditions: &metav1.Preconditions{
							UID:             &q.UID,
							ResourceVersion: &q.ResourceVersion,
						},
					}
					return c.client.Delete(context.TODO(), result, options)
				})

				msg.WithExtend(func() error {
					result := &v1alpha1.OperationQueue{}
					getErr := c.client.Get(context.TODO(), runtimeclient.ObjectKey{Namespace: c.namespace, Name: q.Name}, result)
					if getErr != nil {
						return getErr
					}
					_, err := c.extendItem(context.TODO(), result)
					return err
				})
				out <- msg
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
