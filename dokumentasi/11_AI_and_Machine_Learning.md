# 11 — AI & Machine Learning
## Enterprise Data Center Simulator (EDCS)

---

## 🤖 Overview

EDCS AI/ML Layer menyediakan **platform MLOps end-to-end** dari data prep hingga model serving, serta **8 pre-built ML use cases** yang terintegrasi dengan modul bisnis, ditambah fitur **Generative AI** berbasis LLM untuk asisten cerdas di setiap domain.

---

## 🏗️ ML Platform Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        ML PLATFORM                              │
│                                                                 │
│  ┌────────────────┐   ┌───────────────┐   ┌─────────────────┐  │
│  │  JupyterHub    │   │  MLflow       │   │  Feature Store  │  │
│  │  (Notebooks)   │   │  (Tracking +  │   │  (Feast)        │  │
│  │                │   │   Registry)   │   │                 │  │
│  └────────────────┘   └───────────────┘   └─────────────────┘  │
│                                                                 │
│  ┌────────────────┐   ┌───────────────┐   ┌─────────────────┐  │
│  │  Ray (Distrib) │   │  Airflow      │   │  BentoML        │  │
│  │  Training      │   │  (Pipeline    │   │  (Model Serving)│  │
│  │                │   │   Orchestrate)│   │                 │  │
│  └────────────────┘   └───────────────┘   └─────────────────┘  │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │          DATA SOURCES                                   │    │
│  │  Data Lake (Gold) │ Feature Store │ Operational APIs    │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
```

---

## 🎯 8 Pre-built ML Use Cases

### UC-01: Demand Forecasting (WMS + Sales)
```python
# Model: XGBoost + LSTM Hybrid
# Input features:
features = [
    "product_id", "product_category",
    "sales_qty_lag_7d", "sales_qty_lag_30d", "sales_qty_lag_90d",
    "day_of_week", "month", "is_holiday", "is_ramadan",
    "price", "promotion_active",
    "stock_on_hand", "lead_time_days",
    "competitor_price_index"
]

# Output: Predicted demand per product per hari (30 hari ke depan)
# Metric: MAPE < 15%
# Refresh: Daily (Airflow)
# Serving: REST API → WMS auto-reorder trigger

# Training pipeline
from sklearn.pipeline import Pipeline
from xgboost import XGBRegressor

pipeline = Pipeline([
    ("preprocessor", feature_preprocessor),
    ("model", XGBRegressor(
        n_estimators=500,
        learning_rate=0.05,
        max_depth=6,
        subsample=0.8,
        colsample_bytree=0.8,
        objective="reg:squarederror"
    ))
])

# Cross-validation dengan time-series split
from sklearn.model_selection import TimeSeriesSplit
tscv = TimeSeriesSplit(n_splits=5)
```

### UC-02: Predictive Maintenance (MES + IoT)
```python
# Model: Random Forest + Isolation Forest (anomaly detection)
# Input features:
iot_features = [
    "avg_temperature_24h", "max_vibration_1h",
    "rpm_variance_7d", "power_consumption_trend",
    "days_since_last_maintenance",
    "total_operating_hours",
    "historical_failure_count"
]

# Output: Probability of failure dalam 7 hari (per asset)
# Metric: Recall > 90% (false negative lebih mahal)
# Serving: Real-time via IoT event trigger

# Threshold-based alerting
def predict_maintenance(asset_id: str, features: dict) -> dict:
    proba = model.predict_proba([features])[0][1]
    return {
        "asset_id": asset_id,
        "failure_probability": round(proba, 4),
        "risk_level": "HIGH" if proba > 0.7 else "MEDIUM" if proba > 0.4 else "LOW",
        "recommended_action": "IMMEDIATE" if proba > 0.7 else "SCHEDULE" if proba > 0.4 else "MONITOR",
        "predicted_failure_window": "1-3 days" if proba > 0.7 else "4-7 days"
    }
```

### UC-03: Employee Churn Prediction (HRIS)
```python
# Model: Gradient Boosting (LightGBM)
# Input features:
hr_features = [
    "years_of_service", "age", "gender", "department",
    "avg_monthly_overtime_hours",
    "leave_balance_pct_used",
    "last_promotion_months_ago",
    "salary_vs_market_pct",
    "manager_change_count_1y",
    "performance_score_last",
    "attendance_rate_3m",
    "training_hours_ytd",
    "peer_review_score"
]

# Output: Churn probability per karyawan (monthly)
# Metric: AUC-ROC > 0.85
# Action: Alert HR Manager jika proba > 0.7

