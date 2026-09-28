package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var errOrderNotFound = errors.New("order not found")

// see src/lib/staff/types.ts
type orderItemJSON struct {
	ProductCode       string `json:"productCode"`
	Title             string `json:"title"`
	Artist            string `json:"artist"`
	UnitPriceCents    int    `json:"unitPriceCents"`
	Quantity          int    `json:"quantity"`
	LineSubtotalCents int    `json:"lineSubtotalCents"`
	SoldOut           bool   `json:"soldOut"`
	Collected         bool   `json:"collected"`
	LineAction        string `json:"lineAction"`
	CollectedQuantity *int   `json:"collectedQuantity"`
}

type historyEntryJSON struct {
	ID         string  `json:"id"`
	At         string  `json:"at"`
	StaffName  string  `json:"staffName"`
	Kind       string  `json:"kind"`
	Summary    string  `json:"summary"`
	FromStatus *string `json:"fromStatus"`
	ToStatus   *string `json:"toStatus"`
}

type orderJSON struct {
	OrderID              string             `json:"orderId"`
	Name                 string             `json:"name"`
	DiscordHandle        string             `json:"discordHandle"`
	Email                string             `json:"email"`
	SubtotalCents        int                `json:"subtotalCents"`
	SubmittedAt          string             `json:"submittedAt"`
	CreatedAt            string             `json:"createdAt"`
	Status               string             `json:"status"`
	StaffNotes           string             `json:"staffNotes"`
	Items                []orderItemJSON    `json:"items"`
	SubmittedByStaffName *string            `json:"submittedByStaffName"`
	MerchTableOrder      bool               `json:"merchTableOrder"`
	NotificationChannel  *string            `json:"notificationChannel"`
	ShopifyInvoiceID     *string            `json:"shopifyInvoiceId"`
	PaidReason           *string            `json:"paidReason"`
	PaidReasonOther      *string            `json:"paidReasonOther"`
	ClosedReason         *string            `json:"closedReason"`
	ClosedReasonOther    *string            `json:"closedReasonOther"`
	History              []historyEntryJSON `json:"history"`
	ShopifyDraftOrderID  *string            `json:"shopifyDraftOrderId"`
	ShopifyInvoiceURL    *string            `json:"shopifyInvoiceUrl"`
	ShopifyOrderID       *string            `json:"shopifyOrderId"`
	PaidAmountCents      *int               `json:"paidAmountCents"`
}

type staffOrderInput struct {
	OrderID              string                `json:"orderId"`
	Name                 string                `json:"name"`
	DiscordHandle        string                `json:"discordHandle"`
	Email                string                `json:"email"`
	Items                []staffOrderItemInput `json:"items"`
	SubmittedByStaffName string                `json:"submittedByStaffName"`
	MerchTableOrder      bool                  `json:"merchTableOrder"`
}

type staffOrderItemInput struct {
	ProductCode    string `json:"productCode"`
	Title          string `json:"title"`
	Artist         string `json:"artist"`
	UnitPriceCents int    `json:"unitPriceCents"`
	Quantity       int    `json:"quantity"`
}

type listOrdersFilter struct {
	Statuses []string
	Q        string
	Sort     string
}

func nullStr(v sql.NullString) *string {
	if !v.Valid || v.String == "" {
		return nil
	}
	return &v.String
}

func strPtr(s string) *string { return &s }

func nullInt(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int64)
	return &n
}

