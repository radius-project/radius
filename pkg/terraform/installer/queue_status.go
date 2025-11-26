package installer

import "context"

// updateQueueInfo loads status, ensures queue info exists, applies the update, and persists.
// This is best-effort and returns without error when status can't be loaded or saved.
func updateQueueInfo(ctx context.Context, store StatusStore, update func(*QueueInfo)) {
	status, err := store.Get(ctx)
	if err != nil {
		return
	}
	if status.Queue == nil {
		status.Queue = &QueueInfo{}
	}
	update(status.Queue)
	_ = store.Put(ctx, status)
}
