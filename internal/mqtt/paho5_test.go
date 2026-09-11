package mqtt

import (
	"context"
	"errors"
	"testing"
	"time"
)

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestDialRejectsBadBroker(t *testing.T) {
	for _, u := range []string{"", "not a url", "tcp://"} {
		if _, err := (Paho5Dialer{}).Dial(Options{ProfileID: "p", Broker: u}, &recSink{}); err == nil {
			t.Errorf("Dial(%q) should fail", u)
		}
	}
}

func TestDialAppliesDefaultsAndInitialSubscriptions(t *testing.T) {
	sink := &recSink{}
	s, err := (Paho5Dialer{}).Dial(Options{
		ProfileID: "p", Broker: "tcp://127.0.0.1:1",
		Subscriptions: []Subscription{{Filter: "a/#", QoS: QoS1}},
	}, sink)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	o := s.(*paho5).Options()
	if o.HistoryPerTopic != 1000 || o.KeepAlive != 30*time.Second || o.ReconnectMin != time.Second || o.ReconnectMax != 30*time.Second {
		t.Errorf("defaults not applied: %+v", o)
	}
	if subs := s.Subscriptions(); len(subs) != 1 || subs[0].Filter != "a/#" || subs[0].QoS != QoS1 {
		t.Errorf("initial subscriptions = %+v", subs)
	}
	if s.State() != StateDisconnected {
		t.Errorf("state = %s", s.State())
	}
}

// Port 1 on loopback refuses immediately, so the first attempt fails and
// autopaho enters its retry loop. That is exactly the "wrong broker" case a
// user hits, and it must surface as Reconnecting with an error, not as a
// Connecting spinner that never resolves.
func TestConnectToUnreachableBrokerReportsErrorThenDisconnects(t *testing.T) {
	sink := &recSink{}
	s, err := (Paho5Dialer{}).Dial(Options{ProfileID: "p", Broker: "tcp://127.0.0.1:1", ReconnectMin: 50 * time.Millisecond, ReconnectMax: 100 * time.Millisecond}, sink)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := s.Connect(context.Background()); err == nil {
		t.Error("second Connect should fail while connecting")
	}
	waitFor(t, "reconnecting state", func() bool { return s.State() == StateReconnecting })

	snap := s.Snapshot()
	if snap.State != StateReconnecting || snap.Error == "" {
		t.Errorf("snapshot = %+v, want reconnecting with an error", snap)
	}
	if err := s.Publish(context.Background(), Publish{Topic: "x"}); err == nil {
		t.Error("Publish should fail while not connected")
	}

	if err := s.Disconnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.State() != StateDisconnected {
		t.Fatalf("state after disconnect = %s", s.State())
	}
	if err := s.Disconnect(context.Background()); err != nil {
		t.Fatal("Disconnect must be idempotent")
	}

	// Late callbacks from the torn-down connection must not touch the state.
	time.Sleep(250 * time.Millisecond)
	if s.State() != StateDisconnected {
		t.Fatalf("late callback changed state to %s", s.State())
	}

	// Seq must be strictly increasing in emit order.
	sink.mu.Lock()
	defer sink.mu.Unlock()
	var last uint64
	for _, st := range sink.states {
		if st.Seq <= last {
			t.Errorf("seq not increasing: %+v", sink.states)
		}
		last = st.Seq
	}
	if sink.states[0].State != StateConnecting || sink.states[len(sink.states)-1].State != StateDisconnected {
		t.Errorf("state sequence = %+v", sink.states)
	}
}

func TestSubscribeWhileDisconnectedIsQueuedAndEmitted(t *testing.T) {
	sink := &recSink{}
	s, _ := (Paho5Dialer{}).Dial(Options{ProfileID: "p", Broker: "tcp://127.0.0.1:1"}, sink)
	t.Cleanup(func() { _ = s.Close() })

	if err := s.Subscribe(context.Background(), Subscription{Filter: "a", QoS: QoS0}); err != nil {
		t.Fatal(err)
	}
	if err := s.Unsubscribe(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	if len(sink.subs) != 2 || len(sink.subs[0].Active) != 1 || len(sink.subs[1].Active) != 0 {
		t.Fatalf("subs events = %+v", sink.subs)
	}
}

func TestCloseIsTerminal(t *testing.T) {
	s, _ := (Paho5Dialer{}).Dial(Options{ProfileID: "p", Broker: "tcp://127.0.0.1:1"}, &recSink{})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal("Close must be idempotent")
	}
	if err := s.Connect(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("Connect after Close = %v, want ErrClosed", err)
	}
}

func TestWatchReturnsHistoryConsistentWithStream(t *testing.T) {
	sink := &recSink{}
	s, _ := (Paho5Dialer{}).Dial(Options{ProfileID: "p", Broker: "tcp://127.0.0.1:1", HistoryPerTopic: 5}, sink)
	t.Cleanup(func() { _ = s.Close() })
	p := s.(*paho5)

	for i := 1; i <= 3; i++ {
		p.batch.Push(msg("t", i))
	}
	w := s.Watch("t", 2)
	if len(w.Messages) != 2 || w.Messages[0].ID != 3 {
		t.Fatalf("watch = %+v", w.Messages)
	}
	p.batch.Push(msg("t", 4))
	p.batch.Flush()
	if len(sink.msgs) != 1 || sink.msgs[0].Messages[0].ID != 4 {
		t.Fatalf("stream after watch = %+v", sink.msgs)
	}
	snap := s.Snapshot()
	if len(snap.Topics) != 1 || snap.Topics[0].Count != 4 {
		t.Errorf("snapshot topics = %+v", snap.Topics)
	}
}