// orderFromRow serialize an order row with items and history
func (s *appServer) orderFromRow(ctx context.Context, row *orderRow, withChildren bool) (*orderJSON, error) {
	order := &orderJSON{
		OrderID:              row.OrderID,
		Name:                 row.Name,
		DiscordHandle:        row.DiscordHandle,
		Email:                row.Email,
		SubtotalCents:        row.SubtotalCents,
		SubmittedAt:          row.SubmittedAt,
		CreatedAt:            row.CreatedAt.UTC().Format(time.RFC3339),
		Status:               row.Status,
		StaffNotes:           row.StaffNotes,
		Items:                []orderItemJSON{},
		SubmittedByStaffName: nullStr(row.SubmittedByStaffName),
		MerchTableOrder:      row.MerchTableOrder,
		NotificationChannel:  nullStr(row.NotificationChannel),
		ShopifyInvoiceID:     nullStr(row.ShopifyInvoiceID),
		PaidReason:           nullStr(row.PaidReason),
		PaidReasonOther:      nullStr(row.PaidReasonOther),
		ClosedReason:         nullStr(row.ClosedReason),
		ClosedReasonOther:    nullStr(row.ClosedReasonOther),
		History:              []historyEntryJSON{},
		ShopifyDraftOrderID:  nullStr(row.ShopifyDraftOrderID),
		ShopifyInvoiceURL:    nullStr(row.ShopifyInvoiceURL),
		ShopifyOrderID:       nullStr(row.ShopifyOrderID),
		PaidAmountCents:      nullInt(row.PaidAmountCents),
	}

	if withChildren {
		overrides := s.soldOutOverrides(ctx)
		lines, err := s.db.listOrderLines(ctx, row.RowID)
		if err != nil {
			return nil, err
		}
		for _, line := range lines {
			item := line.Item
			soldOut := soldOutRe.MatchString(item.Title)
			if catalog, ok := s.inventoryItemsByID[inventoryKey(item.ProductCode, item.Title)]; ok {
				soldOut = catalog.SoldOut
			}
			if override, ok := overrides[inventoryKey(item.ProductCode, item.Title)]; ok {
				soldOut = override
			}
			order.Items = append(order.Items, orderItemJSON{
				ProductCode:       item.ProductCode,
				Title:             item.Title,
				Artist:            item.Artist,
				UnitPriceCents:    item.UnitPriceCents,
				Quantity:          item.Quantity,
				LineSubtotalCents: item.LineSubtotalCents,
				SoldOut:           soldOut,
				Collected:         line.Collected,
				LineAction:        item.LineAction,
				CollectedQuantity: line.CollectedQty,
			})
		}

		history, err := s.db.listOrderHistory(ctx, row.RowID)
		if err != nil {
			return nil, err
		}
		order.History = history
	}
	return order, nil
}

// summaryFromRow serializes without items or history
func (s *appServer) summaryFromRow(ctx context.Context, row *orderRow) (*orderJSON, error) {
	return s.orderFromRow(ctx, row, false)
}

func inventoryKey(code, title string) string { return code + "::" + title }

var soldOutRe = regexp.MustCompile(`(?i)sold out`)

// ## AUTH

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *appServer) authenticateStaff(r *http.Request) (*staffUser, error) {
	token := ""
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		token = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		return nil, errors.New("missing bearer token")
	}
	sess, err := s.db.getStaffSession(r.Context(), hashToken(token))
	if err != nil {
		return nil, errors.New("invalid or expired session")
	}
	return &sess.StaffUser, nil
}

func (s *appServer) staffAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := s.authenticateStaff(r)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), staffUserKey{}, user)))
	})
}

type staffUserKey struct{}

func staffUserFrom(r *http.Request) *staffUser {
	user, _ := r.Context().Value(staffUserKey{}).(*staffUser)
	return user
}

func (s *appServer) handleStaffLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := jsonDecode(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Request body must be JSON."})
		return
	}
	user, hash, err := s.db.getStaffUserByUsername(r.Context(), body.Username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Password)) != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid username or password."})
		return
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not create session."})
		return
	}
	token := hex.EncodeToString(tokenBytes)

	ttlHours := s.staffSessionTTLHours
	expiresAt := time.Now().Add(time.Duration(ttlHours) * time.Hour)
	if err := s.db.createStaffSession(r.Context(), user.ID, hashToken(token), expiresAt); err != nil {
		log.Printf("create staff session failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not create session."})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"token":       token,
		"staffName":   user.DisplayName,
		"displayName": user.DisplayName,
	})
}

func (s *appServer) handleStaffLogout(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		_ = s.db.deleteStaffSession(r.Context(), hashToken(token))
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ## Orders

func (s *appServer) parseListFilter(r *http.Request) listOrdersFilter {
	filter := listOrdersFilter{
		Q:    strings.TrimSpace(r.URL.Query().Get("q")),
		Sort: r.URL.Query().Get("sort"),
	}
	if raw := r.URL.Query().Get("statuses"); raw != "" {
		for _, status := range strings.Split(raw, ",") {
			if status = strings.TrimSpace(status); status != "" {
				filter.Statuses = append(filter.Statuses, status)
			}
		}
	}
	return filter
}

func (s *appServer) handleOrderEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// flush now so client knows the connection is established
	rc := http.NewResponseController(w)
	_ = rc.Flush()

	ch := make(chan string, 10)
	s.sseClientsMu.Lock()
	s.sseClients[ch] = struct{}{}
	s.sseClientsMu.Unlock()

	defer func() {
		s.sseClientsMu.Lock()
		delete(s.sseClients, ch)
		s.sseClientsMu.Unlock()
	}()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			_ = rc.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, "data: ping\n\n")
			_ = rc.Flush()
		}
	}
}

