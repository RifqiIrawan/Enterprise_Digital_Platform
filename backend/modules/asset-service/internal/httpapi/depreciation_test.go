package httpapi_test

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type assetView struct {
	ID                      string  `json:"id"`
	AssetCode               string  `json:"asset_code"`
	Status                  string  `json:"status"`
	AcquisitionCost         float64 `json:"acquisition_cost"`
	SalvageValue            float64 `json:"salvage_value"`
	UsefulLifeMonths        *int    `json:"useful_life_months"`
	DepreciationMethod      string  `json:"depreciation_method"`
	AccumulatedDepreciation float64 `json:"accumulated_depreciation"`
	BookValue               float64 `json:"book_value"`
	DepreciationStartDate   *string `json:"depreciation_start_date"`
}

type depreciationEntryView struct {
	AssetID         string  `json:"asset_id"`
	AssetCode       string  `json:"asset_code"`
	Method          string  `json:"method"`
	Amount          float64 `json:"amount"`
	BookValueBefore float64 `json:"book_value_before"`
	BookValueAfter  float64 `json:"book_value_after"`
}

type depreciationRunView struct {
	ID             string                  `json:"id"`
	Period         string                  `json:"period"`
	Status         string                  `json:"status"`
	AssetCount     int                     `json:"asset_count"`
	TotalAmount    float64                 `json:"total_amount"`
	JournalEntryID *string                 `json:"journal_entry_id"`
	PostedAt       *string                 `json:"posted_at"`
	Entries        []depreciationEntryView `json:"entries"`
}

func deleteJSON(t *testing.T, url string) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodDelete, url, nil, uuid.NewString())
}

