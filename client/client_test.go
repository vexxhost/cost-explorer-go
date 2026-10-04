// Copyright 2026 VEXXHOST, Inc.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vexxhost/cost-explorer-go/apiv1"
)

func TestReportSendsTheTokenAndKeepsExplicitlyEmptyLists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/cost_reports" || r.Header.Get("X-Auth-Token") != "token" {
			t.Errorf("request %s with token %q", r.URL.Path, r.Header.Get("X-Auth-Token"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if g, ok := body["group_by"].([]any); !ok || len(g) != 0 {
			t.Errorf("group_by [] means totals and must be sent, got %v", body["group_by"])
		}
		filter := body["filter"].(map[string]any)
		if p := filter["project_ids"].([]any); len(p) != 1 || p[0] != "p1" {
			t.Errorf("project_ids = %v", filter["project_ids"])
		}
		if _, ok := filter["regions"]; ok {
			t.Error("a nil filter field must be left out")
		}
		_, _ = w.Write([]byte(`{"as_of":"2026-03-10T00:00:00Z","currency":"USD","results_by_time":[{"total":{"cost":{"amount":"1.5","unit":"USD"}}}]}`))
	}))
	defer server.Close()
	c := &Client{BaseURL: server.URL + "/"}
	out, err := c.Report(context.Background(), "token", apiv1.Request{
		TimePeriod:  apiv1.TimePeriod{Start: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		Granularity: "monthly", GroupBy: []string{}, Metrics: []string{"cost"},
		Filter: apiv1.Filter{ProjectIDs: []string{"p1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Currency != "USD" || out.ResultsByTime[0].Total["cost"].Amount != "1.5" {
		t.Errorf("decoded %+v", out)
	}
}

func TestFailuresCarryTheStatusAndDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"title":"Forbidden","detail":"token scope does not authorize this request"}`))
	}))
	defer server.Close()
	_, err := (&Client{BaseURL: server.URL}).Forecast(context.Background(), "token", apiv1.ForecastRequest{})
	var status *StatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusForbidden || status.Detail != "token scope does not authorize this request" {
		t.Fatalf("err = %v", err)
	}
}

func TestRedirectsAreNotFollowedAndAnswersAreBounded(t *testing.T) {
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the token was carried to a redirect target")
	}))
	defer elsewhere.Close()
	redirecting := httptest.NewServer(http.RedirectHandler(elsewhere.URL, http.StatusTemporaryRedirect))
	defer redirecting.Close()
	_, err := (&Client{BaseURL: redirecting.URL}).Report(context.Background(), "token", apiv1.Request{})
	var status *StatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("err = %v", err)
	}

	large := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 64))
	}))
	defer large.Close()
	_, err = (&Client{BaseURL: large.URL, MaxResponseBytes: 16}).Report(context.Background(), "token", apiv1.Request{})
	if err == nil || !strings.Contains(err.Error(), "exceeds 16 bytes") {
		t.Fatalf("err = %v", err)
	}
}
