// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// Package apiserver is Kuberentes CRD based queue implementation. To implement the distributed queue using CRD,
// we define QueueMessage Custom Resource and leverage Kuberentes optimistic concurrency.
//
// Kubernetes Concurrency control and consistency:
// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#concurrency-control-and-consistency
//
// We need four operations for the queue:
//
// 1. Enqueue: Creates QueueMessage CR to mimic the queue enqueue operation
// 2. Dequeue: Implements message lease and lock (which is invisible for the other client once message leased).
//             After message lock duration, the leased message must be re-queued.
// 3. FinishMessage: Deletes the leased message CR to remove message in the queue completely if the message is not re-queued.
// 4. ExtendMessage: Extends the leased message to postpone the re-queue operation.
//
// To create new QueueMessage resource, we generate the below unique id to avoid the conflict.
//
//         applications.core.1656452659.70a6f0f8003943a6abe3319c5a4f1b9d
//         ----------------- ---------- --------------------------------
//              name         epoch time         random number
//
// We maintain NextVisibleAt in CR label to implement message `lease` operation. NextVisibleAt is stored as CR label
// `ucp.dev/nextvisibleat` and represents the time when the message is visible for the other clients. Thanks to Kubernetes
// Resource List API, we can use `<` and `>` operation to query resource items by label. Therefore, when client calls
// Dequeue() API, the API queries the first item of which `ucp.dev/nextvisibleat` label value is less than current epoch
// time. It will get the item which was re-queued or was not dequeued message. Then it will increase DequeueCount and update
// `ucp.dev/nextvisibleat` timestamp (current time + 5 mins(default)) and try to update the item. If the other clients already
// fetched message, then Update() API would return conflict error by optimistic concurrency and retry to query new message
// and update it again until the conflict is resolved.
//
// How to handle clock skew - Dequeue operation in this implementation relies on system clock. If Client A and B
// run in the different physical node A and B respectively, client B in node B could deqeueue the same message in the clock skew
// window before client A in node A leased the message. When Client A calls ExtendMessage, it always fetches message with id first
// and checks if its dequeue count matches the dequeue count of Message Client A currently have. We are using DequeueCount as a
// revision number of message here. If it is mismatched, it means that Client B already leased the message. In this case,
// ExtendMessage returns ErrDequeuedMessage to prevent Client A from extending lock.

package apiserver

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/project-radius/radius/pkg/ucp/queue/client"

	v1alpha1 "github.com/project-radius/radius/pkg/ucp/store/apiserverstore/api/ucp.dev/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/util/retry"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// LabelQueueName is the label representing queue name.
	LabelQueueName = "ucp.dev/queuename"
	// LabelNextVisibleAt is the label representing the time when message is visible in the queue or requeued.
	LabelNextVisibleAt = "ucp.dev/nextvisibleat"

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

func copyMessage(msg *client.Message, queueMessage *v1alpha1.QueueMessage) {
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
func New(client runtimeclient.Client, options Options) (*Client, error) {
	if options.Name == "" || options.Namespace == "" {
		return nil, errors.New("Name and Namespace are required")
	}

	if options.MessageLockDuration == time.Duration(0) {
		options.MessageLockDuration = defaultMessageLockDuration
	}

	if options.ExpiryDuration == time.Duration(0) {
		options.ExpiryDuration = defaultExpiryDuration
	}

	return &Client{client: client, opts: options}, nil
}

func (c *Client) generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.%10d.%32x", c.opts.Name, time.Now().Unix(), b), nil
}

func (c *Client) Enqueue(ctx context.Context, msg *client.Message, options ...client.EnqueueOptions) error {
	if msg == nil || msg.Data == nil || len(msg.Data) == 0 {
		return client.ErrEmptyMessage
	}

	if msg.ContentType != client.JSONContentType {
		return client.ErrUnsupportedContentType
	}

	now := time.Now()
	id, err := c.generateID()
	if err != nil {
		return err
	}

	resource := &v1alpha1.QueueMessage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: c.opts.Namespace,
			Labels: map[string]string{
				LabelNextVisibleAt: int64toa(now.UnixNano()),
				LabelQueueName:     c.opts.Name,
			},
		},
		Spec: v1alpha1.QueueMessageSpec{
			DequeueCount: 0,
			EnqueueAt:    metav1.Time{Time: now.UTC()},
			ExpireAt:     metav1.Time{Time: now.Add(c.opts.ExpiryDuration).UTC()},
			ContentType:  client.JSONContentType, // RawExtension supports only JSON seralized data
			Data:         &runtime.RawExtension{Raw: msg.Data},
		},
	}

	return c.client.Create(ctx, resource)
}

