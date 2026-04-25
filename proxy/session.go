package proxy

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"screen/configuration"
	"screen/logging"
	"screen/metrics"
	"sync"
)

const sessionCookie = "_pxs"

var sessionLog = logging.NewLogger("session")

type sessionStore interface {
	ensure(w http.ResponseWriter, r *http.Request) string
	verified(r *http.Request, key string) bool
	mark(id, key string)
	count() int
}

func newSessionStoreFromConfig(cfg configuration.RedisConfig) (sessionStore, error) {
	if cfg.URL == "" {
		return newSessionStore(), nil
	}
	return newRedisSessionStore(cfg.URL, cfg.KeyPrefix)
}

type memSessionStore struct {
	mu   sync.RWMutex
	data map[string]map[string]bool
}

func newSessionStore() *memSessionStore {
	return &memSessionStore{data: make(map[string]map[string]bool)}
}

func (s *memSessionStore) id(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return "", false
	}
	s.mu.RLock()
	_, ok := s.data[c.Value]
	s.mu.RUnlock()
	return c.Value, ok
}

func (s *memSessionStore) ensure(w http.ResponseWriter, r *http.Request) string {
	if id, ok := s.id(r); ok {
		sessionLog.Debugf("existing session %s", id[:8])
		return id
	}
	id := randHex()
	s.mu.Lock()
	s.data[id] = make(map[string]bool)
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	metrics.SessionsCreatedTotal.Inc()
	metrics.SessionsActive.Inc()
	sessionLog.Debugf("new session %s", id[:8])
	return id
}

func (s *memSessionStore) verified(r *http.Request, key string) bool {
	id, ok := s.id(r)
	if !ok {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := s.data[id][key]
	sessionLog.Debugf("session %s key=%s verified=%v", id[:8], key, result)
	return result
}

func (s *memSessionStore) mark(id, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data[id] == nil {
		s.data[id] = make(map[string]bool)
	}
	s.data[id][key] = true
	sessionLog.Debugf("session %s marked %s", id[:8], key)
}

func (s *memSessionStore) count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data)
}

func randHex() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
