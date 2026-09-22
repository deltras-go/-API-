// Command mockserver is a tiny stand-in for TransferSimulator.exe. It lets
// you develop and test the client on a machine that isn't on the lab
// network (192.168.1.200).
//
// It is NOT the graded deliverable and does not replace testing against the
// real API — always verify the real thing with Bruno first, then confirm
// the same request works end-to-end through the client, on the lab PC.
//
// Run it, then point the client at it:
//
//	go run ./mockserver
//	go run . -base-url http://localhost:4444/TransferSimulator
package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// Sample values taken straight from api_info.docx.
var responses = map[string]string{
	"fullName":     "Соколов& Сокол& Соколович",
	"snils":        "789-012-345 67%",
	"inn":          "7707083893%",
	"email":        "sokolov.sokol@aol.com sokolov.sokol@aol.com",
	"identityCard": "10 19 012345",
}

func main() {
	mux := http.NewServeMux()
	for path, value := range responses {
		value := value
		mux.HandleFunc("/TransferSimulator/"+path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(map[string]string{"value": value})
		})
	}

	const addr = ":4444"
	log.Printf("Mock TransferSimulator listening on %s (matches api_info.docx examples)", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
