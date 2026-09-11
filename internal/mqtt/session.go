package mqtt

import (
	"context"
	"crypto/tls"
	"time"
)

// Options is everything needed to open a session. It is built by the session
// manager from a stored profile plus the secret fetched from the keychain, so
// the session layer never touches storage. A Session is immutable with respect
// to its Options: to apply new options, the manager dials a new session.
type Options struct {
	ProfileID string
	Broker    string // "tcp://host:1883", "ssl://host:8883", "ws://host/mqtt"
	ClientID  string
	Username  string
	Password  []byte
	TLS       *tls.Config

	KeepAlive  time.Duration
	CleanStart bool

	// Reconnect policy. autopaho handles the loop; we only expose the knobs.
	// Zero values mean 1s / 30s.
	ReconnectMin time.Duration
	ReconnectMax time.Duration

	// Per-topic ring buffer capacity. Bounded by design: the app must never
	// OOM on a firehose topic. Zero means 1000.
	HistoryPerTopic int

	// Subscriptions to (re)apply as soon as the connection is up. The manager
	// uses this to carry subscriptions across a re-dial.
	Subscriptions []Subscription
}

// Sink receives everything a session wants to tell the outside world.
// The Wails layer implements it by forwarding to app.Event.Emit; tests
// implement it with channels. Sessions never import Wails.
type Sink interface {
	OnState(StateChanged)
	OnMessages(MessagesBatch)
	OnTopics(TopicsChanged)
	OnSubscriptions(SubscriptionsChanged)
}

// Session is one live (or reconnecting) connection to one broker.
// All methods are safe for concurrent use. Context cancellation aborts the
// call, not the session; use Disconnect to tear down.
type Session interface {
	State() ConnState

	Connect(ctx context.Context) error    // Disconnected|Failed -> Connecting
	Disconnect(ctx context.Context) error // any -> Disconnected, idempotent
	Close() error                         // Disconnect and release resources; terminal

	Subscribe(ctx context.Context, sub Subscription) error
	Unsubscribe(ctx context.Context, filter string) error
	Subscriptions() []Subscription

	Publish(ctx context.Context, p Publish) error

	// Snapshot returns state, subscriptions and topic index consistent as of
	// the returned Seq. See the Seq contract in events.go.
	Snapshot() SessionSnapshot

	// Watch marks topic as visible in the UI: from now on its messages are
	// streamed in MessagesBatch events. The returned history is consistent
	// with the stream (every later message has a larger ID).
	Watch(topic string, limit int) WatchResult
	Unwatch(topic string)

	// Clear forgets history for one topic; "" clears everything.
	Clear(topic string)
}

// Dialer creates sessions. There is one implementation per protocol version
// (v5 via paho.golang first; v3.1.1 later behind the same interface) so the
// rest of the app is indifferent to which one is in use.
type Dialer interface {
	Dial(opts Options, sink Sink) (Session, error)
}
