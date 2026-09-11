package services

import (
	"context"

	"mqttc/internal/mqtt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Snapshot is the single startup payload: everything the UI needs to render
// from scratch. Sessions only lists profiles that have a session; the UI
// treats the rest as disconnected.
type Snapshot struct {
	Profiles []Profile              `json:"profiles"`
	Sessions []mqtt.SessionSnapshot `json:"sessions"`
}

// SessionService is the RPC façade the frontend calls. All lifecycle logic
// lives in sessionManager; this type only adapts signatures for Wails.
type SessionService struct {
	profiles *ProfileService
	m        *sessionManager
}

func NewSessionService(profiles *ProfileService, dialer mqtt.Dialer) *SessionService {
	return newSessionService(profiles, dialer, nil)
}

// newSessionService lets tests inject a Sink; production gets the Wails one
// at startup.
func newSessionService(profiles *ProfileService, dialer mqtt.Dialer, sink mqtt.Sink) *SessionService {
	return &SessionService{profiles: profiles, m: newSessionManager(profiles, dialer, sink)}
}

func (s *SessionService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	if s.m.sink == nil {
		s.m.setSink(wailsSink{app: application.Get()})
	}
	return nil
}

func (s *SessionService) ServiceShutdown() error {
	s.m.closeAll()
	return nil
}

func (s *SessionService) Snapshot() Snapshot {
	return Snapshot{Profiles: s.profiles.List(), Sessions: s.m.snapshot()}
}

func (s *SessionService) Connect(ctx context.Context, profileID string) error {
	return s.m.connect(ctx, profileID)
}

// with runs fn against profileID's session. Operations that must have a
// session to mean anything (publish, subscribe) use it directly; view and
// teardown operations wrap it with ifPresent so an unknown session is a no-op.
func (s *SessionService) with(profileID string, fn func(mqtt.Session) error) error {
	sess, err := s.m.get(profileID)
	if err != nil {
		return err
	}
	return fn(sess)
}

func (s *SessionService) ifPresent(profileID string, fn func(mqtt.Session) error) error {
	if sess, err := s.m.get(profileID); err == nil {
		return fn(sess)
	}
	return nil
}

func (s *SessionService) Disconnect(ctx context.Context, profileID string) error {
	return s.ifPresent(profileID, func(sess mqtt.Session) error { return sess.Disconnect(ctx) })
}

func (s *SessionService) Subscribe(ctx context.Context, profileID string, sub mqtt.Subscription) error {
	return s.with(profileID, func(sess mqtt.Session) error { return sess.Subscribe(ctx, sub) })
}

func (s *SessionService) Unsubscribe(ctx context.Context, profileID, filter string) error {
	return s.with(profileID, func(sess mqtt.Session) error { return sess.Unsubscribe(ctx, filter) })
}

func (s *SessionService) Publish(ctx context.Context, profileID string, pub mqtt.Publish) error {
	return s.with(profileID, func(sess mqtt.Session) error { return sess.Publish(ctx, pub) })
}

// Watch starts streaming topic for profileID and returns its history.
func (s *SessionService) Watch(profileID, topic string, limit int) (res mqtt.WatchResult, err error) {
	err = s.with(profileID, func(sess mqtt.Session) error { res = sess.Watch(topic, limit); return nil })
	return res, err
}

func (s *SessionService) Unwatch(profileID, topic string) error {
	return s.ifPresent(profileID, func(sess mqtt.Session) error { sess.Unwatch(topic); return nil })
}

// Clear forgets history for one topic, or everything when topic is "".
func (s *SessionService) Clear(profileID, topic string) error {
	return s.ifPresent(profileID, func(sess mqtt.Session) error { sess.Clear(topic); return nil })
}
