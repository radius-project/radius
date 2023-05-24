/*
Copyright 2023 The Radius Authors.
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

package inmemory

import (
	"container/list"
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
}

func (q *InmemQueue) Dequeue() *client.Message {
	q.updateQueue()

	var found *client.Message

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
	q.elementRange(func(e *list.Element, elem *element) bool {
		if elem.val.ID == msg.ID {
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
