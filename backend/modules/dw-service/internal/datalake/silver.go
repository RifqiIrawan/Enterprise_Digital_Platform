package datalake

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
)

// Silver adalah lapisan kedua data lake: keadaan TERKINI setiap baris fact,
// sudah dibersihkan dari duplikat. Bronze append-only -- baris yang berubah
// (atau yang ditulis dua kali oleh batch ETL dan Kafka streaming ETL) muncul
// di banyak file, dan membaca Bronze mentah-mentah menghitungnya berkali-kali.
//
// Aturannya SAMA dengan ReplacingMergeTree(synced_at) di ClickHouse: kunci
// (CompanyID, id-baris), versi terbaru menang. Bronze dibaca berurutan
// menurut key object (tanggal lalu unix-nano synced_at), jadi "terbaru" di
// sini berarti synced_at terbesar -- keduanya sengaja tidak boleh berbeda
// pendapat tentang baris mana yang benar (lihat silver_test.go).
//
// Baris tanpa kunci, dengan kunci UUID kosong, atau bukan JSON tidak
// dibuang diam-diam: dihitung, dan disimpan di rejected.jsonl supaya bisa
// diperiksa. Kalau tidak ada yang ditolak, berkas lama dihapus supaya tidak
// ada "penolakan" basi yang tersisa dari build sebelumnya.
//
// Build INKREMENTAL: Silver disimpan terurut menurut kunci, ditemani berkas
// state kecil (`_state.json`) berisi penanda objek Bronze terakhir yang sudah
// "mengendap". Build berikutnya hanya membaca objek Bronze yang lebih baru dari
// penanda itu dan MENGGABUNGKANNYA ke Silver lama sebagai aliran (merge dua
// daftar terurut), sehingga biaya waktu mengikuti data baru, bukan seluruh
// riwayat Bronze, dan Silver lama tidak pernah dimuat utuh ke memori.
//
// Penanda hanya maju sampai objek yang umurnya di atas settleLag. Objek yang
// lebih muda dibaca ulang tiap build (idempoten: digabung dengan urutan yang
// sama), karena penulis lain (batch ETL dan streaming ETL) bisa menulis objek
// dengan key yang lebih LAMA sesudah build membaca daftar objek. Kalau asumsi
// itu pernah dilanggar, build penuh otomatis (tiap fullRebuildEvery, atau
// dipaksa dengan full=true) memulihkannya, dan rekonsiliasi akan melihatnya.

const silverPrefix = "silver/"

const zeroUUID = "00000000-0000-0000-0000-000000000000"

const (
	silverStateVersion = 1
	// settleLag: objek Bronze yang lebih muda dari ini belum dianggap "mengendap".
	settleLag = 10 * time.Minute
	// fullRebuildEvery: seberapa lama Silver boleh bertahan tanpa dibangun ulang
	// penuh dari seluruh Bronze.
	fullRebuildEvery = 24 * time.Hour
	// silverPartSize: ukuran bagian unggahan multipart saat Silver dialirkan ke
	// MinIO tanpa ukuran yang diketahui di muka.
	silverPartSize = 16 << 20
)

// errSilverCorrupt: Silver lama tidak bisa digabung (bukan JSON valid atau tidak
// terurut). BuildSilver menanganinya dengan sekali build penuh.
var errSilverCorrupt = errors.New("silver lama rusak atau tidak terurut")