func (s *appServer) handleListOrders(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.listOrders(r.Context(), s.parseListFilter(r))
	if err != nil {
		log.Printf("list orders failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load orders."})
		return
	}
	out := make([]*orderJSON, 0, len(rows))
	for i := range rows {
		summary, err := s.summaryFromRow(r.Context(), &rows[i])
		if err != nil {
			log.Printf("serialize order failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load orders."})
			return
		}
		out = append(out, summary)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *appServer) handleGetOrder(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("orderId")
	row, err := s.db.getOrderByID(r.Context(), orderID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Order not found."})
		return
	}
	if err != nil {
		log.Printf("get order failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
		return
	}
	order, err := s.orderFromRow(r.Context(), row, true)
	if err != nil {
		log.Printf("serialize order failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (s *appServer) handleCreateStaffOrder(w http.ResponseWriter, r *http.Request) {
	var input staffOrderInput
	if err := jsonDecode(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Request body must be JSON."})
		return
	}
	input.OrderID = strings.TrimSpace(input.OrderID)
	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.TrimSpace(input.Email)
	input.DiscordHandle = strings.TrimSpace(input.DiscordHandle)
	input.SubmittedByStaffName = strings.TrimSpace(input.SubmittedByStaffName)

	switch {
	case len(input.Items) == 0:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Order has no items."})
		return
	}


	ctx := r.Context()
	// if _, err := s.db.getOrderByID(ctx, input.OrderID); err == nil {
	// 	writeJSON(w, http.StatusConflict, map[string]string{"error": fmt.Sprintf("Order %s already exists.", input.OrderID)})
	// 	return
	// }

	subtotal := 0
	for _, item := range input.Items {
		subtotal += item.UnitPriceCents * item.Quantity
	}

	now := time.Now().UTC()
	status := "new"
	history := []historyInsert{}
	if input.MerchTableOrder {
		status = "prepared"
		history = append(history, historyInsert{
			Kind: "status", Summary: "New → Prepared",
			FromStatus: strPtr("new"), ToStatus: strPtr("prepared"),
			StaffName: staffUserFrom(r).DisplayName,
		})
	}

	order := &orderRow{
		OrderID:         input.OrderID,
		Name:            input.Name,
		DiscordHandle:   input.DiscordHandle,
		Email:           input.Email,
		SubtotalCents:   subtotal,
		SubmittedAt:     now.Format(time.RFC3339),
		CreatedAt:       now,
		Status:          status,
		MerchTableOrder: input.MerchTableOrder,
	}
	if err := s.db.insertStaffOrder(ctx, input, order, history); err != nil {
		log.Printf("insert staff order failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not store the order."})
		return
	}

	created, err := s.db.getOrderByID(ctx, order.OrderID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load the order."})
		return
	}
	out, err := s.orderFromRow(ctx, created, true)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load the order."})
		return
	}
	s.broadcastOrderEvent("update")
	writeJSON(w, http.StatusOK, out)
}

var allowedTransitions = map[string][]string{
	"new":      {"prepared", "closed"},
	"prepared": {"new", "closed"},
	"invoiced": {"notified", "closed"},
	"notified": {"invoiced", "paid", "closed"},
	"paid":     {"closed", "notified"},
	"closed":   {"new", "prepared", "invoiced", "notified", "paid"},
}

func previousStatusBeforeClose(history []historyEntryJSON) string {
	for i := len(history) - 1; i >= 0; i-- {
		e := history[i]
		if e.Kind == "status" && e.ToStatus != nil && *e.ToStatus == "closed" &&
			e.FromStatus != nil && *e.FromStatus != "" {
			return *e.FromStatus
		}
	}
	return ""
}

func expectedReopenStatus(ctx context.Context, db *store, row *orderRow) string {
	history, err := db.listOrderHistory(ctx, row.RowID)
	if err == nil {
		if prev := previousStatusBeforeClose(history); prev != "" {
			return prev
		}
	}
	if row.ClosedReason.String == "picked_up" || (row.PaidReason.Valid && row.PaidReason.String != "") {
		return "paid"
	}
	return "new"
}

type statusRequest struct {
	Status              string  `json:"status"`
	ClosedReason        *string `json:"closedReason"`
	ClosedReasonOther   *string `json:"closedReasonOther"`
	PaidReason          *string `json:"paidReason"`
	NotificationChannel *string `json:"notificationChannel"`
	PaidAmountCents     *int    `json:"paidAmountCents"`
}

func (s *appServer) handleUpdateOrderStatus(w http.ResponseWriter, r *http.Request) {
	var req statusRequest
	if err := jsonDecode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Request body must be JSON."})
		return
	}
	ctx := r.Context()
	orderID := r.PathValue("orderId")
	row, err := s.db.getOrderByID(ctx, orderID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Order not found."})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
		return
	}

	fromStatus := row.Status
	toStatus := req.Status
	allowed := false
	for _, next := range allowedTransitions[fromStatus] {
		if next == toStatus {
			allowed = true
			break
		}
	}
	if !allowed {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": fmt.Sprintf("Cannot move order from %s to %s.", fromStatus, toStatus)})
		return
	}

	// restore the status to the one right before the most recent close
	if fromStatus == "closed" {
		expected := expectedReopenStatus(ctx, s.db, row)
		if expected != "" && toStatus != expected {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": fmt.Sprintf("Re-open must restore previous status (%s).", expected)})
			return
		}
	}

	// state transition from 'new' to 'prepared' requires every line is marked collected or sold out
	if fromStatus == "new" && toStatus == "prepared" {
		lines, err := s.db.listOrderLines(ctx, row.RowID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
			return
		}
		for _, line := range lines {
			if !line.Collected && line.Item.LineAction != "sold_out" {
				writeJSON(w, http.StatusConflict, map[string]string{
					"error": "Every line must be collected or sold out before preparing."})
				return
			}
		}
	}

	actor := staffUserFrom(r).DisplayName
	summary := statusSummary(fromStatus, toStatus, req.ClosedReason, req.PaidReason)

	if toStatus == "closed" {
		if req.ClosedReason == nil || *req.ClosedReason == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "A closed reason is required."})
			return
		}
		summary = statusSummary(fromStatus, toStatus, req.ClosedReason, nil)
	}
	if toStatus == "paid" {
		if row.PaidReason.String == "shopify" && req.PaidReason != nil && *req.PaidReason != "shopify" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Paid via Shopify is locked and cannot be changed."})
			return
		}
		if row.PaidReason.String != "shopify" && (req.PaidReason == nil || *req.PaidReason == "") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "A paid reason is required."})
			return
		}
		if req.PaidReason != nil && *req.PaidReason == "cash" {
			if req.PaidAmountCents == nil || *req.PaidAmountCents <= 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error": "Cash amount received is required."})
				return
			}
		}
		if req.PaidAmountCents != nil && (*req.PaidAmountCents < 0 || *req.PaidAmountCents > 10_000_00) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Out of range(0<>10,000,00): PaidAmountCents."})
			return
		}
		summary = statusSummary(fromStatus, toStatus, nil, req.PaidReason)
	}
	if toStatus == "notified" {
		channel := req.NotificationChannel
		if channel == nil || *channel == "" {
			channel = nullStr(row.NotificationChannel)
		}
		if channel == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Choose how the customer was notified."})
			return
		}
		if *channel == "discord" && strings.TrimSpace(row.DiscordHandle) == "" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Order has no Discord handle."})
			return
		}
		if fromStatus == "paid" && row.PaidReason.String == "shopify" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Paid via Shopify Invoice cannot be undone."})
			return
		}
		summary = notificationSummary(*channel)
	}

	sets := []string{"status = $" + strconv.Itoa(1)}
	args := []any{toStatus}

	switch toStatus {
	case "closed":
		sets = append(sets, "closed_reason = $"+strconv.Itoa(len(args)+1), "closed_reason_other = $"+strconv.Itoa(len(args)+2))
		args = append(args, *req.ClosedReason)
		other := ""
		if *req.ClosedReason == "other" && req.ClosedReasonOther != nil {
			other = strings.TrimSpace(*req.ClosedReasonOther)
		}
		args = append(args, other)
	default:
		sets = append(sets, "closed_reason = NULL", "closed_reason_other = NULL")
	}

	switch toStatus {
	case "paid":
		reason := req.PaidReason
		if row.PaidReason.String == "shopify" {
			sets = append(sets, "paid_reason = 'shopify'", "paid_reason_other = NULL")
		} else {
			sets = append(sets, "paid_reason = $"+strconv.Itoa(len(args)+1), "paid_reason_other = NULL")
			args = append(args, *reason)
		}
		if req.PaidAmountCents != nil {
			sets = append(sets, "paid_amount_cents = $"+strconv.Itoa(len(args)+1))
			args = append(args, *req.PaidAmountCents)
		}
	case "new", "prepared", "invoiced", "notified":
		sets = append(sets, "paid_reason = NULL", "paid_reason_other = NULL", "paid_amount_cents = NULL")
	}

	if toStatus == "notified" {
		sets = append(sets, "notification_channel = $"+strconv.Itoa(len(args)+1))
		args = append(args, *req.NotificationChannel)
	} else if toStatus == "new" || toStatus == "prepared" || toStatus == "invoiced" {
		sets = append(sets, "notification_channel = NULL")
	}

	if err := s.db.updateOrderFields(ctx, row.RowID, strings.Join(sets, ", "), args, []historyInsert{{
		Kind: "status", Summary: summary,
		FromStatus: strPtr(fromStatus), ToStatus: strPtr(toStatus), StaffName: actor,
	}}); err != nil {
		log.Printf("update order status failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not update the order."})
		return
	}

	// canceled/closed orders reset fulfillment flags
	if toStatus == "closed" && (*req.ClosedReason == "canceled" || *req.ClosedReason == "closed") {
		if err := s.db.clearOrderFulfillment(ctx, row.RowID); err != nil {
			log.Printf("clear fulfillment failed: %v", err)
		}
	}

	// when order is marked paid with cash, pos, paypal
	//   convert the shopify draft into a real paid order
	//   shortcut to automate having to do this in shopify admin
	var completedOrderGID string
	if toStatus == "paid" && s.shopify != nil && row.ShopifyDraftOrderID.Valid {
		orderGID, err := s.shopify.completeDraftOrder(ctx, row.ShopifyDraftOrderID.String)
		if err != nil {
			log.Printf("complete draft order failed for %s: %v", row.OrderID, err)
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"error": "Order marked paid in db, completing the Shopify draft failed. [see shopify api logs]"})
			return
		}
		completedOrderGID = orderGID
		if orderGID != "" {
			if err := s.db.setShopifyOrderID(ctx, row.RowID, orderGID); err != nil {
				log.Printf("record shopify order id failed for %s: %v", row.OrderID, err)
			}
		}
	}

	// @TODO post a Shopify order note/comment for cash payments
	if toStatus == "paid" && req.PaidReason != nil && *req.PaidReason == "cash" && req.PaidAmountCents != nil {
		note := fmt.Sprintf("Received $%.2f CAD received", float64(*req.PaidAmountCents)/100)
		orderRef := completedOrderGID
		if orderRef == "" && row.ShopifyOrderID.Valid {
			orderRef = row.ShopifyOrderID.String
		}
		if orderRef == "" {
			orderRef = row.OrderID
		}
		log.Printf("shopify cash note stub for %s (%s): %s", row.OrderID, orderRef, note)
	}

	updated, err := s.db.getOrderByID(ctx, orderID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
		return
	}
	out, err := s.orderFromRow(ctx, updated, true)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
		return
	}
	s.broadcastOrderEvent("update")
	writeJSON(w, http.StatusOK, out)
}

