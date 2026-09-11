package mqtt

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
)

const flushInterval = 50 * time.Millisecond

var ErrClosed = errors.New("session closed")

type Paho5Dialer struct{}

func (Paho5Dialer) Dial(opts Options, sink Sink) (Session, error) {
	u, _, err := ParseBroker(opts.Broker)
	if err != nil {
		return nil, err
	}
	opts.normalize()
	p := &paho5{opts: opts, url: u, sink: sink, state: StateDisconnected, subs: map[string]QoS{}}
	for _, s := range opts.Subscriptions {
		p.subs[s.Filter] = s.QoS
	}
	p.batch = newBatcher(opts.ProfileID, sink, opts.HistoryPerTopic, flushInterval, func() uint64 { return p.seq.Add(1) })
	return p, nil
}

// normalize fills in defaults for zero-valued knobs. It is the single place
// session defaults are defined.
func (o *Options) normalize() {
	if o.HistoryPerTopic <= 0 {
		o.HistoryPerTopic = 1000
	}
	if o.KeepAlive == 0 {
		o.KeepAlive = 30 * time.Second
	}
	if o.ReconnectMin <= 0 {
		o.ReconnectMin = time.Second
	}
	if o.ReconnectMax <= o.ReconnectMin {
		o.ReconnectMax = max(30*time.Second, 2*o.ReconnectMin)
	}
}

// paho5 is a Session backed by paho.golang's autopaho connection manager.
//
// Locking: mu guards every mutable field. State changes assign their Seq and
// are handed to the sink while mu is held, so Seq order equals emit order.
// Lock order when both are needed is mu then batch.mu (Snapshot only).
//
// gen is bumped on every Connect/Disconnect. Callbacks from autopaho capture
// the gen they were created under and are ignored if it no longer matches,
// which is how a torn-down connection's late errors are kept out of the
// state machine. It is atomic so the per-message callback can check it
// without taking mu.
type paho5 struct {
	opts  Options
	url   *url.URL
	sink  Sink
	batch *batcher
	seq   atomic.Uint64
	gen   atomic.Uint64

	mu      sync.Mutex
	state   ConnState
	lastErr string
	cm      *autopaho.ConnectionManager
	cancel  context.CancelFunc
	subs    map[string]QoS
	closed  bool
}

// Options returns the normalized options the session was dialed with.
func (p *paho5) Options() Options { return p.opts }

func (p *paho5) State() ConnState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// setStateLocked enforces the transition table and notifies the sink.
// Caller holds mu.
func (p *paho5) setStateLocked(next ConnState, errMsg string) bool {
	if !p.state.CanTransitionTo(next) {
		log.Printf("mqtt: profile %s: illegal transition %s -> %s (%s)", p.opts.ProfileID, p.state, next, errMsg)
		return false
	}
	p.state, p.lastErr = next, errMsg
	p.sink.OnState(StateChanged{ProfileID: p.opts.ProfileID, Seq: p.seq.Add(1), State: next, Error: errMsg, At: time.Now()})
	return true
}

// setState is setStateLocked for callbacks, guarded by generation.
func (p *paho5) setState(gen uint64, next ConnState, errMsg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if gen == p.gen.Load() {
		p.setStateLocked(next, errMsg)
	}
}

