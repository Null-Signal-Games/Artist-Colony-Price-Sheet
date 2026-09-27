package main

import (
	"context"
	"fmt"
	"strings"
)

// func verifyInvoiceDescription(desc string, totalCents int) bool {
// 	fmt.Println("desc:\n" + desc)
// 	fmt.Printf("total: $%.2f\n", float64(totalCents)/100)
// 	return strings.Contains(desc, "Artist Colony Order")
// }

// buildInvoiceLines builds the actual order (units and quantity) lines 
//   to use in the order note and invoice email body text
func buildInvoiceLines(ctx context.Context, s *appServer, row *orderRow) (description string, totalCents int, err error) {
	lines, err := s.db.listOrderLines(ctx, row.RowID)
	if err != nil {
		return "", 0, fmt.Errorf("load order lines: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Artist Colony Order %s\n", row.OrderID)

	for _, line := range lines {
		if line.Item.LineAction == "sold_out" {
			continue
		}
		qty := line.Item.Quantity
		if line.Item.LineAction == "partial" && line.CollectedQty != nil {
			qty = *line.CollectedQty
		}
		if qty <= 0 {
			continue
		}
		subtotal := line.Item.UnitPriceCents * qty
		totalCents += subtotal
		fmt.Fprintf(&b, "%d \u00d7 %s \u2014 $%.2f\n", qty, line.Item.Title, float64(subtotal)/100)
	}

	fmt.Fprintf(&b, "Order: %s", row.OrderID)
	return b.String(), totalCents, nil
}

// shopifyDraftOrderNumber extracts the id from a gid
//     this ----------------v
// gid://shopify/DraftOrder/1234567890
func shopifyDraftOrderNumber(gid string) string {
	if i := strings.LastIndex(gid, "/"); i >= 0 {
		return gid[i+1:]
	}
	return gid
}