func statusSummary(from, to string, closedReason, paidReason *string) string {
	labels := map[string]string{
		"new": "New", "prepared": "Prepared", "invoiced": "Invoiced",
		"notified": "Notified", "paid": "Paid", "closed": "Closed",
	}
	toLabel := labels[to]
	if to == "closed" && closedReason != nil {
		closed := map[string]string{
			"picked_up": "Picked Up",
			"closed":    "Closed",
			"canceled":  "Canceled",
			"refunded":  "Refunded",
			"other":     "Other",
		}
		toLabel += " (" + closed[*closedReason] + ")"
	}
	if to == "paid" && paidReason != nil {
		paid := map[string]string{"shopify": "Paid via Shopify Invoice", "credit_card": "Paid via Credit Card", "paypal": "Paid via PayPal", "cash": "Paid via Cash"}
		toLabel += " (" + paid[*paidReason] + ")"
	}
	return labels[from] + " → " + toLabel
}

func notificationSummary(channel string) string {
	labels := map[string]string{
		"discord": "Notified On Discord", "email": "Notified Via Email Only", "in_person": "Notified In Person",
	}
	return labels[channel]
}

func (s *appServer) handleUpdateOrderNotes(w http.ResponseWriter, r *http.Request) {
	var body struct {
		StaffNotes string `json:"staffNotes"`
	}
	if err := jsonDecode(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Request body must be JSON."})
		return
	}
	ctx := r.Context()
	orderID := r.PathValue("orderId")
	row, err := s.db.getOrderByID(ctx, orderID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Order not found."})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
		return
	}
	if err := s.db.updateOrderFields(ctx, row.RowID, "staff_notes = $1", []any{body.StaffNotes}, []historyInsert{{
		Kind: "notes", Summary: "Updated staff notes", StaffName: staffUserFrom(r).DisplayName,
	}}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not update the order."})
		return
	}
	updated, err := s.db.getOrderByID(ctx, orderID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
		return
	}
	out, err := s.orderFromRow(ctx, updated, true)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
		return
	}
	s.broadcastOrderEvent("update")
	writeJSON(w, http.StatusOK, out)
}

