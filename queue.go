package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// CrawlItem is a queue message: URL plus scheduling depth.
type CrawlItem struct {
	URL   string `json:"url"`
	Depth int    `json:"depth"`
}

// RedisQueue wraps queue, dedupe set, and simple domain rate-limit keys.
type RedisQueue struct {
	client        *redis.Client
	queueKey      string
	dedupeKey     string
	rateKeyPrefix string
}

func NewRedisQueue(addr, queueKey, dedupeKey, rateKeyPrefix string) *RedisQueue {
	client := redis.NewClient(&redis.Options{Addr: addr})
	return &RedisQueue{
		client:        client,
		queueKey:      queueKey,
		dedupeKey:     dedupeKey,
		rateKeyPrefix: rateKeyPrefix,
	}
}

func (q *RedisQueue) Ping(ctx context.Context) error {
	return q.client.Ping(ctx).Err()
}

// EnqueueIfNew performs URL deduplication through Redis SADD.
// Only newly seen URLs are pushed into the crawl queue.
func (q *RedisQueue) EnqueueIfNew(ctx context.Context, item CrawlItem) (bool, error) {
	normalized := strings.TrimSpace(item.URL)
	if normalized == "" {
		return false, nil
	}
	item.URL = normalized

	added, err := q.client.SAdd(ctx, q.dedupeKey, item.URL).Result()
	if err != nil {
		return false, err
	}
	if added == 0 {
		return false, nil
	}

	payload, err := json.Marshal(item)
	if err != nil {
		return false, err
	}
	if err := q.client.RPush(ctx, q.queueKey, payload).Err(); err != nil {
		return false, err
	}
	return true, nil
}

func (q *RedisQueue) Requeue(ctx context.Context, item CrawlItem) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return q.client.RPush(ctx, q.queueKey, payload).Err()
}

// Pop uses BLPOP so workers can wait for work without busy polling.
func (q *RedisQueue) Pop(ctx context.Context, timeout time.Duration) (*CrawlItem, error) {
	result, err := q.client.BLPop(ctx, timeout, q.queueKey).Result()
	if err != nil {
		return nil, err
	}
	if len(result) != 2 {
		return nil, fmt.Errorf("unexpected BLPOP response: %v", result)
	}
	var item CrawlItem
	if err := json.Unmarshal([]byte(result[1]), &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// AcquireDomainSlot ensures each domain is crawled at most once per interval.
// It uses SETNX + expiry, which is enough for educational distributed throttling.
func (q *RedisQueue) AcquireDomainSlot(ctx context.Context, rawURL string, interval time.Duration) (bool, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return false, fmt.Errorf("invalid url for rate limit: %s", rawURL)
	}
	key := q.rateKeyPrefix + ":" + strings.ToLower(u.Hostname())
	ok, err := q.client.SetNX(ctx, key, time.Now().Unix(), interval).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (q *RedisQueue) Close() error {
	return q.client.Close()
}
