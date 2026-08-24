#!/usr/bin/env python3
"""Memastikan setiap metrik yang disebut aturan alert benar-benar diekspor.

promtool memeriksa apakah aturan alert SAH secara sintaksis; ia tidak tahu
apakah `http_request_duraton_seconds_bucket` (salah ketik) pernah ada. Alert
yang menyebut metrik yang tidak pernah diekspor tidak akan berbunyi selamanya,
dan diamnya akan dibaca sebagai "semuanya sehat" -- kegagalan yang paling mahal
justru karena tidak terlihat.

Pemeriksaan ini mengambil nama metrik dari infra/prometheus/rules/*.yml lalu
mencocokkannya dengan:

  * metrik buatan sendiri yang terdaftar di backend/**/internal/metrics/*.go
    (dibaca dari field `Name:` pada CounterOpts/HistogramOpts/GaugeOpts),
  * koleksi bawaan client_golang (awalan go_, process_, promhttp_),
  * metrik milik Prometheus sendiri (`up`, awalan scrape_/prometheus_/ALERTS).

Dijalankan di .github/workflows/monitoring-ci.yml. Keluar dengan status 1
kalau ada metrik yang tidak dikenali.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

try:
    import yaml
except ImportError:  # pragma: no cover
    sys.exit("butuh PyYAML: pip install pyyaml")

REPO_ROOT = Path(__file__).resolve().parents[2]
RULES_DIR = REPO_ROOT / "infra" / "prometheus" / "rules"
BACKEND_DIR = REPO_ROOT / "backend"

# Fungsi & kata kunci PromQL; identifier yang cocok dengan daftar ini bukan
# nama metrik.
PROMQL_WORDS = {
    "abs", "absent", "absent_over_time", "avg", "avg_over_time", "bottomk", "ceil",
    "changes", "clamp", "clamp_max", "clamp_min", "count", "count_over_time",
    "count_values", "day_of_month", "day_of_week", "days_in_month", "delta", "deriv",
    "exp", "floor", "group", "histogram_quantile", "holt_winters", "hour", "idelta",
    "increase", "irate", "label_join", "label_replace", "last_over_time", "ln",
    "log10", "log2", "max", "max_over_time", "min", "min_over_time", "minute",
    "month", "predict_linear", "present_over_time", "quantile", "quantile_over_time",
    "rate", "resets", "round", "scalar", "sgn", "sort", "sort_desc", "sqrt",
    "stddev", "stddev_over_time", "stdvar", "sum", "sum_over_time", "time",
    "timestamp", "topk", "vector", "year",
    # kata kunci / pengubah
    "by", "without", "on", "ignoring", "group_left", "group_right", "offset",
    "and", "or", "unless", "bool", "le", "start", "end", "atan2",
}

ALLOWED_PREFIXES = ("go_", "process_", "promhttp_", "scrape_", "prometheus_")
ALLOWED_EXACT = {"up", "ALERTS", "ALERTS_FOR_STATE"}
# Akhiran yang dihasilkan histogram/summary dari satu metrik yang sama.
DERIVED_SUFFIXES = ("_bucket", "_sum", "_count", "_total")

IDENTIFIER = re.compile(r"[A-Za-z_:][A-Za-z0-9_:]*")
LABEL_BLOCK = re.compile(r"\{[^}]*\}", re.S)
RANGE_SELECTOR = re.compile(r"\[[^\]]*\]")
# `sum by (service)` / `on (job)` / `group_left(code)`: isi kurungnya nama
# LABEL, bukan nama metrik.
GROUPING_LABELS = re.compile(
    r"\b(?:by|without|on|ignoring|group_left|group_right)\s*\([^)]*\)", re.S
)
# 1h, 30s, 10m -- durasi menyisakan huruf satuannya kalau tidak dibuang.
DURATION_LITERAL = re.compile(r"\b\d+(?:\.\d+)?(?:ms|s|m|h|d|w|y)\b")
# 1.5e9 -- eksponennya menyisakan "e9".
NUMBER_LITERAL = re.compile(r"\b\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\b")


def exported_metric_names() -> set[str]:
    """Nama metrik yang benar-benar didaftarkan kode Go."""
    names: set[str] = set()
    for path in BACKEND_DIR.rglob("internal/metrics/*.go"):
        for match in re.finditer(r'Name:\s*"([^"]+)"', path.read_text(encoding="utf-8")):
            names.add(match.group(1))
    return names


def referenced_metric_names(expr: str) -> set[str]:
    """Nama metrik yang disebut sebuah ekspresi PromQL."""
    # Buang dulu semua yang BUKAN nama metrik tapi terlihat seperti
    # identifier: blok label {service="x"}, rentang [10m], daftar label
    # pengelompokan by (service), durasi 1h, dan angka 1.5e9.
    cleaned = LABEL_BLOCK.sub(" ", expr)
    cleaned = RANGE_SELECTOR.sub(" ", cleaned)
    cleaned = GROUPING_LABELS.sub(" ", cleaned)
    cleaned = DURATION_LITERAL.sub(" ", cleaned)
    cleaned = NUMBER_LITERAL.sub(" ", cleaned)
    found = set()
    for token in IDENTIFIER.findall(cleaned):
        if token in PROMQL_WORDS:
            continue
        found.add(token)
    return found


def is_known(metric: str, exported: set[str]) -> bool:
    if metric in ALLOWED_EXACT or metric.startswith(ALLOWED_PREFIXES):
        return True
    if metric in exported:
        return True
    for suffix in DERIVED_SUFFIXES:
        if metric.endswith(suffix) and metric[: -len(suffix)] in exported:
            return True
    return False


def main() -> int:
    exported = exported_metric_names()
    if not exported:
        # Penjaga bagi penjaganya sendiri: kalau pembacaan kode Go gagal
        # (struktur folder berubah), pemeriksaan ini akan "lulus" tanpa
        # memeriksa apa pun.
        print("GAGAL: tidak menemukan satu pun metrik di backend/**/internal/metrics/*.go")
        return 1

    rule_files = sorted(RULES_DIR.glob("*.yml"))
    if not rule_files:
        print(f"GAGAL: tidak ada berkas aturan di {RULES_DIR}")
        return 1

    problems: list[str] = []
    checked = 0
    for path in rule_files:
        doc = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
        for group in doc.get("groups", []):
            for rule in group.get("rules", []):
                name = rule.get("alert") or rule.get("record") or "?"
                checked += 1
                for metric in sorted(referenced_metric_names(rule.get("expr", ""))):
                    if not is_known(metric, exported):
                        problems.append(f"{path.name}: {name}: metrik tak dikenal '{metric}'")

    print(f"Metrik yang diekspor kode: {', '.join(sorted(exported))}")
    print(f"Aturan diperiksa: {checked} dari {len(rule_files)} berkas")
    if problems:
        print("\n".join(problems))
        return 1
    print("OK: seluruh metrik yang disebut aturan alert benar-benar diekspor")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