type linePatch struct {
	Collected         *bool   `json:"collected"`
	LineAction        *string `json:"lineAction"`
	CollectedQuantity *int    `json:"collectedQuantity"`
}

func (s *appServer) handleUpdateOrderLine(w http.ResponseWriter, r *http.Request) {
	var patch linePatch
	if err := jsonDecode(r, &patch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Request body must be JSON."})
		return
	}
	ctx := r.Context()
	orderID := r.PathValue("orderId")
	lineIndex, err := strconv.Atoi(r.PathValue("lineIndex"))
	if err != nil || lineIndex < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid line index."})
		return
	}
	row, err := s.db.getOrderByID(ctx, orderID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Order not found."})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
		return
	}
	lines, err := s.db.listOrderLines(ctx, row.RowID)
	if err != nil || lineIndex >= len(lines) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Order line not found."})
		return
	}

	line := lines[lineIndex]
	item := line.Item
	newCollected := line.Collected
	if patch.Collected != nil {
		newCollected = *patch.Collected
	}
	newAction := item.LineAction
	if patch.LineAction != nil {
		newAction = *patch.LineAction
	}
	var newQty *int
	if newAction == "sold_out" {
		newCollected = false
		newQty = nil
	} else if newAction == "partial" {
		newQty = line.CollectedQty
		if patch.CollectedQuantity != nil {
			newQty = patch.CollectedQuantity
		} else if newQty == nil {
			fallback := max(0, item.Quantity-1)
			newQty = &fallback
		}
	} else {
		newQty = nil
	}

	// used to calculate line subtotal
	effectiveQty := item.Quantity
	if newAction == "sold_out" {
		effectiveQty = 0
	} else if newAction == "partial" && newQty != nil {
		effectiveQty = *newQty
	}
	item.LineAction = newAction
	item.LineSubtotalCents = item.UnitPriceCents * effectiveQty

	parts := []string{}
	if patch.Collected != nil {
		parts = append(parts, map[bool]string{true: "collected", false: "uncollected"}[*patch.Collected])
	}
	if patch.LineAction != nil {
		switch newAction {
		case "sold_out":
			parts = append(parts, "sold out")
		case "partial":
			parts = append(parts, "partial")
		default:
			parts = append(parts, "cleared line action")
		}
	}
	if patch.CollectedQuantity != nil {
		parts = append(parts, fmt.Sprintf("qty %d", *patch.CollectedQuantity))
	}
	if len(parts) == 0 {
		parts = append(parts, "updated")
	}

	fromStatus := row.Status
	history := []historyInsert{{
		Kind:       "line",
		Summary:    fmt.Sprintf("Line \u201c%s\u201d: %s", item.Title, strings.Join(parts, ", ")),
		FromStatus: strPtr(fromStatus), ToStatus: strPtr(fromStatus),
		StaffName: staffUserFrom(r).DisplayName,
	}}
	if fromStatus == "prepared" {
		history = append(history, historyInsert{
			Kind: "status", Summary: statusSummary("prepared", "new", nil, nil),
			FromStatus: strPtr("prepared"), ToStatus: strPtr("new"),
			StaffName: staffUserFrom(r).DisplayName,
		})
	}

	if err := s.db.updateOrderLine(ctx, row.RowID, int64(lineIndex), item, newCollected, newQty, history); err != nil {
		log.Printf("update order line failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not update the order line."})
		return
	}

	if fromStatus == "prepared" {
		if err := s.db.updateOrderFields(ctx, row.RowID, "status = $1", []any{"new"}, nil); err != nil {
			log.Printf("reset status failed: %v", err)
		}
	}

	updated, err := s.db.getOrderByID(ctx, orderID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
		return
	}
	out, err := s.orderFromRow(ctx, updated, true)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
		return
	}
	s.broadcastOrderEvent("update")
	writeJSON(w, http.StatusOK, out)
}

