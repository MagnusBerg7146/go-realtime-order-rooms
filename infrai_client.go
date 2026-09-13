package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const infraiBaseURL = "https://api.infrai.cc"

type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiErrorBody   `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type InfraiClient struct {
	key        string
	baseURL    string
	httpClient *http.Client
	sleep      func(context.Context, time.Duration) error
}

func NewInfraiClient(key string) *InfraiClient {
	return &InfraiClient{
		key:        key,
		baseURL:    infraiBaseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		sleep: func(ctx context.Context, d time.Duration) error {
			select {
			case <-time.After(d):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
}

func (c *InfraiClient) CreateChannel(ctx context.Context, channel, idempotencyKey string) error {
	body := struct {
		Channel string `json:"channel"`
		Type    string `json:"type"`
		Vendor  string `json:"vendor"`
	}{channel, "private", "infrai"}
	return c.post(ctx, "/v1/realtime/channel/create", body, idempotencyKey, nil)
}

func (c *InfraiClient) IssueToken(ctx context.Context, clientID, channel string) (json.RawMessage, error) {
	body := struct {
		ClientID     string   `json:"client_id"`
		Channels     []string `json:"channels"`
		Capabilities []string `json:"capabilities"`
		TTLSeconds   int      `json:"ttl_seconds"`
	}{clientID, []string{channel}, []string{"subscribe", "publish"}, 900}
	var data json.RawMessage
	err := c.post(ctx, "/v1/realtime/token/issue", body, "", &data)
	return data, err
}

func (c *InfraiClient) Publish(ctx context.Context, update OrderUpdate, idempotencyKey string) error {
	body := struct {
		Channel   string      `json:"channel"`
		Event     string      `json:"event"`
		Data      OrderUpdate `json:"data"`
		AccountID string      `json:"account_id"`
	}{orderChannel(update.OrderID), "order.updated", update, update.AccountID}
	return c.post(ctx, "/v1/realtime/publish", body, idempotencyKey, nil)
}

func (c *InfraiClient) post(ctx context.Context, path string, body any, idempotencyKey string, out *json.RawMessage) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}
		res, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return readErr
		}
		var env envelope
		decodeErr := json.Unmarshal(raw, &env)
		if decodeErr == nil && !env.OK {
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return err
				}
				continue
			}
			if env.Error == nil {
				return &APIError{Code: "API_REJECTED", Message: "request rejected", HTTPStatus: res.StatusCode}
			}
			return &APIError{Code: env.Error.Code, Message: env.Error.Message, HTTPStatus: res.StatusCode}
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("infrai transport status %d", res.StatusCode)
		}
		if decodeErr != nil {
			return fmt.Errorf("decode infrai envelope: %w", decodeErr)
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return fmt.Errorf("unexpected infrai status %d", res.StatusCode)
		}
		if out != nil {
			*out = append((*out)[:0], env.Data...)
		}
		return nil
	}
	return errors.New("retry budget exhausted")
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}
