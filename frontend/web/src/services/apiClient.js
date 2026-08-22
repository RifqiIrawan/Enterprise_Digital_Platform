import axios from 'axios'

const apiClient = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || 'http://localhost:8079',
})

apiClient.interceptors.request.use((config) => {
  const token = localStorage.getItem('access_token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  // Company yang sedang dipilih ikut dikirim di setiap request karena gateway
  // memakainya untuk menentukan hak akses pada endpoint yang TIDAK menyebut
  // company di query maupun body -- mis. DELETE /api/hr/holidays/{id}. Kalau
  // request menyebutnya sendiri, gateway memakai yang di request, bukan header
  // ini (lihat authz.companyID di api-gateway): header ini pelengkap, bukan
  // sumber kebenaran. Dibaca langsung dari localStorage, bukan dari
  // CompanyContext, karena interceptor bukan komponen React.
  const companyId = localStorage.getItem('current_company_id')
  if (companyId) {
    config.headers['X-Company-Id'] = companyId
  }
  return config
})

apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    const isLoginCall = error.config?.url?.includes('/auth/login')
    // 401 dari /auth/change-password berarti "password lama salah", BUKAN sesi
    // kedaluwarsa -- mengusir user ke halaman login di situ membuat kesalahan
    // ketik terasa seperti logout mendadak tanpa penjelasan.
    const isChangePasswordCall = error.config?.url?.includes('/auth/change-password')
    if (error.response?.status === 401 && !isLoginCall && !isChangePasswordCall) {
      localStorage.removeItem('access_token')
      localStorage.removeItem('current_user')
      window.location.href = '/login'
    }
    return Promise.reject(error)
  }
)

export default apiClient
