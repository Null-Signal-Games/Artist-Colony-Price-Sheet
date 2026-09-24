package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// @TODO: add orm or migration framework
const schema = `
CREATE TABLE IF NOT EXISTS orders (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  order_id TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  discord_handle TEXT NOT NULL DEFAULT '',
  email TEXT NOT NULL,
  subtotal_cents INTEGER NOT NULL DEFAULT 0,
  submitted_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE IF NOT EXISTS order_items (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  order_id INTEGER NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  product_code TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL,
  artist TEXT NOT NULL DEFAULT '',
  unit_price_cents INTEGER NOT NULL DEFAULT 0,
  quantity INTEGER NOT NULL DEFAULT 1,
  line_subtotal_cents INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_order_items_order_id ON order_items(order_id);
`

func openDB(dbPath string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}

	dsn := dbPath + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

type store struct {
	db *sql.DB
}

// func (s *store) verifyOrderExists(orderID string) bool {
// 	var n int
// 	_ = s.db.QueryRow("SELECT 1 FROM orders WHERE order_id=?", orderID).Scan(&n)
// 	return n == 1
// }

// @TODO: @SECURITY check for injections
func (s *store) insertOrder(sub *orderSubmission, computed []computedItem, subtotalCents int) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO orders (order_id, name, discord_handle, email, subtotal_cents, submitted_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		sub.OrderID, sub.Name, sub.DiscordHandle, sub.Email, subtotalCents, sub.SubmittedAt)
	if err != nil {
		return 0, err
	}

	orderRowID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO order_items
			(order_id, product_code, title, artist, unit_price_cents, quantity, line_subtotal_cents)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	for _, item := range computed {
		if _, err := stmt.ExecContext(ctx, orderRowID, item.ProductCode, item.Title,
			item.Artist, item.UnitPriceCents, item.Quantity, item.LineSubtotalCents); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return orderRowID, nil
}
