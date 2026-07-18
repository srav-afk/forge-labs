package routing

import "sync/atomic"

type SnapshotHolder struct {
	ptr atomic.Pointer[RoutingSnapshot]
}

func NewSnapshotHolder() *SnapshotHolder {
	return &SnapshotHolder{}
}

func (h *SnapshotHolder) Load() *RoutingSnapshot {
	return h.ptr.Load()
}

func (h *SnapshotHolder) Store(s *RoutingSnapshot) {
	if s == nil {
		return
	}
	h.ptr.Store(s)
}

func (h *SnapshotHolder) StoreIfNewer(s *RoutingSnapshot) bool {
	if s == nil {
		return false
	}
	for {
		cur := h.ptr.Load()
		if cur != nil && s.Epoch <= cur.Epoch {
			return false
		}
		if h.ptr.CompareAndSwap(cur, s) {
			return true
		}
	}
}
