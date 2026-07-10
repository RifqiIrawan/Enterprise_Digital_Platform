# 12 — Dashboard & Business Intelligence
## Enterprise Data Center Simulator (EDCS)

---

## 📊 Overview

EDCS BI Layer menggunakan **Apache Superset** sebagai platform utama self-service BI, dengan **embedded analytics** di setiap modul bisnis. Target: 50+ pre-built dashboard yang dapat langsung digunakan oleh berbagai persona (C-Level, Manager, Analyst, Operator).

---

## 🏗️ BI Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                     BI CONSUMERS                             │
│  Executive │ Manager │ Analyst │ Operator │ Data Scientist   │
└──────────────────────────┬───────────────────────────────────┘
                           │
            ┌──────────────┼──────────────┐
            ▼              ▼              ▼
    ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
    │   Superset   │ │  Embedded BI │ │   Mobile BI  │
    │  (Self-svc)  │ │  (per module)│ │  (React Nat.)│
    └──────┬───────┘ └──────┬───────┘ └──────┬───────┘
           └────────────────┼────────────────┘
                            │
┌───────────────────────────▼──────────────────────────────────┐
│                     SEMANTIC LAYER                           │
│         Apache Superset Datasets / dbt Metrics              │
└───────────────────────────┬──────────────────────────────────┘
                            │
         ┌──────────────────┼──────────────────┐
         ▼                  ▼                  ▼
   ClickHouse         Trino             PostgreSQL
   (DWH OLAP)    (Federated Query)  (Operational, live)
