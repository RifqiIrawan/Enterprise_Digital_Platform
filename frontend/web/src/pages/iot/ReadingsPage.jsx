import { useEffect, useState } from 'react'
import apiClient from '../../services/apiClient.js'
import DataTable from '../../components/common/DataTable.jsx'
import { useCompany } from '../../store/CompanyContext.jsx'

const READING_TYPES = ['TEMPERATURE', 'HUMIDITY', 'VIBRATION', 'RFID', 'GPS', 'BARCODE']
const AUTO_REFRESH_MS = 15000

function formatValue(r) {
  if (r.value_numeric != null) return r.value_numeric
  return r.value_text ?? '—'
}

function ReadingsPage() {
  const { companyId, branchId } = useCompany()
  const [devices, setDevices] = useState([])
  const [readings, setReadings] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [deviceFilter, setDeviceFilter] = useState('')
  const [typeFilter, setTypeFilter] = useState('')

  function loadReadings(cid, bid, deviceId, readingType) {
    apiClient
      .get('/api/iot/readings', {
        params: { company_id: cid, branch_id: bid, device_id: deviceId || undefined, reading_type: readingType || undefined },
      })
      .then(({ data }) => setReadings(data))
      .catch(() => setError('Gagal memuat data readings. Pastikan iot-service aktif.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!companyId) {
      setLoading(false)
      return
    }
    apiClient.get('/api/iot/devices', { params: { company_id: companyId, branch_id: branchId } }).then(({ data }) => setDevices(data))
  }, [companyId, branchId])

  // Auto-refresh supaya data dari simulator kelihatan "hidup" tanpa perlu
  // reload manual -- lihat catatan di implementation plan soal ini
  // sengaja dibuat sebagai nod kecil ke sifat "simulator" modul ini.
  useEffect(() => {
    if (!companyId) return
    setLoading(true)
    loadReadings(companyId, branchId, deviceFilter, typeFilter)
    const interval = setInterval(() => loadReadings(companyId, branchId, deviceFilter, typeFilter), AUTO_REFRESH_MS)
    return () => clearInterval(interval)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [companyId, branchId, deviceFilter, typeFilter])

  const columns = [
    { key: 'recorded_at', label: 'Waktu', cellClassName: 'text-secondary small', render: (r) => new Date(r.recorded_at).toLocaleString('id-ID') },
    { key: 'device_id', label: 'Device', render: (r) => `${r.device_code} - ${r.device_name}`, sortValue: (r) => r.device_code },
    { key: 'reading_type', label: 'Tipe' },
    { key: 'value', label: 'Nilai', sortable: false, render: formatValue },
  ]

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex align-items-center justify-content-between">
        <div>
          <h2 className="edp-page-title">IoT Readings</h2>
          <div className="text-secondary small">Data pembacaan sensor terbaru dari simulator (auto-refresh tiap {AUTO_REFRESH_MS / 1000} detik).</div>
        </div>
      </div>

      {error && <div className="alert alert-danger py-2 small">{error}</div>}

      <div className="card p-3">
        <div className="row g-2 mb-3">
          <div className="col-6 col-md-4">
            <select className="form-select form-select-sm" value={deviceFilter} onChange={(e) => setDeviceFilter(e.target.value)}>
              <option value="">Semua device</option>
              {devices.map((d) => (
                <option key={d.id} value={d.id}>{d.device_code} - {d.name}</option>
              ))}
            </select>
          </div>
          <div className="col-6 col-md-4">
            <select className="form-select form-select-sm" value={typeFilter} onChange={(e) => setTypeFilter(e.target.value)}>
              <option value="">Semua tipe</option>
              {READING_TYPES.map((t) => (
                <option key={t} value={t}>{t}</option>
              ))}
            </select>
          </div>
        </div>
        <DataTable
          columns={columns}
          data={readings}
          loading={loading}
          searchPlaceholder="Cari device atau tipe..."
          emptyMessage="Belum ada readings. Pastikan ada device ACTIVE dan simulator/Mosquitto sudah jalan."
        />
      </div>
    </div>
  )
}

export default ReadingsPage
