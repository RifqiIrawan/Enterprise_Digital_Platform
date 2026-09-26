package logging

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"
)

type jsonWriter struct {
	service string
}

func (w jsonWriter) Write(p []byte) (int, error) {
	entry := map[string]any{
		"time":    time.Now().UTC().Format(time.RFC3339),
		"level":   "INFO",
		"service": w.service,
		"msg":     strings.TrimRight(string(p), "\n"),
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return 0, err
	}
	b = append(b, '\n')
	return os.Stdout.Write(b)
}

// Init redirects the stdlib "log" package (used by every log.Printf /
// log.Fatalf call in this service, unchanged) to emit one JSON line per
// call instead of the default "2009/11/10 23:00:00 message" text format.
// log.Fatalf's built-in os.Exit(1) still fires after Write returns, so
// fatal-and-exit behavior is unchanged. Every line is level "INFO" --
// Write can't distinguish Printf from Fatalf, both funnel through the same
// stdlib call; true leveled logging would need every call site rewritten
// to a structured logger, out of scope for this pass.
func Init(service string) {
	log.SetFlags(0)
	log.SetOutput(jsonWriter{service: service})
}
