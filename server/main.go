package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
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
	LineAction        string
}

type appServer struct {
	db                   *store
	allowedOrigin        string
	inventoryPrices      map[string]int
	inventoryItems       []inventoryItemJSON
	inventoryItemsByID   map[string]inventoryItemJSON
	inventoryMu          sync.Mutex
	staffSessionTTLHours int
	shopify              *shopifyClient
}

func jsonDecode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

var priceRe = regexp.MustCompile(`^\d+\.\d{2}$`)

func priceCents(value string) (int, bool) {
	if !priceRe.MatchString(value) {
		return 0, false
	}
	whole, frac, _ := strings.Cut(value, ".")
	dollars, _ := strconv.Atoi(whole)
	cents, _ := strconv.Atoi(frac)
	return dollars*100 + cents, true
}

// loadInventoryPrices loads the authoritative data source,
// ignore known unit prices from client unless we don't have the product code
//	could be loaded in frontend before backend catches up
func loadInventoryPrices(path string) (map[string]int, []inventoryItemJSON, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open inventory CSV: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.TrimLeadingSpace = true
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1 // rows have optional trailing columns

	records, err := reader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("read inventory CSV: %w", err)
	}
	if len(records) < 2 {
		return nil, nil, fmt.Errorf("inventory CSV has no data rows")
	}

	codeIdx, priceIdx := -1, -1
	colIdx := map[string]int{}
	for i, col := range records[0] {
		trimmed := strings.TrimSpace(col)
		switch {
		case trimmed == "Product Code":
			codeIdx = i
		case strings.HasPrefix(trimmed, "Price per unit"):
			priceIdx = i
		case trimmed == "Shop Name":
			colIdx["shop"] = i
		case trimmed == "Artist Name":
			colIdx["artist"] = i
		case trimmed == "Item Name":
			colIdx["title"] = i
		case trimmed == "Item Type":
			colIdx["type"] = i
		case trimmed == "Product Display":
			colIdx["display"] = i
		case trimmed == "Quantity":
			colIdx["quantity"] = i
		case trimmed == "Notes":
			colIdx["notes"] = i
		}
	}
	if codeIdx < 0 || priceIdx < 0 {
		return nil, nil, fmt.Errorf("inventory CSV missing Product Code / Price per unit columns")
	}

	pick := func(row []string, key string) string {
		idx, ok := colIdx[key]
		if !ok || idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}

	prices := make(map[string]int, len(records)-1)
	items := make([]inventoryItemJSON, 0, len(records)-1)
	for _, row := range records[1:] {
		if codeIdx >= len(row) || priceIdx >= len(row) {
			continue
		}
		code := strings.TrimSpace(row[codeIdx])
		title := pick(row, "title")
		if code == "" {
			continue
		}
		cents, ok := priceCents(row[priceIdx])
		if !ok {
			return nil, nil, fmt.Errorf("product %q has malformed price %q (expect decimal like 8.00)", code, row[priceIdx])
		}
		prices[code] = cents

		var qty *int
		if raw := pick(row, "quantity"); raw != "" {
			cleaned := strings.Map(func(r rune) rune {
				if r >= '0' && r <= '9' || r == '-' {
					return r
				}
				return -1
			}, raw)
			if parsed, err := strconv.Atoi(cleaned); err == nil {
				qty = &parsed
			}
		}
		items = append(items, inventoryItemJSON{
			ID:             code + "::" + title,
			ShopName:       pick(row, "shop"),
			ArtistName:     pick(row, "artist"),
			ProductCode:    code,
			Title:          title,
			ItemType:       pick(row, "type"),
			ProductDisplay: pick(row, "display"),
			Quantity:       qty,
			PriceCents:     cents,
			Notes:          pick(row, "notes"),
			SoldOut:        soldOutRe.MatchString(title),
		})
	}
	return prices, items, nil
}