// SilverFacts memetakan nama fact (prefix di Bronze) ke nama field Go yang
// jadi id barisnya. Nama field, bukan nama kolom: Bronze menulis struct Go
// apa adanya lewat encoding/json (lihat WriteJSONLines). Kalau fact baru
// ditambahkan ke ETL tapi lupa didaftarkan di sini, TestSilverFactsCoverEveryFact
// gagal.
var SilverFacts = map[string]string{
	"finance_journal_lines":  "LineID",
	"sales_order_lines":      "LineID",
	"inventory_movements":    "MovementID",
	"hr_payroll_details":     "DetailID",
	"hr_leave_requests":      "LeaveID",
	"hr_kpi_reviews":         "ReviewID",
	"purchasing_order_lines": "LineID",
	"production_work_orders": "WOID",
	"production_oee":         "RunID",
	"qc_inspections":         "InspectionID",
	"asset_maintenance":      "ScheduleID",
	"iot_readings":           "ReadingID",
	"opportunities":          "OpportunityID",
	"tickets":                "TicketID",
	"delivery_orders":        "DeliveryID",
	"timesheets":             "TimesheetID",
	"order_items":            "LineID",
}

// LiveKeys mengembalikan kunci ("<CompanyID>/<id>") semua baris yang MASIH ADA
// di sumber untuk satu fact. Bronze append-only tidak pernah tahu bahwa sebuah
// baris dihapus di sumber, jadi tanpa ini Silver menyimpan baris hantu
// selamanya. nil = jangan memangkas.
type LiveKeys func(ctx context.Context) (map[string]struct{}, error)

// SilverStats meringkas satu build Silver. Selalu berlaku (dan diuji):
//
//	PreviousSilverRows + BronzeRows = SilverRows + DuplicatesDropped + Rejected + Pruned
//
// BronzeRows dan BronzeObjects adalah yang DIBACA build ini (pada build
// inkremental hanya objek yang lebih baru dari penanda, plus objek yang belum
// mengendap); PreviousSilverRows adalah isi Silver lama yang digabungkan (0 pada
// build penuh).
type SilverStats struct {
	Fact string `json:"fact"`
	// Mode: "full" (seluruh Bronze) atau "incremental".
	Mode               string `json:"mode"`
	BronzeObjects      int    `json:"bronze_objects"`
	BronzeRows         int    `json:"bronze_rows"`
	PreviousSilverRows int    `json:"previous_silver_rows"`
	SilverRows         int    `json:"silver_rows"`
	// DuplicatesDropped: versi lama sebuah baris yang tergantikan versi lebih baru,
	// baik di antara objek Bronze yang dibaca maupun terhadap Silver lama.
	DuplicatesDropped int `json:"duplicates_dropped"`
	Rejected          int `json:"rejected"`
	// Pruned: baris yang sudah dihapus di sumber. Bronze tetap menyimpannya
	// sebagai riwayat; hanya Silver (keadaan terkini) yang membuangnya.
	Pruned int `json:"pruned"`
}

type silverState struct {
	Version    int       `json:"version"`
	Marker     string    `json:"bronze_marker"`
	LastFullAt time.Time `json:"last_full_at"`
	BuiltAt    time.Time `json:"built_at"`
}

func silverKey(fact string) string       { return silverPrefix + fact + "/current.jsonl" }
func silverRejectKey(fact string) string { return silverPrefix + fact + "/rejected.jsonl" }
func silverStateKey(fact string) string  { return silverPrefix + fact + "/_state.json" }

// BuildSilver membangun Silver satu fact. full=true memaksa build penuh dari
// SELURUH Bronze; selain itu build inkremental dipakai kalau ada state yang
// valid, Silver lama ada, dan build penuh terakhir masih di bawah
// fullRebuildEvery. Hasilnya untuk data yang sama identik dengan build penuh
// (diuji), dan Silver lama dibiarkan utuh kalau build gagal.
func (c *Client) BuildSilver(ctx context.Context, fact string, live LiveKeys, full bool) (SilverStats, error) {
	idField, ok := SilverFacts[fact]
	if !ok {
		return SilverStats{Fact: fact}, fmt.Errorf("datalake: fact %q tidak dikenal", fact)
	}
	return c.buildSilver(ctx, fact, idField, live, full)
}

