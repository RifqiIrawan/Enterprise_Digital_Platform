// Package simulator men-generate pembacaan sensor palsu untuk tiap device
// ACTIVE dan mempublikasikannya ke MQTT (bukan menulis langsung ke
// database -- itu tugas subscriber di cmd/server lewat internal/ingest,
// supaya alur datanya benar-benar lewat broker seperti device IoT
// sungguhan, bukan cuma label "MQTT" tanpa wire protocol beneran).
package simulator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/iot-service/internal/mqttclient"
)

// ReadingMessage adalah bentuk JSON yang dipublikasikan ke topic
// iot/{company_id}/{device_id}/reading dan di-decode kembali oleh
// subscriber ingest di cmd/server -- kontrak wire format-nya "dimiliki"
// paket ini karena di sinilah pesannya pertama kali dibuat.
type ReadingMessage struct {
	DeviceID     string    `json:"device_id"`
	ValueNumeric *float64  `json:"value_numeric,omitempty"`
	ValueText    *string   `json:"value_text,omitempty"`
	RecordedAt   time.Time `json:"recorded_at"`
}

type activeDevice struct {
	id         string
	companyID  string
	deviceType string
}

type Runner struct {
	pool     *pgxpool.Pool
	mqtt     *mqttclient.Client
	interval time.Duration
	rng      *rand.Rand
	gpsWalk  map[string][2]float64 // device_id -> lat,lon terakhir (random walk in-memory)
}

func New(pool *pgxpool.Pool, mqtt *mqttclient.Client, intervalSeconds int) *Runner {
	return &Runner{
		pool:     pool,
		mqtt:     mqtt,
		interval: time.Duration(intervalSeconds) * time.Second,
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
		gpsWalk:  map[string][2]float64{},
	}
}

// Start memblokir sampai ctx dibatalkan -- dipanggil sebagai goroutine dari
// cmd/server.
func (r *Runner) Start(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.tick(ctx)
		}
	}
}

func (r *Runner) tick(ctx context.Context) {
	devices, err := r.activeDevices(ctx)
	if err != nil {
		log.Printf("simulator: load active devices failed: %v", err)
		return
	}
	for _, d := range devices {
		msg := r.generateReading(d)
		payload, err := json.Marshal(msg)
		if err != nil {
			log.Printf("simulator: marshal reading for device %s failed: %v", d.id, err)
			continue
		}
		topic := fmt.Sprintf("iot/%s/%s/reading", d.companyID, d.id)
		r.mqtt.Publish(topic, payload)
	}
}

func (r *Runner) activeDevices(ctx context.Context) ([]activeDevice, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, company_id, device_type FROM devices WHERE status = 'ACTIVE'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []activeDevice
	for rows.Next() {
		var d activeDevice
		if err := rows.Scan(&d.id, &d.companyID, &d.deviceType); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// generateReading membuat nilai yang masuk akal per device_type, dengan
// sesekali "spike" (5% peluang) untuk TEMPERATURE/HUMIDITY/VIBRATION supaya
// threshold alert punya sesuatu untuk ditangkap saat didemokan, bukan cuma
// selalu di dalam rentang normal.
func (r *Runner) generateReading(d activeDevice) ReadingMessage {
	msg := ReadingMessage{DeviceID: d.id, RecordedAt: time.Now()}

	spike := r.rng.Float64() < 0.05
	switch d.deviceType {
	case "TEMPERATURE":
		v := 15 + r.rng.Float64()*20 // 15-35°C normal
		if spike {
			v += 10
		}
		msg.ValueNumeric = &v
	case "HUMIDITY":
		v := 30 + r.rng.Float64()*40 // 30-70% normal
		if spike {
			v += 20
		}
		msg.ValueNumeric = &v
	case "VIBRATION":
		v := r.rng.Float64() * 5 // 0-5mm/s normal
		if spike {
			v += 10
		}
		msg.ValueNumeric = &v
	case "RFID":
		tags := []string{"TAG-A1", "TAG-B2", "TAG-C3", "TAG-D4", "TAG-E5"}
		v := tags[r.rng.Intn(len(tags))]
		msg.ValueText = &v
	case "GPS":
		lat, lon := r.walkGPS(d.id)
		v := fmt.Sprintf("%.6f,%.6f", lat, lon)
		msg.ValueText = &v
	case "BARCODE":
		v := fmt.Sprintf("SKU-%06d", r.rng.Intn(999999))
		msg.ValueText = &v
	}
	return msg
}

// walkGPS menggerakkan posisi device sedikit demi sedikit dari titik dasar
// (Jakarta) tiap tick, disimpan in-memory (bukan di-load dari readings
// terakhir di DB) -- state hilang kalau service di-restart, dianggap wajar
// untuk sebuah simulator.
func (r *Runner) walkGPS(deviceID string) (float64, float64) {
	const baseLat, baseLon = -6.200000, 106.816666

	cur, ok := r.gpsWalk[deviceID]
	if !ok {
		cur = [2]float64{baseLat, baseLon}
	}
	cur[0] += (r.rng.Float64() - 0.5) * 0.001
	cur[1] += (r.rng.Float64() - 0.5) * 0.001
	r.gpsWalk[deviceID] = cur
	return cur[0], cur[1]
}
