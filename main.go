package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config keeps runtime settings simple and environment-driven so containers are easy to wire.
type Config struct {
	Role              string
	RedisAddr         string
	QueueKey          string
	DedupeKey         string
	RateKeyPrefix     string
	StorageDir        string
	WorkerID          string
	Concurrency       int
	MaxDepth          int
	RateLimitSeconds  int
	BLPopTimeoutSec   int
	RequestTimeoutSec int
	SeedURLs          []string
}

func main() {
	cfg := loadConfig()
	ctx := context.Background()

	queue := NewRedisQueue(cfg.RedisAddr, cfg.QueueKey, cfg.DedupeKey, cfg.RateKeyPrefix)
	defer queue.Close()

	if err := waitForRedis(ctx, queue, 10, 2*time.Second); err != nil {
		log.Fatalf("redis not ready: %v", err)
	}

	switch cfg.Role {
	case "seeder":
		if err := runSeeder(ctx, cfg, queue); err != nil {
			log.Fatalf("seeder failed: %v", err)
		}
		log.Println("seeding complete")
	default:
		storage, err := NewFileStorage(cfg.StorageDir, cfg.WorkerID)
		if err != nil {
			log.Fatalf("storage init failed: %v", err)
		}
		defer storage.Close()

		crawler := NewCrawler(time.Duration(cfg.RequestTimeoutSec) * time.Second)
		worker := NewWorker(cfg, queue, crawler, storage)

		log.Printf("worker %s started with %d goroutines", cfg.WorkerID, cfg.Concurrency)
		worker.Run(ctx)
	}
}

func loadConfig() Config {
	role := getEnv("ROLE", "worker")
	if len(os.Args) > 1 {
		role = os.Args[1]
	}

	seedURLs := []string{}
	for _, u := range strings.Split(getEnv("SEED_URLS", "https://example.com"), ",") {
		u = strings.TrimSpace(u)
		if u != "" {
			seedURLs = append(seedURLs, u)
		}
	}

	return Config{
		Role:              role,
		RedisAddr:         getEnv("REDIS_ADDR", "redis:6379"),
		QueueKey:          getEnv("QUEUE_KEY", "crawl:queue"),
		DedupeKey:         getEnv("DEDUPE_KEY", "crawl:seen"),
		RateKeyPrefix:     getEnv("RATE_KEY_PREFIX", "crawl:domain:last"),
		StorageDir:        getEnv("STORAGE_DIR", "./data"),
		WorkerID:          getEnv("WORKER_ID", hostnameOrDefault()),
		Concurrency:       getEnvAsInt("CONCURRENCY", 4),
		MaxDepth:          getEnvAsInt("MAX_DEPTH", 2),
		RateLimitSeconds:  getEnvAsInt("RATE_LIMIT_SECONDS", 3),
		BLPopTimeoutSec:   getEnvAsInt("BLPOP_TIMEOUT_SECONDS", 5),
		RequestTimeoutSec: getEnvAsInt("REQUEST_TIMEOUT_SECONDS", 10),
		SeedURLs:          seedURLs,
	}
}

func waitForRedis(ctx context.Context, queue *RedisQueue, attempts int, delay time.Duration) error {
	var err error
	for i := 0; i < attempts; i++ {
		err = queue.Ping(ctx)
		if err == nil {
			return nil
		}
		log.Printf("waiting for redis (%d/%d): %v", i+1, attempts, err)
		time.Sleep(delay)
	}
	return err
}

func hostnameOrDefault() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "worker-unknown"
	}
	return h
}

func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func getEnvAsInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}
