package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type staffUser struct {
	ID          int64
	Username    string
	DisplayName string
}

type staffSession struct {
	TokenHash string
	StaffUser staffUser
	ExpiresAt time.Time
}

var staffSeedUsers = []struct{ Username, DisplayName string }{
	{"jon", "Jon"},
	{"evilawn", "evilawn"},
	{"redemptor", "Redemptor"},
	{"ariel", "Ariel"},
	{"srn", "SRN"},
	{"grant", "Grant"},
	{"maddykitty", "Maddykitty"},
	{"joey", "Joey"},
}

const passwordAlphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func generatePassword(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = passwordAlphabet[rand.Intn(len(passwordAlphabet))]
	}
	return string(b)
}

func (s *store) seedStaffUsers(ctx context.Context) error {
	for _, seed := range staffSeedUsers {
		var id int64
		err := s.db.QueryRowContext(ctx,
			`SELECT id FROM staff_users WHERE username = $1`, seed.Username).Scan(&id)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		password := generatePassword(8)
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO staff_users (username, display_name, password_hash)
			VALUES ($1, $2, $3)`,
			seed.Username, seed.DisplayName, string(hash)); err != nil {
			return err
		}
		log.Printf("SEEDED STAFF USER %s displayName=%s password=%s", seed.Username, seed.DisplayName, password)
	}
	return nil
}

func (s *store) getStaffUserByUsername(ctx context.Context, username string) (*staffUser, string, error) {
	var u staffUser
	var hash string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, display_name, password_hash
		FROM staff_users WHERE username = LOWER($1)`, username).
		Scan(&u.ID, &u.Username, &u.DisplayName, &hash)
	if err != nil {
		return nil, "", err
	}
	return &u, hash, nil
}

func (s *store) createStaffSession(ctx context.Context, staffUserID int64, tokenHash string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO staff_sessions (token_hash, staff_user_id, expires_at)
		VALUES ($1, $2, $3)`, tokenHash, staffUserID, expiresAt)
	return err
}

func (s *store) getStaffSession(ctx context.Context, tokenHash string) (*staffSession, error) {
	var sess staffSession
	var userID int64
	var username, displayName string
	err := s.db.QueryRowContext(ctx, `
		SELECT ss.token_hash, ss.expires_at, su.id, su.username, su.display_name
		FROM staff_sessions ss
		JOIN staff_users su ON su.id = ss.staff_user_id
		WHERE ss.token_hash = $1 AND ss.expires_at > NOW()`, tokenHash).
		Scan(&sess.TokenHash, &sess.ExpiresAt, &userID, &username, &displayName)
	if err != nil {
		return nil, err
	}
	sess.StaffUser = staffUser{ID: userID, Username: username, DisplayName: displayName}
	return &sess, nil
}

func (s *store) deleteStaffSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM staff_sessions WHERE token_hash = $1`, tokenHash)
	return err
}

// used for order serialization
type orderRow struct {
	RowID                int64
	OrderID              string
	Name                 string
	DiscordHandle        string
	Email                string
	SubtotalCents        int
	SubmittedAt          string
	CreatedAt            time.Time
	Status               string
	StaffNotes           string
	SubmittedByStaffName sql.NullString
	MerchTableOrder      bool
	NotificationChannel  sql.NullString
	ShopifyInvoiceID     sql.NullString
	PaidReason           sql.NullString
	PaidReasonOther      sql.NullString
	ClosedReason         sql.NullString
	ClosedReasonOther    sql.NullString
}

const orderColumns = `
	o.id, o.order_id, o.name, o.discord_handle, o.email, o.subtotal_cents,
	o.submitted_at, o.created_at, o.status, o.staff_notes,
	o.submitted_by_staff_name, o.merch_table_order, o.notification_channel,
	o.shopify_invoice_id, o.paid_reason, o.paid_reason_other,
	o.closed_reason, o.closed_reason_other`

