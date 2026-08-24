<#
.SYNOPSIS
    Mem-backup seluruh database Postgres milik platform ini (Fase 12).

.DESCRIPTION
    Setiap service punya database sendiri (18 buah: auth_service, rbac_service,
    finance_service, ... lihat infra/README.md), jadi "backup platform" berarti
    18 dump terpisah -- bukan satu dump raksasa. Dump per database membuat
    pemulihan bisa dilakukan per service: kalau yang rusak cuma finance_service,
    tidak ada alasan menyentuh 17 database lain.

    Format dump: -Fc (custom, terkompresi, bisa di-restore per tabel dengan
    pg_restore). Bukan SQL polos, karena SQL polos berukuran jauh lebih besar
    dan pemulihan sebagiannya harus dikerjakan dengan editor teks.

    DATABASE _test SENGAJA DILEWATI. Isinya dibuat ulang setiap kali `go test`
    dijalankan; mem-backup-nya hanya membuat arsip membengkak tanpa menambah
    kemampuan pulih apa pun.

    Skrip ini GAGAL (exit 1) kalau ada satu saja database yang gagal di-dump.
    Backup yang melaporkan sukses padahal sebagian gagal lebih berbahaya
    daripada tidak ada backup sama sekali: yang pertama membuat orang berhenti
    khawatir.

.PARAMETER OutputRoot
    Folder induk hasil backup. Default: <repo>/backups (sudah di .gitignore --
    dump berisi data sungguhan dan tidak boleh masuk git).

.PARAMETER KeepDays
    Hapus folder backup yang lebih tua dari sekian hari. Default 7.

.EXAMPLE
    $env:PGPASSWORD = "platform"
    ./backup-postgres.ps1

.NOTES
    Password dibaca dari environment variable PGPASSWORD, tidak pernah dari
    parameter -- parameter tercatat di riwayat shell.
#>
[CmdletBinding()]
param(
    [string]$OutputRoot = (Join-Path $PSScriptRoot "..\..\backups"),
    [int]$KeepDays = 7,
    [string]$PgHost = "localhost",
    [int]$PgPort = 5432,
    [string]$PgUser = "platform"
)

$ErrorActionPreference = "Stop"

if (-not $env:PGPASSWORD) {
    Write-Error "PGPASSWORD belum diisi. Contoh: `$env:PGPASSWORD = 'platform'"
    exit 1
}

foreach ($tool in @("psql", "pg_dump")) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        Write-Error "$tool tidak ada di PATH. Tambahkan folder bin PostgreSQL (mis. C:\Program Files\PostgreSQL\18\bin)."
        exit 1
    }
}

# Daftar database ditemukan dari server, BUKAN ditulis tetap di skrip ini:
# modul baru datang dengan database baru (crm_service, fleet_service,
# project_service, ... semuanya menyusul belakangan), dan daftar tetap akan
# diam-diam melewatkan yang terbaru -- persis database yang paling belum
# tentu ada salinannya di tempat lain.
$listSql = "SELECT datname FROM pg_database WHERE datname LIKE '%\_service' AND NOT datistemplate ORDER BY 1;"
$databases = & psql -h $PgHost -p $PgPort -U $PgUser -d postgres -tAc $listSql
if ($LASTEXITCODE -ne 0) {
    Write-Error "Gagal menghubungi Postgres di ${PgHost}:${PgPort}"
    exit 1
}
$databases = $databases | Where-Object { $_ -and $_.Trim() -ne "" } | ForEach-Object { $_.Trim() }

if ($databases.Count -eq 0) {
    # Penjaga bagi penjaganya sendiri: kalau polanya tidak cocok lagi (mis.
    # penamaan database berubah), skrip ini akan "berhasil" mem-backup nol
    # database dan tidak ada yang tahu sampai butuh memulihkan.
    Write-Error "Tidak ada database yang cocok pola '%_service'. Periksa penamaan database sebelum mempercayai backup ini."
    exit 1
}

$stamp = Get-Date -Format "yyyy-MM-dd_HHmmss"
$targetDir = Join-Path $OutputRoot $stamp
New-Item -ItemType Directory -Force -Path $targetDir | Out-Null

Write-Host "Backup $($databases.Count) database ke $targetDir"
$results = @()
$failed = 0

foreach ($db in $databases) {
    $file = Join-Path $targetDir "$db.dump"
    $started = Get-Date
    & pg_dump -h $PgHost -p $PgPort -U $PgUser -d $db -Fc -f $file
    $ok = ($LASTEXITCODE -eq 0)
    $seconds = [math]::Round(((Get-Date) - $started).TotalSeconds, 1)
    $size = if (Test-Path $file) { (Get-Item $file).Length } else { 0 }

    if ($ok) {
        Write-Host ("  OK   {0,-22} {1,8:N0} KB  {2}s" -f $db, ($size / 1KB), $seconds)
    } else {
        Write-Warning ("  GAGAL {0}" -f $db)
        $failed++
    }
    $results += [pscustomobject]@{
        database   = $db
        file       = "$db.dump"
        size_bytes = $size
        seconds    = $seconds
        ok         = $ok
        sha256     = if ($ok -and (Test-Path $file)) { (Get-FileHash $file -Algorithm SHA256).Hash } else { $null }
    }
}

# Manifest ikut disimpan supaya pemulihan nanti bisa memeriksa arsipnya utuh
# (sha256) tanpa harus mempercayai nama berkas saja.
$manifest = [pscustomobject]@{
    created_at    = (Get-Date).ToString("o")
    host          = $PgHost
    port          = $PgPort
    database_count = $databases.Count
    failed_count  = $failed
    databases     = $results
}
$manifest | ConvertTo-Json -Depth 5 | Set-Content -Path (Join-Path $targetDir "manifest.json") -Encoding utf8

# Retensi: hanya folder yang benar-benar berpola tanggal yang dihapus, supaya
# salah ketik -OutputRoot tidak berujung menghapus isi folder lain.
if ($KeepDays -gt 0 -and (Test-Path $OutputRoot)) {
    $cutoff = (Get-Date).AddDays(-$KeepDays)
    Get-ChildItem -Path $OutputRoot -Directory |
        Where-Object { $_.Name -match '^\d{4}-\d{2}-\d{2}_\d{6}$' -and $_.CreationTime -lt $cutoff } |
        ForEach-Object {
            Write-Host "  hapus backup lama: $($_.Name)"
            Remove-Item -Recurse -Force $_.FullName
        }
}

$totalMB = [math]::Round((($results | Measure-Object -Property size_bytes -Sum).Sum / 1MB), 1)
Write-Host ""
Write-Host "Selesai: $($databases.Count - $failed)/$($databases.Count) database, total $totalMB MB di $targetDir"

if ($failed -gt 0) {
    Write-Error "$failed database gagal di-backup."
    exit 1
}
