// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package apiserver

import (
	"context"
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

	dequeueInterval = time.Duration(5) * time.Millisecond

	defaultMessageLockDuration = time.Duration(5) * time.Minute
	defaultExpiryDuration      = time.Duration(10) * time.Hour
)

var _ client.Client = (*Client)(nil)

// Client is the queue client used for dev and test purpose.
type Client struct {
	client runtimeclient.Client

	opts Options
}

// Options is the options to create apiserver queue client.
type Options struct {
	// Name represents the name of queue.
	Name string
	// Namespace represents the namespace of kubernetes cluster.
	Namespace string

	// MessageLockDuration represents the duration of message lock.
	MessageLockDuration time.Duration
	// ExpiryDuration represents the duration of the expiry.
	ExpiryDuration time.Duration
}

func mustParseInt64(s string) int64 {
	nsec, _ := strconv.ParseInt(s, 10, 64)
	return nsec
}

func int64toa(i int64) string {
	return strconv.FormatInt(int64(i), 10)
}

func getTimeFromString(s string) time.Time {
	nsec := mustParseInt64(s)
	return time.Unix(0, nsec)
}

func copyMessage(msg *client.Message, queueMessage *v1alpha1.OperationQueue) {
	msg.Metadata = client.Metadata{
		ID:            queueMessage.Name,
		DequeueCount:  queueMessage.Spec.DequeueCount,
		EnqueueAt:     queueMessage.Spec.EnqueueAt.Time,
		ExpireAt:      queueMessage.Spec.ExpireAt.Time,
		NextVisibleAt: getTimeFromString(queueMessage.Labels[LabelNextVisibleAt]),
	}
	msg.ContentType = client.JSONContentType
	msg.Data = make([]byte, len(queueMessage.Spec.Data.Raw))
	copy(msg.Data, queueMessage.Spec.Data.Raw)
}

// New creates the queue backed by Kubernetes API server KV store. name is unique name for each service which will consume the queue.
func New(client runtimeclient.Client, options Options) *Client {
	if options.Name == "" || options.Namespace == "" {
		return nil
	}

	if options.MessageLockDuration == time.Duration(0) {
		options.MessageLockDuration = defaultMessageLockDuration
	}

	if options.ExpiryDuration == time.Duration(0) {
		options.ExpiryDuration = defaultExpiryDuration
	}

	return &Client{client: client, opts: options}
}

func (c *Client) Enqueue(ctx context.Context, msg *client.Message, options ...client.EnqueueOptions) error {
	now := time.Now()
	id := fmt.Sprintf("%s.%d", c.opts.Name, now.UnixNano())
	resource := &v1alpha1.OperationQueue{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: c.opts.Namespace,
			Labels: map[string]string{
				LabelNextVisibleAt: int64toa(now.UnixNano()),
				LabelQueueName:     c.opts.Name,
			},
		},
		Spec: v1alpha1.OperationQueueSpec{
			DequeueCount: 0,
			EnqueueAt:    metav1.Time{Time: now.UTC()},
			ExpireAt:     metav1.Time{Time: now.Add(c.opts.ExpiryDuration).UTC()},
			ContentType:  client.JSONContentType, // RawExtension supports only JSON seralized data
			Data:         &runtime.RawExtension{Raw: msg.Data},
		},
	}

	err := c.client.Create(ctx, resource)
	if err != nil && !(apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)) {
		return err
	}

	return nil
}

func newMessageLabelSelector(now time.Time, name string) (labels.Selector, error) {
	selector := labels.NewSelector()

	nextVisibleLabel, err := labels.NewRequirement(LabelNextVisibleAt, selection.LessThan, []string{int64toa(now.UnixNano())})
	if err != nil {
		return nil, err
	}
	selector = selector.Add(*nextVisibleLabel)

	nameLabel, err := labels.NewRequirement(LabelQueueName, selection.Equals, []string{name})
	if err != nil {
		return nil, err
	}

	return selector.Add(*nameLabel), nil
}

// getQueueMessage fetches the first item which is the message in the current queue. We can
// determine whether the message is leased by another client by checking if `NextVisibleAt``
// value is less than `now`.
func (c *Client) getQueueMessage(ctx context.Context, now time.Time) (*v1alpha1.OperationQueue, error) {
	ql := &v1alpha1.OperationQueueList{}

	selector, err := newMessageLabelSelector(now, c.opts.Name)
	if err != nil {
		return nil, err
	}

	err = c.client.List(
		ctx, ql,
		runtimeclient.InNamespace(c.opts.Namespace), runtimeclient.MatchingLabelsSelector{Selector: selector}, runtimeclient.Limit(1))
	if err != nil {
		return nil, err
	}

	if len(ql.Items) > 0 {
		return &ql.Items[0], nil
	}

	return nil, client.ErrMessageNotFound
}

func (c *Client) extendItem(ctx context.Context, item *v1alpha1.OperationQueue, afterTime time.Time, duration time.Duration) (*v1alpha1.OperationQueue, error) {
	nextVisibleAt := afterTime.Add(duration).UnixNano()
	result := &v1alpha1.OperationQueue{}

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		getErr := c.client.Get(ctx, runtimeclient.ObjectKey{Namespace: c.opts.Namespace, Name: item.Name}, result)
		if getErr != nil {
			return getErr
		}

		// The unix time of NextVisibleAt label in item should be less than now.
		// If it is greater than now, then the other instance or client already dequeued the message.
		nsec := mustParseInt64(result.Labels[LabelNextVisibleAt])
		if nsec >= nextVisibleAt {
			return client.ErrDeqeueudMessage
		}

		result.Labels[LabelNextVisibleAt] = int64toa(nextVisibleAt)
		result.Spec.DequeueCount += 1
		return c.client.Update(ctx, result)
	})

	if retryErr != nil {
		return nil, retryErr
	}

	return result, nil
}

func (c *Client) Dequeue(ctx context.Context, opts ...client.DequeueOptions) (*client.Message, error) {
	var result *v1alpha1.OperationQueue

	DequeuedMessageError := func(err error) bool {
		return errors.Is(err, client.ErrDeqeueudMessage)
	}

	now := time.Now()

	// Retry only if the other instance or client already dequeue the message.
	retryErr := retry.OnError(retry.DefaultRetry, DequeuedMessageError, func() error {
		item, err := c.getQueueMessage(ctx, now)
		if err != nil {
			return err
		}
		result, err = c.extendItem(ctx, item, now, c.opts.MessageLockDuration)
		if err != nil {
			return err
		}
		return nil
	})

	if retryErr != nil {
		return nil, retryErr
	}

	msg := &client.Message{}
	copyMessage(msg, result)

	return msg, nil
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

			if err != nil && !errors.Is(err, client.ErrMessageNotFound) {
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
		getErr := c.client.Get(ctx, runtimeclient.ObjectKey{Namespace: c.opts.Namespace, Name: msg.ID}, result)
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
	now := time.Now()
	result := &v1alpha1.OperationQueue{}
	getErr := c.client.Get(ctx, runtimeclient.ObjectKey{Namespace: c.opts.Namespace, Name: msg.ID}, result)
	if getErr != nil {
		return getErr
	}

	// Check if the message is already requeued.
	nsec := mustParseInt64(result.Labels[LabelNextVisibleAt])
	if nsec < now.UnixNano() {
		return client.ErrInvalidMessage
	}

	result, err := c.extendItem(ctx, result, now, c.opts.MessageLockDuration)
	copyMessage(msg, result)
	return err
}
