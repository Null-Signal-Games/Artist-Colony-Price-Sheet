package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// shopifyClient is the connecter to Shopify GraphQL Admin API
type shopifyClient struct {
	domain       string
	clientID     string
	clientSecret string
	staticToken  string
	apiVersion   string

	mu          sync.Mutex
	cachedToken string
	tokenExp    time.Time

	httpc *http.Client
}

// newShopifyClientFromEnv returns nil if shopify is not configured
func newShopifyClientFromEnv() *shopifyClient {
	subdomain := strings.TrimSpace(envOr("PUBLIC_SHOPIFY_STORE_SUBDOMAIN", ""))
	if subdomain == "" {
		return nil
	}
	domain := subdomain + ".myshopify.com"

	staticToken := strings.TrimSpace(envOr("SHOPIFY_ADMIN_API_TOKEN", ""))
	clientID := strings.TrimSpace(envOr("SHOPIFY_CLIENT_ID", ""))
	clientSecret := strings.TrimSpace(envOr("SHOPIFY_CLIENT_SECRET", ""))
	if staticToken == "" && (clientID == "" || clientSecret == "") {
		return nil
	}

	apiVersion := envOr("SHOPIFY_API_VERSION", "2026-07")
	return &shopifyClient{
		domain:       domain,
		clientID:     clientID,
		clientSecret: clientSecret,
		staticToken:  staticToken,
		apiVersion:   apiVersion,
		httpc:        &http.Client{Timeout: 20 * time.Second},
	}
}

type shopifyTokenResponse struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
	ExpiresIn   int    `json:"expires_in"`
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

