package services

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"mqttc/internal/mqtt"
)

// ---- fakes ------------------------------------------------------------------

type fakeSession struct {
	mu     sync.Mutex
	opts   mqtt.Options
	state  mqtt.ConnState
	closed bool
	subs   []mqtt.Subscription
}

func (f *fakeSession) State() mqtt.ConnState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}
func (f *fakeSession) Connect(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.state.CanTransitionTo(mqtt.StateConnecting) {
		return errors.New("already " + string(f.state))
	}
	f.state = mqtt.StateConnected
	return nil
}
func (f *fakeSession) Disconnect(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = mqtt.StateDisconnected
	return nil
}
func (f *fakeSession) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	f.state = mqtt.StateDisconnected
	return nil
}
func (f *fakeSession) Subscribe(_ context.Context, s mqtt.Subscription) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subs = append(f.subs, s)
	return nil
}
func (f *fakeSession) Unsubscribe(context.Context, string) error { return nil }
func (f *fakeSession) Subscriptions() []mqtt.Subscription {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.subs)
}
func (f *fakeSession) Publish(context.Context, mqtt.Publish) error { return nil }
func (f *fakeSession) Snapshot() mqtt.SessionSnapshot {
	return mqtt.SessionSnapshot{ProfileID: f.opts.ProfileID, State: f.State(), Subscriptions: f.Subscriptions()}
}
func (f *fakeSession) Watch(topic string, _ int) mqtt.WatchResult {
	return mqtt.WatchResult{ProfileID: f.opts.ProfileID, Topic: topic, Messages: []mqtt.Message{}}
}
func (f *fakeSession) Unwatch(string) {}
func (f *fakeSession) Clear(string)   {}

type fakeDialer struct {
	mu    sync.Mutex
	dials []*fakeSession
}

func (d *fakeDialer) Dial(opts mqtt.Options, _ mqtt.Sink) (mqtt.Session, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := &fakeSession{opts: opts, state: mqtt.StateDisconnected, subs: slices.Clone(opts.Subscriptions)}
	d.dials = append(d.dials, s)
	return s, nil
}

func (d *fakeDialer) last() *fakeSession {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dials[len(d.dials)-1]
}

type nopSink struct{}

func (nopSink) OnState(mqtt.StateChanged)                 {}
func (nopSink) OnMessages(mqtt.MessagesBatch)             {}
func (nopSink) OnTopics(mqtt.TopicsChanged)               {}
func (nopSink) OnSubscriptions(mqtt.SubscriptionsChanged) {}

func newFixture(t *testing.T) (*SessionService, *ProfileService, *fakeDialer, *MemSecrets) {
	t.Helper()
	secrets := &MemSecrets{}
	profiles := newProfileServiceAt(filepath.Join(t.TempDir(), "profiles.json"), secrets)
	dialer := &fakeDialer{}
	svc := newSessionService(profiles, dialer, nopSink{})
	t.Cleanup(func() { _ = svc.ServiceShutdown() })
	return svc, profiles, dialer, secrets
}

// ---- tests ------------------------------------------------------------------

