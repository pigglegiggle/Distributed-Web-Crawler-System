# Distributed Web Crawler (Go + Redis + Docker)

This project is a small, readable example of how distributed crawling works.
It is built for learning, so the code stays practical and simple.

The system has three pieces:
- a **Seeder** that pushes starting URLs,
- one or more **Workers** that crawl pages,
- and a **Storage module** that writes results to JSON files.

Redis is used as the shared queue and coordination layer between worker processes.

## What This Project Shows

- Distributed workers consuming from one queue
- Concurrent crawling with Go goroutines
- URL deduplication with Redis Set
- Per-domain rate limiting
- Crawl depth scheduling (`MAX_DEPTH`)
- Metadata extraction (title, description, links)
- Simple structured output for later indexing or analysis

## Architecture

### Components

- `Seeder`: Adds initial URLs from `SEED_URLS` into Redis.
- `Worker`: Pulls URLs with `BLPOP`, crawls pages, extracts metadata, and pushes newly found links.
- `Storage`: Appends crawl results into JSON Lines files under `./data`.
- `Redis`: Stores queue items, dedupe set, and domain rate-limit keys.

### Text Diagram

```text
+------------------+           +------------------+
|  Seeder Service  |           |   Worker N       |
|  (ROLE=seeder)   |           | goroutines BLPOP |
+--------+---------+           +---------+--------+
         |                               |
         | RPUSH CrawlItem (url, depth)  |
         v                               |
      +--+-------------------------------+--+
      |              Redis                  |
      |  List: crawl:queue                  |
      |  Set:  crawl:seen (dedupe)          |
      |  Key:  crawl:domain:last:<domain>  |
      +--+-------------------------------+--+
         ^                               |
         | RPUSH discovered links        | Fetch HTML
+--------+---------+                     | Extract metadata
|   Worker 1       |                     | Store JSON
| goroutines BLPOP |---------------------+
+--------+---------+
         |
         v
+----------------------+
| data/results-*.jsonl |
+----------------------+
```

## Workflow (How It Actually Runs)

1. Seeder starts and reads `SEED_URLS`.
2. Each seed URL is added to Redis only if it has not been seen before.
3. Workers block on Redis with `BLPOP` and wait for the next URL.
4. A worker fetches the page and extracts:
   - page title
   - meta description
   - outgoing links
5. The result is written to a JSON Lines file.
6. Discovered links are re-queued with `depth + 1`.
7. Crawling stops expanding once `MAX_DEPTH` is reached.

Important behavior:
- `SEED_URLS` are starting points only.
- If `MAX_DEPTH` is greater than `0`, workers will crawl links discovered from those seeds.
- URL dedupe ensures the same URL is not crawled twice.

## Rate Limiting and Deduplication

- **Deduplication**: Redis `SADD` on `crawl:seen` means only new URLs enter the queue.
- **Rate limiting**: Redis `SETNX` with expiry on `crawl:domain:last:<domain>` limits how often the same domain is crawled.

This keeps the crawler from repeatedly hitting the same URL or hammering one domain too quickly.

## Setup and Run

### 1. Start Everything

```bash
docker compose up --build
```

### 2. Scale Workers (Optional)

```bash
docker compose up --build --scale worker=3
```

### 3. Stop Services

```bash
docker compose down
```

If you want a clean restart (new Redis data and empty results):

```bash
docker compose down -v
rm -rf data
docker compose up --build
```

## Configuration You Will Use Most

- `SEED_URLS`: Comma-separated starting URLs
- `MAX_DEPTH`: How far from seed pages to continue crawling
- `CONCURRENCY`: Goroutines per worker container
- `RATE_LIMIT_SECONDS`: Minimum gap between crawls for the same domain

## Where Results Are Stored

Results are written to:
- `./data/results-<worker-id>.jsonl`

Each line is a single JSON object, for example:

```json
{
  "url": "https://example.com",
  "depth": 0,
  "title": "Example Domain",
  "description": "...",
  "timestamp": "2026-03-16T12:00:00Z",
  "links": [
    "https://www.iana.org/domains/example"
  ]
}
```

## Project File Map

- `main.go`: app entrypoint and config loading
- `seeder.go`: pushes initial URLs into queue
- `worker.go`: worker loop and concurrent crawl workers
- `crawler.go`: HTTP fetch and HTML metadata extraction
- `queue.go`: Redis queue, dedupe, and rate limit logic
- `storage.go`: JSON Lines file writer
- `docker-compose.yml`: Redis + seeder + worker services
- `Dockerfile`: builds one binary for both roles

## Notes

This project is intentionally not production-heavy.
It skips features like robots.txt handling, retries with backoff, advanced observability, and autoscaling policies.

The goal is to clearly explain distributed crawling with a clean codebase you can read in one sitting.
