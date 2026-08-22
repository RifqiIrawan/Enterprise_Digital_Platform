// Config ESLint pertama untuk repo ini. Sebelumnya `npm run lint` selalu gagal
// dengan "couldn't find a configuration file" (lihat komentar di
// .github/workflows/frontend-ci.yml), jadi step lint di CI dinonaktifkan.
//
// Format .eslintrc.cjs (bukan flat config eslint.config.js) karena devDependency
// yang sudah ada di repo ini adalah ESLint 8; naik ke ESLint 9 + flat config
// adalah perubahan tersendiri, bukan efek samping dari menambah config.
//
// Aturan yang dipilih sengaja sedikit dan semuanya "bisa menangkap bug nyata":
// tidak ada aturan gaya penulisan (indentasi, kutip, titik koma) karena
// formatting di repo ini sudah konsisten dan menambah ribuan temuan kosmetik
// hanya membuat lint diabaikan orang.
module.exports = {
  root: true,
  env: { browser: true, es2022: true, node: true },
  parserOptions: { ecmaVersion: 'latest', sourceType: 'module', ecmaFeatures: { jsx: true } },
  settings: { react: { version: 'detect' } },
  plugins: ['react', 'react-hooks', 'react-refresh'],
  extends: [
    'eslint:recommended',
    'plugin:react/recommended',
    'plugin:react/jsx-runtime', // React 18 + JSX transform: tidak perlu `import React`
    'plugin:react-hooks/recommended',
  ],
  ignorePatterns: ['dist', 'node_modules'],
  rules: {
    // Inti dari alasan config ini ditambahkan: rules-of-hooks menangkap
    // kesalahan seperti memanggil usePagePermission() di luar komponen atau di
    // dalam kondisi -- persis jenis kesalahan yang di sesi sebelumnya hanya
    // dijaga skrip pemeriksa statis buatan sendiri.
    'react-hooks/rules-of-hooks': 'error',
    'react-hooks/exhaustive-deps': 'warn',
    // Dimatikan: file context di repo ini memang mengekspor hook di samping
    // provider-nya (pola yang sudah ada sejak CompanyContext, jauh sebelum
    // config lint ini). Aturan ini soal keandalan hot-reload saat ngoding,
    // bukan kebenaran kode -- membiarkannya menyala berarti 4 warning tetap
    // yang tidak akan pernah ditindak, dan lint yang selalu berisik akan
    // diabaikan orang.
    'react-refresh/only-export-components': 'off',
    // Prop-types tidak dipakai di repo ini (tidak ada satu pun komponen yang
    // mendeklarasikannya); menyalakannya berarti ratusan temuan tanpa ada
    // rencana memakainya.
    'react/prop-types': 'off',
    'no-unused-vars': ['error', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
  },
  overrides: [
    {
      // File test memakai API Vitest yang di-import eksplisit (globals: false),
      // tapi tetap butuh env browser untuk jsdom.
      files: ['**/*.test.js', '**/*.test.jsx'],
      env: { browser: true },
    },
    {
      files: ['vite.config.js', '.eslintrc.cjs'],
      env: { node: true },
    },
  ],
}
