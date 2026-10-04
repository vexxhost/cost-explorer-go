// Copyright 2026 VEXXHOST, Inc.
// SPDX-License-Identifier: Apache-2.0

package costexplorer_test

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

	costexplorer "github.com/vexxhost/cost-explorer-go"
	"github.com/vexxhost/cost-explorer-go/apiv1"
	"github.com/vexxhost/cost-explorer-go/costforecasts"
	"github.com/vexxhost/cost-explorer-go/costreports"
)

const report = `{"as_of":"2026-03-10T00:00:00Z","currency":"USD","results_by_time":[{"total":{"cost":{"amount":"1.5","unit":"USD"}}}]}`

func provider(token string) *gophercloud.ProviderClient {
	p := &gophercloud.ProviderClient{TokenID: token}
	p.HTTPClient = http.Client{CheckRedirect: costexplorer.NoRedirects}
	return p
}

var request = apiv1.Request{
	TimePeriod:  apiv1.TimePeriod{Start: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
	Granularity: "monthly", GroupBy: []string{}, Metrics: []string{"cost"},
	Filter: apiv1.Filter{ProjectIDs: []string{"p1"}},
}

func TestReportSendsTheTokenAndKeepsExplicitlyEmptyLists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/cost_reports" || r.Header.Get("X-Auth-Token") != "token" {
			t.Errorf("request %s with token %q", r.URL.Path, r.Header.Get("X-Auth-Token"))
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if g, ok := body["group_by"].([]any); !ok || len(g) != 0 {
			t.Errorf("group_by [] means totals and must be sent, got %v", body["group_by"])
		}
		if _, ok := body["filter"].(map[string]any)["regions"]; ok {
			t.Error("a nil filter field must be left out")
		}
		_, _ = w.Write([]byte(report))
	}))
	defer server.Close()
	out, err := costreports.Create(context.Background(), costexplorer.NewServiceClient(provider("token"), server.URL+"/"), request)
	if err != nil {
		t.Fatal(err)
	}
	if out.Currency != "USD" || out.ResultsByTime[0].Total["cost"].Amount != "1.5" {
		t.Errorf("decoded %+v", out)
	}
}

func TestRefusalsCarryTheDetailAndAreNotRetried(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"title":"Forbidden","detail":"token scope does not authorize this request"}`))
	}))
	defer server.Close()
	p := provider("token")
	p.RetryFunc = costexplorer.Retry(3)
	_, err := costforecasts.Create(context.Background(), costexplorer.NewServiceClient(p, server.URL), apiv1.ForecastRequest{})
	var status *costexplorer.StatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusForbidden || status.Detail != "token scope does not authorize this request" {
		t.Fatalf("err = %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("a refusal was retried: %d calls", calls.Load())
	}
}

func TestTransientFailuresAreRetried(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.WriteHeader(http.StatusServiceUnavailable)
		case 2:
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			_, _ = w.Write([]byte(report))
		}
	}))
	defer server.Close()
	p := provider("token")
	p.RetryFunc = costexplorer.Retry(3)
	if _, err := costreports.Create(context.Background(), costexplorer.NewServiceClient(p, server.URL), request); err != nil || calls.Load() != 3 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}

	calls.Store(0)
	p.RetryFunc = costexplorer.Retry(0)
	_, err := costreports.Create(context.Background(), costexplorer.NewServiceClient(p, server.URL), request)
	var status *costexplorer.StatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusServiceUnavailable || calls.Load() != 1 {
		t.Fatalf("no retries left: err=%v calls=%d", err, calls.Load())
	}
}

func TestGophercloudReauthenticatesAfterA401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Auth-Token") != "fresh" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(report))
	}))
	defer server.Close()
	p := provider("revoked")
	p.UseTokenLock()
	p.ReauthFunc = func(context.Context) error { p.SetToken("fresh"); return nil }
	if _, err := costreports.Create(context.Background(), costexplorer.NewServiceClient(p, server.URL), request); err != nil {
		t.Fatal(err)
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the token was carried to a redirect target")
	}))
	defer elsewhere.Close()
	redirecting := httptest.NewServer(http.RedirectHandler(elsewhere.URL, http.StatusTemporaryRedirect))
	defer redirecting.Close()
	_, err := costreports.Create(context.Background(), costexplorer.NewServiceClient(provider("token"), redirecting.URL), request)
	var status *costexplorer.StatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("err = %v", err)
	}
}