func requireMoney(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.005 {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

// mustSeedDepreciableAsset membuat aset yang memang disusutkan: umur manfaat,
// metode, dan tanggal mulai terisi.
func mustSeedDepreciableAsset(t *testing.T, srv *httptest.Server, companyID string, cost, salvage float64, lifeMonths int, method, startDate string) assetView {
	t.Helper()
	code := "AST-" + uuid.NewString()[:8]
	resp := postJSON(t, srv.URL+"/assets", map[string]any{
		"company_id": companyID, "asset_code": code, "name": "Aset Susut " + code,
		"acquisition_cost": cost, "salvage_value": salvage, "useful_life_months": lifeMonths,
		"depreciation_method": method, "depreciation_start_date": startDate,
	})
	requireStatus(t, resp, http.StatusCreated)
	var a assetView
	resp.decode(t, &a)
	return a
}

func mustCreateRun(t *testing.T, srv *httptest.Server, companyID, period string) depreciationRunView {
	t.Helper()
	resp := postJSON(t, srv.URL+"/depreciation-runs", map[string]any{"company_id": companyID, "period": period})
	requireStatus(t, resp, http.StatusCreated)
	var run depreciationRunView
	resp.decode(t, &run)
	return run
}

func mustPostRun(t *testing.T, srv *httptest.Server, runID string) depreciationRunView {
	t.Helper()
	resp := postJSON(t, srv.URL+"/depreciation-runs/"+runID+"/post", map[string]any{
		"expense_account_id": uuid.NewString(), "accumulated_depreciation_account_id": uuid.NewString(),
	})
	requireStatus(t, resp, http.StatusOK)
	var run depreciationRunView
	resp.decode(t, &run)
	return run
}

func fetchAsset(t *testing.T, srv *httptest.Server, companyID, assetID string) assetView {
	t.Helper()
	var assets []assetView
	getJSON(t, srv.URL+"/assets?company_id="+companyID).decode(t, &assets)
	for _, a := range assets {
		if a.ID == assetID {
			return a
		}
	}
	t.Fatalf("aset %s tidak ada di daftar company %s", assetID, companyID)
	return assetView{}
}

func TestCreateDepreciationRun_StraightLine(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	// 12.000.000 tanpa nilai residu, umur 12 bulan -> 1.000.000 per bulan.
	asset := mustSeedDepreciableAsset(t, srv, companyID, 12000000, 0, 12, "STRAIGHT_LINE", "2026-01-01")

	run := mustCreateRun(t, srv, companyID, "2026-01")
	if run.Status != "DRAFT" || run.AssetCount != 1 {
		t.Fatalf("run = %+v, want DRAFT dengan 1 aset", run)
	}
	requireMoney(t, "total_amount", run.TotalAmount, 1000000)
	if len(run.Entries) != 1 {
		t.Fatalf("entries = %+v, want 1", run.Entries)
	}
	e := run.Entries[0]
	if e.AssetID != asset.ID || e.AssetCode != asset.AssetCode || e.Method != "STRAIGHT_LINE" {
		t.Errorf("entry = %+v, want aset %s metode STRAIGHT_LINE", e, asset.AssetCode)
	}
	requireMoney(t, "book_value_before", e.BookValueBefore, 12000000)
	requireMoney(t, "book_value_after", e.BookValueAfter, 11000000)

	// DRAFT belum menyentuh akumulasi aset -- itu baru terjadi saat diposting.
	reloaded := fetchAsset(t, srv, companyID, asset.ID)
	requireMoney(t, "accumulated_depreciation", reloaded.AccumulatedDepreciation, 0)
	requireMoney(t, "book_value", reloaded.BookValue, 12000000)
}

// Saldo menurun ganda dihitung dari NILAI BUKU, jadi run kedua harus memakai
// akumulasi hasil posting run pertama -- bukan mengulang angka yang sama.
func TestDepreciationRun_DecliningBalanceUsesUpdatedBookValue(t *testing.T) {
	srv, _ := newServerWithFinanceStub(t, false)
	companyID := newCompanyID(t)
	// 10.000.000, umur 20 bulan -> tarif 2/20 = 10% dari nilai buku.
	asset := mustSeedDepreciableAsset(t, srv, companyID, 10000000, 0, 20, "DECLINING_BALANCE", "2026-01-01")

	first := mustCreateRun(t, srv, companyID, "2026-01")
	requireMoney(t, "penyusutan bulan 1", first.TotalAmount, 1000000)
	mustPostRun(t, srv, first.ID)

	afterFirst := fetchAsset(t, srv, companyID, asset.ID)
	requireMoney(t, "akumulasi setelah bulan 1", afterFirst.AccumulatedDepreciation, 1000000)
	requireMoney(t, "nilai buku setelah bulan 1", afterFirst.BookValue, 9000000)

	second := mustCreateRun(t, srv, companyID, "2026-02")
	requireMoney(t, "penyusutan bulan 2", second.TotalAmount, 900000) // 10% dari 9.000.000
	requireMoney(t, "book_value_before bulan 2", second.Entries[0].BookValueBefore, 9000000)
}

// Bulan terakhir dipotong supaya nilai buku berhenti tepat di nilai residu,
// tidak melewatinya.
func TestDepreciationRun_ClampsToSalvageValue(t *testing.T) {
	srv, _ := newServerWithFinanceStub(t, false)
	companyID := newCompanyID(t)
	// Saldo menurun 50% dari 1.000.000 = 500.000, padahal yang boleh disusutkan
	// tinggal 100.000 (residu 900.000).
	asset := mustSeedDepreciableAsset(t, srv, companyID, 1000000, 900000, 4, "DECLINING_BALANCE", "2026-01-01")

	run := mustCreateRun(t, srv, companyID, "2026-01")
	requireMoney(t, "penyusutan terpotong", run.TotalAmount, 100000)
	requireMoney(t, "book_value_after", run.Entries[0].BookValueAfter, 900000)
	mustPostRun(t, srv, run.ID)

	reloaded := fetchAsset(t, srv, companyID, asset.ID)
	requireMoney(t, "nilai buku akhir", reloaded.BookValue, 900000)

	// Aset yang sudah mentok di residu tidak ikut periode berikutnya.
	resp := postJSON(t, srv.URL+"/depreciation-runs", map[string]any{"company_id": companyID, "period": "2026-02"})
	requireStatus(t, resp, http.StatusConflict)
}

func TestCreateDepreciationRun_SkipsIneligibleAssets(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	// Tanpa umur manfaat: memang tidak disusutkan.
	requireStatus(t, postJSON(t, srv.URL+"/assets", map[string]any{
		"company_id": companyID, "asset_code": "AST-" + uuid.NewString()[:8], "name": "Tanah",
		"acquisition_cost": 5000000, "acquisition_date": "2026-01-01",
	}), http.StatusCreated)

	// Mulai disusutkan setelah periode yang dihitung.
	mustSeedDepreciableAsset(t, srv, companyID, 6000000, 0, 12, "STRAIGHT_LINE", "2026-06-01")

	// Sudah dilepas.
	disposed := mustSeedDepreciableAsset(t, srv, companyID, 6000000, 0, 12, "STRAIGHT_LINE", "2026-01-01")
	requireStatus(t, doRequest(t, http.MethodPut, srv.URL+"/assets/"+disposed.ID, map[string]any{
		"name": "Aset Dilepas", "status": "DISPOSED", "useful_life_months": 12, "depreciation_method": "STRAIGHT_LINE",
	}, ""), http.StatusOK)

	resp := postJSON(t, srv.URL+"/depreciation-runs", map[string]any{"company_id": companyID, "period": "2026-01"})
	requireStatus(t, resp, http.StatusConflict)

	// Satu aset yang memenuhi syarat sudah cukup untuk membuat run jalan --
	// dan hanya dia yang masuk.
	eligible := mustSeedDepreciableAsset(t, srv, companyID, 12000000, 0, 12, "STRAIGHT_LINE", "2026-01-01")
	run := mustCreateRun(t, srv, companyID, "2026-01")
	if run.AssetCount != 1 || run.Entries[0].AssetID != eligible.ID {
		t.Fatalf("run = %+v, want hanya aset %s", run, eligible.AssetCode)
	}
}

func TestCreateDepreciationRun_ValidationAndOrdering(t *testing.T) {
	srv, _ := newServerWithFinanceStub(t, false)
	companyID := newCompanyID(t)
	mustSeedDepreciableAsset(t, srv, companyID, 12000000, 0, 60, "STRAIGHT_LINE", "2026-01-01")

	t.Run("field wajib", func(t *testing.T) {
		requireStatus(t, postJSON(t, srv.URL+"/depreciation-runs", map[string]any{"company_id": companyID}), http.StatusBadRequest)
	})
	t.Run("period bukan YYYY-MM", func(t *testing.T) {
		requireStatus(t, postJSON(t, srv.URL+"/depreciation-runs", map[string]any{"company_id": companyID, "period": "Januari 2026"}), http.StatusBadRequest)
	})

	first := mustCreateRun(t, srv, companyID, "2026-01")

	t.Run("run DRAFT lain menghalangi periode berikutnya", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/depreciation-runs", map[string]any{"company_id": companyID, "period": "2026-02"})
		requireStatus(t, resp, http.StatusConflict)
		if got := resp.errorMessage(); got == "" {
			t.Error("penolakan seharusnya menyebutkan periode DRAFT yang menghalangi")
		}
	})

	mustPostRun(t, srv, first.ID)

	t.Run("periode yang sama tidak bisa dihitung dua kali", func(t *testing.T) {
		requireStatus(t, postJSON(t, srv.URL+"/depreciation-runs", map[string]any{"company_id": companyID, "period": "2026-01"}), http.StatusConflict)
	})
	t.Run("tidak bisa menghitung mundur setelah periode lebih baru diposting", func(t *testing.T) {
		requireStatus(t, postJSON(t, srv.URL+"/depreciation-runs", map[string]any{"company_id": companyID, "period": "2025-12"}), http.StatusConflict)
	})
	t.Run("periode berikutnya boleh setelah yang sebelumnya diposting", func(t *testing.T) {
		requireStatus(t, postJSON(t, srv.URL+"/depreciation-runs", map[string]any{"company_id": companyID, "period": "2026-02"}), http.StatusCreated)
	})
}

func TestPostDepreciationRun_JournalAndAccumulation(t *testing.T) {
	srv, calls := newServerWithFinanceStub(t, false)
	companyID := newCompanyID(t)
	asset := mustSeedDepreciableAsset(t, srv, companyID, 12000000, 0, 12, "STRAIGHT_LINE", "2026-01-01")
	run := mustCreateRun(t, srv, companyID, "2026-01")

	expenseAccount, accumulatedAccount := uuid.NewString(), uuid.NewString()
	resp := postJSON(t, srv.URL+"/depreciation-runs/"+run.ID+"/post", map[string]any{
		"expense_account_id": expenseAccount, "accumulated_depreciation_account_id": accumulatedAccount,
	})
	requireStatus(t, resp, http.StatusOK)
	var posted depreciationRunView
	resp.decode(t, &posted)
	if posted.Status != "POSTED" || posted.JournalEntryID == nil || posted.PostedAt == nil {
		t.Fatalf("run setelah posting = %+v", posted)
	}

	if len(*calls) != 2 {
		t.Fatalf("expected 2 calls to finance-service (create + post), got %d", len(*calls))
	}
	var sent struct {
		CompanyID     string `json:"company_id"`
		EntryDate     string `json:"entry_date"`
		ReferenceType string `json:"reference_type"`
		ReferenceID   string `json:"reference_id"`
		Lines         []struct {
			AccountID    string  `json:"account_id"`
			DebitAmount  float64 `json:"debit_amount"`
			CreditAmount float64 `json:"credit_amount"`
		} `json:"lines"`
	}
	if err := json.Unmarshal((*calls)[0].body, &sent); err != nil {
		t.Fatalf("decode journal body: %v", err)
	}
	if sent.CompanyID != companyID || sent.ReferenceType != "ASSET_DEPRECIATION" || sent.ReferenceID != run.ID {
		t.Errorf("metadata jurnal = %+v", sent)
	}
	// Tanggal jurnal = hari terakhir periode, bukan hari ini.
	if sent.EntryDate != "2026-01-31" {
		t.Errorf("entry_date = %q, want 2026-01-31", sent.EntryDate)
	}
	if len(sent.Lines) != 2 {
		t.Fatalf("lines = %+v, want 2", sent.Lines)
	}
	if sent.Lines[0].AccountID != expenseAccount || sent.Lines[0].DebitAmount != 1000000 || sent.Lines[0].CreditAmount != 0 {
		t.Errorf("baris debit = %+v, want beban 1.000.000", sent.Lines[0])
	}
	if sent.Lines[1].AccountID != accumulatedAccount || sent.Lines[1].CreditAmount != 1000000 || sent.Lines[1].DebitAmount != 0 {
		t.Errorf("baris kredit = %+v, want akumulasi 1.000.000", sent.Lines[1])
	}

	reloaded := fetchAsset(t, srv, companyID, asset.ID)
	requireMoney(t, "akumulasi setelah posting", reloaded.AccumulatedDepreciation, 1000000)
	requireMoney(t, "nilai buku setelah posting", reloaded.BookValue, 11000000)

	// Posting kedua kalinya ditolak, dan tidak menghasilkan jurnal baru.
	requireStatus(t, postJSON(t, srv.URL+"/depreciation-runs/"+run.ID+"/post", map[string]any{
		"expense_account_id": expenseAccount, "accumulated_depreciation_account_id": accumulatedAccount,
	}), http.StatusConflict)
	if len(*calls) != 2 {
		t.Errorf("finance-service dipanggil lagi pada posting kedua: %d panggilan", len(*calls))
	}
}

// finance-service gagal: run harus tetap DRAFT dan akumulasi aset tidak boleh
// bergerak, supaya postingnya bisa diulang setelah masalahnya beres.
func TestPostDepreciationRun_FinanceFailureLeavesDraft(t *testing.T) {
	srv, calls := newServerWithFinanceStub(t, true)
	companyID := newCompanyID(t)
	asset := mustSeedDepreciableAsset(t, srv, companyID, 12000000, 0, 12, "STRAIGHT_LINE", "2026-01-01")
	run := mustCreateRun(t, srv, companyID, "2026-01")

	requireStatus(t, postJSON(t, srv.URL+"/depreciation-runs/"+run.ID+"/post", map[string]any{
		"expense_account_id": uuid.NewString(), "accumulated_depreciation_account_id": uuid.NewString(),
	}), http.StatusBadGateway)
	if len(*calls) != 1 {
		t.Errorf("expected 1 (failing) finance call, got %d", len(*calls))
	}

	var reloadedRun depreciationRunView
	getJSON(t, srv.URL+"/depreciation-runs/"+run.ID).decode(t, &reloadedRun)
	if reloadedRun.Status != "DRAFT" || reloadedRun.JournalEntryID != nil {
		t.Errorf("run = %+v, want tetap DRAFT tanpa journal_entry_id", reloadedRun)
	}
	requireMoney(t, "akumulasi", fetchAsset(t, srv, companyID, asset.ID).AccumulatedDepreciation, 0)
}

func TestPostDepreciationRun_ValidationAndNotFound(t *testing.T) {
	srv, _ := newServerWithFinanceStub(t, false)
	companyID := newCompanyID(t)
	mustSeedDepreciableAsset(t, srv, companyID, 12000000, 0, 12, "STRAIGHT_LINE", "2026-01-01")
	run := mustCreateRun(t, srv, companyID, "2026-01")

	requireStatus(t, postJSON(t, srv.URL+"/depreciation-runs/"+run.ID+"/post", map[string]any{}), http.StatusBadRequest)
	requireStatus(t, postJSON(t, srv.URL+"/depreciation-runs/"+run.ID+"/post", map[string]any{
		"expense_account_id": uuid.NewString(),
	}), http.StatusBadRequest)
	requireStatus(t, postJSON(t, srv.URL+"/depreciation-runs/"+uuid.NewString()+"/post", map[string]any{
		"expense_account_id": uuid.NewString(), "accumulated_depreciation_account_id": uuid.NewString(),
	}), http.StatusNotFound)
	requireStatus(t, getJSON(t, srv.URL+"/depreciation-runs/"+uuid.NewString()), http.StatusNotFound)
}

func TestDeleteDepreciationRun_OnlyDraft(t *testing.T) {
	srv, _ := newServerWithFinanceStub(t, false)
	companyID := newCompanyID(t)
	mustSeedDepreciableAsset(t, srv, companyID, 12000000, 0, 12, "STRAIGHT_LINE", "2026-01-01")

	draft := mustCreateRun(t, srv, companyID, "2026-01")
	requireStatus(t, deleteJSON(t, srv.URL+"/depreciation-runs/"+draft.ID), http.StatusNoContent)
	requireStatus(t, getJSON(t, srv.URL+"/depreciation-runs/"+draft.ID), http.StatusNotFound)

	// Setelah dihapus, periode yang sama boleh dihitung ulang.
	again := mustCreateRun(t, srv, companyID, "2026-01")
	mustPostRun(t, srv, again.ID)
	requireStatus(t, deleteJSON(t, srv.URL+"/depreciation-runs/"+again.ID), http.StatusConflict)
}

func TestListDepreciationRuns_ScopedAndFiltered(t *testing.T) {
	srv, _ := newServerWithFinanceStub(t, false)
	companyID := newCompanyID(t)
	mustSeedDepreciableAsset(t, srv, companyID, 12000000, 0, 60, "STRAIGHT_LINE", "2026-01-01")
	first := mustCreateRun(t, srv, companyID, "2026-01")
	mustPostRun(t, srv, first.ID)
	mustCreateRun(t, srv, companyID, "2026-02")

	var all []depreciationRunView
	getJSON(t, srv.URL+"/depreciation-runs?company_id="+companyID).decode(t, &all)
	if len(all) != 2 || all[0].Period != "2026-02" {
		t.Fatalf("daftar run = %+v, want 2 dengan periode terbaru lebih dulu", all)
	}

	var drafts []depreciationRunView
	getJSON(t, srv.URL+"/depreciation-runs?company_id="+companyID+"&status=DRAFT").decode(t, &drafts)
	if len(drafts) != 1 || drafts[0].Period != "2026-02" {
		t.Fatalf("filter DRAFT = %+v", drafts)
	}

	var otherCompany []depreciationRunView
	getJSON(t, srv.URL+"/depreciation-runs?company_id="+newCompanyID(t)).decode(t, &otherCompany)
	if len(otherCompany) != 0 {
		t.Errorf("run company lain ikut terbawa: %+v", otherCompany)
	}

	requireStatus(t, getJSON(t, srv.URL+"/depreciation-runs"), http.StatusBadRequest)
}

func TestAsset_DepreciationFieldValidation(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := map[string]map[string]any{
		"metode tidak dikenal": {
			"company_id": companyID, "asset_code": "AST-" + uuid.NewString()[:8], "name": "X",
			"acquisition_cost": 1000, "depreciation_method": "SUM_OF_YEARS",
		},
		"umur manfaat nol": {
			"company_id": companyID, "asset_code": "AST-" + uuid.NewString()[:8], "name": "X",
			"acquisition_cost": 1000, "useful_life_months": 0,
		},
		"residu melebihi harga perolehan": {
			"company_id": companyID, "asset_code": "AST-" + uuid.NewString()[:8], "name": "X",
			"acquisition_cost": 1000, "salvage_value": 2000,
		},
		"tanggal mulai bukan tanggal": {
			"company_id": companyID, "asset_code": "AST-" + uuid.NewString()[:8], "name": "X",
			"acquisition_cost": 1000, "depreciation_start_date": "awal tahun",
		},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			requireStatus(t, postJSON(t, srv.URL+"/assets", payload), http.StatusBadRequest)
		})
	}

	// Tanggal mulai penyusutan yang tidak diisi jatuh ke tanggal perolehan.
	resp := postJSON(t, srv.URL+"/assets", map[string]any{
		"company_id": companyID, "asset_code": "AST-" + uuid.NewString()[:8], "name": "Mesin",
		"acquisition_cost": 1000000, "acquisition_date": "2026-03-05", "useful_life_months": 10,
	})
	requireStatus(t, resp, http.StatusCreated)
	var a assetView
	resp.decode(t, &a)
	if a.DepreciationStartDate == nil || (*a.DepreciationStartDate)[:10] != "2026-03-05" {
		t.Errorf("depreciation_start_date = %v, want jatuh ke acquisition_date", a.DepreciationStartDate)
	}
	if a.DepreciationMethod != "STRAIGHT_LINE" {
		t.Errorf("depreciation_method = %q, want STRAIGHT_LINE sebagai default", a.DepreciationMethod)
	}
}

