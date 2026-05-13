package cache

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/fulltank-garage/fulltankgarage-api/internal/config"
)

type Store struct {
	client *redis.Client
	ttl    time.Duration
}

func New(ctx context.Context, cfg config.Config) (*Store, error) {
	store := &Store{ttl: cfg.CacheTTL}
	if cfg.RedisAddr == "" {
		return store, nil
	}

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		if cfg.RedisRequired {
			return nil, err
		}

		log.Printf("redis disabled: %v", err)
		return store, nil
	}

	store.client = client
	return store, nil
}

func (s *Store) Enabled() bool {
	return s != nil && s.client != nil
}

func (s *Store) Client() *redis.Client {
	if !s.Enabled() {
		return nil
	}

	return s.client
}

func (s *Store) GetJSON(ctx context.Context, key string, dest any) (bool, error) {
	if !s.Enabled() {
		return false, nil
	}

	raw, err := s.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if err := json.Unmarshal(raw, dest); err != nil {
		return false, err
	}

	return true, nil
}

func (s *Store) SetJSON(ctx context.Context, key string, value any) error {
	if !s.Enabled() {
		return nil
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return s.client.Set(ctx, key, raw, s.ttl).Err()
}

func (s *Store) Delete(ctx context.Context, keys ...string) error {
	if !s.Enabled() || len(keys) == 0 {
		return nil
	}

	return s.client.Del(ctx, keys...).Err()
}
