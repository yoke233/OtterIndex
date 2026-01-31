import argparse
import os
import re
import sys
from dataclasses import dataclass
from datetime import datetime
from typing import Optional


@dataclass(frozen=True)
class Case:
    name: str
    otidx_ms: int
    rg_ms: int


def parse_result_file(path: str) -> tuple[dict[str, str], list[Case]]:
    header: dict[str, str] = {}
    section: Optional[str] = None
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


def find_latest_result_file(root: str) -> Optional[str]:
    best_path: Optional[str] = None
    best_mtime: Optional[float] = None

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


def _xml_escape(s: str) -> str:
    return (
        s.replace("&", "&amp;")
        .replace("<", "&lt;")
        .replace(">", "&gt;")
        .replace('"', "&quot;")
        .replace("'", "&apos;")
    )


def render_svg(header: dict[str, str], cases: list[Case]) -> str:
    cases = sorted(cases, key=lambda c: max(c.otidx_ms, c.rg_ms), reverse=True)
    if not cases:
        raise ValueError("no cases")

    max_ms = max(max(c.otidx_ms, c.rg_ms) for c in cases)
    max_ms = max(max_ms, 1)

    # Layout
    margin_left = 80
    margin_right = 40
    margin_top = 80
    margin_bottom = 170

    group_w = 140
    bar_w = 42
    gap = 10

    chart_h = 420
    chart_w = group_w * len(cases)

    width = margin_left + chart_w + margin_right
    height = margin_top + chart_h + margin_bottom

    # Colors
    c_otidx = "#4C78A8"
    c_rg = "#F58518"
    c_grid = "#e5e5e5"
    c_axis = "#444"
    c_text = "#222"

    # Scale
    y_max = int(max_ms * 1.15) + 1

    def y_for(v: int) -> float:
        return margin_top + chart_h * (1.0 - (v / y_max))

    def h_for(v: int) -> float:
        return chart_h * (v / y_max)

    result_file = header.get("result_file", "")
    rg_version = header.get("rg_version", "")
    title = "otidx vs rg 速度对比（ms，越低越快）"
    subtitle = header.get("time", datetime.now().strftime("%Y-%m-%d %H:%M:%S"))
    if rg_version:
        subtitle += f" | {rg_version}"

    # SVG parts
    parts: list[str] = []
    parts.append('<?xml version="1.0" encoding="UTF-8"?>')
    parts.append(
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{height}" viewBox="0 0 {width} {height}">'
    )
    parts.append(
        '<style>'
        'text{font-family:Segoe UI, Microsoft YaHei, Arial, sans-serif;}'
        '</style>'
    )

    # Title
    parts.append(
        f'<text x="{margin_left}" y="36" font-size="20" fill="{c_text}">{_xml_escape(title)}</text>'
    )
    parts.append(
        f'<text x="{margin_left}" y="60" font-size="12" fill="#666">{_xml_escape(subtitle)}</text>'
    )

    # Legend
    legend_x = width - margin_right - 220
    legend_y = 30
    parts.append(f'<rect x="{legend_x}" y="{legend_y-10}" width="220" height="44" fill="white" opacity="0.9"/>')
    parts.append(f'<rect x="{legend_x+8}" y="{legend_y}" width="14" height="14" fill="{c_otidx}"/>')
    parts.append(
        f'<text x="{legend_x+28}" y="{legend_y+12}" font-size="12" fill="{c_text}">otidx (ms)</text>'
    )
    parts.append(f'<rect x="{legend_x+112}" y="{legend_y}" width="14" height="14" fill="{c_rg}"/>')
    parts.append(
        f'<text x="{legend_x+132}" y="{legend_y+12}" font-size="12" fill="{c_text}">rg (ms)</text>'
    )

    # Grid + y labels
    grid_lines = 6
    for i in range(grid_lines + 1):
        v = int(y_max * i / grid_lines)
        y = y_for(v)
        parts.append(
            f'<line x1="{margin_left}" y1="{y:.2f}" x2="{margin_left+chart_w}" y2="{y:.2f}" stroke="{c_grid}" stroke-width="1"/>'
        )
        parts.append(
            f'<text x="{margin_left-10}" y="{y+4:.2f}" font-size="11" text-anchor="end" fill="#555">{v}</text>'
        )

    # Axes
    parts.append(
        f'<line x1="{margin_left}" y1="{margin_top+chart_h}" x2="{margin_left+chart_w}" y2="{margin_top+chart_h}" stroke="{c_axis}" stroke-width="1.2"/>'
    )
    parts.append(
        f'<line x1="{margin_left}" y1="{margin_top}" x2="{margin_left}" y2="{margin_top+chart_h}" stroke="{c_axis}" stroke-width="1.2"/>'
    )

    # Bars
    for idx, c in enumerate(cases):
        gx = margin_left + idx * group_w
        base_y = margin_top + chart_h

        x_ot = gx + (group_w - (2 * bar_w + gap)) / 2
        x_rg = x_ot + bar_w + gap

        h_ot = h_for(c.otidx_ms)
        h_rg = h_for(c.rg_ms)
        y_ot = base_y - h_ot
        y_rg = base_y - h_rg

        parts.append(f'<rect x="{x_ot:.2f}" y="{y_ot:.2f}" width="{bar_w}" height="{h_ot:.2f}" fill="{c_otidx}"/>')
        parts.append(f'<rect x="{x_rg:.2f}" y="{y_rg:.2f}" width="{bar_w}" height="{h_rg:.2f}" fill="{c_rg}"/>')

        parts.append(
            f'<text x="{x_ot+bar_w/2:.2f}" y="{y_ot-6:.2f}" font-size="11" text-anchor="middle" fill="{c_text}">{c.otidx_ms}</text>'
        )
        parts.append(
            f'<text x="{x_rg+bar_w/2:.2f}" y="{y_rg-6:.2f}" font-size="11" text-anchor="middle" fill="{c_text}">{c.rg_ms}</text>'
        )

        # X labels (rotate for readability)
        label = c.name
        lx = gx + group_w / 2
        ly = margin_top + chart_h + 28
        parts.append(
            f'<text x="{lx:.2f}" y="{ly:.2f}" font-size="12" text-anchor="end" fill="{c_text}" transform="rotate(25 {lx:.2f} {ly:.2f})">{_xml_escape(label)}</text>'
        )

    # Footer
    footer = "说明：otidx 走 SQLite/FTS 索引；rg 为直接扫描文件。数值为脚本取多次运行中的最小 wall time（ms）。"
    parts.append(
        f'<text x="{margin_left}" y="{height-28}" font-size="11" fill="#666">{_xml_escape(footer)}</text>'
    )

    parts.append("</svg>")
    return "\n".join(parts) + "\n"


