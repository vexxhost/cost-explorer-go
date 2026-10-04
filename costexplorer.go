// Copyright 2026 VEXXHOST, Inc.
// SPDX-License-Identifier: Apache-2.0

// Package costexplorer connects gophercloud to the cost-explorer v1 API.
//
// Authenticate a gophercloud ProviderClient the usual way, wrap it with NewServiceClient, and
// call costreports.Create or costforecasts.Create. Re-authentication after a 401 is gophercloud's
// (set AllowReauth); Retry gives the provider a retry policy for transient failures.
//
//	provider, _ := openstack.NewClient(keystoneURL)
//	provider.HTTPClient = http.Client{CheckRedirect: costexplorer.NoRedirects}
//	provider.RetryFunc = costexplorer.Retry(3)
//	_ = openstack.Authenticate(ctx, provider, gophercloud.AuthOptions{
//		IdentityEndpoint: keystoneURL, AllowReauth: true,
//		ApplicationCredentialID: id, ApplicationCredentialSecret: secret,
//	})
//	client := costexplorer.NewServiceClient(provider, "https://cost-explorer.example.com")
//	report, err := costreports.Create(ctx, client, apiv1.Request{...})
package costexplorer

import (
	"context"
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

// NewServiceClient returns a service client for the cost-explorer deployment at endpoint,
// the URL without its /v1 path, authenticated by provider.
func NewServiceClient(provider *gophercloud.ProviderClient, endpoint string) *gophercloud.ServiceClient {
	return &gophercloud.ServiceClient{
		ProviderClient: provider,
		Endpoint:       strings.TrimRight(endpoint, "/") + "/",
		Type:           "cost-explorer",
	}
}

// NoRedirects is an http.Client CheckRedirect that follows no redirect. Requests carry a
// Keystone token in X-Auth-Token, which Go forwards to a redirect's target, so a client that
// follows redirects can hand the token to another host.
func NoRedirects(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

// StatusError is an answer other than 200, with the problem detail cost-explorer gave.
type StatusError struct {
	StatusCode int
	Detail     string
	err        error
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("cost-explorer answered %d: %s", e.StatusCode, e.Detail)
}

func (e *StatusError) Unwrap() error { return e.err }

// AsStatusError converts gophercloud's unexpected-status errors, including one returned after
// a re-authentication, into a StatusError. Other errors are returned unchanged.
func AsStatusError(err error) error {
	unexpected, ok := unexpectedStatus(err)
	if !ok {
		return err
	}
	return &StatusError{StatusCode: unexpected.Actual, Detail: problemDetail(unexpected.Body), err: err}
}

// unexpectedStatus finds the unexpected-status error gophercloud returns, which it does both
// as a value and, after a re-authentication, as a pointer.
func unexpectedStatus(err error) (gophercloud.ErrUnexpectedResponseCode, bool) {
	var value gophercloud.ErrUnexpectedResponseCode
	if errors.As(err, &value) {
		return value, true
	}
	var pointer *gophercloud.ErrUnexpectedResponseCode
	if errors.As(err, &pointer) && pointer != nil {
		return *pointer, true
	}
	return gophercloud.ErrUnexpectedResponseCode{}, false
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

// Retry returns a gophercloud RetryFunc that tries a request up to attempts more times when
// it could plausibly succeed: a connection failure, 408, 429, or a 5xx other than 501. It
// waits with jittered exponential backoff from one second, at most MaxBackoff, honouring
// Retry-After. Anything else, a refusal such as 400 or 403, fails at once. Assign it to a
// ProviderClient's RetryFunc; leave it unset to fail fast.
func Retry(attempts uint) gophercloud.RetryFunc {
	return func(ctx context.Context, _, _ string, _ *gophercloud.RequestOpts, err error, failCount uint) error {
		if failCount > attempts || !retryable(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(backoff(failCount, err)):
			return nil
		}
	}
}

func retryable(err error) bool {
	if unexpected, ok := unexpectedStatus(err); ok {
		code := unexpected.Actual
		return code == http.StatusRequestTimeout || code == http.StatusTooManyRequests ||
			(code >= 500 && code != http.StatusNotImplemented)
	}
	var netErr net.Error
	return errors.As(err, &netErr) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// MaxBackoff bounds one wait between attempts.
const MaxBackoff = 30 * time.Second

func backoff(failCount uint, err error) time.Duration {
	if unexpected, ok := unexpectedStatus(err); ok {
		if seconds, convErr := strconv.Atoi(unexpected.ResponseHeader.Get("Retry-After")); convErr == nil && seconds >= 0 {
			return min(time.Duration(seconds)*time.Second, MaxBackoff)
		}
	}
	wait := min(time.Second<<min(failCount-1, 5), MaxBackoff)
	return wait/2 + rand.N(wait/2+1)
}
