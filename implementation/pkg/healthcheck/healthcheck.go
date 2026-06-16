package healthcheck

import (
	"encoding/json"
	"net/http"
	"time"
)

type CheckResult struct {
	Status    string            `json:"status"`
	Checks    map[string]string `json:"checks,omitempty"`
	Timestamp string            `json:"timestamp"`
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	res := CheckResult{
		Status:    "ok",
		Timestamp: time.Now().Format(time.RFC3339),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(res)
}

func LiveHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"alive"}`))
}

func NewReadyHandler(checks map[string]func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := "ready"
		results := make(map[string]string)

		for name, check := range checks {
			if err := check(); err != nil {
				status = "not ready"
				results[name] = "down: " + err.Error()
			} else {
				results[name] = "up"
			}
		}

		res := CheckResult{
			Status:    status,
			Checks:    results,
			Timestamp: time.Now().Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		if status == "ready" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		json.NewEncoder(w).Encode(res)
	}
}
