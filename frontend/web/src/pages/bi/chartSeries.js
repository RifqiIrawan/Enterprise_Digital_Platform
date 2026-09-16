// Definisi seri chart (nama field, label, warna) yang dipakai BI Dashboards dan
// keenam dashboard per peran (Fase 9). Ditaruh di modul tersendiri justru KARENA
// isinya keputusan warna: dua halaman yang menyalin daftar ini akan menyimpang
// pelan-pelan, dan "biru = masuk" di satu layar tapi bukan di layar sebelahnya
// adalah kebingungan yang mahal untuk penghematan yang tidak ada.
//
// Komentar di bawah dipindahkan apa adanya dari BIDashboardsPage.jsx, tempat
// keputusan-keputusan ini pertama kali diambil.

// Dua dari tiga chart bulanan di halaman ini sama-sama dikotomi "masuk vs
// keluar" -- slot 1 (biru) selalu sisi masuk (Revenue, Stock In), slot 2
// (oranye) selalu sisi keluar (Expense, Stock Out). Urutan tetap ini SENGAJA
// dipakai ulang (bukan warna baru per chart) supaya pembaca dashboard
// belajar polanya sekali: biru = masuk, oranye = keluar, di seluruh halaman
// ini. Sales value bukan dikotomi (cuma satu ukuran magnitude, bukan
// dua sisi berlawanan) jadi cuma satu seri -- tetap slot 1 biru yang sama,
// bukan warna baru, karena tidak ada sisi "keluar" untuk dikontraskan.
export const FINANCE_SERIES = [
  { key: 'revenue', label: 'Revenue', color: 'var(--bs-primary)' },
  { key: 'expense', label: 'Expense', color: 'var(--bs-orange)' },
]
export const STOCK_SERIES = [
  { key: 'stock_in', label: 'Stock In', color: 'var(--bs-primary)' },
  { key: 'stock_out', label: 'Stock Out', color: 'var(--bs-orange)' },
]
export const SALES_SERIES = [{ key: 'sales_value', label: 'Sales Value', color: 'var(--bs-primary)' }]

// Pipeline CRM SENGAJA tidak memakai pasangan biru/oranye di atas. Weighted
// value bukan lawan dari total value, tapi BAGIAN dari total itu (total
// dikalikan probability tiap deal) -- memberinya warna kategorikal kedua akan
// terbaca sebagai dua hal yang berlawanan, persis makna yang sudah dipakai
// biru/oranye di dua chart pertama. Satu hue yang sama dengan opacity lebih
// rendah adalah encoding yang jujur untuk "himpunan bagian dari bar
// sebelahnya", dan tidak mengotori konvensi masuk-vs-keluar halaman ini.
export const CRM_PIPELINE_SERIES = [
  { key: 'total_amount', label: 'Nilai Total', color: 'var(--bs-primary)' },
  { key: 'weighted_amount', label: 'Nilai Terbobot (× probability)', color: 'rgba(var(--bs-primary-rgb), 0.4)' },
]

// Biaya proyek: satu ukuran magnitude (rupiah yang sudah masuk GL), bukan
// dikotomi masuk-vs-keluar dan bukan subset dari bar sebelahnya -- jadi satu
// seri, slot 1 biru yang sama seperti Sales Value. Jam kerja SENGAJA tidak
// dijadikan seri kedua di chart ini: satuannya beda (jam vs rupiah), dan dua
// satuan pada satu sumbu Y membuat tinggi bar-nya tidak bisa dibandingkan.
// Angkanya tetap tersedia lewat tooltip di kolom lain kalau nanti dibutuhkan.
export const PROJECT_COST_SERIES = [{ key: 'posted_amount', label: 'Biaya Diposting', color: 'var(--bs-primary)' }]

// Pengiriman selesai vs dibatalkan. Ini BUKAN dikotomi masuk-vs-keluar yang
// dipakai chart Finance/Stock, tapi pasangan biru/oranye yang sama tetap dipakai
// karena maknanya tetap "dua sisi berlawanan dari satu proses" -- dan menambah
// hue kategorikal ketiga cuma untuk chart ini akan membuat halaman ini punya
// lebih banyak warna daripada makna. Hijau/merah SENGAJA dihindari (di seluruh
// halaman ini) supaya tidak terbaca sebagai penilaian baik/buruk.
// Chart ketujuh & kedelapan (HR). Cuti dipecah per jenis karena yang menarik
// bukan totalnya, melainkan komposisinya: cuti tahunan itu hak yang memang
// dipakai, sementara sakit yang menumpuk dan tanpa gaji punya arti berbeda.
export const HR_LEAVE_SERIES = [
  { key: 'annual_days', label: 'Tahunan', color: 'var(--bs-primary)' },
  { key: 'sick_days', label: 'Sakit', color: 'var(--bs-orange)' },
  { key: 'unpaid_days', label: 'Tanpa Gaji', color: 'var(--bs-danger)' },
  { key: 'other_days', label: 'Lainnya', color: 'var(--bs-secondary)' },
]

