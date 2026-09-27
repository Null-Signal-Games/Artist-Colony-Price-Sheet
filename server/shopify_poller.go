package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"
)

// startShopifyPoller polls shopify api for any open Draft orders 
//   flips orders to Paid if shopify api found payment
func (s *appServer) startShopifyPoller(parent context.Context) {
	if s.shopify == nil {
		log.Printf("shopify poller disabled, invoicing not configured")
		return
	}

	interval := 60
	if raw := os.Getenv("SHOPIFY_POLL_INTERVAL_SECONDS"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			interval = parsed
		}
	}

	go func() {
		log.Printf("shopify poller started, interval=%ds", interval)
		ticker := time.NewTicker(time.Duration(interval) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-parent.Done():
				return
			case <-ticker.C:
				s.pollShopifyPayments(parent)
			}
		}
	}()
}

func (s *appServer) pollShopifyPayments(ctx context.Context) {
	rows, err := s.db.listOrdersByShopifyDraftID(ctx)
	if err != nil {
		log.Printf("shopify poller: list orders failed: %v", err)
		return
	}
	if len(rows) > 0 {
		log.Printf("shopify poller: checking %d open draft orders", len(rows))
	}
	for _, row := range rows {
		select {
		case <-ctx.Done():
			return
		default:
		}

		status, paid, orderGID, err := s.shopify.getDraftOrderStatus(ctx, row.ShopifyDraftOrderID.String)
		if err != nil {
			paid, orderGID, err = s.shopify.findPaidOrderByNumber(ctx, row.OrderID)
			if err != nil {
				log.Printf("shopify poller: %s lookup failed: %v", row.OrderID, err)
				continue
			}
		}
		_ = status

		if !paid {
			continue
		}

		// record amount actually collected
		var amount *int
		_, collectedCents, err := buildInvoiceLines(ctx, s, &row)
		if err != nil {
			log.Printf("shopify poller: collected total for %s faled: %v", row.OrderID, err)
		} else {
			amount = &collectedCents
		}
		if err := s.db.markOrderPaidByShopify(ctx, row.RowID, row.Status, amount, orderGID); err != nil {
			log.Printf("shopify poller: mark %s paid failed: %v", row.OrderID, err)
			continue
		}
		log.Printf("shopify poller: order %s paid via Shopify invoice", row.OrderID)
	}
}