// buildSilver adalah BuildSilver dengan field id yang sudah ditentukan;
// dipisah supaya test bisa memakai nama fact sekali-pakai tanpa menyentuh
// Bronze fact sungguhan di bucket yang sama.
func (c *Client) buildSilver(ctx context.Context, fact, idField string, live LiveKeys, full bool) (SilverStats, error) {
	st, err := c.buildSilverOnce(ctx, fact, idField, live, full)
	if errors.Is(err, errSilverCorrupt) && !full {
		return c.buildSilverOnce(ctx, fact, idField, live, true)
	}
	return st, err
}

func (c *Client) readSilverState(ctx context.Context, fact string) (silverState, bool) {
	data, err := c.Get(ctx, silverStateKey(fact))
	if err != nil {
		return silverState{}, false
	}
	var s silverState
	if json.Unmarshal(data, &s) != nil || s.Version != silverStateVersion {
		return silverState{}, false
	}
	return s, true
}

func (c *Client) objectExists(ctx context.Context, key string) bool {
	_, err := c.mc.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{})
	return err == nil
}

// bronzeObjectTime membaca unix-nano synced_at dari nama objek Bronze
// (`<fact>/YYYY/MM/DD/<unix-nano>.jsonl`).
func bronzeObjectTime(key string) (time.Time, bool) {
	n, err := strconv.ParseInt(strings.TrimSuffix(path.Base(key), ".jsonl"), 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(0, n), true
}

func (c *Client) buildSilverOnce(ctx context.Context, fact, idField string, live LiveKeys, full bool) (SilverStats, error) {
	stats := SilverStats{Fact: fact, Mode: "incremental"}
	if c == nil {
		return stats, fmt.Errorf("datalake: data lake tidak tersedia")
	}

	now := time.Now()
	state, haveState := c.readSilverState(ctx, fact)
	if full || !haveState || now.Sub(state.LastFullAt) > fullRebuildEvery || !c.objectExists(ctx, silverKey(fact)) {
		stats.Mode = "full"
		state = silverState{}
	}
	marker := state.Marker // kosong pada build penuh: tidak ada yang dilewati

	keys, err := c.ListKeys(ctx, fact+"/")
	if err != nil {
		return stats, err
	}

	// Baris Bronze baru, versi terbaru per kunci. Objek dibaca dari yang terlama
	// ke terbaru, jadi menimpa = versi terbaru menang.
	delta := map[string][]byte{}
	rejectedSeen := map[string]struct{}{}
	var rejected [][]byte
	reject := func(line []byte) {
		stats.Rejected++
		if _, dup := rejectedSeen[string(line)]; !dup {
			rejectedSeen[string(line)] = struct{}{}
			rejected = append(rejected, append([]byte(nil), line...))
		}
	}
	newMarker, unsettled := marker, false
	for _, k := range keys {
		if k <= marker {
			continue
		}
		data, err := c.Get(ctx, k)
		if err != nil {
			return stats, err
		}
		stats.BronzeObjects++

		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for sc.Scan() {
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 {
				continue
			}
			stats.BronzeRows++
			key, ok := rowKey(idField, line)
			if !ok {
				reject(line)
				continue
			}
			if _, dup := delta[key]; dup {
				stats.DuplicatesDropped++
			}
			delta[key] = append([]byte(nil), line...)
		}
		if err := sc.Err(); err != nil {
			return stats, fmt.Errorf("scan %s: %w", k, err)
		}

		// Penanda maju hanya melewati awalan objek yang sudah mengendap.
		if t, ok := bronzeObjectTime(k); !unsettled && ok && now.Sub(t) >= settleLag {
			newMarker = k
		} else {
			unsettled = true
		}
	}

	// Memangkas baris yang sudah dihapus di sumber. URUTANNYA penting: Bronze
	// dibaca DULU, kunci hidup diambil SESUDAHNYA. Kebalikannya bisa memangkas
	// baris yang baru dibuat di antara dua langkah itu (ada di Bronze, belum ada
	// di daftar kunci hidup yang lebih tua). Kalau sumber tidak bisa ditanya,
	// build fact ini GAGAL dan Silver lama dibiarkan -- lebih baik usang daripada
	// menerbitkan baris hantu atau, lebih buruk, menghapus baris yang masih ada.
	var alive map[string]struct{}
	if live != nil {
		alive, err = live(ctx)
		if err != nil {
			return stats, fmt.Errorf("ambil kunci hidup dari sumber: %w", err)
		}
	}

	var prev io.Reader
	if stats.Mode == "incremental" {
		obj, err := c.mc.GetObject(ctx, c.bucket, silverKey(fact), minio.GetObjectOptions{})
		if err != nil {
			return stats, fmt.Errorf("baca silver lama: %w", err)
		}
		defer obj.Close()
		prev = obj
	}

	deltaKeys := make([]string, 0, len(delta))
	for k := range delta {
		deltaKeys = append(deltaKeys, k)
	}
	sort.Strings(deltaKeys)

	// Menggabungkan secara streaming: goroutine menulis hasil merge ke pipe,
	// PutObject membacanya. Kalau merge gagal di tengah, pipe ditutup dengan
	// galat, unggahan dibatalkan, dan Silver lama tetap utuh.
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() {
		bw := bufio.NewWriterSize(pw, 1<<20)
		err := mergeSilver(bw, prev, idField, deltaKeys, delta, alive, &stats)
		if err == nil {
			err = bw.Flush()
		}
		pw.CloseWithError(err)
		done <- err
	}()
	_, putErr := c.mc.PutObject(ctx, c.bucket, silverKey(fact), pr, -1,
		minio.PutObjectOptions{ContentType: "application/x-ndjson", PartSize: silverPartSize})
	pr.CloseWithError(putErr)
	mergeErr := <-done
	if mergeErr != nil {
		return stats, mergeErr
	}
	if putErr != nil {
		return stats, fmt.Errorf("put object %s: %w", silverKey(fact), putErr)
	}

	if err := c.writeRejected(ctx, fact, stats.Mode, rejected); err != nil {
		return stats, err
	}

	next := silverState{Version: silverStateVersion, Marker: newMarker, LastFullAt: state.LastFullAt, BuiltAt: now}
	if stats.Mode == "full" {
		next.LastFullAt = now
	}
	sb, _ := json.Marshal(next)
	if err := c.put(ctx, silverStateKey(fact), sb); err != nil {
		return stats, err
	}
	return stats, nil
}

// writeRejected menulis rejected.jsonl. Build inkremental MENAMBAH ke isi lama
// (baris yang ditolak di build sebelumnya sudah tidak dibaca lagi), tanpa
// duplikat: objek yang belum mengendap dibaca ulang dan menolak baris yang sama.
func (c *Client) writeRejected(ctx context.Context, fact, mode string, fresh [][]byte) error {
	seen := map[string]struct{}{}
	var all [][]byte
	add := func(l []byte) {
		if _, dup := seen[string(l)]; !dup {
			seen[string(l)] = struct{}{}
			all = append(all, l)
		}
	}
	if mode == "incremental" {
		old, err := c.readLines(ctx, silverRejectKey(fact))
		if err != nil {
			return err
		}
		for _, l := range old {
			add(l)
		}
	}
	for _, l := range fresh {
		add(l)
	}
	if len(all) == 0 {
		if err := c.mc.RemoveObject(ctx, c.bucket, silverRejectKey(fact), minio.RemoveObjectOptions{}); err != nil {
			return fmt.Errorf("remove stale %s: %w", silverRejectKey(fact), err)
		}
		return nil
	}
	var rb bytes.Buffer
	for _, l := range all {
		rb.Write(l)
		rb.WriteByte('\n')
	}
	return c.put(ctx, silverRejectKey(fact), rb.Bytes())
}

// mergeSilver menggabungkan Silver lama (terurut menurut kunci, boleh nil) dengan
// delta (kunci terurut di deltaKeys) menjadi satu keluaran terurut, membuang
// baris yang sudah tidak hidup di sumber. Pada kunci yang sama delta menang
// (lebih baru). Silver lama dibaca baris demi baris; yang ada di memori hanya
// delta dan daftar kunci hidup.
func mergeSilver(w io.Writer, prev io.Reader, idField string, deltaKeys []string, delta map[string][]byte, alive map[string]struct{}, stats *SilverStats) error {
	var sc *bufio.Scanner
	if prev != nil {
		sc = bufio.NewScanner(prev)
		sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	}
	idPattern, companyPattern := []byte(`"`+idField+`":"`), []byte(`"CompanyID":"`)
	lastPrev := ""
	nextPrev := func() (string, []byte, bool, error) {
		for sc != nil && sc.Scan() {
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 {
				continue
			}
			key, ok := fastKey(line, idPattern, companyPattern)
			if !ok {
				return "", nil, false, fmt.Errorf("%w: baris tanpa kunci valid", errSilverCorrupt)
			}
			if key <= lastPrev {
				return "", nil, false, fmt.Errorf("%w: kunci %q tidak naik", errSilverCorrupt, key)
			}
			lastPrev = key
			stats.PreviousSilverRows++
			return key, line, true, nil
		}
		if sc != nil && sc.Err() != nil {
			return "", nil, false, fmt.Errorf("baca silver lama: %w", sc.Err())
		}
		return "", nil, false, nil
	}

	newline := []byte("\n")
	emit := func(key string, line []byte) error {
		if alive != nil {
			if _, ok := alive[key]; !ok {
				stats.Pruned++
				return nil
			}
		}
		if _, err := w.Write(line); err != nil {
			return err
		}
		if _, err := w.Write(newline); err != nil {
			return err
		}
		stats.SilverRows++
		return nil
	}

	pk, pline, pok, err := nextPrev()
	if err != nil {
		return err
	}
	i := 0
	for pok || i < len(deltaKeys) {
		switch {
		case !pok || (i < len(deltaKeys) && deltaKeys[i] < pk):
			if err := emit(deltaKeys[i], delta[deltaKeys[i]]); err != nil {
				return err
			}
			i++
		case i >= len(deltaKeys) || pk < deltaKeys[i]:
			if err := emit(pk, pline); err != nil {
				return err
			}
			if pk, pline, pok, err = nextPrev(); err != nil {
				return err
			}
		default: // kunci sama: versi delta (lebih baru) menggantikan yang lama
			stats.DuplicatesDropped++
			if err := emit(deltaKeys[i], delta[deltaKeys[i]]); err != nil {
				return err
			}
			i++
			if pk, pline, pok, err = nextPrev(); err != nil {
				return err
			}
		}
	}
	return nil
}

// fastKey mengambil kunci dari baris Silver yang SUDAH TERVALIDASI tanpa
// meng-unmarshal seluruh baris. Aman karena baris itu JSON kompak hasil
// encoding/json dari struct datar: pola `"Nama":"` diawali tanda kutip sehingga
// tidak cocok dengan akhiran nama field lain (`ParentCompanyID`), dan nilai
// string apa pun meloloskan tanda kutipnya sebagai `\"`. Hasilnya HARUS sama
// dengan rowKey (diuji); kalau polanya tidak ketemu, build dianggap menemukan
// Silver yang rusak dan jatuh ke build penuh.
func fastKey(line, idPattern, companyPattern []byte) (string, bool) {
	company, ok1 := quotedAfter(line, companyPattern)
	id, ok2 := quotedAfter(line, idPattern)
	if ok1 && ok2 && validID(company) && validID(id) {
		return company + "/" + id, true
	}
	return "", false
}

func quotedAfter(line, pattern []byte) (string, bool) {
	i := bytes.Index(line, pattern)
	if i < 0 {
		return "", false
	}
	start := i + len(pattern)
	j := bytes.IndexByte(line[start:], '"')
	if j < 0 {
		return "", false
	}
	return string(line[start : start+j]), true
}

// EachSilver memanggil fn untuk tiap baris Silver satu fact tanpa memuat seluruh
// berkas ke memori. Fact yang belum pernah di-build dianggap kosong.
func (c *Client) EachSilver(ctx context.Context, fact string, fn func(line []byte) error) error {
	if c == nil {
		return nil
	}
	if !c.objectExists(ctx, silverKey(fact)) {
		return nil
	}
	obj, err := c.mc.GetObject(ctx, c.bucket, silverKey(fact), minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("baca %s: %w", silverKey(fact), err)
	}
	defer obj.Close()
	sc := bufio.NewScanner(obj)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		if l := bytes.TrimSpace(sc.Bytes()); len(l) > 0 {
			if err := fn(l); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

// CountSilver menghitung baris Silver satu fact secara streaming.
func (c *Client) CountSilver(ctx context.Context, fact string) (int, error) {
	n := 0
	err := c.EachSilver(ctx, fact, func([]byte) error { n++; return nil })
	return n, err
}

// ReadSilver mengembalikan baris-baris Silver satu fact (satu JSON per
// elemen). Fact yang belum pernah di-build mengembalikan slice kosong, bukan
// galat -- "belum ada" dan "kosong" sama artinya bagi pemanggil. Memuat semuanya
// ke memori: untuk data besar pakai EachSilver/CountSilver.
func (c *Client) ReadSilver(ctx context.Context, fact string) ([][]byte, error) {
	return c.readLines(ctx, silverKey(fact))
}

func (c *Client) readLines(ctx context.Context, key string) ([][]byte, error) {
	if c == nil {
		return nil, nil
	}
	if _, err := c.mc.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{}); err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, nil
		}
		return nil, fmt.Errorf("stat %s: %w", key, err)
	}
	data, err := c.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	var out [][]byte
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		if l := bytes.TrimSpace(sc.Bytes()); len(l) > 0 {
			out = append(out, append([]byte(nil), l...))
		}
	}
	return out, sc.Err()
}

