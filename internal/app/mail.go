package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type Sender interface {
	Send(ctx context.Context, to, subject, text, html string) error
}

// Resend envia pela API do Resend. Sem APIKey, só escreve o e-mail no log (modo dev).
type Resend struct {
	APIKey string
	From   string
	client *http.Client
}

func NewResend(apiKey, from string) *Resend {
	return &Resend{APIKey: apiKey, From: from, client: &http.Client{Timeout: 15 * time.Second}}
}

func (m *Resend) Send(ctx context.Context, to, subject, text, html string) error {
	if m.APIKey == "" {
		log.Printf("[mail dev] para=%s assunto=%q\n%s", to, subject, text)
		return nil
	}
	body, _ := json.Marshal(map[string]any{
		"from":    m.From,
		"to":      []string{to},
		"subject": subject,
		"text":    text,
		"html":    html,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.APIKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return fmt.Errorf("resend %d: %s", res.StatusCode, b)
	}
	return nil
}