// Sebaran rating, bukan rata-rata sebagai batang: rata-rata sudah ditampilkan
// sebagai teks di bawah judul (satuannya poin, tidak bisa berbagi sumbu Y
// dengan cacah orang) -- pola yang sama dengan avg_delivery_hours di fleet.
export const HR_KPI_SERIES = [
  { key: 'sangat_baik_count', label: 'Sangat Baik', color: 'var(--bs-success)' },
  { key: 'baik_count', label: 'Baik', color: 'var(--bs-primary)' },
  { key: 'cukup_count', label: 'Cukup', color: 'var(--bs-orange)' },
  { key: 'perlu_perbaikan_count', label: 'Perlu Perbaikan', color: 'var(--bs-danger)' },
]

// Chart kesembilan: satu seri saja (rata-rata nilai per departemen). Rentang
// min-max ditampilkan sebagai teks di bawah grafik, bukan seri tambahan --
// tiga batang berdampingan untuk min/rata/max mudah terbaca sebagai "tiga
// kelompok berbeda" padahal ketiganya menggambarkan kelompok yang sama.
export const HR_KPI_DEPARTMENT_SERIES = [{ key: 'avg_score', label: 'Rata-rata Nilai', color: 'var(--bs-primary)' }]

// Chart 10-13, mengisi empat fact table yang datanya sudah lama masuk warehouse
// tapi belum pernah punya grafik.
export const QC_SERIES = [
  { key: 'pass_count', label: 'Lolos', color: 'var(--bs-success)' },
  { key: 'partial_count', label: 'Sebagian', color: 'var(--bs-orange)' },
  { key: 'fail_count', label: 'Gagal', color: 'var(--bs-danger)' },
]

export const PRODUCTION_SERIES = [
  { key: 'quantity_planned', label: 'Rencana', color: 'var(--bs-secondary)' },
  { key: 'quantity_produced', label: 'Realisasi', color: 'var(--bs-primary)' },
]

export const PURCHASING_SUPPLIER_SERIES = [
  { key: 'total_spend', label: 'Belanja', color: 'var(--bs-primary)' },
]

export const TICKETING_SERIES = [
  { key: 'resolved_count', label: 'Selesai', color: 'var(--bs-primary)' },
  { key: 'open_count', label: 'Belum selesai', color: 'var(--bs-orange)' },
]

// Chart 14-17: empat fact table terakhir yang belum punya grafik. Dengan ini
// seluruh 16 fact table di warehouse terwakili di dashboard.
export const PAYROLL_SERIES = [
  { key: 'total_net', label: 'Gaji Bersih', color: 'var(--bs-primary)' },
  { key: 'total_deduction', label: 'Potongan', color: 'var(--bs-orange)' },
]

export const ASSET_MAINTENANCE_SERIES = [
  { key: 'completed_count', label: 'Selesai', color: 'var(--bs-success)' },
  { key: 'overdue_count', label: 'Terlambat', color: 'var(--bs-danger)' },
  { key: 'cancelled_count', label: 'Dibatalkan', color: 'var(--bs-secondary)' },
]

export const ECOMMERCE_SERIES = [{ key: 'revenue', label: 'Penjualan', color: 'var(--bs-primary)' }]

export const FLEET_DELIVERY_SERIES = [
  { key: 'delivered_count', label: 'Selesai', color: 'var(--bs-primary)' },
  { key: 'cancelled_count', label: 'Dibatalkan', color: 'var(--bs-orange)' },
]

// Chart ke-18 (fact table ke-17: fact_production_oee). Tiga faktor OEE plus
// hasil kalinya, semuanya dalam persen sehingga satu sumbu Y sudah cukup --
// ini satu-satunya chart di sini yang seluruh serinya berbagi satuan yang
// sama, dan justru itu yang membuat empat seri bisa dibaca berdampingan.
//
// OEE (hasil kali ketiganya) diberi warna primer yang penuh, ketiga faktornya
// warna yang lebih redup: yang pertama dicari orang adalah satu angka OEE-nya,
// tiga faktor di sebelahnya menjawab "kenapa segitu". Merah/hijau SENGAJA
// dihindari seperti di seluruh halaman ini -- 70% OEE bagus untuk satu pabrik
// dan buruk untuk pabrik lain, dan warna tidak boleh memutuskan itu.
export const PRODUCTION_OEE_SERIES = [
  { key: 'availability_pct', label: 'Availability', color: 'rgba(var(--bs-primary-rgb), 0.35)' },
  { key: 'performance_pct', label: 'Performance', color: 'rgba(var(--bs-primary-rgb), 0.55)' },
  { key: 'quality_pct', label: 'Quality', color: 'rgba(var(--bs-primary-rgb), 0.75)' },
  { key: 'oee_pct', label: 'OEE', color: 'var(--bs-primary)' },
]

// Satu-satunya formatter yang ikut tinggal di modul ini, dengan alasan yang
// sama seperti warnanya: faktor OEE yang TIDAK BISA dihitung (bulan tanpa unit
// selesai) bernilai null, dan menampilkannya sebagai "0%" adalah klaim yang
// berbeda -- "mesinnya tidak menghasilkan apa-apa" versus "belum ada angkanya".
export function formatPercent(v) {
  if (v == null) return '—'
  return `${new Intl.NumberFormat('id-ID', { maximumFractionDigits: 1 }).format(v)}%`
}
