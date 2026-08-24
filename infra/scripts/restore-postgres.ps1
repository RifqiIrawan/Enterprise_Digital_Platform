<#
.SYNOPSIS
    Memulihkan satu database Postgres dari hasil backup-postgres.ps1 (Fase 12).

.DESCRIPTION
    Memulihkan SATU database ke SATU nama tujuan. Sengaja tidak ada mode
    "pulihkan semuanya sekaligus": pemulihan hampir selalu soal satu service
    yang rusak, dan perintah yang bisa menimpa 18 database dalam sekali jalan
    adalah perintah yang cepat atau lambat akan dijalankan orang di database
    yang salah.

    Tujuan yang SUDAH ADA tidak akan ditimpa tanpa -Force. Ini penjaga
    terpenting di skrip ini: memulihkan ke database yang sedang dipakai berarti
    menghapus data yang belum tentu perlu dihapus.

    Untuk LATIHAN pemulihan (yang wajib dilakukan berkala -- backup yang tidak
    pernah dipulihkan belum terbukti bisa dipulihkan), pulihkan ke nama lain,
    mis. -TargetDatabase finance_service_dr_drill, lalu bandingkan jumlah
    barisnya.

.EXAMPLE
    $env:PGPASSWORD = "platform"
    ./restore-postgres.ps1 -DumpFile ..\..\backups\2026-08-25_120000\finance_service.dump `
                           -TargetDatabase finance_service_dr_drill

.EXAMPLE
    # Pemulihan sungguhan, menimpa database yang sedang ada:
    ./restore-postgres.ps1 -DumpFile ...\finance_service.dump -TargetDatabase finance_service -Force
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$DumpFile,
    [Parameter(Mandatory = $true)][string]$TargetDatabase,
    [switch]$Force,
    [string]$PgHost = "localhost",
    [int]$PgPort = 5432,
    [string]$PgUser = "platform"
)

$ErrorActionPreference = "Stop"

if (-not $env:PGPASSWORD) {
    Write-Error "PGPASSWORD belum diisi. Contoh: `$env:PGPASSWORD = 'platform'"
    exit 1
}
foreach ($tool in @("psql", "pg_restore")) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        Write-Error "$tool tidak ada di PATH."
        exit 1
    }
}
if (-not (Test-Path $DumpFile)) {
    Write-Error "Berkas dump tidak ditemukan: $DumpFile"
    exit 1
}

# Kalau ada manifest.json di folder yang sama, arsipnya diperiksa dulu:
# memulihkan dari berkas yang rusak separuh menghasilkan database yang
# terlihat pulih padahal isinya tidak lengkap.
$manifestPath = Join-Path (Split-Path -Parent (Resolve-Path $DumpFile)) "manifest.json"
if (Test-Path $manifestPath) {
    $manifest = Get-Content $manifestPath -Raw | ConvertFrom-Json
    $entry = $manifest.databases | Where-Object { $_.file -eq (Split-Path -Leaf $DumpFile) }
    if ($entry -and $entry.sha256) {
        $actual = (Get-FileHash $DumpFile -Algorithm SHA256).Hash
        if ($actual -ne $entry.sha256) {
            Write-Error "sha256 dump tidak cocok dengan manifest -- berkas kemungkinan rusak atau tertukar."
            exit 1
        }
        Write-Host "sha256 cocok dengan manifest."
    }
}

$exists = & psql -h $PgHost -p $PgPort -U $PgUser -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname = '$TargetDatabase';"
if ($LASTEXITCODE -ne 0) {
    Write-Error "Gagal menghubungi Postgres di ${PgHost}:${PgPort}"
    exit 1
}

if ($exists -and $exists.Trim() -eq "1") {
    if (-not $Force) {
        Write-Error "Database '$TargetDatabase' sudah ada. Jalankan ulang dengan -Force kalau memang mau menimpanya, atau pilih nama lain untuk latihan pemulihan."
        exit 1
    }
    Write-Warning "Menimpa database '$TargetDatabase' yang sudah ada (-Force)."
    # Sesi lain harus diputus dulu, kalau tidak DROP DATABASE akan menggantung
    # menunggu koneksi yang mungkin tidak akan pernah tutup sendiri.
    $terminate = "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$TargetDatabase' AND pid <> pg_backend_pid();"
    & psql -h $PgHost -p $PgPort -U $PgUser -d postgres -c $terminate | Out-Null
    & psql -h $PgHost -p $PgPort -U $PgUser -d postgres -c "DROP DATABASE `"$TargetDatabase`";"
    if ($LASTEXITCODE -ne 0) { Write-Error "Gagal menghapus database lama."; exit 1 }
}

& psql -h $PgHost -p $PgPort -U $PgUser -d postgres -c "CREATE DATABASE `"$TargetDatabase`";"
if ($LASTEXITCODE -ne 0) { Write-Error "Gagal membuat database '$TargetDatabase'."; exit 1 }

$started = Get-Date
# --no-owner: dump dibuat oleh user 'platform' di mesin dev; di tujuan lain
# pemiliknya bisa berbeda, dan kepemilikan bukan bagian dari data.
& pg_restore -h $PgHost -p $PgPort -U $PgUser -d $TargetDatabase --no-owner --no-privileges $DumpFile
$restoreExit = $LASTEXITCODE
$seconds = [math]::Round(((Get-Date) - $started).TotalSeconds, 1)

# pg_restore mengembalikan 1 untuk peringatan yang tidak fatal juga; yang
# menentukan berhasil atau tidak adalah isi databasenya, jadi itu yang
# dilaporkan di bawah -- bukan cuma exit code-nya.
$countSql = "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public';"
$tableCount = (& psql -h $PgHost -p $PgPort -U $PgUser -d $TargetDatabase -tAc $countSql).Trim()

Write-Host ""
Write-Host "Pulih ke '$TargetDatabase' dalam ${seconds}s: $tableCount tabel di schema public."
if ($restoreExit -ne 0) {
    Write-Warning "pg_restore keluar dengan kode $restoreExit -- periksa pesan di atas. Jumlah tabel di atas tetap dihitung dari database hasil pemulihan."
}
Write-Host "Langkah berikutnya: bandingkan jumlah baris tabel penting dengan yang diharapkan sebelum memakai database ini."
