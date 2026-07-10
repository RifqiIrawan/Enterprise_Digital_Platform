import StatTile from '../../components/dashboard/StatTile.jsx'
import { CORE_SERVICES, BUSINESS_MODULES } from '../../utils/modules.js'

function DashboardPage() {
  return (
    <div className="d-flex flex-column gap-4">
      <div className="alert alert-primary d-flex align-items-start gap-2 mb-0" role="alert">
        <i className="bi bi-info-circle mt-1" />
        <div>
          Platform masih di <strong>Fase 1 (skeleton)</strong>. Widget di bawah akan terisi data nyata
          setelah API gateway mem-proxy service bisnis — lihat{' '}
          <code>20_Implementation_Guide.md</code>.
        </div>
      </div>

      <div className="row g-3">
        <StatTile
          icon="bi-hdd-network"
          label="Service backend aktif"
          value={`${CORE_SERVICES.length} / ${CORE_SERVICES.length + BUSINESS_MODULES.length}`}
          hint="backend/services berjalan, backend/modules belum diimplementasi"
          color="primary"
        />
        <StatTile
          icon="bi-building"
          label="Total perusahaan"
          value="—"
          hint="Menunggu integrasi company-service"
          color="green"
        />
        <StatTile
          icon="bi-people"
          label="Total pengguna"
          value="—"
          hint="Menunggu integrasi rbac-service"
          color="amber"
        />
        <StatTile
          icon="bi-journal-text"
          label="Aktivitas hari ini"
          value="—"
          hint="Menunggu integrasi audit-service"
          color="blue"
        />
      </div>

      <div className="row g-3">
        <div className="col-12 col-xl-8">
          <div className="card h-100">
            <div className="card-header">
              <span className="fw-semibold">Status modul</span>
            </div>
            <div className="card-body d-flex flex-wrap gap-2">
              {CORE_SERVICES.map((service) => (
                <span className="edp-module-pill is-active" key={service.key}>
                  <span className="edp-dot" />
                  {service.label}
                </span>
              ))}
              {BUSINESS_MODULES.map((mod) => (
                <span className="edp-module-pill" key={mod.key}>
                  <span className="edp-dot" />
                  {mod.label}
                </span>
              ))}
            </div>
            <div className="card-footer text-secondary small">
              <span className="edp-dot d-inline-block rounded-circle bg-success" style={{ width: 8, height: 8 }} />{' '}
              Berjalan &nbsp;&nbsp;
              <span className="d-inline-block rounded-circle bg-secondary" style={{ width: 8, height: 8 }} />{' '}
              Belum diimplementasi
            </div>
          </div>
        </div>

        <div className="col-12 col-xl-4">
          <div className="card h-100">
            <div className="card-header">
              <span className="fw-semibold">Aktivitas terbaru</span>
            </div>
            <div className="card-body d-flex flex-column align-items-center justify-content-center text-center text-secondary py-5">
              <i className="bi bi-inbox fs-2 mb-2" />
              <div className="small">Belum ada aktivitas.</div>
              <div className="small">audit-service belum terhubung ke dashboard.</div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

export default DashboardPage
