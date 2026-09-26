package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
)

type ingestResponseView struct {
	Documents   int `json:"documents"`
	Chunks      int `json:"chunks"`
	Changed     int `json:"changed"`
	RemovedDocs int `json:"removed_documents"`
	Files       []struct {
		SourcePath string `json:"source_path"`
		Title      string `json:"title"`
		Chunks     int    `json:"chunks"`
		Status     string `json:"status"`
	} `json:"files"`
}

type askResponseView struct {
	Outcome string `json:"outcome"`
	Answer  string `json:"answer"`
	Note    string `json:"note"`
	Model   string `json:"model"`
	Sources []struct {
		SourcePath   string   `json:"source_path"`
		Title        string   `json:"title"`
		Heading      string   `json:"heading"`
		Content      string   `json:"content"`
		Score        float64  `json:"score"`
		Coverage     float64  `json:"coverage"`
		MatchType    string   `json:"match_type"`
		MatchedTerms []string `json:"matched_terms"`
	} `json:"sources"`
	InputTokens   int    `json:"input_tokens"`
	OutputTokens  int    `json:"output_tokens"`
	RetrievalNote string `json:"retrieval_note"`
}

func mustIngest(t *testing.T, url string) ingestResponseView {
	t.Helper()
	resp := postJSON(t, url+"/ingest", nil)
	requireStatus(t, resp, http.StatusOK)
	var out ingestResponseView
	resp.decode(t, &out)
	return out
}

func TestIngest_IndexesCorpusAndIsIdempotent(t *testing.T) {
	resetCorpus(t)
	dir := writeCorpus(t, sampleCorpus())
	srv := newServer(t, dir, nil)

	first := mustIngest(t, srv.URL)
	if first.Documents != 2 || first.Changed != 2 {
		t.Fatalf("ingest pertama = %d dokumen / %d berubah, mau 2/2", first.Documents, first.Changed)
	}
	if first.Chunks < 4 {
		t.Errorf("mau minimal 4 potongan dari 2 dokumen berheading, dapat %d", first.Chunks)
	}
	for _, f := range first.Files {
		if f.Status != "INDEXED" {
			t.Errorf("%s status %q, mau INDEXED", f.SourcePath, f.Status)
		}
	}

	// Ingest kedua tanpa perubahan berkas: tidak boleh ada yang ditulis ulang.
	// Ini yang membuat menjalankan /ingest berkali-kali aman.
	second := mustIngest(t, srv.URL)
	if second.Changed != 0 {
		t.Errorf("ingest kedua menulis ulang %d dokumen, mau 0", second.Changed)
	}
	if second.Documents != first.Documents || second.Chunks != first.Chunks {
		t.Errorf("ingest kedua = %d/%d, mau sama dengan yang pertama %d/%d",
			second.Documents, second.Chunks, first.Documents, first.Chunks)
	}
	for _, f := range second.Files {
		if f.Status != "UNCHANGED" {
			t.Errorf("%s status %q, mau UNCHANGED", f.SourcePath, f.Status)
		}
	}

	docs := getJSON(t, srv.URL+"/documents")
	requireStatus(t, docs, http.StatusOK)
	var list []struct {
		SourcePath string `json:"source_path"`
		Title      string `json:"title"`
		ChunkCount int    `json:"chunk_count"`
	}
	docs.decode(t, &list)
	if len(list) != 2 {
		t.Fatalf("GET /documents = %d dokumen, mau 2", len(list))
	}
	// Judul diambil dari heading H1, bukan dari nama berkas.
	if list[0].Title != "Modul Produksi" {
		t.Errorf("judul dokumen pertama = %q, mau \"Modul Produksi\"", list[0].Title)
	}
}

// Dokumentasi yang dihapus dari korpus harus hilang dari index. Chatbot yang
// masih mengutip halaman yang sudah dihapus adalah cara paling halus untuk
// menyesatkan orang -- dan tidak ada yang akan menyadarinya.
func TestIngest_RemovesDocumentsDeletedFromCorpus(t *testing.T) {
	resetCorpus(t)
	files := sampleCorpus()
	dir := writeCorpus(t, files)
	srv := newServer(t, dir, nil)

	mustIngest(t, srv.URL)

	// Korpus baru tanpa dokumen keuangan.
	delete(files, "02_keuangan.md")
	dir2 := writeCorpus(t, files)
	srv2 := newServer(t, dir2, nil)
	second := mustIngest(t, srv2.URL)

	if second.RemovedDocs != 1 {
		t.Errorf("removed_documents = %d, mau 1", second.RemovedDocs)
	}
	resp := postJSON(t, srv2.URL+"/ask", map[string]any{"question": "bagaimana penyusutan aset diposting"})
	requireStatus(t, resp, http.StatusOK)
	var ask askResponseView
	resp.decode(t, &ask)
	if ask.Outcome != "NO_MATCH" {
		t.Errorf("outcome = %q, mau NO_MATCH setelah dokumennya dihapus dari korpus (sumber: %+v)", ask.Outcome, ask.Sources)
	}
}