func (s *appServer) handleSendInvoice(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orderID := r.PathValue("orderId")
	row, err := s.db.getOrderByID(ctx, orderID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Order not found."})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
		return
	}
	if row.Status != "prepared" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Only prepared orders can be invoiced."})
		return
	}

	actor := staffUserFrom(r).DisplayName

	// @TODO send real Shopify API requests to create and email invoices
	description, totalCents, err := buildInvoiceLines(ctx, s, row)
	if err != nil {
		log.Printf("build invoice lines failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not build the invoice."})
		return
	}

	// @TODO add shopify api retries on error
	invoiceID := fmt.Sprintf("SH-%s-%05d", regexp.MustCompile(`\W`).ReplaceAllString(row.OrderID, ""), time.Now().Unix()%100000)
	invoiceURL := ""
	draftOrderGID := ""

	if s.shopify == nil {
		log.Printf("shopify not configured, using stub invoice for %s", row.OrderID)
	} else {
		draft, err := s.shopify.createDraftOrder(ctx, shopifyDraftRequest{
			Email:       row.Email,
			Note:        description,
			OrderNumber: row.OrderID,
			LineTitle:   fmt.Sprintf("Artist Colony Order %s", row.OrderID),
			PriceCents:  totalCents,
		})
		if err != nil {
			log.Printf("create draft order failed for %s: %v", row.OrderID, err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Shopify rejected create invoice request."})
			return
		}
		if err := s.shopify.sendDraftOrderInvoice(ctx, draft.ID, row.Email,
			fmt.Sprintf("Artist Colony Order %s", row.OrderID), description); err != nil {
			log.Printf("send draft order invoice failed for %s: %v", row.OrderID, err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Draft order created but the invoice email failed"})
			return
		}
		invoiceID = shopifyDraftOrderNumber(draft.ID)
		invoiceURL = draft.InvoiceURL
		draftOrderGID = draft.ID
	}

	history := []historyInsert{{
		Kind: "invoice", Summary: fmt.Sprintf("Sent invoice (%s)", invoiceID),
		FromStatus: strPtr("prepared"), ToStatus: strPtr("invoiced"), StaffName: actor,
	}, {
		Kind: "status", Summary: statusSummary("prepared", "invoiced", nil, nil),
		FromStatus: strPtr("prepared"), ToStatus: strPtr("invoiced"), StaffName: actor,
	}}
	channel := ""
	if row.MerchTableOrder {
		history = append(history, historyInsert{
			Kind: "status", Summary: notificationSummary("in_person"),
			FromStatus: strPtr("invoiced"), ToStatus: strPtr("notified"), StaffName: actor,
		})
		channel = "in_person"
	}

	sets := []string{"shopify_invoice_id = $1", "shopify_draft_order_id = NULLIF($2, '')", "shopify_invoice_url = NULLIF($3, '')", "status = $4", "notification_channel = NULL"}
	args := []any{invoiceID, draftOrderGID, invoiceURL, "invoiced"}
	if channel != "" {
		sets = []string{"shopify_invoice_id = $1", "shopify_draft_order_id = NULLIF($2, '')", "shopify_invoice_url = NULLIF($3, '')", "status = $4", "notification_channel = $5"}
		args = append(args, channel)
	}
	if err := s.db.updateOrderFields(ctx, row.RowID, strings.Join(sets, ", "), args, history); err != nil {
		log.Printf("send invoice failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not update the order."})
		return
	}

	updated, err := s.db.getOrderByID(ctx, orderID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
		return
	}
	out, err := s.orderFromRow(ctx, updated, true)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
		return
	}
	total := fmt.Sprintf("$%.2f", float64(row.SubtotalCents)/100)
	message := fmt.Sprintf("Shopify order %s created for %s and emailed to %s.", invoiceID, total, row.Email)
	if row.MerchTableOrder {
		message = fmt.Sprintf("Shopify order %s created for %s. Marked Notified in Person.", invoiceID, total)
	}
	s.broadcastOrderEvent("update")
	writeJSON(w, http.StatusOK, map[string]any{"order": out, "invoiceId": invoiceID, "message": message})
}

