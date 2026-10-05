// Copyright 2026 VEXXHOST, Inc.
// SPDX-License-Identifier: Apache-2.0

// This file connects the generated client to gophercloud: the provider supplies the token,
// re-authenticates when Keystone revokes it, and finds the endpoint in the service catalog.

package costexplorer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/v2"
)

// ServiceType is cost-explorer's type in the Keystone service catalog.
const ServiceType = "cost-explorer"

// Options say where cost-explorer is and how persistently to call it.
type Options struct {
	// Endpoint overrides the service catalog: the deployment's URL, with or without /v1.
	Endpoint string
	// EndpointOpts select the catalog entry; Type defaults to ServiceType.
	EndpointOpts gophercloud.EndpointOpts
	// Retries is how many more times a request is tried after a connection failure, 408, 429,
	// or a 5xx other than 501. Zero fails at once.
	Retries int
}

// New returns the generated client, authenticated by provider. With AllowReauth set on the
// provider, a token Keystone revokes before it expires is replaced and the request repeated.
func New(provider *gophercloud.ProviderClient, opts Options) (*ClientWithResponses, error) {
	endpoint := opts.Endpoint
	if endpoint == "" {
		if provider.EndpointLocator == nil {
			return nil, errors.New("the provider has no service catalog; set Options.Endpoint")
		}
		eo := opts.EndpointOpts
		eo.ApplyDefaults(ServiceType)
		var err error
		if endpoint, err = provider.EndpointLocator(eo); err != nil {
			return nil, err
		}
	}
	// The schema's paths carry /v1, so the server URL is the root.
	endpoint = strings.TrimSuffix(strings.TrimRight(endpoint, "/"), "/v1")
	httpClient := provider.HTTPClient
	// Requests carry the token in X-Auth-Token, which Go forwards to a redirect's target.
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return NewClientWithResponses(endpoint, WithHTTPClient(&doer{provider: provider, http: httpClient, retries: opts.Retries}))
}

type doer struct {
	provider *gophercloud.ProviderClient
	http     http.Client
	retries  int
}

func (d *doer) Do(req *http.Request) (*http.Response, error) {
	reauthenticated := false
	for attempt := 0; ; attempt++ {
		attemptReq, err := rewind(req, attempt)
		if err != nil {
			return nil, err
		}
		token := d.provider.Token()
		attemptReq.Header.Set("X-Auth-Token", token)
		resp, err := d.http.Do(attemptReq)
		if err == nil && resp.StatusCode == http.StatusUnauthorized && !reauthenticated && d.provider.ReauthFunc != nil {
			drain(resp)
			if err := d.provider.Reauthenticate(req.Context(), token); err != nil {
				return nil, fmt.Errorf("re-authenticating: %w", err)
			}
			reauthenticated = true
			attempt-- // a re-authentication is not a retry
			continue
		}
		if attempt >= d.retries || !retryable(resp, err) || req.Context().Err() != nil {
			return resp, err
		}
		wait := backoff(attempt+1, resp)
		drain(resp)
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(wait):
		}
	}
}

// rewind returns the request for an attempt, with its body read from the start.
func rewind(req *http.Request, attempt int) (*http.Request, error) {
	if attempt == 0 {
		return req, nil
	}
	clone := req.Clone(req.Context())
	if req.Body != nil {
		if req.GetBody == nil {
			return nil, errors.New("request body cannot be sent again")
		}
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		clone.Body = body
	}
	return clone, nil
}

func drain(resp *http.Response) {
	if resp != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
	}
}

func retryable(resp *http.Response, err error) bool {
	if err != nil {
		var netErr net.Error
		return errors.As(err, &netErr) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
	}
	code := resp.StatusCode
	return code == http.StatusRequestTimeout || code == http.StatusTooManyRequests ||
		(code >= 500 && code != http.StatusNotImplemented)
}

// MaxBackoff bounds one wait between attempts.
const MaxBackoff = 30 * time.Second

func backoff(retry int, resp *http.Response) time.Duration {
	if resp != nil {
		if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && seconds >= 0 {
			return min(time.Duration(seconds)*time.Second, MaxBackoff)
		}
	}
	wait := min(time.Second<<min(retry-1, 5), MaxBackoff)
	return wait/2 + rand.N(wait/2+1)
}

// StatusError is an answer other than 200, with the problem detail cost-explorer gave.
type StatusError struct {
	StatusCode int
	Detail     string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("cost-explorer answered %d: %s", e.StatusCode, e.Detail)
}

// Problem returns a *StatusError for an answer other than 200, and nil for a 200.
func Problem(resp *http.Response, body []byte) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	detail := "no detail"
	var problem ErrorModel
	if json.Unmarshal(body, &problem) == nil {
		switch {
		case problem.Detail != nil && *problem.Detail != "":
			detail = *problem.Detail
		case problem.Title != nil && *problem.Title != "":
			detail = *problem.Title
		}
	}
	return &StatusError{StatusCode: resp.StatusCode, Detail: detail}
}
