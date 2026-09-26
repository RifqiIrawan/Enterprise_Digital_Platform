#!/usr/bin/env python3
"""Memastikan setiap env var yang DIBACA kode juga ADA di config deployment.

Kegagalan yang dicegah pemeriksaan ini sudah pernah terjadi di repo ini:
`OTLP_ENDPOINT` ditambahkan ke config.go, docker-compose, dan env example saat
tracing dibangun -- tapi tidak pernah masuk ke config Kustomize. Akibatnya di
K8s ke-21 service jatuh ke default `localhost:4318`, tidak menemukan collector
apa pun, lalu DIAM: exporter OTLP cuma mencatat kegagalan kirim, aplikasinya
tetap jalan, dan trace-nya hilang tanpa ada yang tahu.

Yang diperiksa untuk tiap service yang punya internal/config/config.go:

  1. Ada Dockerfile, entry docker-compose, manifest + config Kustomize,
     env example staging & production, dan entry di backend-ci.
  2. Setiap kunci yang dibaca `getEnv*("NAMA", ...)` muncul di config
     Kustomize DAN di kedua env example.

Kunci yang sengaja TIDAK datang dari ConfigMap (mis. JWT_SECRET, yang
diberikan lewat Secret `jwt-secret`) didaftarkan di SECRET_KEYS di bawah --
menaruh rahasia di ConfigMap justru yang salah.

Dijalankan di .github/workflows/infra-ci.yml. Exit 1 kalau ada selisih.
"""

from __future__ import annotations

import os
import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
SERVICE_DIRS = [REPO_ROOT / "backend" / "services", REPO_ROOT / "backend" / "modules"]
KUSTOMIZATION = REPO_ROOT / "infra" / "kubernetes" / "base" / "kustomization.yaml"
COMPOSE = REPO_ROOT / "infra" / "docker-compose.yml"
BACKEND_CI = REPO_ROOT / ".github" / "workflows" / "backend-ci.yml"
ENV_DIRS = {
    "staging": REPO_ROOT / "infra" / "environments" / "staging",
    "production": REPO_ROOT / "infra" / "environments" / "production",
}

# Kunci yang diberikan lewat Secret, bukan ConfigMap (lihat secretRef di
# infra/kubernetes/base/{auth-service,api-gateway,rag-service}.yaml).
# ANTHROPIC_API_KEY masuk daftar ini dengan alasan yang sama seperti
# JWT_SECRET: menaruh kunci API berbayar di ConfigMap -- yang bisa dibaca
# siapa pun yang punya akses baca namespace -- justru yang salah.
SECRET_KEYS = {"JWT_SECRET", "ANTHROPIC_API_KEY"}

ENV_READ = re.compile(r'getEnv(?:Int|Bool|Duration)?\(\s*"([A-Z0-9_]+)"')


def services() -> list[tuple[str, Path]]:
    found = []
    for base in SERVICE_DIRS:
        for path in sorted(base.iterdir()):
            if (path / "go.mod").exists():
                found.append((path.name, path))
    return found


def kustomize_literals(text: str, service: str) -> set[str]:
    match = re.search(
        rf"- name: {re.escape(service)}-config\n\s+literals:\n((?:\s+- .*\n)+)", text
    )
    if not match:
        return set()
    return {
        line.strip()[2:].split("=")[0]
        for line in match.group(1).splitlines()
        if line.strip().startswith("- ")
    }


def env_file_keys(path: Path) -> set[str]:
    if not path.exists():
        return set()
    keys = set()
    for line in path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            keys.add(line.split("=")[0])
    return keys


def main() -> int:
    kust = KUSTOMIZATION.read_text(encoding="utf-8")
    compose = COMPOSE.read_text(encoding="utf-8")
    ci = BACKEND_CI.read_text(encoding="utf-8")

    found = services()
    if len(found) < 10:
        # Penjaga bagi penjaganya sendiri: kalau struktur folder berubah,
        # pemeriksaan ini akan "lulus" tanpa memeriksa apa pun.
        print(f"GAGAL: hanya menemukan {len(found)} service di backend/ -- pembacaannya kemungkinan rusak")
        return 1

    problems: list[str] = []
    for name, path in found:
        config_go = path / "internal" / "config" / "config.go"
        if not config_go.exists():
            problems.append(f"{name}: tidak punya internal/config/config.go")
            continue

        # 1. kelengkapan berkas deployment
        if not (path / "deployments" / "Dockerfile").exists():
            problems.append(f"{name}: tidak ada deployments/Dockerfile")
        if f"\n  {name}:" not in compose:
            problems.append(f"{name}: tidak ada entry di infra/docker-compose.yml")
        if not (REPO_ROOT / "infra" / "kubernetes" / "base" / f"{name}.yaml").exists():
            problems.append(f"{name}: tidak ada manifest K8s")
        if f"- {name}.yaml" not in kust:
            problems.append(f"{name}: manifest K8s tidak terdaftar di kustomization.yaml")
        if not re.search(rf"-\s+(?:services|modules)/{re.escape(name)}\b", ci):
            problems.append(f"{name}: tidak ada entry di backend-ci.yml")

        # 2. kelengkapan kunci env
        read_keys = set(ENV_READ.findall(config_go.read_text(encoding="utf-8"))) - SECRET_KEYS
        missing_kust = sorted(read_keys - kustomize_literals(kust, name))
        if missing_kust:
            problems.append(f"{name}: kunci hilang di config Kustomize: {', '.join(missing_kust)}")
        for env_name, env_dir in ENV_DIRS.items():
            env_path = env_dir / f"{name}.env.example"
            if not env_path.exists():
                problems.append(f"{name}: tidak ada {env_name}/{name}.env.example")
                continue
            missing_env = sorted(read_keys - env_file_keys(env_path))
            if missing_env:
                problems.append(f"{name}: kunci hilang di {env_name}.env.example: {', '.join(missing_env)}")

    print(f"Service diperiksa: {len(found)}")
    if problems:
        print("\n".join(f"  - {p}" for p in problems))
        print(f"\nGAGAL: {len(problems)} selisih")
        return 1
    print("OK: seluruh service lengkap berkas deployment-nya, dan setiap env var yang dibaca kode ada di config Kustomize serta kedua env example")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
