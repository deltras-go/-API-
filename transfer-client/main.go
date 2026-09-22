// Command transfersimulator-client is a small web client for the
// TransferSimulator lab API (see api_info.docx).
//
// It serves a single page with a dropdown of the data types the API
// exposes (fullName, snils, inn, email, identityCard). When the user picks
// one and clicks the button, the BROWSER calls this Go server, and the Go
// server itself performs the GET request to the real API and forwards the
// result back as JSON.
//
// Why a server-side proxy instead of calling the API straight from
// JavaScript in the browser: TransferSimulator does not send CORS headers,
// so a direct fetch() from a page served on a different origin/port would
// be blocked by the browser. Doing the request from Go avoids that problem
// entirely, since browser CORS rules only apply to requests made by
// JavaScript, not to server-to-server calls.
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

//go:embed static/index.html
var staticFS embed.FS

// dataType describes one field the TransferSimulator API can return.
// This slice is the single source of truth for the dropdown: add a method
// here and it shows up in the UI automatically, nothing to change in the
// HTML/JS.
type dataType struct {
	Key   string `json:"key"`   // path segment sent to the API, e.g. "fullName"
	Label string `json:"label"` // human-readable label shown in the dropdown
}

var allowedTypes = []dataType{
	{Key: "fullName", Label: "ФИО клиента"},
	{Key: "snils", Label: "СНИЛС"},
	{Key: "inn", Label: "ИНН"},
	{Key: "email", Label: "Email"},
	{Key: "identityCard", Label: "Номер карты-пропуска"},
}

func isAllowed(key string) bool {
	for _, t := range allowedTypes {
		if t.Key == key {
			return true
		}
	}
	return false
}

// apiResponse matches the documented shape of every TransferSimulator
// response: {"value": "..."}.
type apiResponse struct {
	Value string `json:"value"`
}

type config struct {
	BaseURL string
	Addr    string
	Timeout time.Duration
}

func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func loadConfig() config {
	// Default points at the LAB SERVER, not localhost, per the assignment
	// ("по-умолчанию используем URL сервера, а не localhost"). Override with
	// -base-url or the TRANSFER_API_BASE_URL env var, e.g. when testing
	// against the local mock server in ./mockserver.
	baseURL := flag.String("base-url",
		envOr("TRANSFER_API_BASE_URL", "http://192.168.1.200:4444/TransferSimulator"),
		"Base URL of the TransferSimulator API")
	addr := flag.String("addr", envOr("APP_ADDR", ":8080"), "Address this client listens on")
	timeoutSec := flag.Int("timeout", 10, "HTTP timeout (seconds) when calling the API")
	flag.Parse()

	return config{
		BaseURL: strings.TrimRight(*baseURL, "/"),
		Addr:    *addr,
		Timeout: time.Duration(*timeoutSec) * time.Second,
	}
}

func main() {
	cfg := loadConfig()
	client := &http.Client{Timeout: cfg.Timeout}

	indexHTML, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		log.Fatalf("cannot read embedded index.html: %v", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})

	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"baseUrl": cfg.BaseURL,
			"types":   allowedTypes,
		})
	})

	mux.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		handleData(w, r, client, cfg)
	})

	log.Printf("TransferSimulator client listening on %s", cfg.Addr)
	log.Printf("Forwarding data requests to: %s/<type>", cfg.BaseURL)
	log.Fatal(http.ListenAndServe(cfg.Addr, mux))
}

func handleData(w http.ResponseWriter, r *http.Request, client *http.Client, cfg config) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "метод не поддерживается"})
		return
	}

	dtype := r.URL.Query().Get("type")
	if !isAllowed(dtype) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("неизвестный тип данных: %q", dtype),
		})
		return
	}

	// dtype is checked against the fixed allow-list above, so it is safe to
	// concatenate into the URL — an arbitrary query value could never reach
	// this point.
	targetURL := cfg.BaseURL + "/" + dtype

	resp, err := client.Get(targetURL)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": fmt.Sprintf("не удалось обратиться к API (%s): %v", targetURL, err),
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": fmt.Sprintf("ошибка чтения ответа API: %v", err),
		})
		return
	}

	if resp.StatusCode != http.StatusOK {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": fmt.Sprintf("API вернул %d %s: %s", resp.StatusCode, http.StatusText(resp.StatusCode), strings.TrimSpace(string(body))),
		})
		return
	}

	var parsed apiResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		// The response wasn't the documented {"value": "..."} shape.
		// Surface the raw body instead of failing silently.
		writeJSON(w, http.StatusOK, map[string]string{"raw": strings.TrimSpace(string(body))})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"type":  dtype,
		"value": parsed.Value,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
