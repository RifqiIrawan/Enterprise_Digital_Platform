// Service inti (backend/services) — dipakai kartu "Modul terpasang" di
// DashboardPage sebagai pendamping daftar modul bisnis.
//
// Daftar modul BISNIS sengaja TIDAK ada di sini lagi. Dulu ada konstanta
// BUSINESS_MODULES statis, dan dia tertinggal jauh dari kenyataan: masih
// menyebut `ai` dan `bi` (dua folder di backend/modules yang tidak pernah
// di-scaffold — lingkupnya dikerjakan ai-bi-service) sementara 7 modul yang
// sudah jadi (iot, dw, crm, ticketing, ecommerce, fleet, project) tidak
// terdaftar sama sekali. Sekarang Dashboard membacanya dari
// `GET /api/rbac/modules`, sumber yang sama dengan menu sidebar, sehingga
// tidak bisa lagi basi tanpa ketahuan.
//
// Service inti tetap statis karena dia bukan bagian dari registry menu RBAC:
// api-gateway dan auth-service tidak punya menu, jadi tidak akan pernah muncul
// di endpoint itu.
export const CORE_SERVICES = [
  { key: 'api-gateway', label: 'API Gateway' },
  { key: 'auth', label: 'Auth' },
  { key: 'company', label: 'Company' },
  { key: 'rbac', label: 'RBAC' },
  { key: 'audit', label: 'Audit' },
]
