// Package mqtt owns all broker state. The frontend is a projection of the
// types in this file; nothing here may depend on Wails.
package mqtt

import (
	"fmt"
	"net/url"
	"slices"
	"time"
)

// ---- Connection state machine ---------------------------------------------

// ConnState is the only source of truth for a session's status. Transitions
// are enforced in session.go; the frontend mirrors this enum exactly.
type ConnState string

const (
	StateDisconnected ConnState = "disconnected"
	StateConnecting   ConnState = "connecting"
	StateConnected    ConnState = "connected"
	StateReconnecting ConnState = "reconnecting"
	StateFailed       ConnState = "failed" // terminal until Connect is called again
)

// transitions is the full set of legal moves. Anything else is a programming
// error: setState logs it and refuses the move rather than corrupting state.
//
// Connecting -> Reconnecting is legal because autopaho retries the very first
// attempt too; the UI shows "reconnecting" plus the last error rather than a
// spinner that never resolves.
var transitions = map[ConnState][]ConnState{
	StateDisconnected: {StateConnecting},
	StateConnecting:   {StateConnected, StateReconnecting, StateFailed, StateDisconnected},
	StateConnected:    {StateReconnecting, StateDisconnected, StateFailed},
	StateReconnecting: {StateConnected, StateReconnecting, StateFailed, StateDisconnected},
	StateFailed:       {StateConnecting, StateDisconnected},
}

func (s ConnState) CanTransitionTo(next ConnState) bool {
	return slices.Contains(transitions[s], next)
}

// Live reports whether a session in this state owns a broker connection
// (possibly one that is being re-established).
func (s ConnState) Live() bool {
	return s == StateConnecting || s == StateConnected || s == StateReconnecting
}

// ---- Broker URLs -------------------------------------------------------------

// schemes is the single list of accepted broker URL schemes and whether each
// one implies TLS. Profile validation and dialing both consult it.
var schemes = map[string]bool{
	"tcp": false, "mqtt": false, "ws": false,
	"ssl": true, "tls": true, "mqtts": true, "wss": true,
}

// ParseBroker validates a broker URL and reports whether it uses TLS.
func ParseBroker(raw string) (u *url.URL, tls bool, err error) {
	if raw == "" {
		return nil, false, fmt.Errorf("broker is required")
	}
	u, err = url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, false, fmt.Errorf("broker must be a URL like tcp://host:1883, got %q", raw)
	}
	tls, ok := schemes[u.Scheme]
	if !ok {
		return nil, false, fmt.Errorf("unsupported broker scheme %q", u.Scheme)
	}
	return u, tls, nil
}

// ---- Wire-level values ------------------------------------------------------

type QoS byte

const (
	QoS0 QoS = 0
	QoS1 QoS = 1
	QoS2 QoS = 2
)

// Properties is the MQTT 5 subset worth showing in the inspector.
// All fields optional; zero value means "absent".
type Properties struct {
	ContentType     string            `json:"contentType,omitempty"`
	ResponseTopic   string            `json:"responseTopic,omitempty"`
	CorrelationData []byte            `json:"correlationData,omitempty"`
	MessageExpiry   *uint32           `json:"messageExpiry,omitempty"` // seconds
	User            map[string]string `json:"user,omitempty"`
}

// Message is one received (or echoed) publish.
// Payload is []byte on purpose: the frontend decides how to render it.
// Wails serialises it as base64; the frontend decodes it once in api.ts.
type Message struct {
	ID         uint64     `json:"id"` // monotonic per session, used as list key and for dedupe
	ProfileID  string     `json:"profileId"`
	Topic      string     `json:"topic"`
	Payload    []byte     `json:"payload"`
	QoS        QoS        `json:"qos"`
	Retained   bool       `json:"retained"`
	ReceivedAt time.Time  `json:"receivedAt"`
	Props      Properties `json:"props"`
}

// Publish is what the UI sends. Kept separate from Message so the
// publish bar can't accidentally carry received-only fields.
type Publish struct {
	Topic   string     `json:"topic"`
	Payload []byte     `json:"payload"`
	QoS     QoS        `json:"qos"`
	Retain  bool       `json:"retain"`
	Props   Properties `json:"props"`
}

type Subscription struct {
	Filter string `json:"filter"` // may contain + and #
	QoS    QoS    `json:"qos"`
}

// PreviewBytes caps TopicStats.LastPayload: the tree only ever shows one
// line, and the full payload is available through Watch.
const PreviewBytes = 256

