// Package mqttclient adalah wrapper tipis di atas paho.mqtt.golang, dipakai
// oleh internal/simulator (publish reading) dan cmd/server (subscribe untuk
// ingest). Sama seperti eventbus.Publisher terhadap Kafka, koneksi & publish
// ke broker bersifat best-effort -- gagal konek/publish dicatat sebagai log
// warning, tidak pernah membuat service crash atau menggagalkan request
// HTTP. Ini penting karena Mosquitto adalah infra tambahan (bukan dependency
// inti seperti Postgres) -- endpoint CRUD devices/readings/alerts tetap
// harus bisa dipakai walau Mosquitto sedang tidak jalan, hanya saja tidak
// ada data baru yang mengalir masuk.
package mqttclient

import (
	"fmt"
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Client struct {
	c mqtt.Client
}

// Connect mencoba konek ke broker dengan timeout pendek. Kegagalan
// dikembalikan sebagai error supaya pemanggil (cmd/server) bisa memutuskan
// untuk lanjut jalan tanpa MQTT (bukan log.Fatal) -- lihat komentar paket.
func Connect(brokerURL, clientID string) (*Client, error) {
	opts := mqtt.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID(clientID).
		SetAutoReconnect(true).
		SetConnectTimeout(5 * time.Second)

	c := mqtt.NewClient(opts)
	token := c.Connect()
	if !token.WaitTimeout(5 * time.Second) {
		return nil, fmt.Errorf("connect to mqtt broker %s: timeout", brokerURL)
	}
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("connect to mqtt broker %s: %w", brokerURL, err)
	}
	return &Client{c: c}, nil
}

func (cl *Client) Publish(topic string, payload []byte) {
	if cl == nil || cl.c == nil {
		return
	}
	token := cl.c.Publish(topic, 0, false, payload)
	go func() {
		if !token.WaitTimeout(3*time.Second) || token.Error() != nil {
			log.Printf("mqttclient: publish to %s failed (continuing without it): %v", topic, token.Error())
		}
	}()
}

// Subscribe mendaftarkan handler untuk sebuah topic filter (boleh
// mengandung wildcard MQTT seperti "iot/+/+/reading"). handler dipanggil
// dari goroutine internal paho untuk tiap pesan yang masuk.
func (cl *Client) Subscribe(topicFilter string, handler func(topic string, payload []byte)) error {
	if cl == nil || cl.c == nil {
		return fmt.Errorf("mqttclient: not connected")
	}
	token := cl.c.Subscribe(topicFilter, 0, func(_ mqtt.Client, msg mqtt.Message) {
		handler(msg.Topic(), msg.Payload())
	})
	if !token.WaitTimeout(5*time.Second) || token.Error() != nil {
		return fmt.Errorf("subscribe to %s: %w", topicFilter, token.Error())
	}
	return nil
}

func (cl *Client) Close() {
	if cl != nil && cl.c != nil && cl.c.IsConnected() {
		cl.c.Disconnect(250)
	}
}
