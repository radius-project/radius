// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package inmemory

import (
	"container/list"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/ucp/queue/client"
)

var (
	messageLockDuration   = 5 * time.Minute
	messageExpireDuration = 24 * time.Hour

	defaultQueue = NewInMemQueue(messageLockDuration)
)

type element struct {
	val *client.Message

	visible bool
}

// InmemQueue implements in-memory queue for dev/test
type InmemQueue struct {
	v   *list.List
	vMu sync.Mutex

	lockDuration time.Duration
}

func NewInMemQueue(lockDuration time.Duration) *InmemQueue {
	return &InmemQueue{
		v:            &list.List{},
		lockDuration: lockDuration,
	}
}

func (q *InmemQueue) Len() int {
	q.vMu.Lock()
	defer q.vMu.Unlock()
	return q.v.Len()
}

func (q *InmemQueue) DeleteAll() {
	q.vMu.Lock()
	defer q.vMu.Unlock()
	_ = q.v.Init()
}

func (q *InmemQueue) Enqueue(msg *client.Message) {
	q.updateQueue()

	q.vMu.Lock()
	defer q.vMu.Unlock()

	msg.Metadata.ID = uuid.NewString()
	msg.Metadata.DequeueCount = 0
	msg.Metadata.EnqueueAt = time.Now().UTC()
	msg.Metadata.ExpireAt = time.Now().UTC().Add(messageExpireDuration)

	q.v.PushBack(&element{val: msg, visible: true})
	fmt.Println("inMemory - client - Enqueue - should be enqueued now")
}

func (q *InmemQueue) Dequeue() *client.Message {
	q.updateQueue()

	var found *client.Message

	q.elementRange(func(e *list.Element, elem *element) bool {
		if elem.visible {
			elem.val.DequeueCount++
			// FIXME: There might be a small delay between setting the nextVisibleAt and actual finishing of the processing
			// nextVisibleAt = time.Now().Add(q.lockDuration) + (time it takes to do the changes as in the function and worker)
			elem.val.NextVisibleAt = time.Now().Add(q.lockDuration)
			elem.visible = false
			found = elem.val
			return true
		}
		return false
	})

	return found
}

func (q *InmemQueue) Complete(msg *client.Message) error {
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
		return client.ErrInvalidMessage
	}

	return nil
}

func (q *InmemQueue) Extend(msg *client.Message) error {
	found := false
	now := time.Now()
	// Would it be possible to just update the nextVisibleAt of this specific message without iterating through the whole list?
	// By using pointers?
	q.elementRange(func(e *list.Element, elem *element) bool {
		if elem.val.ID == msg.ID {
			// Why do we set the visibility of this message to false?
			if elem.val.NextVisibleAt.UnixNano() < now.UnixNano() || elem.val.DequeueCount != msg.DequeueCount {
				elem.visible = false
				return false
			} else {
				found = true
				elem.val.NextVisibleAt = elem.val.NextVisibleAt.Add(q.lockDuration)
				msg.NextVisibleAt = elem.val.NextVisibleAt
				return true
			}
		}
		return false
	})

	if !found {
		return client.ErrInvalidMessage
	}

	return nil
}

func (q *InmemQueue) updateQueue() {
	// FIXME: There might be a small delay between setting the nextVisibleAt and actual finishing of the processing
	// nextVisibleAt = time.Now().Add(q.lockDuration) + (time it takes to do the changes as in the function and worker)
	q.elementRange(func(e *list.Element, elem *element) bool {
		now := time.Now().UTC()
		if elem.val.ExpireAt.UnixNano() < now.UnixNano() {
			q.v.Remove(e)
		} else if elem.val.NextVisibleAt.UnixNano() < now.UnixNano() {
			elem.visible = true
		}
		return false
	})
}

func (q *InmemQueue) elementRange(fn func(*list.Element, *element) bool) {
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
