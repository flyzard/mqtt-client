package mqtt

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recSink records every event in emit order.
type recSink struct {
	mu     sync.Mutex
	states []StateChanged
	msgs   []MessagesBatch
	topics []TopicsChanged
	subs   []SubscriptionsChanged
	order  []string // "state" | "messages" | "topics" | "subs", with seq
}

func (r *recSink) OnState(e StateChanged) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.states = append(r.states, e)
	r.order = append(r.order, fmt.Sprintf("state:%d", e.Seq))
}
func (r *recSink) OnMessages(e MessagesBatch) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, e)
	r.order = append(r.order, fmt.Sprintf("messages:%d", e.Seq))
}
func (r *recSink) OnTopics(e TopicsChanged) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.topics = append(r.topics, e)
	r.order = append(r.order, fmt.Sprintf("topics:%d", e.Seq))
}
func (r *recSink) OnSubscriptions(e SubscriptionsChanged) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subs = append(r.subs, e)
	r.order = append(r.order, fmt.Sprintf("subs:%d", e.Seq))
}

func newTestBatcher(t *testing.T, perTopic int) (*batcher, *recSink) {
	t.Helper()
	var seq atomic.Uint64
	sink := &recSink{}
	// A very long interval so tests drive Flush explicitly.
	b := newBatcher("p1", sink, perTopic, time.Hour, func() uint64 { return seq.Add(1) })
	t.Cleanup(b.Close)
	return b, sink
}

func msg(topic string, n int) Message {
	return Message{Topic: topic, Payload: fmt.Appendf(nil, "m%d", n), ReceivedAt: time.Unix(int64(n), 0)}
}

func TestRingWrapsAndReturnsNewestFirst(t *testing.T) {
	r := newRing(3)
	for i := 1; i <= 5; i++ {
		evicted := r.push(Message{ID: uint64(i)})
		if evicted != (i > 3) {
			t.Errorf("push %d: evicted=%v", i, evicted)
		}
	}
	got := r.newestFirst(0)
	if len(got) != 3 || got[0].ID != 5 || got[1].ID != 4 || got[2].ID != 3 {
		t.Fatalf("newestFirst = %+v", got)
	}
	if got := r.newestFirst(2); len(got) != 2 || got[0].ID != 5 {
		t.Fatalf("newestFirst(2) = %+v", got)
	}
}

func TestPushAssignsIDsAndIndexesEveryTopic(t *testing.T) {
	b, sink := newTestBatcher(t, 10)
	b.Push(msg("a/1", 1))
	b.Push(msg("a/2", 2))
	b.Push(msg("a/1", 3))
	b.Flush()

	// Nothing is watched, so no message batch, but the topic index is pushed.
	if len(sink.msgs) != 0 {
		t.Fatalf("unexpected message batch for unwatched topics: %+v", sink.msgs)
	}
	if len(sink.topics) != 1 {
		t.Fatalf("want 1 topics event, got %d", len(sink.topics))
	}
	ups := sink.topics[0].Upserts
	if len(ups) != 2 || ups[0].Path != "a/1" || ups[0].Count != 2 || ups[1].Path != "a/2" || ups[1].Count != 1 {
		t.Fatalf("upserts = %+v", ups)
	}
	if ups[0].Bytes != 4 { // "m1" + "m3"
		t.Errorf("bytes = %d", ups[0].Bytes)
	}

	// History is kept for unwatched topics too.
	w := b.Watch("a/1", 0)
	if len(w.Messages) != 2 || w.Messages[0].ID != 3 || w.Messages[1].ID != 1 {
		t.Fatalf("watch history = %+v", w.Messages)
	}
	if w.Messages[0].ProfileID != "p1" {
		t.Errorf("profile id not stamped")
	}
}

func TestWatchedTopicsStreamAndFlushOrder(t *testing.T) {
	b, sink := newTestBatcher(t, 10)
	b.Watch("t", 0)
	b.Push(msg("t", 1))
	b.Push(msg("other", 2))
	b.Push(msg("t", 3))
	b.Flush()
	b.Flush() // nothing pending: must emit nothing

	if len(sink.msgs) != 1 || len(sink.msgs[0].Messages) != 2 {
		t.Fatalf("msgs = %+v", sink.msgs)
	}
	if sink.msgs[0].Messages[0].ID != 1 || sink.msgs[0].Messages[1].ID != 3 {
		t.Errorf("stream is oldest-first within a batch: %+v", sink.msgs[0].Messages)
	}
	if len(sink.order) != 2 || sink.order[0] != "messages:1" || sink.order[1] != "topics:2" {
		t.Fatalf("emit order/seq = %v", sink.order)
	}

	b.Unwatch("t")
	b.Push(msg("t", 4))
	b.Flush()
	if len(sink.msgs) != 1 {
		t.Fatalf("unwatched topic still streamed: %+v", sink.msgs)
	}
}

func TestPendingOverflowCountsDropped(t *testing.T) {
	b, sink := newTestBatcher(t, 10)
	b.pending = newRing(3) // shrink the stream queue; history stays at 10
	b.Watch("t", 0)
	for i := 1; i <= 5; i++ {
		b.Push(msg("t", i))
	}
	b.Flush()
	got := sink.msgs[0]
	if got.Dropped != 2 {
		t.Errorf("dropped = %d, want 2", got.Dropped)
	}
	if len(got.Messages) != 3 || got.Messages[0].ID != 3 || got.Messages[2].ID != 5 {
		t.Errorf("kept the wrong messages: %+v", got.Messages)
	}
	// The next flush starts from a clean slate.
	b.Push(msg("t", 6))
	b.Flush()
	if sink.msgs[1].Dropped != 0 || len(sink.msgs[1].Messages) != 1 {
		t.Errorf("second batch = %+v", sink.msgs[1])
	}
}