// accessToken returns a cached admin api token
//   refreshes client credentials grant when needed (on 401)
func (c *shopifyClient) accessToken(ctx context.Context) (string, error) {
	if c.staticToken != "" {
		return c.staticToken, nil
	}

	c.mu.Lock()
	if c.cachedToken != "" && time.Now().Before(c.tokenExp) {
		tok := c.cachedToken
		c.mu.Unlock()
		return tok, nil
	}
	c.mu.Unlock()

	body, _ := json.Marshal(map[string]string{
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
		"grant_type":    "client_credentials",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("https://%s/admin/oauth/access_token", c.domain), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.httpc.Do(req)
	if err != nil {
		return "", fmt.Errorf("shopify token request: %w", err)
	}
	defer res.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("shopify token request: status %d: %s", res.StatusCode, truncateShopifyBody(raw))
	}
	var tok shopifyTokenResponse
	if err := json.Unmarshal(raw, &tok); err != nil {
		return "", fmt.Errorf("decode shopify token response: %w", err)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("shopify token response missing access_token: %s", truncateShopifyBody(raw))
	}

	c.mu.Lock()
	c.cachedToken = tok.AccessToken
	// refresh a bit early, shopify tokens ttl=24h
	c.tokenExp = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	if c.tokenExp.Before(time.Now().Add(time.Minute)) {
		c.tokenExp = time.Now().Add(23 * time.Hour)
	}
	c.mu.Unlock()
	return tok.AccessToken, nil
}

func (c *shopifyClient) invalidateToken() {
	c.mu.Lock()
	c.cachedToken = ""
	c.tokenExp = time.Time{}
	c.mu.Unlock()
}

// adminGraphQL posts graphql requests to the admin API
//   decodes the `data` envelope as `out`
//   retries on 401 with a fresh new token
func (c *shopifyClient) adminGraphQL(ctx context.Context, query string, variables map[string]any, out any) error {
	for attempt := 0; ; attempt++ {
		token, err := c.accessToken(ctx)
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"query": query, "variables": variables})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			fmt.Sprintf("https://%s/admin/api/%s/graphql.json", c.domain, c.apiVersion),
			bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Shopify-Access-Token", token)

		res, err := c.httpc.Do(req)
		if err != nil {
			return fmt.Errorf("shopify graphql request: %w", err)
		}
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()

		if res.StatusCode == http.StatusUnauthorized && attempt == 0 && c.staticToken == "" {
			// token probably rotated, refetch and retry once
			c.invalidateToken()
			continue
		}
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("shopify graphql: status %d: %s", res.StatusCode, truncateShopifyBody(raw))
		}

		var envelope struct {
			Data   json.RawMessage `json:"data"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return fmt.Errorf("decode shopify graphql response: %w", err)
		}
		if len(envelope.Errors) > 0 {
			msgs := make([]string, 0, len(envelope.Errors))
			for _, e := range envelope.Errors {
				msgs = append(msgs, e.Message)
			}
			return fmt.Errorf("shopify graphql errors: %s", strings.Join(msgs, "; "))
		}
		if out != nil && len(envelope.Data) > 0 {
			if err := json.Unmarshal(envelope.Data, out); err != nil {
				return fmt.Errorf("decode shopify graphql data: %w", err)
			}
		}
		return nil
	}
}

func truncateShopifyBody(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}

// ## Draft orders

type shopifyDraftRequest struct {
	Email       string
	Note        string
	OrderNumber string
	LineTitle   string
	PriceCents  int
}

type shopifyDraftOrderResult struct {
	ID         string // gid://shopify/DraftOrder/123
	InvoiceURL string
	Name       string
}

type shopifyDraftOrderCreatePayload struct {
	DraftOrderCreate struct {
		DraftOrder struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			InvoiceURL string `json:"invoiceUrl"`
		} `json:"draftOrder"`
		UserErrors []shopifyUserError `json:"userErrors"`
	} `json:"draftOrderCreate"`
}

type shopifyUserError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e shopifyUserError) String() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return e.Message
}

const draftOrderCreateMutation = `
mutation draftOrderCreate($input: DraftOrderInput!) {
  draftOrderCreate(input: $input) {
    draftOrder { id name invoiceUrl }
    userErrors { field message }
  }
}`

func (c *shopifyClient) createDraftOrder(ctx context.Context, req shopifyDraftRequest) (shopifyDraftOrderResult, error) {
	// custom line items use originalUnitPriceWithCurrency
	// "price" is not a valid field on DraftOrderLineItemInput
	// note_attributes are customAttributes in graphql schema
	input := map[string]any{
		"email":     req.Email,
		"note":      req.Note,
		"taxExempt": true,
		"lineItems": []map[string]any{{
			"title":    req.LineTitle,
			"quantity": 1,
			"originalUnitPriceWithCurrency": map[string]any{
				"amount":       fmt.Sprintf("%.2f", float64(req.PriceCents)/100),
				"currencyCode": strings.TrimSpace(envOr("SHOPIFY_CURRENCY", "CAD")),
			},
			"requiresShipping": false,
		}},
		"customAttributes": []map[string]any{
			{"key": "order_number", "value": req.OrderNumber},
		},
	}

	var out shopifyDraftOrderCreatePayload
	if err := c.adminGraphQL(ctx, draftOrderCreateMutation, map[string]any{"input": input}, &out); err != nil {
		return shopifyDraftOrderResult{}, err
	}
	if len(out.DraftOrderCreate.UserErrors) > 0 {
		msgs := make([]string, 0, len(out.DraftOrderCreate.UserErrors))
		for _, ue := range out.DraftOrderCreate.UserErrors {
			msgs = append(msgs, ue.String())
		}
		return shopifyDraftOrderResult{}, fmt.Errorf("draftOrderCreate: %s", strings.Join(msgs, "; "))
	}
	d := out.DraftOrderCreate.DraftOrder
	return shopifyDraftOrderResult{ID: d.ID, InvoiceURL: d.InvoiceURL, Name: d.Name}, nil
}

const draftOrderInvoiceSendMutation = `
mutation draftOrderInvoiceSend($id: ID!, $email: EmailInput) {
  draftOrderInvoiceSend(id: $id, email: $email) {
    draftOrder { id }
    userErrors { field message }
  }
}`

// sends invoice email for draft orders
func (c *shopifyClient) sendDraftOrderInvoice(ctx context.Context, draftOrderGID, to, subject, customMessage string) error {
	variables := map[string]any{
		"id": draftOrderGID,
		"email": map[string]any{
			"to":            to,
			"subject":       subject,
			"customMessage": customMessage,
		},
	}
	var out struct {
		DraftOrderInvoiceSend struct {
			UserErrors []shopifyUserError `json:"userErrors"`
		} `json:"draftOrderInvoiceSend"`
	}
	if err := c.adminGraphQL(ctx, draftOrderInvoiceSendMutation, variables, &out); err != nil {
		return err
	}
	for _, ue := range out.DraftOrderInvoiceSend.UserErrors {
		msg := strings.ToLower(ue.Message)
		if strings.Contains(msg, "already been sent") || strings.Contains(msg, "already sent") {
			continue
		}
		return fmt.Errorf("draftOrderInvoiceSend: %s", ue.String())
	}
	return nil
}

const draftOrderCompleteMutation = `
mutation draftOrderComplete($id: ID!) {
  draftOrderComplete(id: $id) {
    draftOrder {
      id
      status
      order { id displayFinancialStatus }
    }
    userErrors { field message }
  }
}`

const orderMarkAsPaidMutation = `
mutation orderMarkAsPaid($input: OrderMarkAsPaidInput!) {
  orderMarkAsPaid(input: $input) {
    order { id displayFinancialStatus }
    userErrors { field message }
  }
}`

// completeDraftOrder creates a "real" Shopify order and "completes" the draft order
//   marks it as `Paid`
// returns the real order gid when it's created
func (c *shopifyClient) completeDraftOrder(ctx context.Context, draftOrderGID string) (string, error) {
	var out struct {
		DraftOrderComplete struct {
			DraftOrder *struct {
				ID     string `json:"id"`
				Status string `json:"status"`
				Order  *struct {
					ID                     string `json:"id"`
					DisplayFinancialStatus string `json:"displayFinancialStatus"`
				} `json:"order"`
			} `json:"draftOrder"`
			UserErrors []shopifyUserError `json:"userErrors"`
		} `json:"draftOrderComplete"`
	}
	if err := c.adminGraphQL(ctx, draftOrderCompleteMutation,
		map[string]any{"id": draftOrderGID}, &out); err != nil {
		return "", err
	}
	ue := out.DraftOrderComplete.UserErrors
	var orderGID string
	if d := out.DraftOrderComplete.DraftOrder; d != nil {
		if d.Order != nil {
			orderGID = d.Order.ID
		}
	}
	if orderGID == "" && len(ue) > 0 {
		// try to find the order using the order_number in `note` attribute
		num := shopifyDraftOrderNumber(draftOrderGID)
		if num != "" {
			var found struct {
				Orders struct {
					Nodes []struct {
						ID string `json:"id"`
					} `json:"nodes"`
				} `json:"orders"`
			}
			q := "note_attributes.name:order_number AND note_attributes.value:" + num
			if err := c.adminGraphQL(ctx, `
query orders($q: String!) {
  orders(first: 1, query: $q) { nodes { id } }
}`, map[string]any{"q": q}, &found); err == nil && len(found.Orders.Nodes) > 0 {
				orderGID = found.Orders.Nodes[0].ID
			}
		}
	}
	if orderGID == "" {
		msgs := make([]string, 0, len(ue))
		for _, e := range ue {
			msgs = append(msgs, e.String())
		}
		if len(msgs) == 0 {
			return "", fmt.Errorf("draftOrderComplete: no order created and no user errors")
		}
		return "", fmt.Errorf("draftOrderComplete: %s", strings.Join(msgs, "; "))
	}

	// mark it Paid
	var paid struct {
		OrderMarkAsPaid struct {
			Order *struct {
				ID                     string `json:"id"`
				DisplayFinancialStatus string `json:"displayFinancialStatus"`
			} `json:"order"`
			UserErrors []shopifyUserError `json:"userErrors"`
		} `json:"orderMarkAsPaid"`
	}
	if err := c.adminGraphQL(ctx, orderMarkAsPaidMutation,
		map[string]any{"input": map[string]any{"id": orderGID}}, &paid); err != nil {
		return orderGID, err
	}
	for _, e := range paid.OrderMarkAsPaid.UserErrors {
		msg := strings.ToLower(e.Message)
		if strings.Contains(msg, "already") || strings.Contains(msg, "paid") {
			continue
		}
		return orderGID, fmt.Errorf("orderMarkAsPaid: %s", e.String())
	}
	if o := paid.OrderMarkAsPaid.Order; o != nil {
		log.Printf("shopify: order %s marked paid (%s)", o.ID, o.DisplayFinancialStatus)
	}
	return orderGID, nil
}

type shopifyDraftOrderStatus struct {
	Status      string `json:"status"` // OPEN | COMPLETED
	CompletedAt string `json:"completedAt"`
	Order       *struct {
		ID                     string `json:"id"`
		DisplayFinancialStatus string `json:"displayFinancialStatus"`
	} `json:"order"`
}

// getDraftOrderStatus checks if a Draft order turned into a paid order
// returns the "real" order gid if the Draft has completed
func (c *shopifyClient) getDraftOrderStatus(ctx context.Context, draftOrderGID string) (status string, hasPaidOrder bool, orderGID string, err error) {
	var out struct {
		DraftOrder *shopifyDraftOrderStatus `json:"draftOrder"`
	}
	const q = `
query draftOrder($id: ID!) {
  draftOrder(id: $id) {
    status
    completedAt
    order { id displayFinancialStatus }
  }
}`
	if err := c.adminGraphQL(ctx, q, map[string]any{"id": draftOrderGID}, &out); err != nil {
		return "", false, "", err
	}
	if out.DraftOrder == nil {
		return "", false, "", fmt.Errorf("draft order %s not found", draftOrderGID)
	}
	status = out.DraftOrder.Status
	if out.DraftOrder.Order != nil {
		orderGID = out.DraftOrder.Order.ID
		fin := strings.ToUpper(out.DraftOrder.Order.DisplayFinancialStatus)
		if fin == "PAID" || fin == "PARTIALLY_PAID" || fin == "PARTIALLY_REFUNDED" {
			hasPaidOrder = true
		}
	}
	return status, hasPaidOrder, orderGID, nil
}

// func (c *shopifyClient) debugDump(ctx context.Context, gid string) {
// 	status, paid, err := c.getDraftOrderStatus(ctx, gid)
// 	fmt.Println("shopify debug:", gid, "status=", status, "paid=", paid, "err=", err)
// }

// findPaidOrderByNumber is the fallback using the order_number `note` attribute
// returns true and the order gid if it finds the paid order
func (c *shopifyClient) findPaidOrderByNumber(ctx context.Context, orderNumber string) (bool, string, error) {
	var out struct {
		Orders struct {
			Nodes []struct {
				ID                     string `json:"id"`
				DisplayFinancialStatus string `json:"displayFinancialStatus"`
			} `json:"nodes"`
		} `json:"orders"`
	}
	const q = `
query orders($q: String!) {
  orders(first: 5, query: $q) {
    nodes { id displayFinancialStatus }
  }
}`
	query := fmt.Sprintf("note_attributes.name:order_number AND note_attributes.value:%s AND financial_status:paid", orderNumber)
	if err := c.adminGraphQL(ctx, q, map[string]any{"q": query}, &out); err != nil {
		return false, "", err
	}
	if len(out.Orders.Nodes) == 0 {
		return false, "", nil
	}
	return true, out.Orders.Nodes[0].ID, nil
}