func (s *appServer) handleResendInvoice(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orderID := r.PathValue("orderId")
	row, err := s.db.getOrderByID(ctx, orderID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Order not found."})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
		return
	}
	if row.Status != "invoiced" && row.Status != "notified" {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "Only invoiced or notified orders can resend an invoice."})
		return
	}
	if !row.ShopifyInvoiceID.Valid || strings.TrimSpace(row.ShopifyInvoiceID.String) == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Order has no invoice to resend."})
		return
	}

	actor := staffUserFrom(r).DisplayName
	invoiceID := row.ShopifyInvoiceID.String

	// @TODO resend the existing Shopify Draft order invoice email
	log.Printf("⚠️ @TODO @STUB resend invoice for %s (%s) to %s", row.OrderID, invoiceID, row.Email)

	if err := s.db.updateOrderFields(ctx, row.RowID, "status = status", []any{}, []historyInsert{{
		Kind: "invoice", Summary: fmt.Sprintf("Resent invoice (%s)", invoiceID),
		StaffName: actor,
	}}); err != nil {
		log.Printf("resend invoice failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not update the order."})
		return
	}

	updated, err := s.db.getOrderByID(ctx, orderID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
		return
	}
	out, err := s.orderFromRow(ctx, updated, true)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
		return
	}
	message := fmt.Sprintf("Invoice %s resent to %s.", invoiceID, row.Email)
	s.broadcastOrderEvent("update")
	writeJSON(w, http.StatusOK, map[string]any{"order": out, "invoiceId": invoiceID, "message": message})
}

