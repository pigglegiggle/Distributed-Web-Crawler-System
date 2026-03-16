package main

import (
	"context"
	"log"
)

// Seeder is a separate process/container that pushes initial URLs into Redis.
func runSeeder(ctx context.Context, cfg Config, queue *RedisQueue) error {
	for _, u := range cfg.SeedURLs {
		added, err := queue.EnqueueIfNew(ctx, CrawlItem{URL: u, Depth: 0})
		if err != nil {
			return err
		}
		if added {
			log.Printf("seeded: %s", u)
		} else {
			log.Printf("already seen, skipped seed: %s", u)
		}
	}
	return nil
}
