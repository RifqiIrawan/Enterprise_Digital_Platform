import { useLocation } from 'react-router-dom'

function PlaceholderPage() {
  const { pathname } = useLocation()

  return (
    <div className="card">
      <div className="card-body d-flex flex-column align-items-center justify-content-center text-center text-secondary py-5">
        <i className="bi bi-cone-striped fs-1 mb-3" />
        <div className="fw-semibold text-body">Modul ini belum diimplementasikan</div>
        <div className="small mt-1">
          Menu sudah terdaftar di database (<code>{pathname}</code>) tapi halamannya menyusul di fase berikutnya.
        </div>
      </div>
    </div>
  )
}

export default PlaceholderPage