// TopicStats is one node in the topic tree. Path is the full topic;
// the frontend derives the hierarchy by splitting on '/'.
type TopicStats struct {
	Path     string    `json:"path"`
	Count    uint64    `json:"count"`
	Bytes    uint64    `json:"bytes"`
	LastSeen time.Time `json:"lastSeen"`
	Retained bool      `json:"retained"`
	// LastPayload is the newest payload truncated to PreviewBytes, so the
	// tree can show a value next to every topic without watching it.
	LastPayload []byte `json:"lastPayload"`
	// AvgInterval is a smoothed gap between consecutive messages in seconds
	// (0 until a topic has seen two). The UI uses it to flag topics that
	// have gone quiet for much longer than usual.
	AvgInterval float64 `json:"avgInterval"`
}

// observe folds one message into the stats. It is the single place the
// per-topic aggregates are defined. LastPayload is always a fresh slice, so
// a TopicStats value can be copied and handed out without aliasing.
func (s *TopicStats) observe(m Message) {
	gap := max(0, m.ReceivedAt.Sub(s.LastSeen).Seconds())
	switch s.Count {
	case 0: // first message: no gap yet
	case 1:
		s.AvgInterval = gap
	default:
		// EWMA with alpha 0.3: reacts within a few messages, ignores one-off jitter.
		s.AvgInterval += 0.3 * (gap - s.AvgInterval)
	}
	s.Count++
	s.Bytes += uint64(len(m.Payload))
	s.LastSeen = m.ReceivedAt
	s.Retained = m.Retained
	s.LastPayload = slices.Clone(m.Payload[:min(len(m.Payload), PreviewBytes)])
}

// ---- Events pushed to the frontend -----------------------------------------
//
// Event names are the single place both sides must agree on. Each name has
// exactly one payload type; events.ts mirrors this as a discriminated union.
//
// Every event carries Seq, a per-session monotonic counter. Snapshots carry
// the Seq current at the moment they were taken, so the frontend can discard
// any event with Seq <= snapshot.Seq: that event's effect is already in the
// snapshot. This is what makes "fetch a snapshot, then apply the stream"
// race-free.

const (
	EvStateChanged  = "state:changed"
	EvMessagesBatch = "messages:batch"
	EvTopicsChanged = "topics:changed"
	EvSubsChanged   = "subscriptions:changed"
)

type StateChanged struct {
	ProfileID string    `json:"profileId"`
	Seq       uint64    `json:"seq"`
	State     ConnState `json:"state"`
	Error     string    `json:"error,omitempty"` // populated for Failed / Reconnecting
	At        time.Time `json:"at"`
}

// MessagesBatch is emitted at most every 50 ms per session (see batcher.go)
// and only contains messages on topics the UI is watching. Dropped > 0 means
// the batcher's pending queue overflowed between ticks, i.e. the UI really did
// not see those messages in the stream (they may still be in history).
type MessagesBatch struct {
	ProfileID string    `json:"profileId"`
	Seq       uint64    `json:"seq"`
	Messages  []Message `json:"messages"`
	Dropped   uint64    `json:"dropped"`
}

// TopicsChanged carries only the delta since the last tick.
type TopicsChanged struct {
	ProfileID string       `json:"profileId"`
	Seq       uint64       `json:"seq"`
	Upserts   []TopicStats `json:"upserts"`
	Removed   []string     `json:"removed"` // full paths, e.g. after a clear
}

type SubscriptionsChanged struct {
	ProfileID string         `json:"profileId"`
	Seq       uint64         `json:"seq"`
	Active    []Subscription `json:"active"`
}

// ---- Snapshots returned by RPC ---------------------------------------------

// SessionSnapshot is everything the UI needs to render one session from
// scratch, consistent as of Seq.
type SessionSnapshot struct {
	ProfileID     string         `json:"profileId"`
	Seq           uint64         `json:"seq"`
	State         ConnState      `json:"state"`
	Error         string         `json:"error,omitempty"`
	Subscriptions []Subscription `json:"subscriptions"`
	Topics        []TopicStats   `json:"topics"`
}

// WatchResult is the history of one topic at the moment the UI started
// watching it, newest first. Live batches follow; the frontend reconciles
// the two by message ID alone (every later message has a larger ID), so
// no Seq is needed here.
type WatchResult struct {
	ProfileID string    `json:"profileId"`
	Topic     string    `json:"topic"`
	Messages  []Message `json:"messages"`
}