func TestIngest_MissingCorpusDirReportsPath(t *testing.T) {
	resetCorpus(t)
	srv := newServer(t, "C:/tidak/ada/folder/korpus", nil)
	resp := postJSON(t, srv.URL+"/ingest", nil)
	requireStatus(t, resp, http.StatusFailedDependency)
	if !strings.Contains(resp.errorMessage(), "korpus") {
		t.Errorf("pesan galat harus menyebut folder korpusnya, dapat: %q", resp.errorMessage())
	}
}

// Pencarian memakai konfigurasi 'indonesian', jadi stemming-nya bekerja:
// "penyusutan" menemukan bagian yang menulis "disusutkan"/"penyusutan" tanpa
// harus sama persis.
func TestAsk_RetrievesRelevantChunk(t *testing.T) {
	resetCorpus(t)
	dir := writeCorpus(t, sampleCorpus())
	stub := &stubAnswerer{answer: "OEE adalah Availability x Performance x Quality [1]."}
	srv := newServer(t, dir, stub)
	mustIngest(t, srv.URL)

	resp := postJSON(t, srv.URL+"/ask", map[string]any{"question": "bagaimana rumus OEE dihitung"})
	requireStatus(t, resp, http.StatusOK)
	var ask askResponseView
	resp.decode(t, &ask)

	if ask.Outcome != "ANSWERED" {
		t.Fatalf("outcome = %q, mau ANSWERED (note: %s)", ask.Outcome, ask.Note)
	}
	if len(ask.Sources) == 0 {
		t.Fatal("mau setidaknya satu sumber")
	}
	top := ask.Sources[0]
	if !strings.Contains(strings.ToLower(top.Heading), "oee") {
		t.Errorf("kutipan teratas = %q, mau bagian tentang OEE", top.Heading)
	}
	if top.MatchType != "fts" {
		t.Errorf("match_type = %q, mau fts", top.MatchType)
	}
	if top.Score <= 0 {
		t.Errorf("score = %v, mau > 0 supaya peringkatnya bisa diperiksa orang", top.Score)
	}
	if ask.RetrievalNote == "" {
		t.Error("retrieval_note kosong: batas metode pencarian harus disebut di setiap jawaban")
	}
	// Nomor kutipan ditentukan service, bukan model -- passage pertama harus
	// bernomor 1 supaya "[1]" di jawaban menunjuk sumber pertama di UI.
	if len(stub.lastPrompt) == 0 || stub.lastPrompt[0].Ref != 1 {
		t.Errorf("passage pertama = %+v, mau Ref 1", stub.lastPrompt)
	}
	if ask.InputTokens != 1200 || ask.OutputTokens != 240 {
		t.Errorf("token = %d/%d, mau diteruskan apa adanya dari model (1200/240)", ask.InputTokens, ask.OutputTokens)
	}
}

// Ini aturan yang membuat chatbot ini boleh dipercaya: tanpa bahan, model
// TIDAK dipanggil sama sekali. Kalau dipanggil, jawabannya akan datang dari
// ingatan model tentang ERP pada umumnya -- terdengar meyakinkan, tidak ada
// hubungannya dengan platform ini, dan tetap dibayar per token.
func TestAsk_NoMatchDoesNotCallTheModel(t *testing.T) {
	resetCorpus(t)
	dir := writeCorpus(t, sampleCorpus())
	stub := &stubAnswerer{answer: "seharusnya tidak pernah dipanggil"}
	srv := newServer(t, dir, stub)
	mustIngest(t, srv.URL)

	resp := postJSON(t, srv.URL+"/ask", map[string]any{"question": "resep rendang padang pedas"})
	requireStatus(t, resp, http.StatusOK)
	var ask askResponseView
	resp.decode(t, &ask)

	if ask.Outcome != "NO_MATCH" {
		t.Fatalf("outcome = %q, mau NO_MATCH", ask.Outcome)
	}
	if stub.calls != 0 {
		t.Errorf("model dipanggil %d kali untuk pertanyaan tanpa bahan, mau 0", stub.calls)
	}
	if ask.Answer != "" {
		t.Errorf("answer = %q, mau kosong", ask.Answer)
	}
	if ask.Note == "" {
		t.Error("note kosong: pengguna harus diberi tahu kenapa tidak ada jawaban")
	}
}

