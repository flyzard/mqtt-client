package services

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"log"
	"sync"

	"mqttc/internal/mqtt"
)

// defaultSubscriptions is what a brand-new session subscribes to. Later
// subscriptions are carried across re-dials by the manager.
var defaultSubscriptions = []mqtt.Subscription{{Filter: "#", QoS: mqtt.QoS0}}

// sessionManager is the only place sessions are created, replaced and
// destroyed. A session is always built from the *current* profile: if the
// profile changed since the session was dialed, the manager closes it and
// dials a fresh one (carrying subscriptions over), so a user can never be
// talking to a broker they already edited away.
//
// Whether a Connect is legal is the session's decision (its transition
// table); the manager only decides whether to reuse or re-dial.
type sessionManager struct {
	profiles *ProfileService
	dialer   mqtt.Dialer

	mu       sync.Mutex
	sink     mqtt.Sink
	sessions map[string]*entry
}

type entry struct {
	sess mqtt.Session
	fp   string // fingerprint of the options the session was dialed with
}

func newSessionManager(profiles *ProfileService, dialer mqtt.Dialer, sink mqtt.Sink) *sessionManager {
	m := &sessionManager{profiles: profiles, dialer: dialer, sink: sink, sessions: map[string]*entry{}}
	profiles.onChange = m.onProfileChange
	return m
}

func (m *sessionManager) setSink(s mqtt.Sink) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sink = s
}

// optionsFor is the single definition of which profile fields reach the
// session. Defaults for the remaining knobs are the dialer's job.
func optionsFor(p Profile, password string) mqtt.Options {
	opts := mqtt.Options{
		ProfileID: p.ID, Broker: p.Broker, ClientID: p.ClientID,
		Username: p.Username, Password: []byte(password), CleanStart: p.CleanStart,
	}
	if _, useTLS, err := mqtt.ParseBroker(p.Broker); err == nil && useTLS {
		opts.TLS = &tls.Config{InsecureSkipVerify: p.Insecure} //nolint:gosec // user opted in for dev brokers
	}
	return opts
}

// fingerprint identifies the connection-relevant content of opts, so any
// field optionsFor sets automatically counts as relevant. Subscriptions are
// excluded because they are carried across re-dials rather than causing one.
func fingerprint(opts mqtt.Options) string {
	opts.Subscriptions = nil
	insecure := opts.TLS != nil && opts.TLS.InsecureSkipVerify
	opts.TLS = nil
	h := sha256.Sum256(fmt.Appendf(nil, "%+v|%t", opts, insecure))
	return hex.EncodeToString(h[:])
}

// dialLocked replaces (or creates) the session for opts. Caller holds mu.
// The old session, if any, is returned so the caller can close it outside
// the lock. Subscriptions are carried over from the old session.
func (m *sessionManager) dialLocked(opts mqtt.Options) (mqtt.Session, mqtt.Session, error) {
	fp := fingerprint(opts)
	var old mqtt.Session
	if e, ok := m.sessions[opts.ProfileID]; ok {
		old = e.sess
		opts.Subscriptions = old.Subscriptions()
	} else {
		opts.Subscriptions = defaultSubscriptions
	}
	sess, err := m.dialer.Dial(opts, m.sink)
	if err != nil {
		return nil, nil, err
	}
	m.sessions[opts.ProfileID] = &entry{sess: sess, fp: fp}
	return sess, old, nil
}

func (m *sessionManager) connect(ctx context.Context, profileID string) error {
	p, pw, err := m.profiles.withSecret(profileID)
	if err != nil {
		return err
	}
	opts := optionsFor(p, pw)

	m.mu.Lock()
	var sess, old mqtt.Session
	if e := m.sessions[profileID]; e != nil && e.fp == fingerprint(opts) {
		sess = e.sess
	} else if sess, old, err = m.dialLocked(opts); err != nil {
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()

	if old != nil {
		_ = old.Close()
	}
	return sess.Connect(ctx)
}

func (m *sessionManager) get(profileID string) (mqtt.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.sessions[profileID]
	if !ok {
		return nil, fmt.Errorf("profile %q is not connected", profileID)
	}
	return e.sess, nil
}

func (m *sessionManager) onProfileChange(ch ProfileChange) {
	if ch.Deleted {
		m.mu.Lock()
		e, ok := m.sessions[ch.Profile.ID]
		delete(m.sessions, ch.Profile.ID)
		m.mu.Unlock()
		if ok {
			_ = e.sess.Close()
		}
		return
	}

	m.mu.Lock()
	e, ok := m.sessions[ch.Profile.ID]
	m.mu.Unlock()
	if !ok {
		return
	}
	pw, err := m.profiles.secret(ch.Profile) // keychain I/O outside the lock
	if err != nil {
		log.Printf("sessions: profile %s changed but password unreadable: %v", ch.Profile.ID, err)
		return
	}
	opts := optionsFor(ch.Profile, pw)

	m.mu.Lock()
	if cur := m.sessions[ch.Profile.ID]; cur != e || cur.fp == fingerprint(opts) {
		m.mu.Unlock()
		return // cosmetic change (e.g. name), or the session changed under us
	}
	wasLive := e.sess.State().Live()
	sess, old, err := m.dialLocked(opts)
	m.mu.Unlock()
	if err != nil {
		log.Printf("sessions: re-dial after profile change: %v", err)
		return
	}
	_ = old.Close()
	if wasLive {
		if err := sess.Connect(context.Background()); err != nil {
			log.Printf("sessions: reconnect after profile change: %v", err)
		}
	}
}

func (m *sessionManager) snapshot() []mqtt.SessionSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]mqtt.SessionSnapshot, 0, len(m.sessions))
	for _, e := range m.sessions {
		out = append(out, e.sess.Snapshot())
	}
	return out
}

func (m *sessionManager) closeAll() {
	m.mu.Lock()
	all := m.sessions
	m.sessions = map[string]*entry{}
	m.mu.Unlock()
	for _, e := range all {
		_ = e.sess.Close()
	}
}
