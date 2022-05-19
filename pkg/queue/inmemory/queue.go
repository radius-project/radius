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

type element struct {
	val     *queue.Message
	visible bool
}

var defaultQueue = NewInMemQueue()

// InmemQueue implements in-memory queue for dev/test
type InmemQueue struct {
	v   *list.List
	vMu sync.Mutex
}

func NewInMemQueue() *InmemQueue {
	return &InmemQueue{
		v: &list.List{},
	}
}

func (q *InmemQueue) Enqueue(msg *queue.Message) {
	q.updateQueue()

	q.vMu.Lock()
	defer q.vMu.Unlock()

	msg.Metadata.ID = uuid.NewString()
	msg.Metadata.DequeueCount = 0
	msg.Metadata.EnqueueAt = time.Now().UTC()
	msg.Metadata.ExpireAt = time.Now().UTC().Add(messageExpireDuration)

	q.v.PushBack(&element{val: msg, visible: true})
}

func (q *InmemQueue) Dequeue() *queue.Message {
	q.updateQueue()

	q.vMu.Lock()
	defer q.vMu.Unlock()

	for e := q.v.Front(); e != nil; e = e.Next() {
		elem := e.Value.(*element)
		if elem.visible {
			elem.val.DequeueCount++
			elem.val.NextVisibleAt = time.Now().Add(messageLockDuration)
			elem.visible = false
			return elem.val
		}
	}

	return nil
}

func (q *InmemQueue) Complete(msg *queue.Message) error {
	q.vMu.Lock()
	defer q.vMu.Unlock()

	for e := q.v.Front(); e != nil; e = e.Next() {
		elem := e.Value.(*element)
		if elem.val.ID == msg.ID {
			q.v.Remove(e)
			return nil
		}
	}
	return errors.New("id not found")
}

func (q *InmemQueue) updateQueue() {
	q.vMu.Lock()
	defer q.vMu.Unlock()

	for e := q.v.Front(); e != nil; e = e.Next() {
		elem, ok := e.Value.(*element)
		if !ok {
			continue
		}

		now := time.Now().UTC()
		if elem.val.ExpireAt.UnixNano() <= now.UnixNano() {
			q.v.Remove(e)
		} else if elem.val.NextVisibleAt.UnixNano() <= now.UnixNano() {
			elem.visible = true
		}
	}
}