func TestConnectDialsFromProfileAndKeychain(t *testing.T) {
	svc, profiles, dialer, _ := newFixture(t)
	p, err := profiles.Save(Profile{Name: "dev", Broker: "ssl://broker:8883", Username: "u", Password: "secret", Insecure: true})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Connect(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	s := dialer.last()
	if string(s.opts.Password) != "secret" || s.opts.Username != "u" || s.opts.Broker != "ssl://broker:8883" {
		t.Errorf("dialed with %+v", s.opts)
	}
	if s.opts.TLS == nil || !s.opts.TLS.InsecureSkipVerify {
		t.Errorf("TLS not configured from profile: %+v", s.opts.TLS)
	}
	if len(s.opts.Subscriptions) != 1 || s.opts.Subscriptions[0].Filter != "#" {
		t.Errorf("default subscriptions = %+v", s.opts.Subscriptions)
	}
	if s.State() != mqtt.StateConnected {
		t.Errorf("state = %s", s.State())
	}
	if err := svc.Connect(context.Background(), p.ID); err == nil {
		t.Error("Connect while live must fail")
	}
	if err := svc.Connect(context.Background(), "nope"); err == nil {
		t.Error("Connect for unknown profile must fail")
	}
}

func TestReconnectReusesSessionWhenProfileUnchanged(t *testing.T) {
	svc, profiles, dialer, _ := newFixture(t)
	p, _ := profiles.Save(Profile{Name: "dev", Broker: "tcp://b:1883"})
	_ = svc.Connect(context.Background(), p.ID)
	_ = svc.Disconnect(context.Background(), p.ID)
	_ = svc.Connect(context.Background(), p.ID)
	if len(dialer.dials) != 1 {
		t.Fatalf("dialed %d times, want 1", len(dialer.dials))
	}
}

func TestProfileEditWhileLiveRedialsAndCarriesSubscriptions(t *testing.T) {
	svc, profiles, dialer, _ := newFixture(t)
	p, _ := profiles.Save(Profile{Name: "dev", Broker: "tcp://old:1883"})
	_ = svc.Connect(context.Background(), p.ID)
	_ = svc.Subscribe(context.Background(), p.ID, mqtt.Subscription{Filter: "a/+", QoS: mqtt.QoS1})
	first := dialer.last()

	// Cosmetic change: keep the connection.
	p.Name = "renamed"
	if _, err := profiles.Save(p); err != nil {
		t.Fatal(err)
	}
	if len(dialer.dials) != 1 || first.closed {
		t.Fatalf("rename should not re-dial")
	}

	// Connection-relevant change: re-dial, carry subs, reconnect because it was live.
	p.Broker = "tcp://new:1883"
	if _, err := profiles.Save(p); err != nil {
		t.Fatal(err)
	}
	if len(dialer.dials) != 2 {
		t.Fatalf("dialed %d times, want 2", len(dialer.dials))
	}
	if !first.closed {
		t.Error("old session not closed")
	}
	second := dialer.last()
	if second.opts.Broker != "tcp://new:1883" {
		t.Errorf("new session broker = %s", second.opts.Broker)
	}
	if second.State() != mqtt.StateConnected {
		t.Errorf("new session should be reconnected, state = %s", second.State())
	}
	subs := second.opts.Subscriptions
	if len(subs) != 2 || subs[0].Filter != "#" || subs[1].Filter != "a/+" {
		t.Errorf("subscriptions not carried over: %+v", subs)
	}

	// Password change is connection-relevant too.
	p.Password = "new-secret"
	if _, err := profiles.Save(p); err != nil {
		t.Fatal(err)
	}
	if len(dialer.dials) != 3 || string(dialer.last().opts.Password) != "new-secret" {
		t.Errorf("password change did not re-dial with new secret")
	}
}

func TestProfileEditWhileDisconnectedRedialsOnNextConnect(t *testing.T) {
	svc, profiles, dialer, _ := newFixture(t)
	p, _ := profiles.Save(Profile{Name: "dev", Broker: "tcp://old:1883"})
	_ = svc.Connect(context.Background(), p.ID)
	_ = svc.Disconnect(context.Background(), p.ID)

	p.Broker = "tcp://new:1883"
	_, _ = profiles.Save(p)
	if dialer.last().State() != mqtt.StateDisconnected {
		t.Fatal("must not auto-connect a session that was disconnected")
	}
	_ = svc.Connect(context.Background(), p.ID)
	if dialer.last().opts.Broker != "tcp://new:1883" {
		t.Errorf("connected with stale options: %s", dialer.last().opts.Broker)
	}
}

func TestDeleteClosesSession(t *testing.T) {
	svc, profiles, dialer, secrets := newFixture(t)
	p, _ := profiles.Save(Profile{Name: "dev", Broker: "tcp://b:1883", Password: "pw"})
	_ = svc.Connect(context.Background(), p.ID)
	if err := profiles.Delete(p.ID); err != nil {
		t.Fatal(err)
	}
	if !dialer.last().closed {
		t.Error("session not closed on delete")
	}
	if _, err := svc.m.get(p.ID); err == nil {
		t.Error("session still registered after delete")
	}
	if pw, _ := secrets.Get(p.ID); pw != "" {
		t.Error("password not removed from keychain")
	}
	if len(svc.Snapshot().Profiles) != 0 {
		t.Error("profile still listed")
	}
}

func TestSnapshotListsProfilesAndSessions(t *testing.T) {
	svc, profiles, _, _ := newFixture(t)
	a, _ := profiles.Save(Profile{Name: "a", Broker: "tcp://a:1883"})
	_, _ = profiles.Save(Profile{Name: "b", Broker: "tcp://b:1883"})
	_ = svc.Connect(context.Background(), a.ID)

	snap := svc.Snapshot()
	if len(snap.Profiles) != 2 {
		t.Errorf("profiles = %+v", snap.Profiles)
	}
	if len(snap.Sessions) != 1 || snap.Sessions[0].ProfileID != a.ID || snap.Sessions[0].State != mqtt.StateConnected {
		t.Errorf("sessions = %+v", snap.Sessions)
	}
	if _, err := svc.Watch("missing", "t", 10); err == nil {
		t.Error("Watch on unknown session must fail")
	}
	if err := svc.Unwatch("missing", "t"); err != nil {
		t.Error("Unwatch on unknown session must be a no-op")
	}
	if err := svc.Disconnect(context.Background(), "missing"); err != nil {
		t.Error("Disconnect on unknown session must be a no-op")
	}
	if w, err := svc.Watch(a.ID, "t", 10); err != nil || w.Topic != "t" {
		t.Errorf("Watch = %+v, %v", w, err)
	}
}