// Tanpa kunci API service tetap berguna: kutipan tetap keluar, dan alasannya
// disebut. Ini satu-satunya alasan seluruh fitur bisa dipasang sebelum ada
// kredensial.
func TestAsk_WithoutLLMReturnsCitationsNotError(t *testing.T) {
	resetCorpus(t)
	dir := writeCorpus(t, sampleCorpus())
	srv := newServer(t, dir, nil)
	mustIngest(t, srv.URL)

	resp := postJSON(t, srv.URL+"/ask", map[string]any{"question": "kapan stok bermutasi dari work order"})
	requireStatus(t, resp, http.StatusOK)
	var ask askResponseView
	resp.decode(t, &ask)

	if ask.Outcome != "RETRIEVED_ONLY" {
		t.Fatalf("outcome = %q, mau RETRIEVED_ONLY", ask.Outcome)
	}
	if len(ask.Sources) == 0 {
		t.Error("kutipan hilang: tanpa LLM pun pencarian dokumentasinya tetap berguna")
	}
	if !strings.Contains(ask.Note, "ANTHROPIC_API_KEY") {
		t.Errorf("note = %q, mau menyebut kunci API yang belum diisi", ask.Note)
	}
	if ask.Answer != "" {
		t.Errorf("answer = %q, mau kosong -- bukan jawaban karangan", ask.Answer)
	}
}

// Model yang tidak bisa dihubungi tidak boleh membuang kutipan yang sudah
// ditemukan: bagian yang masih berguna tetap dikembalikan, dengan 200, bukan
// 502 kosong.
func TestAsk_ModelFailureKeepsCitations(t *testing.T) {
	resetCorpus(t)
	dir := writeCorpus(t, sampleCorpus())
	stub := &stubAnswerer{err: errBoom{}}
	srv := newServer(t, dir, stub)
	mustIngest(t, srv.URL)

	resp := postJSON(t, srv.URL+"/ask", map[string]any{"question": "apa itu jurnal umum"})
	requireStatus(t, resp, http.StatusOK)
	var ask askResponseView
	resp.decode(t, &ask)

	if ask.Outcome != "RETRIEVED_ONLY" {
		t.Fatalf("outcome = %q, mau RETRIEVED_ONLY", ask.Outcome)
	}
	if len(ask.Sources) == 0 {
		t.Error("kutipan hilang padahal pencariannya berhasil")
	}
	if !strings.Contains(ask.Note, "tidak bisa dihubungi") {
		t.Errorf("note = %q, mau menjelaskan bahwa modelnya gagal dihubungi", ask.Note)
	}
}

// Penolakan datang sebagai 200 dengan stop_reason refusal, bukan sebagai
// galat. Yang penting di sini: statusnya tidak dilaporkan sebagai ANSWERED
// dengan jawaban kosong.
func TestAsk_RefusalIsReportedNotSwallowed(t *testing.T) {
	resetCorpus(t)
	dir := writeCorpus(t, sampleCorpus())
	stub := &stubAnswerer{refused: true}
	srv := newServer(t, dir, stub)
	mustIngest(t, srv.URL)

	resp := postJSON(t, srv.URL+"/ask", map[string]any{"question": "apa itu jurnal umum"})
	requireStatus(t, resp, http.StatusOK)
	var ask askResponseView
	resp.decode(t, &ask)

	if ask.Outcome != "RETRIEVED_ONLY" {
		t.Fatalf("outcome = %q, mau RETRIEVED_ONLY", ask.Outcome)
	}
	if !strings.Contains(ask.Note, "menolak") {
		t.Errorf("note = %q, mau menyebut penolakannya", ask.Note)
	}
}

func TestAsk_ValidationErrors(t *testing.T) {
	resetCorpus(t)
	dir := writeCorpus(t, sampleCorpus())
	srv := newServer(t, dir, nil)

	t.Run("pertanyaan kosong", func(t *testing.T) {
		requireStatus(t, postJSON(t, srv.URL+"/ask", map[string]any{"question": "   "}), http.StatusBadRequest)
	})
	t.Run("pertanyaan kelewat panjang", func(t *testing.T) {
		long := strings.Repeat("a", 1001)
		requireStatus(t, postJSON(t, srv.URL+"/ask", map[string]any{"question": long}), http.StatusBadRequest)
	})
}

