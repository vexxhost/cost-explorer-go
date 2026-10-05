// Copyright 2026 VEXXHOST, Inc.
// SPDX-License-Identifier: Apache-2.0

package costexplorer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/v2"
)

const report = `{"as_of":"2026-03-10T00:00:00Z","currency":"USD","pricing_version":"v1","group_definitions":[],
	"results_by_time":[{"time_period":{"start":"2026-03-01T00:00:00Z","end":"2026-04-01T00:00:00Z"},"estimated":true,
	"total":{"cost":{"amount":"1.5","unit":"USD"}},"groups":[]}]}`

func request() Request {
	return Request{
		TimePeriod:  TimePeriod{Start: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		Granularity: RequestGranularityMonthly,
		GroupBy:     &[]RequestGroupBy{},
		Metrics:     &[]RequestMetrics{RequestMetricsCost},
		Filter:      &Filter{ProjectIds: &[]string{"p1"}, Regions: &[]string{}},
	}
}

func client(t *testing.T, p *gophercloud.ProviderClient, url string, retries int) *ClientWithResponses {
	t.Helper()
	c, err := New(p, Options{Endpoint: url, Retries: retries})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestReportUsesTheCatalogTheTokenAndExplicitlyEmptyLists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/cost_reports" || r.Header.Get("X-Auth-Token") != "token" {
			t.Errorf("request %s with token %q", r.URL.Path, r.Header.Get("X-Auth-Token"))
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if g, ok := body["group_by"].([]any); !ok || len(g) != 0 {
			t.Errorf("group_by [] means totals and must be sent, got %v", body["group_by"])
		}
		filter := body["filter"].(map[string]any)
		if r, ok := filter["regions"].([]any); !ok || len(r) != 0 {
			t.Errorf("regions [] matches nothing and must be sent, got %v", filter["regions"])
		}
		if _, ok := filter["services"]; ok {
			t.Error("a nil filter field must be left out")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(report))
	}))
	defer server.Close()
	var asked gophercloud.EndpointOpts
	p := &gophercloud.ProviderClient{TokenID: "token", EndpointLocator: func(eo gophercloud.EndpointOpts) (string, error) {
		asked = eo
		return server.URL + "/v1/", nil
	}}
	c, err := New(p, Options{EndpointOpts: gophercloud.EndpointOpts{Region: "ca-east-1"}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.CreateCostReportWithResponse(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	if asked.Type != ServiceType || asked.Region != "ca-east-1" {
		t.Errorf("catalog asked for %+v", asked)
	}
	if out.JSON200 == nil || out.JSON200.Currency != "USD" || out.JSON200.ResultsByTime == nil || (*out.JSON200.ResultsByTime)[0].Total["cost"].Amount != "1.5" {
		t.Fatalf("decoded %+v", out)
	}
}

func TestTransientFailuresAreRetriedAndRefusalsAreNot(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.WriteHeader(http.StatusServiceUnavailable)
		case 2:
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
		case 3:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(report))
		default:
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"title":"Forbidden","detail":"token scope does not authorize this request"}`))
		}
	}))
	defer server.Close()
	c := client(t, &gophercloud.ProviderClient{TokenID: "token"}, server.URL, 3)
	out, err := c.CreateCostReportWithResponse(context.Background(), request())
	if err != nil || out.JSON200 == nil || calls.Load() != 3 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}

	out, err = c.CreateCostReportWithResponse(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	var status *StatusError
	if err := Problem(out.HTTPResponse, out.Body); !errors.As(err, &status) || status.StatusCode != http.StatusForbidden ||
		status.Detail != "token scope does not authorize this request" || calls.Load() != 4 {
		t.Fatalf("refusal: %v after %d calls", err, calls.Load())
	}
}

func TestARevokedTokenIsReplaced(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Auth-Token") != "fresh" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(report))
	}))
	defer server.Close()
	p := &gophercloud.ProviderClient{TokenID: "revoked"}
	p.UseTokenLock()
	p.ReauthFunc = func(context.Context) error { p.SetToken("fresh"); return nil }
	out, err := client(t, p, server.URL, 0).CreateCostReportWithResponse(context.Background(), request())
	if err != nil || out.JSON200 == nil {
		t.Fatalf("err=%v status=%d", err, out.StatusCode())
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the token was carried to a redirect target")
	}))
	defer elsewhere.Close()
	redirecting := httptest.NewServer(http.RedirectHandler(elsewhere.URL, http.StatusTemporaryRedirect))
	defer redirecting.Close()
	out, err := client(t, &gophercloud.ProviderClient{TokenID: "token"}, redirecting.URL, 0).CreateCostReportWithResponse(context.Background(), request())
	if err != nil || out.StatusCode() != http.StatusTemporaryRedirect {
		t.Fatalf("err=%v status=%v", err, out)
	}
}

func TestNoCatalogNeedsAnEndpoint(t *testing.T) {
	if _, err := New(&gophercloud.ProviderClient{}, Options{}); err == nil {
		t.Fatal("a provider without a catalog and no endpoint produced a client")
	}
}
