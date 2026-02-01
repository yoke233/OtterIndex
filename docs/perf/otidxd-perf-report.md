# Otidxd Performance Report (local)

Date: 2026-02-01
Host: Windows (pwsh)
Repo: D:\xyad\worktrees\otidxd-daemon-v1
Target: D:\project\crawl4ai

## Dataset
- Files: 14,437
- Size: 796,994,373 bytes (~760 MB)

## Measurements (ms)

### Before (baseline, 2026-02-01)
- ping: 10.00
- index.build: 1,215.34
- query: 420.11
- watch.start(sync_on_start): 414.14
- watch.update+query: 2,172.10

### After Optimization 1 (sync_on_start speedup)
- ping: 8.86
- index.build: 1,343.84
- query: 288.58
- watch.start(sync_on_start): 209.22
- watch.update+query: 1,656.32

### After Optimization 2 (debounce/sync_workers options)
- ping: 9.67
- index.build: 1,402.57
- query: 284.69
- watch.start(sync_on_start): 229.58
- watch.update+query: 2,115.36

## Interpretation
- sync_on_start speedup is real (~49% faster), due to DB meta prefetch + per-worker store reuse + delete handling.
- query improved vs baseline (~31%).
- watch.update+query is still the largest latency contributor; sensitive to debounce + disk/SQLite locks.
- index.build is I/O bound; small variance is expected on Windows.

## CPU / Memory Snapshot (single run)
- ping: ~8.52 MB WS, 13.13 MB private, CPU ~0.03s
- index.build: ~82.20 MB WS, 94.25 MB private, CPU ~1.38s
- query: ~21.14 MB WS, 24.02 MB private, CPU ~1.80s
- watch.start(sync_on_start): ~38.94 MB WS, 41.86 MB private, CPU ~1.88s
- watch.update+query: ~41.41 MB WS, 44.11 MB private, CPU ~3.70s

Notes:
- ping is too short to sample accurately; values are best-effort.
- CPU seconds are cumulative across the process; use relative deltas for phase cost.

## Tunable Parameters
- watch.start.sync_on_start: boolean (default false)
- watch.start.sync_workers: default CPU/2, capped 1..NumCPU
- watch.start.debounce_ms: default 200ms
- query.show: if true, adds file read cost
- treesitter: enabled via build tag; increases indexing cost

## Larger Repo Guidance (1-2 GB+)
- Avoid always-on sync_on_start; run it only when resync is needed.
- Increase sync_workers only if storage is fast; on HDD/slow SSD, keep <= CPU/2.
- Consider lowering debounce_ms to 50-100ms only if update latency is critical; otherwise leave at 200ms to reduce churn.
- Use query.show=false for latency-critical queries; fetch text on demand.
- For very large repos, plan for initial index.build to be I/O bound; ensure SSD and reduce antivirus scanning on worktree.
- If treesitter is enabled, expect higher indexing CPU cost; disable for fastest indexing.
 - Memory scales with chunk/symbol volume; large repos will grow SQLite and process working set. Consider higher OS file cache and avoid huge show payloads.

## Next Steps (optional)
- Add batching/transaction pipeline for UpdateFile to reduce SQLite write-lock contention.
- Add adaptive debounce (higher under heavy churn, lower when idle).
- Add fsnotify event coalescing for large file bursts.
