package main

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Worker represents one distributed worker process.
// Each worker process runs multiple goroutines for concurrent crawling.
type Worker struct {
	cfg     Config
	queue   *RedisQueue
	crawler *Crawler
	store   *FileStorage
}

func NewWorker(cfg Config, queue *RedisQueue, crawler *Crawler, store *FileStorage) *Worker {
	return &Worker{
		cfg:     cfg,
		queue:   queue,
		crawler: crawler,
		store:   store,
	}
}

func (w *Worker) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for i := 0; i < w.cfg.Concurrency; i++ {
		wg.Add(1)
		go func(workerSlot int) {
			defer wg.Done()
			w.loop(ctx, workerSlot)
		}(i)
	}
	wg.Wait()
}

func (w *Worker) loop(ctx context.Context, workerSlot int) {
	for {
		item, err := w.queue.Pop(ctx, time.Duration(w.cfg.BLPopTimeoutSec)*time.Second)
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue
			}
			log.Printf("slot=%d pop error: %v", workerSlot, err)
			time.Sleep(1 * time.Second)
			continue
		}

		if item.Depth > w.cfg.MaxDepth {
			continue
		}

		acquired, err := w.queue.AcquireDomainSlot(ctx, item.URL, time.Duration(w.cfg.RateLimitSeconds)*time.Second)
		if err != nil {
			log.Printf("slot=%d rate-limit key error for %s: %v", workerSlot, item.URL, err)
			continue
		}
		if !acquired {
			// If another worker recently hit this domain, put it back for later.
			if err := w.queue.Requeue(ctx, *item); err != nil {
				log.Printf("slot=%d requeue failed: %v", workerSlot, err)
			}
			time.Sleep(200 * time.Millisecond)
			continue
		}

		result, err := w.crawler.FetchAndExtract(ctx, item.URL)
		if err != nil {
			log.Printf("slot=%d crawl failed for %s: %v", workerSlot, item.URL, err)
			continue
		}
		result.Depth = item.Depth

		if err := w.store.Save(ctx, *result); err != nil {
			log.Printf("slot=%d save failed for %s: %v", workerSlot, item.URL, err)
		}

		nextDepth := item.Depth + 1
		if nextDepth > w.cfg.MaxDepth {
			continue
		}

		for _, link := range result.Links {
			_, err := w.queue.EnqueueIfNew(ctx, CrawlItem{URL: link, Depth: nextDepth})
			if err != nil {
				log.Printf("slot=%d enqueue discovered link failed: %v", workerSlot, err)
			}
		}

		log.Printf("slot=%d crawled depth=%d url=%s links=%d", workerSlot, item.Depth, item.URL, len(result.Links))
	}
}