# SHAP untuk explainability
import shap
explainer = shap.TreeExplainer(model)
shap_values = explainer.shap_values(X_test)
# → "Alasan utama: Lembur berlebihan, gaji di bawah pasar"
```

### UC-04: Customer Churn (CRM)
```python
# Model: Logistic Regression + Neural Network Ensemble
crm_features = [
    "contract_tenure_months", "monthly_spend",
    "ticket_count_90d", "ticket_resolution_time_avg",
    "last_purchase_days_ago", "purchase_frequency",
    "nps_score", "product_usage_rate",
    "payment_delays_count_1y"
]
# Target: Monthly renewal probability
# Metric: F1 Score > 0.80
# Action: Trigger retention campaign jika proba < 0.6
```

### UC-05: Anomaly Detection — Finance (Finance)
```python
# Model: Isolation Forest + Autoencoder
finance_features = [
    "transaction_amount", "amount_vs_avg_vendor",
    "time_of_day", "day_of_week",
    "vendor_first_transaction",
    "duplicate_invoice_flag",
    "amount_round_number",
    "approver_hierarchy_skipped"
]
# Output: Anomaly score per transaksi
# Metric: Precision > 85% (minimize false positives)
# Use: Fraud detection, audit support
```

### UC-06: Vendor Risk Scoring (Procurement)
```python
# Model: Weighted Scorecard + ML
vendor_features = [
    "on_time_delivery_rate",
    "quality_acceptance_rate",
    "invoice_accuracy_rate",
    "credit_rating",
    "years_in_business",
    "geographic_risk",
    "financial_stability_score",
    "compliance_violations_count"
]
# Output: Risk score 0-100, kategori: LOW/MEDIUM/HIGH/CRITICAL
```

### UC-07: Sales Revenue Forecasting (Sales)
```python
# Model: Prophet + XGBoost
# Forecast: 90 hari ke depan per sales rep / region / product
# Metric: MAPE < 10%
# Refresh: Weekly
```

### UC-08: Quality Defect Prediction (MES)
```python
# Model: Neural Network (tabular)
mes_features = [
    "material_supplier", "material_batch_id",
    "machine_id", "operator_id", "shift",
    "ambient_temperature", "ambient_humidity",
    "machine_last_maintenance_days",
    "previous_lot_defect_rate"
]
# Output: Probability defect sebelum produksi dimulai
# Metric: Recall > 88%
# Action: Trigger QC inspection lebih ketat jika proba > 0.5
```

---

## 🧠 Generative AI Features

### 1. Multi-Domain AI Assistant (RAG)
```python
# Arsitektur: Ollama (Llama 3.1) + LangChain + Qdrant

from langchain.chat_models import ChatOllama
from langchain.vectorstores import Qdrant
from langchain.embeddings import OllamaEmbeddings
from langchain.chains import RetrievalQA

# Vector store diisi dengan:
# - SOP & kebijakan perusahaan
# - Manual produk
# - FAQ HR
# - Laporan keuangan (teks)
# - Data master (product catalog, dll)

rag_chain = RetrievalQA.from_chain_type(
    llm=ChatOllama(model="llama3.1:8b"),
    retriever=vector_store.as_retriever(search_kwargs={"k": 5}),
    chain_type="stuff"
)

# Contoh query:
# "Berapa saldo cuti tahunan saya?"
# "Apa kebijakan lembur hari Sabtu?"
# "Bagaimana proses pengajuan reimbursement?"
```

### 2. Invoice OCR & Auto-Extraction
```python
# Pipeline: Tesseract / PaddleOCR → LLM extraction → Validation
def process_invoice_image(image_bytes: bytes) -> dict:
    # OCR text
    raw_text = ocr_engine.extract(image_bytes)

    # LLM structured extraction
    prompt = f"""
    Extract invoice fields dari teks berikut dalam format JSON:
    Fields: invoice_number, vendor_name, invoice_date, due_date,
            line_items (list of: description, qty, unit_price, amount),
            subtotal, tax_amount, total_amount

    Teks invoice:
    {raw_text}

    Kembalikan HANYA JSON yang valid.
    """

    result = llm.invoke(prompt)
    extracted = json.loads(result.content)

    # Validasi bisnis
    validate_invoice(extracted)
    return extracted
```

### 3. Text-to-SQL (Natural Language Query)
```python
# Pengguna non-teknis bisa query data dengan bahasa alami
# "Tampilkan 10 karyawan dengan gaji tertinggi di departemen IT"
# → SELECT e.full_name, s.net_salary FROM employees e ...

def nl_to_sql(question: str, schema_context: str) -> str:
    prompt = f"""
    Kamu adalah SQL expert untuk database enterprise berikut:
    {schema_context}

    Konversi pertanyaan ini ke SQL yang valid:
    "{question}"

    Rules:
    - Gunakan hanya tabel yang ada di schema
    - Tambahkan LIMIT 1000 untuk semua query SELECT
    - Jangan gunakan DROP, DELETE, UPDATE, INSERT
    - Kembalikan HANYA SQL, tanpa penjelasan

    SQL:
    """
    return llm.invoke(prompt).content
```

---

## 📊 MLflow Model Registry

```python
# Lifecycle model di MLflow
stages = {
    "None":       "Model baru di-train, belum dievaluasi",
    "Staging":    "Model lolos evaluasi, testing di staging env",
    "Production": "Model aktif melayani prediksi",
    "Archived":   "Model lama, tidak aktif"
}

# Promotion flow
# None → Staging (setelah metric threshold terpenuhi)
# Staging → Production (setelah A/B test)
# Production → Archived (ketika versi baru dipromote)

# Auto-retraining trigger:
# - Data drift terdeteksi (PSI > 0.2)
# - Model performance turun (MAPE > 20%)
# - Jadwal bulanan
```

---

## 🚀 Model Serving (BentoML)

```python
# src/serving/demand_forecast_service.py
import bentoml
from bentoml.io import JSON

demand_model = bentoml.mlflow.get("demand_forecast:production")
runner = demand_model.to_runner()

svc = bentoml.Service("demand_forecast_svc", runners=[runner])

@svc.api(input=JSON(), output=JSON())
async def predict(input_data: dict) -> dict:
    features = preprocess(input_data)
    prediction = await runner.predict.async_run(features)
    return {
        "product_id": input_data["product_id"],
        "predictions": [
            {"date": d, "predicted_qty": q}
            for d, q in zip(prediction["dates"], prediction["quantities"])
        ],
        "confidence_interval": prediction["ci"],
        "model_version": demand_model.tag.version
    }
```
