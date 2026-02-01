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

### After Optimization 3 (single writer + batch tx)
- ping: 9.37
- index.build: 1,320.98
- query: 422.77
- watch.start(sync_on_start): 149.27
- watch.update+query: 2,668.67

## Interpretation
- sync_on_start speedup is real (~49% faster), due to DB meta prefetch + per-worker store reuse + delete handling.
- query improved vs baseline (~31%).
- watch.update+query is still the largest latency contributor; sensitive to debounce + disk/SQLite locks.
- index.build is I/O bound; small variance is expected on Windows.
- single-writer + batch transaction improves sync_on_start latency, but may increase update+query tail latency under heavy watch churn (trade-off).

## CPU / Memory Snapshot (single run)
- ping: ~8.52 MB WS, 13.13 MB private, CPU ~0.03s
- index.build: ~82.20 MB WS, 94.25 MB private, CPU ~1.38s
- query: ~21.14 MB WS, 24.02 MB private, CPU ~1.80s
- watch.start(sync_on_start): ~38.94 MB WS, 41.86 MB private, CPU ~1.88s
- watch.update+query: ~41.41 MB WS, 44.11 MB private, CPU ~3.70s

## CPU / Memory Snapshot (single run, Optimization 3)
- ping: ~8.60 MB WS, 13.05 MB private, CPU ~0.00s
- index.build: ~63.88 MB WS, 73.16 MB private, CPU ~1.19s
- query: ~20.52 MB WS, 22.85 MB private, CPU ~1.73s
- watch.start(sync_on_start): ~34.23 MB WS, 36.66 MB private, CPU ~2.05s
- watch.update+query: ~36.93 MB WS, 39.41 MB private, CPU ~4.89s

Notes:
- ping is too short to sample accurately; values are best-effort.
- CPU seconds are cumulative across the process; use relative deltas for phase cost.

## Tunable Parameters
- watch.start.sync_on_start: boolean (default false)
- watch.start.sync_workers: default CPU/2, capped 1..NumCPU
- watch.start.debounce_ms: default 200ms
- watch.start.adaptive_debounce: default false
- watch.start.debounce_min_ms / debounce_max_ms: default 50/500 when adaptive
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
- Add fsnotify event coalescing for large file bursts.
## Optimization 4 (single writer + batch tx, defaults)
- ping: 9.37
- index.build: 1,369.88
- query: 566.90
- watch.start(sync_on_start): 154.29
- watch.update+query: 3,312.15

## Optimization 4 (single writer + batch tx, show=true)
- ping: 10.87
- index.build: 1,577.46
- query: 402.96
- watch.start(sync_on_start): 152.97
- watch.update+query: 2,595.08
## Optimization 5 (adaptive debounce, min=50 max=500)
- ping: 9.32
- index.build: 1,296.63
- query: 338.50
- watch.start(sync_on_start): 145.57
- watch.update+query: 2,248.54

## CPU / Memory Snapshot (single run, Optimization 5)
- ping: ~8.61 MB WS, 13.05 MB private, CPU ~0.03s
- index.build: ~64.09 MB WS, 74.23 MB private, CPU ~1.22s
- query: ~20.58 MB WS, 24.01 MB private, CPU ~1.66s
- watch.start(sync_on_start): ~38.09 MB WS, 41.50 MB private, CPU ~1.95s
- watch.update+query: ~40.98 MB WS, 44.36 MB private, CPU ~4.08s
## PaddleOCR (default)
Dataset:
- Files: 1,930
- Size: 1,668,917,944 bytes

Timings (ms):
- ping: 10.23
- index.build: 1,918.95
- query: 299.36
- watch.start(sync_on_start): 660.85
- watch.update+query: 1,601.75

## PaddleOCR (adaptive debounce, min=50 max=500)
Dataset:
- Files: 1,931
- Size: 1,694,980,792 bytes