// Setiap pertanyaan tercatat, termasuk yang tidak menemukan apa-apa. Tanpa ini
// tidak ada cara memeriksa berapa biaya chatbot dan seberapa sering dia gagal
// menjawab.
func TestAsk_LogsEveryQuestionWithOutcome(t *testing.T) {
	resetCorpus(t)
	if _, err := pool.Exec(t.Context(), `DELETE FROM rag_queries`); err != nil {
		t.Fatalf("reset rag_queries: %v", err)
	}
	dir := writeCorpus(t, sampleCorpus())
	stub := &stubAnswerer{answer: "Jawaban dari kutipan [1]."}
	srv := newServer(t, dir, stub)
	mustIngest(t, srv.URL)

	requireStatus(t, postJSON(t, srv.URL+"/ask", map[string]any{"question": "apa itu jurnal umum"}), http.StatusOK)
	requireStatus(t, postJSON(t, srv.URL+"/ask", map[string]any{"question": "resep rendang padang pedas"}), http.StatusOK)

	resp := getJSON(t, srv.URL+"/queries")
	requireStatus(t, resp, http.StatusOK)
	var queries []struct {
		Question     string `json:"question"`
		Outcome      string `json:"outcome"`
		Model        string `json:"model"`
		InputTokens  int    `json:"input_tokens"`
		OutputTokens int    `json:"output_tokens"`
	}
	resp.decode(t, &queries)
	if len(queries) != 2 {
		t.Fatalf("mau 2 baris riwayat, dapat %d", len(queries))
	}

	byOutcome := map[string]int{}
	for _, q := range queries {
		byOutcome[q.Outcome]++
		if q.Outcome == "ANSWERED" && (q.InputTokens == 0 || q.Model == "") {
			t.Errorf("baris ANSWERED tidak mencatat model/token: %+v", q)
		}
	}
	if byOutcome["ANSWERED"] != 1 || byOutcome["NO_MATCH"] != 1 {
		t.Errorf("sebaran outcome = %v, mau satu ANSWERED dan satu NO_MATCH", byOutcome)
	}
}

type errBoom struct{}

func (errBoom) Error() string { return "koneksi ke penyedia model gagal" }

// Dua perilaku yang lahir dari menjalankan pencarian terhadap dokumentasi
// sungguhan, dan yang paling mudah hilang lagi kalau ada yang menyetel ulang
// angkanya tanpa mengukur.

// Kata tanya bukan sinyal. Tanpa penyaringan questionWords, "apa itu jurnal"
// mencocokkan setiap potongan yang kebetulan memuat "apa" atau "itu", dan
// potongan yang benar tenggelam.
func TestAsk_QuestionWordsDoNotDriveRanking(t *testing.T) {
	resetCorpus(t)
	dir := writeCorpus(t, sampleCorpus())
	srv := newServer(t, dir, nil)
	mustIngest(t, srv.URL)

	resp := postJSON(t, srv.URL+"/ask", map[string]any{"question": "apa itu jurnal umum"})
	requireStatus(t, resp, http.StatusOK)
	var ask askResponseView
	resp.decode(t, &ask)

	if len(ask.Sources) == 0 {
		t.Fatal("mau setidaknya satu kutipan")
	}
	for _, term := range ask.Sources[0].MatchedTerms {
		switch term {
		case "apa", "itu", "yang", "dan":
			t.Errorf("kata tanya %q ikut dihitung sebagai kecocokan", term)
		}
	}
	if !strings.Contains(strings.ToLower(ask.Sources[0].Heading), "jurnal") {
		t.Errorf("kutipan teratas = %q, mau bagian tentang jurnal", ask.Sources[0].Heading)
	}
}

// Pertanyaan yang cuma menyerempet satu kata umum dijawab NO_MATCH, bukan
// disodori kutipan yang kebetulan memuat kata itu. Ini yang membedakan
// "dokumentasinya memang tidak membahas" dari "pencariannya meleset" -- dan
// yang pertama jauh lebih jujur untuk ditampilkan.
func TestAsk_WeakCoverageIsReportedAsNoMatch(t *testing.T) {
	resetCorpus(t)
	dir := writeCorpus(t, sampleCorpus())
	stub := &stubAnswerer{answer: "seharusnya tidak dipanggil"}
	srv := newServer(t, dir, stub)
	mustIngest(t, srv.URL)

	// "dihitung" ada di korpus (bagian OEE), tapi selebihnya tidak: topiknya
	// tidak dibahas sama sekali.
	resp := postJSON(t, srv.URL+"/ask", map[string]any{"question": "bagaimana pajak penghasilan karyawan ekspatriat dihitung"})
	requireStatus(t, resp, http.StatusOK)
	var ask askResponseView
	resp.decode(t, &ask)

	if ask.Outcome != "NO_MATCH" {
		t.Errorf("outcome = %q, mau NO_MATCH (kutipan: %+v)", ask.Outcome, ask.Sources)
	}
	if stub.calls != 0 {
		t.Errorf("model dipanggil %d kali padahal bahannya menyerempet, mau 0", stub.calls)
	}
}
