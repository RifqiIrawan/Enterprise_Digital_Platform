-- IoT Service: alat/sensor simulasi (devices), hasil pembacaan (readings),
-- dan alert ambang batas (alerts). Mengikuti prinsip platform (README root):
-- setiap data transaksi membawa company_id/branch_id, tidak ada FK fisik
-- lintas database.
--
-- warehouse_id di devices sengaja opsional & informational saja (lokasi
-- fisik device ditaruh di gudang mana), sama seperti pola assets.warehouse_id
-- di asset-service -- tanpa FK fisik ke warehouse-service dan TIDAK memicu
-- apa pun di sana.
--
-- readings & alerts pakai FK fisik ke devices karena satu database yang sama
-- (bukan lintas service).
--
-- threshold_min/threshold_max di devices hanya relevan untuk device_type
-- numerik (TEMPERATURE/HUMIDITY/VIBRATION); RFID/GPS/BARCODE selalu NULL di
-- kedua kolom itu dan tidak pernah memicu alert (lihat internal/ingest).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE devices (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id     UUID NOT NULL,
    branch_id      UUID,
    warehouse_id   UUID, -- opsional, lokasi fisik device; informational saja, lihat komentar di atas
    device_code    VARCHAR(30) NOT NULL,
    device_type    VARCHAR(20) NOT NULL CHECK (device_type IN ('TEMPERATURE', 'HUMIDITY', 'VIBRATION', 'RFID', 'GPS', 'BARCODE')),
    name           VARCHAR(200) NOT NULL,
    status         VARCHAR(20) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE', 'MAINTENANCE')),
    threshold_min  NUMERIC(15, 4),
    threshold_max  NUMERIC(15, 4),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, device_code)
);

CREATE INDEX idx_devices_company_id ON devices (company_id);
CREATE INDEX idx_devices_status ON devices (company_id, status);

-- Pembacaan sensor. value_numeric dipakai untuk TEMPERATURE/HUMIDITY/
-- VIBRATION, value_text untuk RFID (tag id)/GPS ("lat,lon")/BARCODE (kode
-- barcode) -- persis satu dari keduanya terisi tergantung reading_type,
-- divalidasi di internal/ingest (bukan CHECK constraint DB, supaya pesan
-- error lebih jelas dari sisi aplikasi).
CREATE TABLE readings (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id     UUID NOT NULL REFERENCES devices(id),
    company_id    UUID NOT NULL,
    branch_id     UUID,
    reading_type  VARCHAR(20) NOT NULL CHECK (reading_type IN ('TEMPERATURE', 'HUMIDITY', 'VIBRATION', 'RFID', 'GPS', 'BARCODE')),
    value_numeric NUMERIC(15, 4),
    value_text    VARCHAR(200),
    recorded_at   TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_readings_company_id ON readings (company_id);
CREATE INDEX idx_readings_device_id ON readings (device_id, recorded_at DESC);
CREATE INDEX idx_readings_recorded_at ON readings (company_id, recorded_at DESC);

-- Alert ambang batas. Tidak auto-resolve saat pembacaan berikutnya kembali
-- normal -- acknowledge/resolve murni aksi manual pengguna, sama seperti
-- penyelesaian maintenance_schedules di asset-service.
CREATE TABLE alerts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id       UUID NOT NULL REFERENCES devices(id),
    reading_id      UUID REFERENCES readings(id),
    company_id      UUID NOT NULL,
    branch_id       UUID,
    alert_type      VARCHAR(30) NOT NULL DEFAULT 'THRESHOLD_BREACH',
    severity        VARCHAR(10) NOT NULL CHECK (severity IN ('MEDIUM', 'HIGH')),
    message         TEXT NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN', 'ACKNOWLEDGED', 'RESOLVED')),
    triggered_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    acknowledged_at TIMESTAMPTZ,
    acknowledged_by UUID,
    resolved_at     TIMESTAMPTZ,
    resolved_by     UUID
);

CREATE INDEX idx_alerts_company_id ON alerts (company_id);
CREATE INDEX idx_alerts_device_id ON alerts (device_id);
CREATE INDEX idx_alerts_status ON alerts (company_id, status);
