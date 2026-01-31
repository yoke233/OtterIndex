import argparse
import os
import re
import sys
from dataclasses import dataclass
from datetime import datetime


@dataclass(frozen=True)
class Case:
    name: str
    otidx_ms: int
    rg_ms: int


def parse_result_file(path: str) -> tuple[dict[str, str], list[Case]]:
    header: dict[str, str] = {}
    section: str | None = None
    sections: dict[str, dict[str, str]] = {}

    with open(path, "r", encoding="utf-8") as f:
        for raw_line in f:
            line = raw_line.rstrip("\r\n")
            if not line:
                continue

            m = re.match(r"^---\s+(.+?)\s+---$", line)
            if m:
                section = m.group(1)
                sections.setdefault(section, {})
                continue

            if ":" not in line:
                continue

            key, value = line.split(":", 1)
            key = key.strip()
            value = value.strip()
            if not key:
                continue

            if section is None:
                header[key] = value
            else:
                sections[section][key] = value

    cases: list[Case] = []
    for name, kv in sections.items():
        if "otidx_wall_ms_min" not in kv or "rg_wall_ms_min" not in kv:
            continue
        try:
            otidx_ms = int(kv["otidx_wall_ms_min"])
            rg_ms = int(kv["rg_wall_ms_min"])
        except ValueError:
            continue
        cases.append(Case(name=name, otidx_ms=otidx_ms, rg_ms=rg_ms))

    return header, cases


def find_latest_result_file(root: str) -> str | None:
    best_path: str | None = None
    best_mtime: float | None = None

    for name in os.listdir(root):
        if not name.startswith("result-") or not name.endswith(".txt"):
            continue
        path = os.path.join(root, name)
        try:
            st = os.stat(path)
        except OSError:
            continue
        if best_mtime is None or st.st_mtime > best_mtime:
            best_mtime = st.st_mtime
            best_path = path

    return best_path


def default_out_path(in_path: str) -> str:
    base, _ = os.path.splitext(in_path)
    return base + ".png"


def main() -> int:
    ap = argparse.ArgumentParser(description="Plot otidx vs rg benchmark result-*.txt")
    ap.add_argument(
        "--in",
        dest="in_path",
        default="",
        help="input result-*.txt (default: latest in bench/out or cwd)",
    )
    ap.add_argument("--out", dest="out_path", default="", help="output png path (default: same name .png)")
    args = ap.parse_args()

    script_dir = os.path.dirname(os.path.abspath(__file__))
    default_result_root = os.path.join(script_dir, "out")
    root = default_result_root if os.path.isdir(default_result_root) else os.getcwd()
    in_path = args.in_path.strip()
    if not in_path:
        latest = find_latest_result_file(root)
        if not latest:
            print("找不到 result-*.txt（请先运行 bench/bench-vs-rg.ps1）", file=sys.stderr)
            return 2
        in_path = latest
    if not os.path.isabs(in_path):
        in_path = os.path.join(os.getcwd(), in_path)

    out_path = args.out_path.strip() or default_out_path(in_path)
    if not os.path.isabs(out_path):
        out_path = os.path.join(os.getcwd(), out_path)

    header, cases = parse_result_file(in_path)
    if not cases:
        print(f"没有解析到可对比的 case（需要包含 otidx_wall_ms_min / rg_wall_ms_min）：{in_path}", file=sys.stderr)
        return 3

    try:
        import matplotlib.pyplot as plt
    except Exception as e:
        print("缺少 matplotlib，先安装：python -m pip install matplotlib", file=sys.stderr)
        print(f"import error: {e}", file=sys.stderr)
        return 4

    # Best-effort Chinese font on Windows; safe fallback if unavailable.
    try:
        from matplotlib import font_manager

        preferred = ["Microsoft YaHei", "SimHei", "Noto Sans CJK SC", "Arial Unicode MS"]
        available = {f.name for f in font_manager.fontManager.ttflist}
        for name in preferred:
            if name in available:
                plt.rcParams["font.sans-serif"] = [name] + plt.rcParams.get("font.sans-serif", [])
                break
        plt.rcParams["axes.unicode_minus"] = False
    except Exception:
        pass

    cases_sorted = sorted(cases, key=lambda c: max(c.otidx_ms, c.rg_ms), reverse=True)
    labels = [c.name for c in cases_sorted]
    otidx_vals = [c.otidx_ms for c in cases_sorted]
    rg_vals = [c.rg_ms for c in cases_sorted]

    n = len(cases_sorted)
    fig_w = min(22.0, max(10.0, 1.4 * n + 6.0))
    fig_h = 6.5
    fig, ax = plt.subplots(figsize=(fig_w, fig_h))

    x = list(range(n))
    width = 0.38
    b1 = ax.bar([i - width / 2 for i in x], otidx_vals, width=width, label="otidx (ms)")
    b2 = ax.bar([i + width / 2 for i in x], rg_vals, width=width, label="rg (ms)")

    def annotate(bars, vals):
        for bar, v in zip(bars, vals, strict=True):
            ax.text(
                bar.get_x() + bar.get_width() / 2,
                bar.get_height(),
                str(v),
                ha="center",
                va="bottom",
                fontsize=9,
                rotation=0,
            )

    annotate(b1, otidx_vals)
    annotate(b2, rg_vals)

    rg_version = header.get("rg_version", "")
    title = "otidx vs rg 速度对比（ms，越低越快）"
    subtitle = f"{os.path.basename(in_path)}"
    if rg_version:
        subtitle += f" | {rg_version}"

    ax.set_title(title + "\n" + subtitle, fontsize=13)
    ax.set_ylabel("耗时 (ms)")
    ax.set_xticks(x)
    ax.set_xticklabels(labels, rotation=25, ha="right")
    ax.grid(axis="y", linestyle="--", alpha=0.35)
    ax.legend()

    # Slight headroom for labels.
    max_y = max(max(otidx_vals), max(rg_vals))
    ax.set_ylim(0, max_y * 1.25 + 1)

    fig.tight_layout()
    out_dir = os.path.dirname(out_path)
    if out_dir:
        os.makedirs(out_dir, exist_ok=True)
    fig.savefig(out_path, dpi=160)

    print(out_path)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