func (c *Client) put(ctx context.Context, key string, data []byte) error {
	_, err := c.mc.PutObject(ctx, c.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: "application/x-ndjson"})
	if err != nil {
		return fmt.Errorf("put object %s: %w", key, err)
	}
	return nil
}

// Put dan Remove dipakai test untuk menyiapkan/membersihkan objek; API
// produksi cukup BuildSilver/BuildGold.
func (c *Client) Put(ctx context.Context, key string, data []byte) error {
	return c.put(ctx, key, data)
}

func (c *Client) Remove(ctx context.Context, key string) error {
	return c.mc.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{})
}

// rowKey adalah SATU-SATUNYA aturan kunci baris: "<CompanyID>/<id>". Silver
// memakainya untuk mendeduplikasi, dan RowKey (di bawah) memakainya untuk
// daftar kunci hidup dari sumber -- kalau keduanya berbeda satu karakter pun,
// pemangkasan akan membuang baris yang masih ada.
func rowKey(idField string, line []byte) (string, bool) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(line, &probe); err != nil {
		return "", false
	}
	rowID := unquote(probe[idField])
	companyID := unquote(probe["CompanyID"])
	if !validID(rowID) || !validID(companyID) {
		return "", false
	}
	return companyID + "/" + rowID, true
}

// RowKey menghitung kunci Silver sebuah baris fact dari bentuk JSON-nya (baris
// hasil ekstrak sumber di-marshal dengan encoding/json yang sama dengan
// WriteJSONLines, jadi kuncinya identik dengan yang ada di Bronze).
func RowKey(fact string, row any) (string, bool) {
	idField, ok := SilverFacts[fact]
	if !ok {
		return "", false
	}
	b, err := json.Marshal(row)
	if err != nil {
		return "", false
	}
	return rowKey(idField, b)
}

func unquote(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func validID(s string) bool {
	return strings.TrimSpace(s) != "" && s != zeroUUID
}
