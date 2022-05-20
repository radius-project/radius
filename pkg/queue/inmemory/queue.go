// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package inmemory

import (
	"container/list"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/queue"
)

var (
	messageLockDuration   = 5 * time.Minute
	messageExpireDuration = 24 * time.Hour
)

var ErrAlreadyCompletedMessage = errors.New("message has already completed")

type element struct {
	val *queue.Message

	visible bool
}

var defaultQueue = newInMemQueue(messageLockDuration)

// inmemQueue implements in-memory queue for dev/test
type inmemQueue struct {
	v   *list.List
	vMu sync.Mutex

	lockDuration time.Duration
}

func newInMemQueue(lockDuration time.Duration) *inmemQueue {
	return &inmemQueue{
		v:            &list.List{},
		lockDuration: lockDuration,
	}
}

func (q *inmemQueue) Enqueue(msg *queue.Message) {
	q.updateQueue()

	q.vMu.Lock()
	defer q.vMu.Unlock()

	msg.Metadata.ID = uuid.NewString()
	msg.Metadata.DequeueCount = 0
	msg.Metadata.EnqueueAt = time.Now().UTC()
	msg.Metadata.ExpireAt = time.Now().UTC().Add(messageExpireDuration)

	q.v.PushBack(&element{val: msg, visible: true})
}

func (q *inmemQueue) Dequeue() *queue.Message {
	q.updateQueue()

	var found *queue.Message

	q.elementRange(func(e *list.Element, elem *element) bool {
		if elem.visible {
			elem.val.DequeueCount++
			elem.val.NextVisibleAt = time.Now().Add(q.lockDuration)
			elem.visible = false
			found = elem.val
			return true
		}
		return false
	})

	return found
}

func (q *inmemQueue) Complete(msg *queue.Message) error {
	found := false
	q.elementRange(func(e *list.Element, elem *element) bool {
		if elem.val.ID == msg.ID {
			found = true
			q.v.Remove(e)
			return true
		}
		return false
	})

	if !found {
		return ErrAlreadyCompletedMessage
	}

	return nil
}

func (q *inmemQueue) Extend(msg *queue.Message) error {
	found := false
	q.elementRange(func(e *list.Element, elem *element) bool {
		if elem.val.ID == msg.ID {
			found = true
			elem.val.NextVisibleAt.Add(q.lockDuration)
			return true
		}
		return false
	})

	if !found {
		return ErrAlreadyCompletedMessage
	}

	return nil
}

func (q *inmemQueue) updateQueue() {
	q.elementRange(func(e *list.Element, elem *element) bool {
		now := time.Now().UTC()
		if elem.val.ExpireAt.UnixNano() <= now.UnixNano() {
			q.v.Remove(e)
		} else if elem.val.NextVisibleAt.UnixNano() <= now.UnixNano() {
			elem.visible = true
		}
		return false
	})
}

func (q *inmemQueue) elementRange(fn func(*list.Element, *element) bool) {
	q.vMu.Lock()
	defer q.vMu.Unlock()

	for e := q.v.Front(); e != nil; e = e.Next() {
		elem := e.Value.(*element)
		done := fn(e, elem)
		if done {
			return
		}
	}
}
