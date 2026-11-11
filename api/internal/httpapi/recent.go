package httpapi

import "sync"

type recent[V any] struct {
	mu    sync.Mutex
	max   int
	order []string
	items map[string]V
}

func newRecent[V any](max int) *recent[V] {
	return &recent[V]{max: max, items: make(map[string]V, max)}
}

func (r *recent[V]) get(key string) (V, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.items[key]
	return v, ok
}

func (r *recent[V]) put(key string, v V) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[key]; !ok {
		r.order = append(r.order, key)
	}
	r.items[key] = v
	if len(r.order) > r.max {
		delete(r.items, r.order[0])
		r.order = r.order[1:]
	}
}