def main() -> int:
    ap = argparse.ArgumentParser(description="Render otidx vs rg benchmark to SVG (no dependencies).")
    ap.add_argument(
        "--in",
        dest="in_path",
        default="",
        help="input result-*.txt (default: latest in bench/out or cwd)",
    )
    ap.add_argument(
        "--out",
        dest="out_path",
        default="",
        help="output svg path (default: bench/docs/bench-vs-rg.svg)",
    )
    args = ap.parse_args()

    script_dir = os.path.dirname(os.path.abspath(__file__))
    default_result_root = os.path.join(script_dir, "out")
    result_root = default_result_root if os.path.isdir(default_result_root) else os.getcwd()
    in_path = args.in_path.strip()
    if not in_path:
        latest = find_latest_result_file(result_root)
        if not latest:
            print("找不到 result-*.txt（请先运行 bench/bench-vs-rg.ps1）", file=sys.stderr)
            return 2
        in_path = latest
    if not os.path.isabs(in_path):
        in_path = os.path.join(os.getcwd(), in_path)

    out_path = args.out_path.strip() or os.path.join(script_dir, "docs", "bench-vs-rg.svg")
    if not os.path.isabs(out_path):
        out_path = os.path.join(os.getcwd(), out_path)

    header, cases = parse_result_file(in_path)
    header.setdefault("result_file", os.path.basename(in_path))

    svg = render_svg(header, cases)
    out_dir = os.path.dirname(out_path)
    if out_dir:
        os.makedirs(out_dir, exist_ok=True)
    with open(out_path, "w", encoding="utf-8", newline="\n") as f:
        f.write(svg)

    print(out_path)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
