// Copyright 2026 VEXXHOST, Inc.
// SPDX-License-Identifier: Apache-2.0

// Package client calls a cost-explorer deployment, regional or aggregator, over its v1 API.
//
// It sends one request and decodes one answer. Retries, timeouts and how a token is obtained
// belong to the caller, through the http.Client it passes and the token it sends: the
// aggregator forwards its caller's token and fails fast, and a background service mints its
// own token and retries.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/vexxhost/cost-explorer-go/apiv1"
)

// DefaultMaxResponseBytes bounds an answer. A report at max_groups is tens of MB.
const DefaultMaxResponseBytes = 256 << 20

// Client calls one deployment.
type Client struct {
	// BaseURL is the deployment's URL, without the /v1 path.
	BaseURL string
	// HTTP sends the requests. Nil uses a client that follows no redirects, because a
	// redirect could hand the request's token to another host.
	HTTP *http.Client
	// MaxResponseBytes bounds an answer; zero is DefaultMaxResponseBytes.
	MaxResponseBytes int64
}

// StatusError is an answer other than 200, with the problem detail the deployment gave.
type StatusError struct {
	StatusCode int
	Detail     string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("cost-explorer answered %d: %s", e.StatusCode, e.Detail)
}

var noRedirects = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// Report asks for a cost report.
func (c *Client) Report(ctx context.Context, token string, r apiv1.Request) (apiv1.Response, error) {
	var out apiv1.Response
	return out, c.post(ctx, "/v1/cost_reports", token, r, &out)
}

// Forecast asks for a cost forecast.
func (c *Client) Forecast(ctx context.Context, token string, r apiv1.ForecastRequest) (apiv1.ForecastResponse, error) {
	var out apiv1.ForecastResponse
	return out, c.post(ctx, "/v1/cost_forecasts", token, r, &out)
}

func (c *Client) post(ctx context.Context, path, token string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("encoding the request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Auth-Token", token)
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = noRedirects
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	limit := c.MaxResponseBytes
	if limit <= 0 {
		limit = DefaultMaxResponseBytes
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return err
	}
	if int64(len(raw)) > limit {
		return fmt.Errorf("response exceeds %d bytes", limit)
	}
	if resp.StatusCode != http.StatusOK {
		return &StatusError{StatusCode: resp.StatusCode, Detail: problemDetail(raw)}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// problemDetail reads an RFC 9457 problem document's detail, or its title.
func problemDetail(raw []byte) string {
	var problem struct {
		Detail string `json:"detail"`
		Title  string `json:"title"`
	}
	if json.Unmarshal(raw, &problem) == nil {
		if problem.Detail != "" {
			return problem.Detail
		}
		if problem.Title != "" {
			return problem.Title
		}
	}
	return "no detail"
}
