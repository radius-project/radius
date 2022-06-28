// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package apiserver

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/project-radius/radius/pkg/queue/client"

	"github.com/go-logr/logr"
	v1alpha1 "github.com/project-radius/radius/pkg/queue/apiserver/api/ucp.dev/v1alpha1"
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
	return fmt.Sprintf("%s.%d%x", c.opts.Name, time.Now().Unix(), b), nil
}

func (c *Client) Enqueue(ctx context.Context, msg *client.Message, options ...client.EnqueueOptions) error {
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
// determine whether the message is leased by another client by checking if `NextVisibleAt``
// value is less than `now`.
func (c *Client) getQueueMessage(ctx context.Context, now time.Time) (*v1alpha1.QueueMessage, error) {
	ql := &v1alpha1.QueueMessageList{}

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

func (c *Client) extendItem(ctx context.Context, item *v1alpha1.QueueMessage, afterTime time.Time, duration time.Duration) (*v1alpha1.QueueMessage, error) {
	nextVisibleAt := afterTime.Add(duration).UnixNano()
	result := &v1alpha1.QueueMessage{}

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

		// Update supports optimistic concurrency. Retry until conflict is solved.
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
		return errors.Is(err, client.ErrDeqeueudMessage)
	}

	now := time.Now()

	// Retry only if the other instance or client already dequeue the message.
	retryErr := retry.OnError(retry.DefaultRetry, DequeuedMessageError, func() error {
		// Since multiple client can get the same message, we tried to get the next queue
		// message whenever extendItem is failed.
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
	now := time.Now()
	result := &v1alpha1.QueueMessage{}
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
