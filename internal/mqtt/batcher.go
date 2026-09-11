package mqtt

import (
	"cmp"
	"slices"
	"sync"
	"time"
)

// maxPending bounds how many watched-topic messages may wait for the next
// flush. It is sized against the flush interval (a UI cannot usefully render
// more than this per 50 ms tick), not against history depth.
const maxPending = 2000

// ring is a fixed-capacity FIFO of messages.
type ring struct {
	buf  []Message
	head int
	n    int
}

func newRing(cap int) *ring { return &ring{buf: make([]Message, cap)} }

func (r *ring) push(m Message) (evicted bool) {
	if r.n == len(r.buf) {
		r.buf[r.head] = m
		r.head = (r.head + 1) % len(r.buf)
		return true
	}
	r.buf[(r.head+r.n)%len(r.buf)] = m
	r.n++
	return false
}

func (r *ring) newestFirst(limit int) []Message {
	if limit <= 0 || limit > r.n {
		limit = r.n
	}
	out := make([]Message, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, r.buf[(r.head+r.n-1-i)%len(r.buf)])
	}
	return out
}

// drain returns everything oldest-first and empties the ring.
func (r *ring) drain() []Message {
	if r.n == 0 {
		return nil
	}
	out := make([]Message, 0, r.n)
	for i := 0; i < r.n; i++ {
		out = append(out, r.buf[(r.head+i)%len(r.buf)])
	}
	r.head, r.n = 0, 0
	return out
}

// topicEntry is everything the batcher knows about one topic, in one map
// slot so the hot path does a single lookup per message.
type topicEntry struct {
	history *ring
	stats   TopicStats
	watched bool
	dirty   bool
}

// batcher sits between a raw client and the Sink. It absorbs per-message
// callbacks and flushes on a ticker so the UI receives at most ~20 events/s
// per session regardless of broker throughput. It owns the ring buffers and
// the topic index, which is why history reads go through it.
//
// Only messages on watched topics are queued for the UI; everything else is
// recorded in history and the topic index only. This keeps bridge traffic
// proportional to what is on screen, not to broker throughput.
//
// Locking: mu guards the data and is held only briefly by Push. emitMu is
// taken around "assign Seq, detach, hand to sink" so Seq order equals emit
// order without blocking Push while the sink serialises a batch.
// Lock order is emitMu then mu.
type batcher struct {
	profileID string
	sink      Sink
	perTopic  int
	nextSeq   func() uint64

	emitMu  sync.Mutex
	mu      sync.Mutex
	nextID  uint64
	topics  map[string]*topicEntry
	dirty   []*topicEntry
	pending *ring
	dropped uint64

	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

func newBatcher(profileID string, sink Sink, perTopic int, interval time.Duration, nextSeq func() uint64) *batcher {
	b := &batcher{
		profileID: profileID, sink: sink, perTopic: perTopic, nextSeq: nextSeq,
		topics: map[string]*topicEntry{}, pending: newRing(maxPending),
		stop: make(chan struct{}), done: make(chan struct{}),
	}
	go b.loop(interval)
	return b
}

func (b *batcher) loop(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	defer close(b.done)
	for {
		select {
		case <-t.C:
			b.Flush()
		case <-b.stop:
			b.Flush()
			return
		}
	}
}

// Close stops the ticker after a final flush. Idempotent.
func (b *batcher) Close() {
	b.stopOnce.Do(func() { close(b.stop) })
	<-b.done
}

// Push records m. Called from the client's receive goroutine.
func (b *batcher) Push(m Message) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	m.ID = b.nextID
	m.ProfileID = b.profileID

	e, ok := b.topics[m.Topic]
	if !ok {
		e = &topicEntry{history: newRing(b.perTopic), stats: TopicStats{Path: m.Topic}}
		b.topics[m.Topic] = e
	}
	e.history.push(m)
	e.stats.observe(m)
	if !e.dirty {
		e.dirty = true
		b.dirty = append(b.dirty, e)
	}
	if e.watched && b.pending.push(m) {
		b.dropped++
	}
}