func (s *appServer) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := s.allowedOrigin
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Add("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, PATCH, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

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

	for _, item := range sub.Items {
		if _, ok := priceCents(item.UnitPrice); !ok {
			log.Printf("order %q: rejecting unit price %q", sub.OrderID, item.UnitPrice)
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"result": "error",
				"error":  "Unit prices must be plain decimal strings like 8.00",
			})
			return
		}
	}

	computed := make([]computedItem, 0, len(sub.Items))
	var subtotalCents int
	for _, item := range sub.Items {
		quantity := item.Quantity
		if quantity <= 0 {
			quantity = 1
		}

		code := strings.TrimSpace(item.ProductCode)
		unitPriceCents, known := s.inventoryPrices[code]
		if !known {
			log.Printf("order %q: product %q not in inventory, using client price", sub.OrderID, code)
			unitPriceCents, _ = priceCents(item.UnitPrice)
		}

		lineSubtotalCents := unitPriceCents * quantity
		subtotalCents += lineSubtotalCents
		computed = append(computed, computedItem{
			ProductCode:       code,
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
	allowedOrigin := os.Getenv("ALLOWED_ORIGIN")

	connStr := os.Getenv("DB_CONNECTION_STRING")
	if connStr == "" {
		log.Fatal("Missing env DB_CONNECTION_STRING")
	}

	database, err := openDB(connStr)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	inventoryPath := envOr("INVENTORY_CSV", "/usr/local/share/artist-colony/w26-inventory.csv")
	prices, items, err := loadInventoryPrices(inventoryPath)
	if err != nil {
		log.Fatalf("load inventory: %v", err)
	}
	log.Printf("loaded %d inventory prices from %s", len(prices), inventoryPath)

	ttlHours := 96 // should cover all 4 days of the event
	if raw := os.Getenv("STAFF_SESSION_TTL_HOURS"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			ttlHours = parsed
		}
	}

	s := &appServer{
		db:                   &store{db: database},
		allowedOrigin:        allowedOrigin,
		inventoryPrices:      prices,
		inventoryItems:       items,
		inventoryItemsByID:   make(map[string]inventoryItemJSON, len(items)),
		staffSessionTTLHours: ttlHours,
		shopify:              newShopifyClientFromEnv(),
	}
	for _, item := range items {
		s.inventoryItemsByID[item.ID] = item
	}
	if s.shopify == nil {
		log.Printf("⚠️ shopify not configured. /send-invoice will be stubbed out")
	} else {
		log.Printf("✅ shopify enabled: %s", s.shopify.domain)
	}

	ctx := context.Background()
	if err := s.db.seedStaffUsers(ctx); err != nil {
		log.Fatalf("seed staff users: %v", err)
	}

	s.startShopifyPoller(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /order", s.handleOrder)

	mux.HandleFunc("POST /staff/login", s.handleStaffLogin)
	mux.HandleFunc("POST /staff/logout", s.handleStaffLogout)

	staffMux := http.NewServeMux()
	staffMux.HandleFunc("GET /staff/orders", s.handleListOrders)
	staffMux.HandleFunc("POST /staff/orders", s.handleCreateStaffOrder)
	staffMux.HandleFunc("GET /staff/orders/{orderId}", s.handleGetOrder)
	staffMux.HandleFunc("POST /staff/orders/{orderId}/status", s.handleUpdateOrderStatus)
	staffMux.HandleFunc("PATCH /staff/orders/{orderId}/notes", s.handleUpdateOrderNotes)
	staffMux.HandleFunc("PATCH /staff/orders/{orderId}/lines/{lineIndex}", s.handleUpdateOrderLine)
	staffMux.HandleFunc("POST /staff/orders/{orderId}/send-invoice", s.handleSendInvoice)
	staffMux.HandleFunc("POST /staff/orders/{orderId}/notified", s.handleMarkNotified)
	staffMux.HandleFunc("POST /staff/orders/{orderId}/resend-invoice", s.handleResendInvoice)
	staffMux.HandleFunc("GET /staff/inventory", s.handleListInventory)
	staffMux.HandleFunc("PATCH /staff/inventory/{id}/sold-out", s.handleSetSoldOut)
	mux.Handle("/staff/", s.staffAuthMiddleware(staffMux))

	// no auth required
	mux.HandleFunc("GET /sold-out", s.handlePublicSoldOut)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	addr := ":" + port
	log.Printf("order server listening on %s", addr)
	if err := http.ListenAndServe(addr, s.corsMiddleware(mux)); err != nil {
		log.Fatal(err)
	}
}