func TestClearRemovesAndDropsPending(t *testing.T) {
	b, sink := newTestBatcher(t, 10)
	b.Watch("a", 0)
	b.Watch("b", 0)
	b.Push(msg("a", 1))
	b.Push(msg("b", 2))
	b.Clear("a")
	b.Flush()

	if len(sink.topics) != 2 {
		t.Fatalf("topics events = %+v", sink.topics)
	}
	if rm := sink.topics[0].Removed; len(rm) != 1 || rm[0] != "a" {
		t.Errorf("removed = %v", rm)
	}
	if ups := sink.topics[1].Upserts; len(ups) != 1 || ups[0].Path != "b" {
		t.Errorf("upserts after clear = %+v", ups)
	}
	if len(sink.msgs) != 1 || len(sink.msgs[0].Messages) != 1 || sink.msgs[0].Messages[0].Topic != "b" {
		t.Errorf("pending not filtered: %+v", sink.msgs)
	}
	if len(b.Watch("a", 0).Messages) != 0 {
		t.Errorf("history survived clear")
	}
	b.mu.Lock()
	if got := b.topicsLocked(); len(got) != 1 || got[0].Path != "b" {
		t.Errorf("cleared topic still indexed: %+v", got)
	}
	b.mu.Unlock()

	b.Push(msg("b", 3))
	b.Clear("")
	b.Flush()
	last := sink.topics[len(sink.topics)-1]
	if len(last.Removed) != 1 || last.Removed[0] != "b" {
		t.Errorf("clear all removed = %v", last.Removed)
	}
	if len(sink.msgs) != 1 {
		t.Errorf("pending survived clear all: %+v", sink.msgs)
	}
}

func TestCloseFlushesAndIsIdempotent(t *testing.T) {
	var seq atomic.Uint64
	sink := &recSink{}
	b := newBatcher("p1", sink, 10, time.Hour, func() uint64 { return seq.Add(1) })
	b.Watch("t", 0)
	b.Push(msg("t", 1))
	b.Close()
	b.Close()
	if len(sink.msgs) != 1 {
		t.Fatalf("close did not flush: %+v", sink.msgs)
	}
}

func TestWatchingAnUnknownTopicDoesNotIndexIt(t *testing.T) {
	b, sink := newTestBatcher(t, 10)
	b.Watch("ghost", 0)
	b.Flush()
	if len(sink.topics) != 0 {
		t.Fatalf("watch alone must not create a topic: %+v", sink.topics)
	}
	b.mu.Lock()
	n := len(b.topicsLocked())
	b.mu.Unlock()
	if n != 0 {
		t.Fatalf("watched-only topic appears in index")
	}
	b.Push(msg("ghost", 1))
	b.Flush()
	if len(sink.msgs) != 1 {
		t.Fatalf("watch set before first message was lost")
	}
}

func TestConcurrentPushAndFlush(t *testing.T) {
	b, sink := newTestBatcher(t, 100)
	b.Watch("t", 0)
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 250; i++ {
				b.Push(msg("t", i))
				if i%50 == 0 {
					b.Flush()
				}
			}
		}()
	}
	wg.Wait()
	b.Flush()
	var total int
	var dropped uint64
	for _, m := range sink.msgs {
		total += len(m.Messages)
		dropped += m.Dropped
	}
	if total+int(dropped) != 1000 {
		t.Fatalf("streamed %d + dropped %d != 1000", total, dropped)
	}
	if got := b.Watch("t", 0); len(got.Messages) != 100 {
		t.Fatalf("history holds %d, want ring capacity 100", len(got.Messages))
	}
}

func TestStatsTrackPreviewAndInterval(t *testing.T) {
	b, sink := newTestBatcher(t, 10)
	b.Push(msg("a", 10)) // t=10s
	b.Flush()
	if len(sink.topics) != 1 {
		t.Fatalf("want 1 topics event, got %d", len(sink.topics))
	}
	s := sink.topics[0].Upserts[0]
	if string(s.LastPayload) != "m10" || s.AvgInterval != 0 {
		t.Fatalf("first message: got preview %q interval %v", s.LastPayload, s.AvgInterval)
	}

	b.Push(msg("a", 12)) // gap 2s -> avg 2
	b.Push(msg("a", 14)) // gap 2s -> avg 2
	b.Flush()
	s = sink.topics[1].Upserts[0]
	if string(s.LastPayload) != "m14" || s.AvgInterval != 2 {
		t.Fatalf("after steady gaps: got preview %q interval %v", s.LastPayload, s.AvgInterval)
	}

	// The emitted copy must not change under later pushes.
	b.Push(msg("a", 16))
	if string(s.LastPayload) != "m14" {
		t.Fatalf("emitted stats mutated by a later push: %q", s.LastPayload)
	}

	big := Message{Topic: "big", Payload: make([]byte, PreviewBytes*3), ReceivedAt: time.Unix(1, 0)}
	b.Push(big)
	b.Flush()
	for _, u := range sink.topics[2].Upserts {
		if u.Path == "big" && len(u.LastPayload) != PreviewBytes {
			t.Fatalf("preview not capped: %d bytes", len(u.LastPayload))
		}
		if u.Path == "big" && u.Bytes != uint64(PreviewBytes*3) {
			t.Fatalf("bytes should count the full payload, got %d", u.Bytes)
		}
	}
}
