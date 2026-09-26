package corpus

import (
	"strings"
	"testing"
)

// Pemotongan per heading adalah alasan kutipan chatbot ini bisa dibaca orang.
// Yang dijaga di berkas ini adalah kasus-kasus yang membuat pemotongan naif
// menghasilkan kutipan rusak: pagar kode berisi tanda pagar, bagian pengantar
// satu kalimat, dan bagian yang kelewat panjang.

func TestParse_SplitsByHeading(t *testing.T) {
	doc := Parse("modul.md", `# Modul Produksi

Pengantar singkat yang tidak berdiri sendiri sebagai jawaban, tapi tetap perlu ikut terbawa ke bagian berikutnya supaya konteksnya tidak hilang sama sekali.

## Rumus OEE

OEE dihitung sebagai Availability dikali Performance dikali Quality. Availability adalah waktu jalan dibagi waktu rencana produksi pada shift itu, dan angkanya ikut dipotong oleh downtime yang dicatat operator.

## Work Order

Work order adalah rencana produksi yang mengambil komponen dari bill of material, dan stoknya hanya bermutasi ketika work order berstatus COMPLETED di akhir proses.
`)

	if doc.Title != "Modul Produksi" {
		t.Errorf("title = %q, mau \"Modul Produksi\"", doc.Title)
	}
	if len(doc.Chunks) < 2 {
		t.Fatalf("mau minimal 2 potongan, dapat %d", len(doc.Chunks))
	}

	headings := map[string]string{}
	for _, c := range doc.Chunks {
		headings[c.Heading] = c.Content
	}
	oee, ok := headings["Rumus OEE"]
	if !ok {
		t.Fatalf("tidak ada potongan berheading \"Rumus OEE\": %v", headings)
	}
	if !strings.Contains(oee, "Availability") {
		t.Errorf("potongan OEE tidak memuat isinya: %q", oee)
	}
	// Isi bagian lain tidak boleh bocor ke potongan ini -- kutipan yang
	// menyeret paragraf tetangga membuat jawaban model ikut melantur.
	if strings.Contains(oee, "bill of material") {
		t.Errorf("potongan OEE ikut membawa isi bagian Work Order: %q", oee)
	}
}

// Tanda pagar di dalam blok kode adalah komentar shell, bukan heading. Tanpa
// pemeriksaan pagar, satu skrip di dokumentasi memecah dokumen jadi belasan
// potongan palsu yang headingnya berisi perintah.
func TestParse_IgnoresHashesInsideCodeFence(t *testing.T) {
	doc := Parse("infra.md", "# Infrastruktur\n\n## Menjalankan Platform\n\nJalankan perintah berikut dari folder infra untuk menghidupkan seluruh service yang dibutuhkan platform ini di mesin pengembangan.\n\n```bash\n# nyalakan seluruh service\ndocker compose up -d\n# lihat lognya\ndocker compose logs -f\n```\n")

	for _, c := range doc.Chunks {
		if strings.HasPrefix(c.Heading, "nyalakan") || strings.HasPrefix(c.Heading, "lihat") {
			t.Errorf("komentar di dalam blok kode dijadikan heading: %q", c.Heading)
		}
	}
	joined := ""
	for _, c := range doc.Chunks {
		joined += c.Content
	}
	if !strings.Contains(joined, "docker compose up -d") {
		t.Error("isi blok kode hilang dari hasil pemotongan")
	}
}

// Bagian sependek satu kalimat digabung ke bagian berikutnya: dikutip
// sendirian, potongan seperti itu tidak menjawab apa pun.
func TestParse_MergesVeryShortSections(t *testing.T) {
	doc := Parse("ringkas.md", `# Judul

## Sekilas

Satu kalimat saja.

## Rincian

Bagian ini cukup panjang untuk berdiri sendiri sebagai jawaban, memuat penjelasan yang benar-benar dicari orang ketika mereka membuka dokumentasi dan mengetikkan pertanyaannya ke kotak pencarian.
`)

	for _, c := range doc.Chunks {
		if c.CharCount < minChunkChars {
			t.Errorf("masih ada potongan kelewat pendek (%d karakter): %q", c.CharCount, c.Content)
		}
	}
	joined := ""
	for _, c := range doc.Chunks {
		joined += c.Content
	}
	if !strings.Contains(joined, "Satu kalimat saja.") {
		t.Error("isi bagian pendek hilang, bukan digabung")
	}
}

func TestParse_SplitsOverlongSectionAtParagraphBoundary(t *testing.T) {
	long := strings.Repeat("Kalimat panjang tentang proses produksi yang berulang-ulang. ", 60)
	doc := Parse("panjang.md", "# Judul\n\n## Bagian Panjang\n\n"+long+"\n\n"+long+"\n")

	if len(doc.Chunks) < 2 {
		t.Fatalf("bagian kelewat panjang tidak dipecah: %d potongan", len(doc.Chunks))
	}
	for _, c := range doc.Chunks {
		if c.Heading != "Bagian Panjang" {
			t.Errorf("potongan hasil pecahan kehilangan headingnya: %q", c.Heading)
		}
		// Pemecahan di batas paragraf: tidak boleh ada potongan yang berakhir
		// di tengah kata.
		if strings.HasSuffix(c.Content, "Kalimat panjang tentang proses produ") {
			t.Error("potongan terpotong di tengah kata")
		}
	}
}

// Hash dipakai ingest untuk memutuskan "berkas ini berubah atau tidak", jadi
// dia harus stabil untuk isi yang sama dan berbeda begitu isinya berubah.
func TestParse_ContentHashIsStableAndSensitive(t *testing.T) {
	a := Parse("a.md", "# Judul\n\nIsi dokumen yang sama persis.\n")
	b := Parse("a.md", "# Judul\n\nIsi dokumen yang sama persis.\n")
	c := Parse("a.md", "# Judul\n\nIsi dokumen yang sudah diubah.\n")

	if a.ContentHash != b.ContentHash {
		t.Error("hash berbeda untuk isi yang sama: ingest akan menulis ulang terus-menerus")
	}
	if a.ContentHash == c.ContentHash {
		t.Error("hash sama untuk isi yang berbeda: perubahan dokumentasi tidak akan pernah ter-index")
	}
}

func TestParse_FallsBackToFilenameTitle(t *testing.T) {
	doc := Parse("catatan/rilis-2026.md", "Tidak ada heading H1 di berkas ini, hanya paragraf biasa yang panjangnya cukup untuk jadi satu potongan utuh dalam hasil pemotongan.\n")
	if doc.Title != "rilis-2026" {
		t.Errorf("title = %q, mau jatuh ke nama berkas \"rilis-2026\"", doc.Title)
	}
}
