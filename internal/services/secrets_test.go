package services

import "sync"

// MemSecrets is an in-memory SecretStore for tests.
type MemSecrets struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *MemSecrets) Get(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[id], nil
}

func (s *MemSecrets) Set(id, secret string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[id] = secret
	return nil
}

func (s *MemSecrets) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, id)
	return nil
}
