# Otidxd Performance Report (local)

Date: 2026-02-01
Host: Windows (pwsh)
Repo: D:\xyad\worktrees\otidxd-daemon-v1
Query: import

Notes:
- indexer.Build workers default to NumCPU/2
- watch.start auto_tune enabled unless auto_tune=false
- sync_on_start=true in all successful runs below

## Dataset: D:\project\crawl4ai
Files: 14,446
Size: 844,607,815 bytes

### Store: sqlite (otidxd-perf-res.ps1)
| Phase | Duration ms | WS MB | Private MB | CPU s |
| --- | ---: | ---: | ---: | ---: |
| ping | 14.31 | 11.91 | 15.14 | 0.05 |
| index.build | 930.24 | 61.61 | 69.35 | 0.84 |
| query | 96.12 | 22.03 | 23.79 | 1.02 |
| watch.start(sync_on_start) | 159.70 | 42.80 | 45.07 | 1.16 |
| watch.update+query | 1,384.13 | 48.25 | 50.43 | 2.41 |

### Store: sqlite (optimized query path)
| Phase | Duration ms | WS MB | Private MB | CPU s |
| --- | ---: | ---: | ---: | ---: |
| ping | 11.36 | 12.04 | 15.40 | 0.00 |
| index.build | 933.71 | 61.98 | 70.44 | 1.03 |
| query | 34.04 | 21.72 | 23.97 | 1.16 |
| watch.start(sync_on_start) | 156.88 | 43.20 | 45.94 | 1.25 |
| watch.update+query | 6.42 | 42.78 | 45.38 | 1.25 |

### Store: sqlite (optimized query + early stop)
| Phase | Duration ms | WS MB | Private MB | CPU s |
| --- | ---: | ---: | ---: | ---: |
| ping | 11.47 | 11.94 | 15.43 | 0.00 |
| index.build | 967.59 | 61.06 | 69.01 | 1.28 |
| query | 32.09 | 21.98 | 24.16 | 1.58 |
| watch.start(sync_on_start) | 170.34 | 43.12 | 45.84 | 1.72 |
| watch.update+query | 6.58 | 42.71 | 45.28 | 1.72 |

### Store: bleve
- Timed out (index.build did not finish within ~4 minutes). Run aborted.

## Dataset: D:\project\PaddleOCR
Files: 1,930
Size: 1,668,917,944 bytes

### Store: sqlite (otidxd-perf-res.ps1)
| Phase | Duration ms | WS MB | Private MB | CPU s |
| --- | ---: | ---: | ---: | ---: |
| ping | 11.43 | 12.02 | 15.39 | 0.02 |
| index.build | 1,553.08 | 74.38 | 84.59 | 1.78 |
| query | 92.51 | 21.97 | 24.43 | 1.91 |
| watch.start(sync_on_start) | 785.31 | 77.45 | 80.12 | 2.19 |
| watch.update+query | 402.55 | 75.04 | 77.86 | 2.64 |

### Store: sqlite (optimized query + early stop)
| Phase | Duration ms | WS MB | Private MB | CPU s |
| --- | ---: | ---: | ---: | ---: |
| ping | 11.40 | 11.86 | 15.05 | 0.05 |
| index.build | 1,411.30 | 79.70 | 91.13 | 1.41 |
| query | 21.87 | 21.56 | 24.28 | 1.62 |
| watch.start(sync_on_start) | 675.65 | 74.29 | 77.09 | 2.02 |
| watch.update+query | 4.74 | 73.40 | 76.04 | 2.06 |

### Store: bleve
- Timed out (index.build did not finish within ~3 minutes). Run aborted.

## Observations
- sqlite completes index.build quickly on both datasets in this environment.
- watch.update+query is the longest phase for crawl4ai (large file count).
- bleve indexing is substantially slower on these datasets; runs did not complete within 3-4 minutes.
- sqlite query optimized by removing FTS snippet and avoiding chunk join; query latency dropped from ~96ms to ~34ms (crawl4ai).
- early stop after dedupe/limit cuts match scan cost further; query latency dropped to ~32ms (crawl4ai).

## Tunable Parameters
- watch.start.sync_on_start: boolean
- watch.start.sync_workers: default NumCPU/2, clamped to 1..NumCPU
- watch.start.debounce_ms: default 200ms
- watch.start.adaptive_debounce: default false
- watch.start.debounce_min_ms / debounce_max_ms: default 50/500 when adaptive
- watch.start.queue_mode: simple/priority/direct
- watch.start.auto_tune: default true (auto profile by file count + average size)
- query.show: if true, adds file read cost
- treesitter: build tag on/off; affects indexing CPU cost

## Large Repo Guidance (1-2 GB+)
- Keep sync_on_start off for hot reload unless you explicitly need full reconciliation.
- Use auto_tune for debounce/queue defaults; override only after measuring.
- On HDD or slow SSD, keep sync_workers <= NumCPU/2 to reduce IO contention.
- For latency-sensitive queries, keep show=false and fetch text on demand.
- If bleve remains too slow for initial indexing, prefer sqlite+fts5 or an external search service.

## Storage Alternatives (if sqlite/bleve insufficient)
- Tantivy (Rust full-text search engine library)
- Meilisearch (standalone search service)
- Typesense (standalone search service)
- Elasticsearch / OpenSearch (distributed search service)
- PostgreSQL full text search