Timings (ms):
- ping: 8.95
- index.build: 2,100.63
- query: 276.63
- watch.start(sync_on_start): 673.10
- watch.update+query: 1,974.15

## PaddleOCR CPU / Memory Snapshot (default)
- ping: ~8.94 MB WS, 13.07 MB private, CPU ~0.02s
- index.build: ~85.49 MB WS, 96.45 MB private, CPU ~2.03s
- query: ~22.95 MB WS, 25.44 MB private, CPU ~2.45s
- watch.start(sync_on_start): ~81.33 MB WS, 84.34 MB private, CPU ~2.91s
- watch.update+query: ~71.24 MB WS, 74.48 MB private, CPU ~5.36s
## PaddleOCR (default, async queue + single writer)
Dataset:
- Files: 1,932
- Size: 1,756,731,576 bytes

Timings (ms):
- ping: 11.71
- index.build: 2,184.59
- query: 628.43
- watch.start(sync_on_start): 669.70
- watch.update+query: 3,537.97

## PaddleOCR (adaptive debounce 50-500, async queue + single writer)
Dataset:
- Files: 1,934
- Size: 1,784,836,968 bytes

Timings (ms):
- ping: 9.41
- index.build: 2,336.45
- query: 584.84
- watch.start(sync_on_start): 676.48
- watch.update+query: 2,524.22
## PaddleOCR (default, priority+batch+dynamic queue)
Dataset:
- Files: 1,934
- Size: 1,814,850,288 bytes

Timings (ms):
- ping: 9.51
- index.build: 2,265.47
- query: 470.38
- watch.start(sync_on_start): 716.68
- watch.update+query: 3,260.35

## PaddleOCR (adaptive debounce 50-500, priority+batch+dynamic queue)
Dataset:
- Files: 1,934
- Size: 1,844,332,656 bytes

Timings (ms):
- ping: 9.20
- index.build: 2,151.57
- query: 520.48
- watch.start(sync_on_start): 700.57
- watch.update+query: 3,701.57
## PaddleOCR (auto-tuned)
Dataset:
- Files: 1,934
- Size: 1,870,477,424 bytes

Timings (ms):
- ping: 9.31
- index.build: 2,855.80
- query: 428.89
- watch.start(sync_on_start): 721.03
- watch.update+query: 1,891.71

Auto-tune decision:
- profile: large-files (count<5000, avg>512KB)
- debounce_ms=100, adaptive_debounce=false, debounce_min/max=50/200
- sync_workers=2
- queue tuning: smaller batches + shorter intervals
## PaddleOCR (auto-tuned, dynamic rate + hot/short-path priority)
Dataset:
- Files: 1,934
- Size: 1,903,657,720 bytes

Timings (ms):
- ping: 9.14
- index.build: 2,106.54
- query: 999.82
- watch.start(sync_on_start): 735.82
- watch.update+query: 3,805.94
## PaddleOCR (auto-tuned v2, dynamic rate + hot/short priority)
Dataset:
- Files: 1,934
- Size: 1,926,816,504 bytes

Timings (ms):
- ping: 11.04
- index.build: 2,312.67
- query: 918.70
- watch.start(sync_on_start): 745.31
- watch.update+query: 3,992.74

## PaddleOCR (manual: queue_mode=simple, debounce=100ms, sync_workers=2)
Dataset:
- Files: 1,934
- Size: 1,951,236,856 bytes

Timings (ms):
- ping: 10.21
- index.build: 2,146.82
- query: 921.72
- watch.start(sync_on_start): 753.60
- watch.update+query: 4,352.71
## PaddleOCR (auto-tuned v3: large-file uses simple queue)
Dataset:
- Files: 1,934
- Size: 2,052,899,576 bytes

Timings (ms):
- ping: 10.81
- index.build: 2,170.67
- query: 1,023.79
- watch.start(sync_on_start): 758.74
- watch.update+query: 4,724.01
