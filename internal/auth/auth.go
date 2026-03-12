package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

type SessionStore struct {
	mu sync.Map
}

func NewSessionStore() *SessionStore {
	return &SessionStore{}
}

func (s *SessionStore) CreateSession() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	s.mu.Store(token, struct{}{})
	return token, nil
}

func (s *SessionStore) ValidateSession(token string) bool {
	_, ok := s.mu.Load(token)
	return ok
}

func (s *SessionStore) DeleteSession(token string) {
	s.mu.Delete(token)
}