```

---

## 🗺️ Dashboard Catalog (50+ Dashboard)

### 🎯 C-Level Executive (5 Dashboard)

#### EX-01: Company Performance Scorecard
- KPI Cards: Revenue MTD/YTD, EBITDA %, Headcount, Customer Count, OEE %
- Revenue vs Target (Gauge chart)
- YoY Trend (Multi-axis line chart)
- Top 5 Risk Alerts (Real-time)
- Quick navigation ke modul detail

#### EX-02: Financial Health Dashboard
- P&L Waterfall (Bulan berjalan)
- Cash Position & Forecast
- AR/AP Aging Summary
- Budget Utilization (Heatmap per dept)
- Working Capital Trend

#### EX-03: Operations Command Center
- Live OEE per Line Produksi
- Warehouse Utilization %
- Order Fulfillment Rate
- Supply Chain Risk Map (Geo)
- Active Alerts Counter

#### EX-04: People & Culture
- Headcount Trend
- Turnover Rate (vs Industri)
- Engagement Score Trend
- Training Completion Rate
- Open Positions Age

#### EX-05: Customer & Sales
- Revenue by Segment
- Customer Acquisition Cost (CAC) vs LTV
- NPS Trend
- Sales Pipeline Value
- Top Customer Concentration

---

### 💰 Finance Dashboards (10 Dashboard)

#### FIN-01: Profit & Loss
```
Visualisasi:
- P&L Waterfall chart (Revenue → EBIT)
- Monthly comparison (Actual vs Budget vs Last Year)
- Contribution margin per product line
- Cost breakdown treemap
Filters: Period, Business Unit, Currency
Refresh: Daily (T+1 hari kerja)
```

#### FIN-02: Balance Sheet
- Asset komposisi (Donut)
- Liability & Equity structure
- Current Ratio, Quick Ratio trend
- Net Asset Value history

#### FIN-03: Cash Flow
- Cash flow statement (Operations/Investing/Financing)
- 13-week cash forecast
- Bank balance per rekening
- Collections efficiency

#### FIN-04: AP Aging
- Aging buckets (0-30, 31-60, 61-90, 90+)
- Top 10 overdue vendors
- Payment run schedule
- Days Payable Outstanding (DPO) trend

#### FIN-05: AR Aging
- Aging buckets per customer
- Collection efficiency ratio
- Days Sales Outstanding (DSO) trend
- Bad debt provision

#### FIN-06: Budget vs Actual
- Variance analysis per cost center
- Traffic light RAG status
- Drill-down ke GL account
- YTD spending forecast

#### FIN-07: Fixed Assets
- Asset register summary
- Depreciation schedule
- Asset utilization rate
- Upcoming maintenance cost

#### FIN-08: Tax Dashboard
- PPN summary (in/out)
- PPh 21 per periode
- Tax compliance calendar
- E-Faktur submission status

#### FIN-09: Vendor Performance (Finance View)
- On-time payment rate
- Early payment discount captured
- Duplicate invoice detection
- Spend by vendor category

#### FIN-10: FP&A Rolling Forecast
- 12-month rolling forecast
- Scenario comparison (Base/Optimistic/Pessimistic)
- Assumptions tracker
- Sensitivity analysis

---

### 👥 HR Dashboards (8 Dashboard)

#### HR-01: Workforce Overview
```
Metrics:
- Total headcount (by dept, location, type)
- New hire / termination this month
- Gender diversity ratio
- Age distribution pyramid
- Tenure distribution
```

#### HR-02: Attendance & Leave
- Daily attendance rate heatmap
- Leave type breakdown
- Overtime hours trend
- Late arrival pattern (day of week)
- Absenteeism rate

#### HR-03: Payroll Summary
- Total payroll cost trend
- Payroll by component (donut)
- Cost per employee by dept
- Overtime cost analysis
- BPJS compliance rate

#### HR-04: Recruitment Pipeline
- Funnel: Applied → Screened → Interviewed → Offered → Joined
- Time-to-hire by position
- Source of hire effectiveness
- Offer acceptance rate
- Cost per hire

#### HR-05: Performance Management
- Performance distribution (Bell curve)
- Rating by department
- OKR completion rate
- Correlasi gaji vs performa
- High performer retention rate

#### HR-06: Training & Development
- Training hours per employee
- Skill gap matrix (Heatmap)
- Training ROI estimation
- Certification expiry tracker
- Learning completion rate

#### HR-07: Employee Churn (AI-Powered)
- Churn risk score distribution
- High-risk employees (masked PII)
- Churn drivers (SHAP values)
- Historical churn cohort analysis
- Retention action tracking

#### HR-08: HR Cost Analytics
- Total HR cost as % revenue
- Compensation benchmarking
- Benefits utilization
- HR cost per headcount trend

---

### 🛒 Sales & CRM Dashboards (8 Dashboard)

#### CRM-01: Sales Pipeline
- Kanban pipeline view
- Pipeline value by stage
- Win rate by stage
- Opportunity velocity
- Sales rep leaderboard

#### CRM-02: Revenue Analytics
- Revenue by customer / segment / product
- Monthly recurring revenue (MRR)
- Revenue waterfall (new/expansion/churn)
- Sales cycle duration

#### CRM-03: Lead Analytics
- Lead source breakdown
- Lead-to-opportunity conversion
- Lead response time
- Scoring distribution

#### CRM-04: Customer Analytics
- Customer segmentation (RFM)
- Customer lifetime value
- Customer health score
- Churn risk dashboard (AI)
- Net Promoter Score trend

#### CRM-05: Support & Service
- Ticket volume by channel
- SLA compliance rate
- Average resolution time
- CSAT score trend
- Backlog age distribution

#### CRM-06: Marketing Performance
- Campaign ROI
- Email open/click rates
- Lead generation by campaign
- Cost per lead

#### CRM-07: Sales Forecast
- AI-powered 90-day forecast
- Actual vs forecast accuracy
- Quota attainment
- Pipeline coverage ratio

#### CRM-08: Product Performance
- Revenue by product
- Top/Bottom performers
- Cross-sell / upsell rate
- Return rate analysis

---

### 🏭 Operations Dashboards (10 Dashboard)

#### OPS-01: MES OEE Real-time
```
Live refresh: 1 menit
- OEE gauge per mesin
- Availability / Performance / Quality components
- Downtime Pareto (real-time)
- Production rate vs plan
- Active alerts counter
```

#### OPS-02: Production Plan vs Actual
- Work order status board
- Daily production vs target
- Scrap & yield trend
- Shift performance comparison

#### OPS-03: Quality Management
- Defect rate trend
- Quality by product / line / shift
- CAPA status tracker
- First Pass Yield

#### OPS-04: Warehouse Overview
- Stock level by location (heatmap)
- Inventory turnover by category
- Inbound / outbound volume
- Pick accuracy rate
- Putaway time distribution

#### OPS-05: Inventory Analytics
- ABC analysis (Pie)
- Slow/fast moving items
- Stockout frequency
- Days of Supply heatmap
- Inventory valuation trend

#### OPS-06: Supply Chain KPIs
- Order fulfillment rate
- On-time delivery rate
- Perfect order rate
- Lead time analysis
- Supplier performance matrix

#### OPS-07: Asset Maintenance
- Asset health score distribution
- Upcoming PM schedule
- MTTR / MTBF trend
- Maintenance cost breakdown
- Asset availability %

#### OPS-08: IoT Real-time Dashboard
- Live sensor readings (Gauges)
- Device connectivity status
- Anomaly detection alerts
- Energy consumption trend
- Geographic device map

#### OPS-09: Procurement Analytics
- Spend by category (Treemap)
- Vendor concentration risk
- Savings achieved vs target
- PR-to-PO cycle time
- Supplier lead time variance

#### OPS-10: Logistics & Delivery
- Delivery performance by carrier
- On-time delivery rate
- Shipping cost trend
- Returns rate by product

---

## 🛠️ Superset Configuration

### Dataset Connections
```python
# superset_config.py
DATABASES = {
    "clickhouse_dwh": {
        "sqlalchemy_uri": "clickhouse+http://clickhouse:8123/edcs_dwh",
        "description": "EDCS Data Warehouse (OLAP)",
        "expose_in_sqllab": True,
    },
    "trino_federated": {
        "sqlalchemy_uri": "trino://admin@trino:8080/delta",
        "description": "Trino Federated Query (Delta Lake + PostgreSQL)",
        "expose_in_sqllab": True,
    },
    "postgres_live": {
        "sqlalchemy_uri": "postgresql://readonly@postgres-erp:5432/erp_db",
        "description": "Live operational data (read-only)",
        "expose_in_sqllab": False,  # Hanya via curated datasets
    }
}

# Row-level security per role
RLS_FILTER_TABLES = {
    "fact_sales": "sales_rep_id = '{{ current_user_id() }}'",
    "fact_payroll": "department_id IN ({{ user_departments() }})",
}
```

### Cache Configuration
```python
CACHE_CONFIG = {
    "CACHE_TYPE": "RedisCache",
    "CACHE_DEFAULT_TIMEOUT": 300,    # 5 menit default
    "CACHE_KEY_PREFIX": "superset_",
    "CACHE_REDIS_URL": "redis://redis:6379/1",
}

# Dashboard-level cache timeout
DASHBOARD_CACHE = {
    "executive_scorecard": 300,    # 5 menit
    "iot_realtime": 60,            # 1 menit
    "monthly_pnl": 3600,           # 1 jam
}
```