func (p *paho5) Connect(ctx context.Context) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return ErrClosed
	}
	if !p.state.CanTransitionTo(StateConnecting) {
		st := p.state
		p.mu.Unlock()
		return fmt.Errorf("already %s", st)
	}
	gen := p.gen.Add(1)
	p.setStateLocked(StateConnecting, "")
	runCtx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.mu.Unlock()

	cfg := autopaho.ClientConfig{
		ServerUrls:                    []*url.URL{p.url},
		KeepAlive:                     uint16(p.opts.KeepAlive.Seconds()),
		CleanStartOnInitialConnection: p.opts.CleanStart,
		ConnectUsername:               p.opts.Username,
		ConnectPassword:               p.opts.Password,
		TlsCfg:                        p.opts.TLS,
		ReconnectBackoff:              autopaho.NewExponentialBackoff(p.opts.ReconnectMin, p.opts.ReconnectMax, p.opts.ReconnectMin, 2),
		OnConnectionUp: func(cm *autopaho.ConnectionManager, _ *paho.Connack) {
			p.setState(gen, StateConnected, "")
			p.resubscribe(runCtx, gen, cm)
		},
		OnConnectError: func(err error) { p.setState(gen, StateReconnecting, err.Error()) },
		ClientConfig: paho.ClientConfig{
			ClientID: p.opts.ClientID,
			OnPublishReceived: []func(paho.PublishReceived) (bool, error){
				func(pr paho.PublishReceived) (bool, error) { p.onPublish(gen, pr.Packet); return true, nil },
			},
			OnClientError: func(err error) { p.setState(gen, StateReconnecting, err.Error()) },
			OnServerDisconnect: func(d *paho.Disconnect) {
				p.setState(gen, StateReconnecting, fmt.Sprintf("server disconnect: reason %d", d.ReasonCode))
			},
		},
	}

	cm, err := autopaho.NewConnection(runCtx, cfg)

	p.mu.Lock()
	defer p.mu.Unlock()
	if gen != p.gen.Load() { // Disconnect raced us; it already cancelled runCtx.
		if cm != nil {
			_ = cm.Disconnect(ctx)
		}
		return nil
	}
	if err != nil {
		cancel()
		p.cancel = nil
		p.setStateLocked(StateFailed, err.Error())
		return err
	}
	p.cm = cm
	return nil // state flips to Connected asynchronously via OnConnectionUp
}

func (p *paho5) onPublish(gen uint64, pk *paho.Publish) {
	if gen != p.gen.Load() {
		return
	}
	m := Message{
		Topic: pk.Topic, Payload: pk.Payload, QoS: QoS(pk.QoS),
		Retained: pk.Retain, ReceivedAt: time.Now(),
	}
	if pr := pk.Properties; pr != nil {
		m.Props.ContentType = pr.ContentType
		m.Props.ResponseTopic = pr.ResponseTopic
		m.Props.CorrelationData = pr.CorrelationData
		m.Props.MessageExpiry = pr.MessageExpiry
		if len(pr.User) > 0 {
			m.Props.User = make(map[string]string, len(pr.User))
			for _, kv := range pr.User {
				m.Props.User[kv.Key] = kv.Value
			}
		}
	}
	p.batch.Push(m)
}

