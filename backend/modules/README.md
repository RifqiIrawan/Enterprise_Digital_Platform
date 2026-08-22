# Modules (Business Services)

Microservice modul bisnis. Semuanya sudah berjalan — masing-masing punya `go.mod`,
migrasi, HTTP API, test, Dockerfile, dan manifest K8s sendiri, mengikuti pola yang
sama dengan core services di `../services/`.

| Module | Cakupan | Role utama |
|---|---|---|
| finance-service | Chart of Accounts, Jurnal GL, Invoice, AR/AP | Finance |
| hr-service | Karyawan, Absensi, Payroll, Cuti, Lembur, Kalender Libur, Kuota Cuti, KPI | HR |
| sales-service | Customer, Quotation, Sales Order | Sales |
| purchasing-service | Supplier, Purchase Requisition, Purchase Order | Purchasing |
| warehouse-service | Produk, Gudang, Stok, Mutasi Antar Branch, Stock Opname | Warehouse |
| production-service | Bill of Material, Work Order, Jadwal Produksi | Production (MES) |
| qc-service | Standar Mutu, Inspeksi Kualitas | QC |
| asset-service | Pendataan Aset, Maintenance Schedule | Asset |
| ai-bi-service | BI Dashboard, Forecasting, Anomaly Detection | AI Analyst |
| iot-service | Device Registry, MQTT Pipeline, Threshold Alert | Operasional |
| dw-service | Data Warehouse (ClickHouse), ETL batch & streaming, Data Lake (MinIO) | AI Analyst |
| crm-service | Lead, Account, Contact, Opportunity, Activity | Sales |
| ticketing-service | Kategori Tiket, Tiket, Komentar | Support |
| ecommerce-service | Order & Order Item (checkout memakai katalog warehouse) | Sales |
| fleet-service | Kendaraan, Pengemudi, Surat Jalan | Operasional |
| project-service | Proyek, Tugas, Timesheet (posting biaya ke GL) | Project |

## Catatan sejarah

`ai-service` dan `bi-service` pernah ada di sini sebagai dua folder placeholder
(hanya berisi README "Planned"). Keduanya **dihapus** karena lingkupnya sudah
dikerjakan satu service: **`ai-bi-service`** — dashboard, forecasting, dan anomaly
detection ada di sana. Memisahkannya kembali jadi dua service baru masuk akal
kalau nanti model serving butuh siklus rilis atau kebutuhan resource yang berbeda
dari penyajian dashboard.