func scanOrderRow(scan func(...any) error) (*orderRow, error) {
	var o orderRow
	err := scan(&o.RowID, &o.OrderID, &o.Name, &o.DiscordHandle, &o.Email,
		&o.SubtotalCents, &o.SubmittedAt, &o.CreatedAt, &o.Status, &o.StaffNotes,
		&o.SubmittedByStaffName, &o.MerchTableOrder, &o.NotificationChannel,
		&o.ShopifyInvoiceID, &o.PaidReason, &o.PaidReasonOther,
		&o.ClosedReason, &o.ClosedReasonOther)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *store) listOrders(ctx context.Context, filter listOrdersFilter) ([]orderRow, error) {
	where := []string{"1 = 1"}
	args := []any{}
	if len(filter.Statuses) > 0 {
		placeholders := make([]string, len(filter.Statuses))
		for i, status := range filter.Statuses {
			args = append(args, status)
			placeholders[i] = fmt.Sprintf("$%d", len(args))
		}
		where = append(where, fmt.Sprintf("o.status IN (%s)", strings.Join(placeholders, ",")))
	}
	if filter.Q != "" {
		args = append(args, "%"+strings.ToLower(filter.Q)+"%")
		n := len(args)
		where = append(where, fmt.Sprintf(`(
			LOWER(o.order_id) LIKE $%d OR LOWER(o.name) LIKE $%d OR LOWER(o.email) LIKE $%d
			OR LOWER(o.discord_handle) LIKE $%d OR LOWER(COALESCE(o.submitted_by_staff_name, '')) LIKE $%d)`,
			n, n, n, n, n))
	}
	order := "o.submitted_at ASC"
	if filter.Sort == "newest" {
		order = "o.submitted_at DESC"
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT `+orderColumns+` FROM orders o WHERE `+strings.Join(where, " AND ")+` ORDER BY `+order, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []orderRow
	for rows.Next() {
		o, err := scanOrderRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, rows.Err()
}

func (s *store) getOrderByID(ctx context.Context, orderID string) (*orderRow, error) {
	return scanOrderRow(func(dest ...any) error {
		return s.db.QueryRowContext(ctx,
			`SELECT `+orderColumns+` FROM orders o WHERE o.order_id = $1`, orderID).Scan(dest...)
	})
}

// a persisted order_items row and fulfillment columns
type orderLine struct {
	Item         computedItem
	Collected    bool
	CollectedQty *int
}

func (s *store) listOrderLines(ctx context.Context, orderRowID int64) ([]orderLine, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT product_code, title, artist, unit_price_cents, quantity, line_subtotal_cents,
		       line_action, collected, collected_quantity
		FROM order_items WHERE order_id = $1 ORDER BY id`, orderRowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []orderLine
	for rows.Next() {
		var line orderLine
		var cq sql.NullInt64
		if err := rows.Scan(&line.Item.ProductCode, &line.Item.Title, &line.Item.Artist,
			&line.Item.UnitPriceCents, &line.Item.Quantity, &line.Item.LineSubtotalCents,
			&line.Item.LineAction, &line.Collected, &cq); err != nil {
			return nil, err
		}
		if cq.Valid {
			v := int(cq.Int64)
			line.CollectedQty = &v
		}
		out = append(out, line)
	}
	return out, rows.Err()
}

type historyInsert struct {
	Kind       string
	Summary    string
	FromStatus *string
	ToStatus   *string
	StaffName  string
}

func (s *store) insertHistory(ctx context.Context, tx *sql.Tx, orderRowID int64, entries ...historyInsert) error {
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO order_history (order_id, kind, summary, from_status, to_status, staff_name)
		VALUES ($1, $2, $3, $4, $5, $6)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range entries {
		if _, err := stmt.ExecContext(ctx, orderRowID, e.Kind, e.Summary,
			e.FromStatus, e.ToStatus, e.StaffName); err != nil {
			return err
		}
	}
	return nil
}

func (s *store) listOrderHistory(ctx context.Context, orderRowID int64) ([]historyEntryJSON, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, at, staff_name, kind, summary, from_status, to_status
		FROM order_history WHERE order_id = $1 ORDER BY at ASC, id ASC`, orderRowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []historyEntryJSON
	for rows.Next() {
		var e historyEntryJSON
		var at time.Time
		var from, to sql.NullString
		if err := rows.Scan(&e.ID, &at, &e.StaffName, &e.Kind, &e.Summary, &from, &to); err != nil {
			return nil, err
		}
		e.At = at.UTC().Format(time.RFC3339)
		if from.Valid {
			e.FromStatus = &from.String
		}
		if to.Valid {
			e.ToStatus = &to.String
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// insertStaffOrder creates an order with fulfillment columns set
func (s *store) insertStaffOrder(ctx context.Context, input staffOrderInput, order *orderRow, history []historyInsert) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = tx.QueryRowContext(ctx, `
		INSERT INTO orders (order_id, name, discord_handle, email, subtotal_cents, submitted_at,
			status, staff_notes, submitted_by_staff_name, merch_table_order)
		VALUES ($1, $2, $3, $4, $5, $6, $7, '', $8, $9)
		RETURNING id`,
		order.OrderID, order.Name, order.DiscordHandle, order.Email, order.SubtotalCents,
		order.SubmittedAt, order.Status, nullIfEmpty(input.SubmittedByStaffName),
		order.MerchTableOrder).Scan(&order.RowID)
	if err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO order_items (order_id, product_code, title, artist, unit_price_cents,
			quantity, line_subtotal_cents, collected, line_action, collected_quantity)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, '', NULL)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, item := range input.Items {
		if _, err := stmt.ExecContext(ctx, order.RowID, item.ProductCode, item.Title,
			item.Artist, item.UnitPriceCents, item.Quantity,
			item.UnitPriceCents*item.Quantity, order.MerchTableOrder); err != nil {
			return err
		}
	}

	if len(history) > 0 {
		if err := s.insertHistory(ctx, tx, order.RowID, history...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// applies a partial update and append history entries
func (s *store) updateOrderFields(ctx context.Context, orderRowID int64, setSQL string, args []any, history []historyInsert) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `UPDATE orders SET `+setSQL+` WHERE id = $`+fmt.Sprint(len(args)+1),
		append(args, orderRowID)...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errOrderNotFound
	}
	if len(history) > 0 {
		if err := s.insertHistory(ctx, tx, orderRowID, history...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// apply a line patch and recomputes order subtotal
func (s *store) updateOrderLine(ctx context.Context, orderRowID, lineIndex int64, item computedItem, collected bool, collectedQty *int, history []historyInsert) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE order_items SET collected = $1, line_action = $2, collected_quantity = $3,
			line_subtotal_cents = $4
		WHERE order_id = $5
		  AND id = (SELECT id FROM order_items WHERE order_id = $5 ORDER BY id OFFSET $6 LIMIT 1)`,
		collected, item.LineAction, collectedQty, item.LineSubtotalCents, orderRowID, lineIndex)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errOrderNotFound
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE orders SET subtotal_cents = (
			SELECT COALESCE(SUM(line_subtotal_cents), 0) FROM order_items WHERE order_id = $1
		) WHERE id = $1`, orderRowID); err != nil {
		return err
	}

	if len(history) > 0 {
		if err := s.insertHistory(ctx, tx, orderRowID, history...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// clearOrderFulfillment resets action marks and canceled orders
func (s *store) clearOrderFulfillment(ctx context.Context, orderRowID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE order_items SET collected = FALSE, line_action = '', collected_quantity = NULL
		WHERE order_id = $1`, orderRowID)
	return err
}

func (s *store) listSoldOutOverrides(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT item_id, sold_out FROM inventory_sold_out`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]bool)
	for rows.Next() {
		var id string
		var soldOut bool
		if err := rows.Scan(&id, &soldOut); err != nil {
			return nil, err
		}
		out[id] = soldOut
	}
	return out, rows.Err()
}

func (s *store) upsertSoldOut(ctx context.Context, itemID string, soldOut bool) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO inventory_sold_out (item_id, sold_out, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (item_id) DO UPDATE SET sold_out = $2, updated_at = NOW()`,
		itemID, soldOut)
	return err
}
