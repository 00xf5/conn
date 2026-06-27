package session

import (
	"crypto/rand"
	"fmt"
	"strings"
	"sync"
	"time"
)

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
const codeLen = 6
const defaultTTL = 30 * time.Minute

type Session struct {
	Code      string    `json:"code"`
	DeviceID  string    `json:"deviceId"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type Store struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	byDevice map[string]string
}

func NewStore() *Store {
	return &Store{
		sessions: make(map[string]*Session),
		byDevice: make(map[string]string),
	}
}

func (s *Store) Create(deviceID string, ttl time.Duration) (*Session, error) {
	if ttl <= 0 {
		ttl = defaultTTL
	}
	code, err := randomCode(codeLen)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	sess := &Session{
		Code:      code,
		DeviceID:  deviceID,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[code] = sess
	if deviceID != "" {
		s.byDevice[deviceID] = code
	}
	return sess, nil
}

func (s *Store) Get(code string) (*Session, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	s.mu.RLock()
	sess, ok := s.sessions[code]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(sess.ExpiresAt) {
		s.Delete(code)
		return nil, false
	}
	return sess, true
}

func (s *Store) Delete(code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[code]; ok {
		delete(s.byDevice, sess.DeviceID)
		delete(s.sessions, code)
	}
}

func (s *Store) List() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Session, 0, len(s.sessions))
	now := time.Now()
	for _, sess := range s.sessions {
		if now.Before(sess.ExpiresAt) {
			out = append(out, sess)
		}
	}
	return out
}

func randomCode(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i := range b {
		out[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}
	return string(out), nil
}

func (s *Session) ViewerURL(base string) string {
	return fmt.Sprintf("%s/v/%s", base, s.Code)
}
