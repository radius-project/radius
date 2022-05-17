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

	"github.com/project-radius/radius/pkg/jobqueue"
)

var defaultQueue = inmemQueue{}

type element struct {
	val     *jobqueue.JobMessage
	visible bool
}

type inmemQueue struct {
	v   *list.List
	vMu sync.Mutex

	maxDequeueCount int
}

func newInMemQueue(maxDequeueCnt int) *inmemQueue {
	return &inmemQueue{
		v:               &list.List{},
		maxDequeueCount: maxDequeueCnt,
	}
}

func (q *inmemQueue) Enqueue(msg *jobqueue.JobMessage) {
	q.updateQueue()

	q.vMu.Lock()
	defer q.vMu.Unlock()

	q.v.PushBack(&element{val: msg, visible: true})
}

func (q *inmemQueue) Dequeue() *jobqueue.JobMessage {
	q.updateQueue()

	q.vMu.Lock()
	defer q.vMu.Unlock()

	for e := q.v.Front(); e != nil; e = e.Next() {
		elem := e.Value.(*element)
		if elem.visible {
			elem.val.DequeueCount = elem.val.DequeueCount + 1
			elem.val.NextVisibleTime = time.Now().Add(time.Minute * 5)
			elem.visible = false
			return elem.val
		}
	}

	return nil
}

func (q *inmemQueue) Complete(msg *jobqueue.JobMessage) error {
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

func (q *inmemQueue) updateQueue() {
	q.vMu.Lock()
	defer q.vMu.Unlock()

	for e := q.v.Front(); e != nil; e = e.Next() {
		elem, ok := e.Value.(*element)
		if !ok {
			continue
		}

		if elem.val.ExpireTime.UnixNano() >= time.Now().UnixNano() {
			q.v.Remove(e)
		}

		if elem.val.NextVisibleTime.UnixNano() >= time.Now().UnixNano() {
			elem.visible = true
		}

		if elem.val.DequeueCount >= q.maxDequeueCount {
			q.v.Remove(e)
		}
	}
}