// Flush emits MessagesBatch (if any) then TopicsChanged (if any).
func (b *batcher) Flush() {
	b.emitMu.Lock()
	defer b.emitMu.Unlock()

	b.mu.Lock()
	msgs, dropped := b.pending.drain(), b.dropped
	b.dropped = 0
	ups := make([]TopicStats, 0, len(b.dirty))
	for _, e := range b.dirty {
		e.dirty = false
		ups = append(ups, e.stats)
	}
	b.dirty = b.dirty[:0]
	var msgSeq, topSeq uint64
	if len(msgs) > 0 || dropped > 0 {
		msgSeq = b.nextSeq()
	}
	if len(ups) > 0 {
		topSeq = b.nextSeq()
	}
	b.mu.Unlock()

	if msgSeq != 0 {
		b.sink.OnMessages(MessagesBatch{ProfileID: b.profileID, Seq: msgSeq, Messages: msgs, Dropped: dropped})
	}
	if topSeq != 0 {
		b.sink.OnTopics(TopicsChanged{ProfileID: b.profileID, Seq: topSeq, Upserts: ups})
	}
}

func byPath(a, b TopicStats) int { return cmp.Compare(a.Path, b.Path) }

// Watch starts streaming topic and returns its history, newest first.
func (b *batcher) Watch(topic string, limit int) WatchResult {
	b.mu.Lock()
	defer b.mu.Unlock()
	res := WatchResult{ProfileID: b.profileID, Topic: topic, Messages: []Message{}}
	e, ok := b.topics[topic]
	if !ok {
		e = &topicEntry{history: newRing(b.perTopic), stats: TopicStats{Path: topic}}
		b.topics[topic] = e
	}
	e.watched = true
	res.Messages = e.history.newestFirst(limit)
	return res
}

func (b *batcher) Unwatch(topic string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if e, ok := b.topics[topic]; ok {
		e.watched = false
		b.dropPending(func(m Message) bool { return m.Topic == topic })
	}
}

// dropPending removes queued messages matching pred. Caller holds mu.
func (b *batcher) dropPending(pred func(Message) bool) {
	kept := slices.DeleteFunc(b.pending.drain(), pred)
	for _, m := range kept {
		b.pending.push(m)
	}
}

// topicsLocked returns the sorted topic index, excluding topics that were
// only ever watched and never received anything. Caller holds mu.
func (b *batcher) topicsLocked() []TopicStats {
	out := make([]TopicStats, 0, len(b.topics))
	for _, e := range b.topics {
		if e.stats.Count > 0 {
			out = append(out, e.stats)
		}
	}
	slices.SortFunc(out, byPath)
	return out
}

// Clear forgets one topic ("" clears everything) and tells the UI.
func (b *batcher) Clear(topic string) {
	b.emitMu.Lock()
	defer b.emitMu.Unlock()

	b.mu.Lock()
	var removed []string
	if topic == "" {
		for path, e := range b.topics {
			if e.stats.Count > 0 {
				removed = append(removed, path)
			}
			e.stats = TopicStats{Path: path}
			e.history = newRing(b.perTopic)
			e.dirty = false
		}
		b.dirty = b.dirty[:0]
		b.pending.drain()
	} else if e, ok := b.topics[topic]; ok && e.stats.Count > 0 {
		e.stats = TopicStats{Path: topic}
		e.history = newRing(b.perTopic)
		if e.dirty {
			e.dirty = false
			b.dirty = slices.DeleteFunc(b.dirty, func(d *topicEntry) bool { return d == e })
		}
		b.dropPending(func(m Message) bool { return m.Topic == topic })
		removed = []string{topic}
	}
	var seq uint64
	if len(removed) > 0 {
		seq = b.nextSeq()
	}
	b.mu.Unlock()

	if seq != 0 {
		slices.Sort(removed)
		b.sink.OnTopics(TopicsChanged{ProfileID: b.profileID, Seq: seq, Removed: removed})
	}
}
