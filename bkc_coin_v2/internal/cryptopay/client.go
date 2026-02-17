package cryptopay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://pay.crypt.bot/api"

type Client struct {
	Token   string
	BaseURL string
	HTTP    *http.Client
}

type apiResponseRaw struct {
	OK     bool   `json:"ok"`
	Error  string `json:"error"`
	Result json.RawMessage `json:"result"`
}

type Invoice struct {
	InvoiceID        int64  `json:"invoice_id"`
	Status           string `json:"status"`
	CurrencyType     string `json:"currency_type"`
	Fiat             string `json:"fiat"`
	Amount           string `json:"amount"`
	Payload          string `json:"payload"`
	BotInvoiceURL    string `json:"bot_invoice_url"`
	MiniAppInvoiceURL string `json:"mini_app_invoice_url"`
	WebAppInvoiceURL string `json:"web_app_invoice_url"`
	CreatedAt        string `json:"created_at"`
}

type CreateInvoiceRequest struct {
	CurrencyType   string `json:"currency_type,omitempty"`
	Asset          string `json:"asset,omitempty"`
	Fiat           string `json:"fiat,omitempty"`
	AcceptedAssets string `json:"accepted_assets,omitempty"`
	Amount         string `json:"amount"`
	Description    string `json:"description,omitempty"`
	Payload        string `json:"payload,omitempty"`
	ExpiresIn      int    `json:"expires_in,omitempty"`
	AllowComments  bool   `json:"allow_comments,omitempty"`
	AllowAnonymous bool   `json:"allow_anonymous,omitempty"`
}

type GetInvoicesRequest struct {
	InvoiceIDs string `json:"invoice_ids,omitempty"`
	Status     string `json:"status,omitempty"`
}

type GetInvoicesResult struct {
	Items []Invoice `json:"items"`
}

type WebhookUpdate struct {
	UpdateID    int64   `json:"update_id"`
	UpdateType  string  `json:"update_type"`
	RequestDate string  `json:"request_date"`
	Payload     Invoice `json:"payload"`
}

func New(token string) *Client {
	return &Client{
		Token:   strings.TrimSpace(token),
		BaseURL: DefaultBaseURL,
		HTTP:    &http.Client{Timeout: 12 * time.Second},
	}
}

func (c *Client) CreateInvoice(ctx context.Context, req CreateInvoiceRequest) (Invoice, error) {
	var out Invoice
	if err := c.post(ctx, "createInvoice", req, &out); err != nil {
		return Invoice{}, err
	}
	return out, nil
}

func (c *Client) GetInvoices(ctx context.Context, invoiceIDs string) ([]Invoice, error) {
	var out GetInvoicesResult
	if err := c.post(ctx, "getInvoices", GetInvoicesRequest{InvoiceIDs: invoiceIDs}, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

func (c *Client) post(ctx context.Context, method string, body any, out any) error {
	if c.Token == "" {
		return errors.New("CRYPTOPAY_API_TOKEN not set")
	}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}

	url := strings.TrimRight(c.BaseURL, "/") + "/" + method
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Crypto-Pay-API-Token", c.Token)

	httpRes, err := c.HTTP.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpRes.Body.Close()
	payload, _ := io.ReadAll(httpRes.Body)

	if httpRes.StatusCode >= 400 {
		return fmt.Errorf("cryptopay http %d: %s", httpRes.StatusCode, string(payload))
	}

	var parsed apiResponseRaw
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return err
	}
	if !parsed.OK {
		return fmt.Errorf("cryptopay error: %s", parsed.Error)
	}
	if out != nil {
		if err := json.Unmarshal(parsed.Result, out); err != nil {
			return err
		}
	}
	return nil
}

// VerifyWebhookSignature verifies crypto-pay-api-signature header.
// Signature is HMAC-SHA256 of raw body, using secret = sha256(appToken).
func VerifyWebhookSignature(appToken string, rawBody []byte, headerSignature string) bool {
	appToken = strings.TrimSpace(appToken)
	if appToken == "" || len(rawBody) == 0 {
		return false
	}
	headerSignature = strings.TrimSpace(headerSignature)
	if headerSignature == "" {
		return false
	}

	secretHash := sha256.Sum256([]byte(appToken))
	mac := hmac.New(sha256.New, secretHash[:])
	mac.Write(rawBody)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(headerSignature))
}

func ParseAmountInt(amount string) int64 {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return 0
	}
	// CryptoPay uses float strings; we only accept integers for USD here.
	if strings.Contains(amount, ".") {
		parts := strings.SplitN(amount, ".", 2)
		amount = parts[0]
	}
	n, _ := strconv.ParseInt(amount, 10, 64)
	return n
}
