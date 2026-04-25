package proxy

import (
	"context"
	"net/http"
	"screen/metrics"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	sessionTTL = 7 * 24 * time.Hour
	backTTL    = 30 * time.Minute
)

type redisSessionStore struct {
	client *redis.Client
	prefix string
}

func newRedisSessionStore(url, keyPrefix string) (*redisSessionStore, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	sessionLog.Infof("redis session store connected, key prefix %q", keyPrefix)
	return &redisSessionStore{client: client, prefix: keyPrefix}, nil
}

func (s *redisSessionStore) sessKey(id string) string {
	return s.prefix + "sess:" + id
}

func (s *redisSessionStore) ensure(w http.ResponseWriter, r *http.Request) string {
	c, err := r.Cookie(sessionCookie)
	if err == nil {
		ctx := context.Background()
		if n, _ := s.client.Exists(ctx, s.sessKey(c.Value)).Result(); n > 0 {
			sessionLog.Debugf("existing session %s", c.Value[:min(8, len(c.Value))])
			return c.Value
		}
	}
	id := randHex()
	ctx := context.Background()
	pipe := s.client.Pipeline()
	pipe.HSet(ctx, s.sessKey(id), "_", "1")
	pipe.Expire(ctx, s.sessKey(id), sessionTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		sessionLog.Errorf("redis ensure: %v", err)
	}
	setSessionCookie(w, id)
	metrics.SessionsCreatedTotal.Inc()
	metrics.SessionsActive.Inc()
	sessionLog.Debugf("new session %s", id[:8])
	return id
}

func (s *redisSessionStore) verified(r *http.Request, key string) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	ctx := context.Background()
	result, err := s.client.HExists(ctx, s.sessKey(c.Value), key).Result()
	if err != nil {
		sessionLog.Errorf("redis verified: %v", err)
		return false
	}
	sessionLog.Debugf("session %s key=%s verified=%v", c.Value[:min(8, len(c.Value))], key, result)
	return result
}

func (s *redisSessionStore) mark(id, key string) {
	ctx := context.Background()
	pipe := s.client.Pipeline()
	pipe.HSet(ctx, s.sessKey(id), key, "1")
	pipe.Expire(ctx, s.sessKey(id), sessionTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		sessionLog.Errorf("redis mark: %v", err)
	}
	sessionLog.Debugf("session %s marked %s", id[:8], key)
}

func (s *redisSessionStore) count() int {
	ctx := context.Background()
	var n int
	iter := s.client.Scan(ctx, 0, s.prefix+"sess:*", 100).Iterator()
	for iter.Next(ctx) {
		n++
	}
	if err := iter.Err(); err != nil {
		sessionLog.Errorf("redis count: %v", err)
	}
	return n
}

func (s *redisSessionStore) storeBack(url string) string {
	token := randHex()
	ctx := context.Background()
	if err := s.client.Set(ctx, s.prefix+"back:"+token, url, backTTL).Err(); err != nil {
		sessionLog.Errorf("redis storeBack: %v", err)
	}
	return token
}

func (s *redisSessionStore) loadBack(token string) string {
	ctx := context.Background()
	url, err := s.client.GetDel(ctx, s.prefix+"back:"+token).Result()
	if err != nil {
		if err != redis.Nil {
			sessionLog.Errorf("redis loadBack: %v", err)
		}
		return "/"
	}
	return url
}
