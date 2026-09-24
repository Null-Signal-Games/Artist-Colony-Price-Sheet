package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// json shapes for incoming POST body
// @TODO: unify cartItemJSON with computedItem
type cartItemJSON struct {
	ProductCode string `json:"productCode"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	UnitPrice   string `json:"cad"`
	Quantity    int    `json:"quantity"`
}

type orderSubmission struct {
	OrderID       string         `json:"orderId"`
	Name          string         `json:"name"`
	DiscordHandle string         `json:"discordHandle"`
	Email         string         `json:"email"`
	Items         []cartItemJSON `json:"items"`
	Subtotal      string         `json:"subtotal"`
	SubmittedAt   string         `json:"submittedAt"`
}

// computedItem holds the cent value rows for DB
type computedItem struct {
	ProductCode       string
	Title             string
	Artist            string
	UnitPriceCents    int
	Quantity          int
	LineSubtotalCents int
}

type appServer struct {
	db            *store
	allowedOrigin string
}

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func (s *appServer) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := s.allowedOrigin
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Add("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// func (s *appServer) handleOrder(w http.ResponseWriter, r *http.Request) {
//   log.Printf("handleOrder: %s", r.RemoteAddr)
//   r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
//
//   var sub orderSubmission
//   if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
//   	log.Printf("  bad JSON: %v", err)
//   	writeJSON(w, http.StatusBadRequest, map[string]string{
//   		"result": "error",
//   		"error":  "Request body must be JSON.",
//   	})
//   	return
//   }
//   log.Printf("  name=%v email=%v items=%v", sub.Name, sub.Email, len(sub.Items))
//   writeJSON(w, http.StatusOK, map[string]any{
//     "result":  "success",
//     "orderId": "W26-12345678",
//   })
//
//
//
// }

func (s *appServer) handleOrder(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var sub orderSubmission
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"result": "error",
			"error":  "Request body must be JSON",
		})
		return
	}

	sub.Name = strings.TrimSpace(sub.Name)
	sub.Email = strings.TrimSpace(sub.Email)
	sub.DiscordHandle = strings.TrimSpace(sub.DiscordHandle)
	sub.OrderID = strings.TrimSpace(sub.OrderID)
	sub.SubmittedAt = strings.TrimSpace(sub.SubmittedAt)

	if sub.OrderID == "" {
		sub.OrderID = fmt.Sprintf("W26-%d", time.Now().UnixMilli())
	}
	if sub.SubmittedAt == "" {
		sub.SubmittedAt = time.Now().UTC().Format(time.RFC3339)
	}

	switch {
	case sub.Name == "":
		writeJSON(w, http.StatusBadRequest, map[string]string{"result": "error", "error": "Name is required."})
		return
	case sub.Email == "":
		writeJSON(w, http.StatusBadRequest, map[string]string{"result": "error", "error": "Email is required."})
		return
	case len(sub.Items) == 0:
		writeJSON(w, http.StatusBadRequest, map[string]string{"result": "error", "error": "Order has no items."})
		return
	}

	computed := make([]computedItem, 0, len(sub.Items))
	var subtotalCents int
	for _, item := range sub.Items {
		quantity := item.Quantity
		if quantity <= 0 {
			quantity = 1
		}
		
		f, _ := strconv.ParseFloat(strings.TrimSpace(item.UnitPrice), 64)
		unitPriceCents := int(math.Round(f * 100))
		
		lineSubtotalCents := unitPriceCents * quantity
		subtotalCents += lineSubtotalCents
		computed = append(computed, computedItem{
			ProductCode:       strings.TrimSpace(item.ProductCode),
			Title:             strings.TrimSpace(item.Title),
			Artist:            strings.TrimSpace(item.Artist),
			UnitPriceCents:    unitPriceCents,
			Quantity:          quantity,
			LineSubtotalCents: lineSubtotalCents,
		})
	}

	if _, err := s.db.insertOrder(&sub, computed, subtotalCents); err != nil {
		log.Printf("insert order %q failed: %v", sub.OrderID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"result": "error",
			"error":  "Could not store the order.",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"result":  "success",
		"orderId": sub.OrderID,
	})
}

func main() {
	port := envOr("PORT", "8080")
	dbPath := envOr("DB_PATH", "/data/db.sqlite3")
	allowedOrigin := os.Getenv("ALLOWED_ORIGIN")

	database, err := openDB(dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	s := &appServer{db: &store{db: database}, allowedOrigin: allowedOrigin}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /order", s.handleOrder)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	addr := ":" + port
	log.Printf("order server listening on %s (db: %s)", addr, dbPath)
	if err := http.ListenAndServe(addr, s.corsMiddleware(mux)); err != nil {
		log.Fatal(err)
	}
}