func newMessageLabelSelector(now time.Time, name string) (labels.Selector, error) {
	selector := labels.NewSelector()

	// To determine whether the message is currently leased by client or not, it uses NextVisibleAt timestamp.
	// For example, if NextVisibleAt time is less than current time, the message has been requeued or never
	// leased by the client. We use Label to compare the timestamp since List() supports GreaterThan and
	// LessThan Operator for Label.
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
// determine whether the message is leased by another client by checking if `NextVisibleAt“
// value is less than `now`.
func (c *Client) getQueueMessage(ctx context.Context, now time.Time) (*v1alpha1.QueueMessage, error) {
	ql := &v1alpha1.QueueMessageList{}

	selector, err := newMessageLabelSelector(now, c.opts.Name)
	if err != nil {
		return nil, err
	}

	err = c.client.List(
		ctx, ql,
		runtimeclient.InNamespace(c.opts.Namespace),
		runtimeclient.MatchingLabelsSelector{Selector: selector},
		runtimeclient.Limit(1))
	if err != nil {
		return nil, err
	}

	if len(ql.Items) > 0 {
		return &ql.Items[0], nil
	}

	return nil, client.ErrMessageNotFound
}

// extendItem udpates LabelNextVisibleAt to extend the lease time of message. Dequeue and ExtendMessage
// use this function. Dequeue Operation updates DequeueCount and LabelNextVisibleAt whereas ExtendMessage
// updates only LabelNextVisibleAt -- handled by isDequeue flag. We can use DequeueCount as a revision
// number of the message so this func could easily catch the clock skew issue.
func (c *Client) extendItem(ctx context.Context, id string, expectedDequeueCount int, afterTime time.Time, duration time.Duration, isDequeue bool) (*v1alpha1.QueueMessage, error) {
	nextVisibleAt := afterTime.Add(duration).UnixNano()
	result := &v1alpha1.QueueMessage{}

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		getErr := c.client.Get(ctx, runtimeclient.ObjectKey{Namespace: c.opts.Namespace, Name: id}, result)
		if getErr != nil {
			return getErr
		}

		// Ensure that it doesn't extend the message that another client leased. DequeueCount must be
		// mismatched if another client leased this message. This can happen by clock skew Because
		// Dequeue() operation relies on system clock.
		if result.Spec.DequeueCount != expectedDequeueCount {
			return client.ErrDequeuedMessage
		}

		nsec := mustParseInt64(result.Labels[LabelNextVisibleAt])

		// Check if the message is already requeued. This condition is required for ExtendMessage because we cannot extend the message which was requeued.
		if !isDequeue && nsec < afterTime.UnixNano() {
			return client.ErrInvalidMessage
		}

		result.Labels[LabelNextVisibleAt] = int64toa(nextVisibleAt)
		if isDequeue {
			result.Spec.DequeueCount += 1
		}

		// Update supports optimistic concurrency. Retry until conflict is resolved.
		// Reference: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#concurrency-control-and-consistency
		return c.client.Update(ctx, result)
	})

	if retryErr != nil {
		return nil, retryErr
	}

	return result, nil
}

func (c *Client) Dequeue(ctx context.Context, opts ...client.DequeueOptions) (*client.Message, error) {
	var result *v1alpha1.QueueMessage

	DequeuedMessageError := func(err error) bool {
		return errors.Is(err, client.ErrDequeuedMessage)
	}

	now := time.Now()

	// Retry only if the other instances or clients already dequeue the message.
	retryErr := retry.OnError(retry.DefaultRetry, DequeuedMessageError, func() error {
		// Since multiple clients can get the same message, it tries to get the next queue
		// message whenever extendItem is failed.
		item, err := c.getQueueMessage(ctx, now)
		if err != nil {
			return err
		}
		result, err = c.extendItem(ctx, item.Name, item.Spec.DequeueCount, now, c.opts.MessageLockDuration, true)
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

func (c *Client) FinishMessage(ctx context.Context, msg *client.Message) error {
	if msg == nil {
		return client.ErrEmptyMessage
	}

	result := &v1alpha1.QueueMessage{}
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
	if msg == nil {
		return client.ErrEmptyMessage
	}

	now := time.Now()
	result, err := c.extendItem(ctx, msg.ID, msg.DequeueCount, now, c.opts.MessageLockDuration, false)
	if err != nil {
		return err
	}

	copyMessage(msg, result)
	return nil
}
