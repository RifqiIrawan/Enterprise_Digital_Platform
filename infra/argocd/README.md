# ArgoCD (Fase 12)

Dua `Application` yang menunjuk overlay Kustomize yang sudah ada di
`infra/kubernetes/overlays/{staging,prod}`.

**ArgoCD di sini MEMAKAI Kustomize, bukan menggantikannya.** Itu sebabnya
bagian ini kecil: yang dideklarasikan cuma "repo mana, path mana, namespace
mana, dan seberapa otomatis sinkronnya" — sementara isi manifesnya tetap satu
sumber, yaitu `infra/kubernetes/`. Menambahkan Helm chart di samping Kustomize
justru akan membuat dua sumber kebenaran untuk hal yang sama (lihat catatan di
bawah).

## Yang perlu diganti sebelum dipakai

- `repoURL` — sekarang `https://github.com/RifqiIrawan/Enterprise_Digital_Platform.git`,
  sesuai remote repo ini. Ganti kalau nanti pindah.
- `destination.server` — `https://kubernetes.default.svc` berarti "cluster
  tempat ArgoCD sendiri berjalan". Untuk cluster terpisah, daftarkan dulu
  clusternya di ArgoCD lalu isi URL-nya di sini.
- Overlay `prod` dan `staging` sendiri **belum deployable** apa adanya (lihat
  README di masing-masing overlay): masih butuh DATABASE_URL sungguhan,
  jwt-secret, registry image, dan host Ingress. ArgoCD akan mensinkronkan apa
  yang ada — termasuk nilai REPLACE_ME — jadi menyalakan auto-sync sebelum
  overlay-nya benar-benar terisi hanya akan membuat cluster penuh pod yang
  gagal start.

## Beda perlakuan staging vs prod (disengaja)

| | staging | prod |
|---|---|---|
| `syncPolicy.automated` | ya | **tidak** |
| `prune` | ya | — (sinkron manual) |
| `selfHeal` | ya | — |

Staging boleh mengikuti `master` sendiri; itu memang gunanya. Produksi
disinkronkan **manual**: sinkron otomatis ke produksi berarti setiap commit
yang lolos CI langsung mengubah produksi, dan keputusan "sekarang waktunya
mengubah produksi" bukan keputusan yang layak diambil oleh proses yang tidak
tahu apa-apa tentang jam sibuk, rilis yang sedang berjalan, atau orang yang
sedang siaga.

## Memasang

```bash
kubectl apply -f infra/argocd/application-staging.yaml
kubectl apply -f infra/argocd/application-prod.yaml
```

Keduanya diletakkan di namespace `argocd` (namespace bawaan pemasangan
ArgoCD). Belum pernah dijalankan terhadap cluster sungguhan dari repo ini —
yang sudah diverifikasi baru bahwa path yang ditunjuk memang bisa di-render
(`kubectl kustomize infra/kubernetes/overlays/staging` dan `.../prod`).

## Jenkins & Helm: sengaja tidak dikerjakan

Roadmap Fase 12 menyebut keduanya. Keduanya dilewati dengan alasan, bukan
karena lupa:

- **Jenkins** — repo ini sudah punya tiga workflow GitHub Actions (backend,
  frontend, monitoring) yang benar-benar berjalan. Menambahkan Jenkinsfile
  berarti dua definisi pipeline untuk pekerjaan yang sama, dan yang kedua
  tidak akan pernah dijalankan siapa pun di sini; pipeline yang tidak pernah
  jalan akan diam-diam menjadi salah tanpa ada yang tahu.
- **Helm** — deployment repo ini memakai Kustomize (base + tiga overlay) dan
  ArgoCD di atas sudah mengonsumsinya. Chart Helm di samping itu bukan
  tambahan kemampuan, melainkan salinan kedua dari manifes yang sama yang
  harus ikut diperbarui setiap kali salah satunya berubah. Kalau nanti ada
  kebutuhan yang benar-benar menuntut Helm (mis. mendistribusikan platform ini
  ke pihak lain untuk dipasang sendiri), pindahnya sebaiknya **menggantikan**
  Kustomize, bukan mendampinginya.