func (s *appServer) handleMarkNotified(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Channel string `json:"channel"`
	}
	if err := jsonDecode(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Request body must be JSON."})
		return
	}
	ctx := r.Context()
	orderID := r.PathValue("orderId")
	row, err := s.db.getOrderByID(ctx, orderID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Order not found."})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
		return
	}
	if row.Status != "invoiced" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Only invoiced orders can be marked notified."})
		return
	}
	if !row.ShopifyInvoiceID.Valid {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Shopify has not returned an order number yet."})
		return
	}
	switch body.Channel {
	case "discord":
		if strings.TrimSpace(row.DiscordHandle) == "" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Order has no Discord handle."})
			return
		}
	case "email":
		if strings.TrimSpace(row.DiscordHandle) != "" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Use Notified on Discord when a Discord handle is present."})
			return
		}
	case "in_person":
		if !row.MerchTableOrder {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Notified in Person is only for merch table orders."})
			return
		}
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid notification channel."})
		return
	}

	if err := s.db.updateOrderFields(ctx, row.RowID, "notification_channel = $1, status = $2",
		[]any{body.Channel, "notified"}, []historyInsert{{
			Kind: "status", Summary: notificationSummary(body.Channel),
			FromStatus: strPtr("invoiced"), ToStatus: strPtr("notified"),
			StaffName: staffUserFrom(r).DisplayName,
		}}); err != nil {
		log.Printf("mark notified failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not update the order."})
		return
	}

	updated, err := s.db.getOrderByID(ctx, orderID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
		return
	}
	out, err := s.orderFromRow(ctx, updated, true)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not load order."})
		return
	}
	s.broadcastOrderEvent("update")
	writeJSON(w, http.StatusOK, out)
}

// ## Inventory

type inventoryItemJSON struct {
	ID             string `json:"id"`
	ShopName       string `json:"shopName"`
	ArtistName     string `json:"artistName"`
	ProductCode    string `json:"productCode"`
	Title          string `json:"title"`
	ItemType       string `json:"itemType"`
	ProductDisplay string `json:"productDisplay"`
	Quantity       *int   `json:"quantity"`
	PriceCents     int    `json:"priceCents"`
	Notes          string `json:"notes"`
	SoldOut        bool   `json:"soldOut"`
}

func (s *appServer) soldOutOverrides(ctx context.Context) map[string]bool {
	overrides, err := s.db.listSoldOutOverrides(ctx)
	if err != nil {
		log.Printf("list sold-out overrides failed: %v", err)
		return nil
	}
	return overrides
}

func (s *appServer) handleListInventory(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	shop := strings.TrimSpace(r.URL.Query().Get("shop"))

	s.inventoryMu.Lock()
	items := make([]inventoryItemJSON, len(s.inventoryItems))
	copy(items, s.inventoryItems)
	s.inventoryMu.Unlock()

	overrides := s.soldOutOverrides(r.Context())

	out := make([]inventoryItemJSON, 0, len(items))
	for _, item := range items {
		if shop != "" && item.ShopName != shop {
			continue
		}
		if override, ok := overrides[item.ID]; ok {
			item.SoldOut = override
		}
		if q != "" {
			haystack := strings.ToLower(strings.Join([]string{
				item.ProductCode, item.Title, item.ShopName, item.ArtistName,
				item.ItemType, item.ProductDisplay, item.Notes}, " "))
			if !strings.Contains(haystack, q) {
				continue
			}
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *appServer) handleSetSoldOut(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SoldOut bool `json:"soldOut"`
	}
	if err := jsonDecode(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Request body must be JSON."})
		return
	}

	id := r.PathValue("id") // like "EE1::Credit Buddies"

	s.inventoryMu.Lock()
	var item *inventoryItemJSON
	for i := range s.inventoryItems {
		if s.inventoryItems[i].ID == id {
			item = &s.inventoryItems[i]
			break
		}
	}
	s.inventoryMu.Unlock()
	if item == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Inventory item not found."})
		return
	}
	if err := s.db.upsertSoldOut(r.Context(), id, body.SoldOut); err != nil {
		log.Printf("upsert sold-out failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not update sold-out flag."})
		return
	}
	updated := *item
	updated.SoldOut = body.SoldOut
	writeJSON(w, http.StatusOK, updated)
}

// public (no auth)
// returns ids of items that are sold out
func (s *appServer) handlePublicSoldOut(w http.ResponseWriter, r *http.Request) {
	s.inventoryMu.Lock()
	items := make([]inventoryItemJSON, len(s.inventoryItems))
	copy(items, s.inventoryItems)
	s.inventoryMu.Unlock()

	overrides := s.soldOutOverrides(r.Context())

	out := make([]string, 0)
	for _, item := range items {
		soldOut := item.SoldOut
		if override, ok := overrides[item.ID]; ok {
			soldOut = override
		}
		if soldOut {
			out = append(out, item.ID)
		}
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}