// Klien lama yang hanya mengirim nama & status (halaman aset sebelum Fase 5)
// tidak boleh diam-diam menghapus umur manfaat aset -- kalau itu terjadi,
// asetnya berhenti disusutkan tanpa ada yang menyadarinya.
func TestUpdateAsset_KeepsDepreciationFieldsWhenOmitted(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	asset := mustSeedDepreciableAsset(t, srv, companyID, 12000000, 1000000, 24, "DECLINING_BALANCE", "2026-01-01")

	requireStatus(t, doRequest(t, http.MethodPut, srv.URL+"/assets/"+asset.ID, map[string]any{
		"name": "Nama Baru", "status": "ACTIVE",
	}, ""), http.StatusOK)

	reloaded := fetchAsset(t, srv, companyID, asset.ID)
	if reloaded.UsefulLifeMonths == nil || *reloaded.UsefulLifeMonths != 24 {
		t.Errorf("useful_life_months = %v, want tetap 24", reloaded.UsefulLifeMonths)
	}
	if reloaded.DepreciationMethod != "DECLINING_BALANCE" {
		t.Errorf("depreciation_method = %q, want tetap DECLINING_BALANCE", reloaded.DepreciationMethod)
	}
	requireMoney(t, "salvage_value", reloaded.SalvageValue, 1000000)

	// Mengosongkan umur manfaat tetap bisa, tapi harus eksplisit.
	requireStatus(t, doRequest(t, http.MethodPut, srv.URL+"/assets/"+asset.ID, map[string]any{
		"name": "Nama Baru", "status": "ACTIVE", "useful_life_months": 0,
	}, ""), http.StatusOK)
	if fetchAsset(t, srv, companyID, asset.ID).UsefulLifeMonths != nil {
		t.Error("useful_life_months seharusnya kosong setelah dikirim 0")
	}
}
