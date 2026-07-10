# Frontend Web

React 18 + Bootstrap 5, dikonsumsi via [Vite](https://vitejs.dev). Berkomunikasi ke seluruh backend melalui satu pintu: [`api-gateway`](../../backend/services/api-gateway).

## Menjalankan secara lokal
```
npm install
cp .env.example .env
npm run dev
```
Default port: `3000`. Set `VITE_API_BASE_URL` mengarah ke api-gateway (default `http://localhost:8080`).

## Struktur
```
frontend/web/
├── public/
├── src/
│   ├── assets/
│   ├── components/     # komponen reusable
│   ├── layouts/        # MainLayout (sidebar + content)
│   ├── pages/           # halaman per fitur (auth, dashboard, dan modul lain menyusul)
│   ├── routes/          # reserved: route guard berbasis role
│   ├── services/        # apiClient.js (axios + interceptor JWT)
│   ├── store/            # reserved: state management (mis. Zustand/Redux)
│   ├── hooks/            # reserved: custom hooks
│   ├── utils/            # reserved: helper
│   ├── App.jsx
│   └── main.jsx
├── index.html
├── package.json
└── vite.config.js
```

## Status
Fase 1 — skeleton dengan routing dasar (login, dashboard) dan API client. Halaman per modul bisnis (Finance, HR, Sales, dst) menyusul sesuai `20_Implementation_Guide.md`.