func (p *paho5) resubscribe(ctx context.Context, gen uint64, cm *autopaho.ConnectionManager) {
	p.mu.Lock()
	if gen != p.gen.Load() {
		p.mu.Unlock()
		return
	}
	var so []paho.SubscribeOptions
	for f, q := range p.subs {
		so = append(so, paho.SubscribeOptions{Topic: f, QoS: byte(q)})
	}
	p.mu.Unlock()
	if len(so) > 0 {
		if _, err := cm.Subscribe(ctx, &paho.Subscribe{Subscriptions: so}); err != nil {
			log.Printf("mqtt: profile %s: resubscribe: %v", p.opts.ProfileID, err)
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if gen == p.gen.Load() {
		p.emitSubsLocked()
	}
}

func (p *paho5) Disconnect(ctx context.Context) error {
	p.mu.Lock()
	if p.state == StateDisconnected {
		p.mu.Unlock()
		return nil
	}
	cm, cancel := p.cm, p.cancel
	p.cm, p.cancel = nil, nil
	p.gen.Add(1)
	p.setStateLocked(StateDisconnected, "")
	p.mu.Unlock()

	if cm != nil {
		dctx, done := context.WithTimeout(ctx, 3*time.Second)
		defer done()
		_ = cm.Disconnect(dctx)
	}
	if cancel != nil {
		cancel()
	}
	return nil
}

// Close disconnects and stops the batcher. The session is unusable afterwards.
func (p *paho5) Close() error {
	err := p.Disconnect(context.Background())
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	p.batch.Close()
	return err
}

// updateSubs applies mutate to the local subscription set, sends the change
// to the broker if connected, undoes the mutation if the broker refused it,
// and emits the resulting set. Offline changes are applied on the next
// OnConnectionUp.
func (p *paho5) updateSubs(mutate, undo func(), send func(*autopaho.ConnectionManager) error) error {
	p.mu.Lock()
	mutate()
	cm := p.cm
	if cm == nil {
		p.emitSubsLocked()
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	err := send(cm)

	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil {
		undo()
	}
	p.emitSubsLocked()
	return err
}

func (p *paho5) Subscribe(ctx context.Context, s Subscription) error {
	var prev QoS
	var had bool
	return p.updateSubs(
		func() { prev, had = p.subs[s.Filter]; p.subs[s.Filter] = s.QoS },
		func() { // don't keep retrying a filter the broker rejects
			if had {
				p.subs[s.Filter] = prev
			} else {
				delete(p.subs, s.Filter)
			}
		},
		func(cm *autopaho.ConnectionManager) error {
			_, err := cm.Subscribe(ctx, &paho.Subscribe{Subscriptions: []paho.SubscribeOptions{{Topic: s.Filter, QoS: byte(s.QoS)}}})
			return err
		},
	)
}

func (p *paho5) Unsubscribe(ctx context.Context, filter string) error {
	return p.updateSubs(
		func() { delete(p.subs, filter) },
		func() {}, // the broker keeps it, but we no longer want it: leave it removed
		func(cm *autopaho.ConnectionManager) error {
			_, err := cm.Unsubscribe(ctx, &paho.Unsubscribe{Topics: []string{filter}})
			return err
		},
	)
}

func (p *paho5) Subscriptions() []Subscription {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.subsLocked()
}

func (p *paho5) subsLocked() []Subscription {
	out := make([]Subscription, 0, len(p.subs))
	for f, q := range p.subs {
		out = append(out, Subscription{Filter: f, QoS: q})
	}
	return out
}

func (p *paho5) emitSubsLocked() {
	p.sink.OnSubscriptions(SubscriptionsChanged{ProfileID: p.opts.ProfileID, Seq: p.seq.Add(1), Active: p.subsLocked()})
}

func (p *paho5) Publish(ctx context.Context, pub Publish) error {
	p.mu.Lock()
	cm, st := p.cm, p.state
	p.mu.Unlock()
	if cm == nil || st != StateConnected {
		return fmt.Errorf("not connected (%s)", st)
	}
	pk := &paho.Publish{Topic: pub.Topic, QoS: byte(pub.QoS), Retain: pub.Retain, Payload: pub.Payload}
	if pub.Props.ContentType != "" || pub.Props.ResponseTopic != "" || len(pub.Props.User) > 0 {
		pk.Properties = &paho.PublishProperties{ContentType: pub.Props.ContentType, ResponseTopic: pub.Props.ResponseTopic}
		for k, v := range pub.Props.User {
			pk.Properties.User = append(pk.Properties.User, paho.UserProperty{Key: k, Value: v})
		}
	}
	_, err := cm.Publish(ctx, pk)
	return err
}

func (p *paho5) Snapshot() SessionSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.batch.mu.Lock()
	defer p.batch.mu.Unlock()
	return SessionSnapshot{
		ProfileID:     p.opts.ProfileID,
		Seq:           p.seq.Load(), // read while holding both locks: see events.go
		State:         p.state,
		Error:         p.lastErr,
		Subscriptions: p.subsLocked(),
		Topics:        p.batch.topicsLocked(),
	}
}

func (p *paho5) Watch(topic string, limit int) WatchResult { return p.batch.Watch(topic, limit) }
func (p *paho5) Unwatch(topic string)                      { p.batch.Unwatch(topic) }
func (p *paho5) Clear(topic string)                        { p.batch.Clear(topic) }
