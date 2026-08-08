// Sinkron manual dengan "version" di package.json -- Vite tidak expose
// package.json ke kode client secara default tanpa config define() tambahan,
// jadi sengaja di-hardcode di sini daripada menambah mekanisme baru untuk
// satu angka versi.
const APP_VERSION = '0.1.0'

function Footer() {
  return (
    <footer className="edp-footer px-3 px-md-4 py-3 d-flex flex-column flex-sm-row align-items-center justify-content-between gap-2">
      <span className="text-secondary small">
        &copy; {new Date().getFullYear()} Enterprise Digital Platform. All rights reserved.
      </span>
      <span className="text-secondary small">v{APP_VERSION}</span>
    </footer>
  )
}

export default Footer
