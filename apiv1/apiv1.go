// Copyright 2026 VEXXHOST, Inc.
// SPDX-License-Identifier: Apache-2.0

// Package apiv1 is the cost-explorer v1 API's wire format: the request and response bodies of
// POST /v1/cost_reports and POST /v1/cost_forecasts. The server, the aggregator and other Go
// services that call cost-explorer all use these types, so a field changed here is changed for
// every one of them at once.
//
// Amounts are decimal strings. Optional lists use omitzero rather than omitempty: an absent list
// (nil) imposes no restriction and is left out, while an explicitly empty one is sent as [] and
// keeps its meaning. For a filter that meaning is "matches nothing"; for group_by it is "totals
// only".
package apiv1

import "time"

// TimePeriod is a half-open interval [Start, End).
type TimePeriod struct {
	Start time.Time `json:"start" doc:"Inclusive UTC interval start (RFC3339)."`
	End   time.Time `json:"end" doc:"Exclusive interval end (RFC3339)."`
}

// Filter narrows a report. Values within a field are ORed and fields are ANDed; a nil field
// imposes no restriction and an empty one matches nothing.
type Filter struct {
	ProjectIDs []string `json:"project_ids,omitzero" maxItems:"1000"`
	Regions    []string `json:"regions,omitzero" maxItems:"1000"`
	Services   []string `json:"services,omitzero" maxItems:"1000" enum:"compute,volume,image"`
	UsageTypes []string `json:"usage_types,omitzero" maxItems:"1000"`
}

// Request is the body of POST /v1/cost_reports.
type Request struct {
	TimePeriod  TimePeriod `json:"time_period"`
	Granularity string     `json:"granularity" enum:"hourly,daily,monthly"`
	Filter      Filter     `json:"filter,omitempty"`
	// GroupBy nil groups by service and usage type; empty asks for totals only.
	GroupBy []string   `json:"group_by,omitzero" maxItems:"5" uniqueItems:"true" enum:"project_id,region,service,usage_type,resource_id"`
	Metrics []string   `json:"metrics,omitzero" minItems:"1" maxItems:"2" uniqueItems:"true" enum:"cost,usage_quantity"`
	AsOf    *time.Time `json:"as_of,omitempty" doc:"Optional reporting cutoff; defaults to the current time and cannot be in the future."`
	// AllowPartial is read by the aggregator only: a regional deployment serves one
	// region, so there is nothing for it to leave out.
	AllowPartial bool `json:"allow_partial,omitempty" doc:"Aggregator only: return the regions that answered, listing the rest in missing_regions, instead of failing the report."`
}

// Metric is an amount and its unit.
type Metric struct {
	Amount string `json:"amount"`
	Unit   string `json:"unit"`
}

// Group is one group's metrics, with its keys in grouping order.
type Group struct {
	Keys    []string          `json:"keys"`
	Metrics map[string]Metric `json:"metrics"`
}

// ResultByTime is one time bucket of a report.
type ResultByTime struct {
	TimePeriod  TimePeriod        `json:"time_period"`
	Estimated   bool              `json:"estimated"`
	Total       map[string]Metric `json:"total"`
	UsageByUnit map[string]Metric `json:"usage_by_unit,omitempty"`
	Groups      []Group           `json:"groups"`
}

// Response is the body POST /v1/cost_reports answers with.
type Response struct {
	AsOf             time.Time      `json:"as_of"`
	Currency         string         `json:"currency"`
	PricingVersion   string         `json:"pricing_version"`
	GroupDefinitions []string       `json:"group_definitions"`
	ResultsByTime    []ResultByTime `json:"results_by_time"`
	Regions          []RegionStatus `json:"regions,omitempty" doc:"The regions this report covers and how far each one's data is complete."`
	MissingRegions   []string       `json:"missing_regions,omitempty" doc:"Regions left out of an allow_partial report because they did not answer."`
}

// RegionStatus says how far a region's data is complete. A report served from the
// reconciled store is complete through the region's last completed sync, which can
// be earlier than as_of; a report read live from the databases is complete to as_of.
type RegionStatus struct {
	Name        string    `json:"name"`
	DataThrough time.Time `json:"data_through"`
}

// ForecastRequest is the body of POST /v1/cost_forecasts.
type ForecastRequest struct {
	Metric       string     `json:"metric,omitempty" enum:"cost"`
	TimePeriod   TimePeriod `json:"time_period"`
	Granularity  string     `json:"granularity" enum:"daily,monthly"`
	Filter       Filter     `json:"filter,omitempty"`
	LookbackDays int        `json:"lookback_days,omitempty" minimum:"1" maximum:"90" default:"7"`
	AllowPartial bool       `json:"allow_partial,omitempty" doc:"Aggregator only: forecast from the regions that answered, listing the rest in missing_regions."`
}

// ForecastResult is one forecast bucket.
type ForecastResult struct {
	TimePeriod TimePeriod `json:"time_period"`
	MeanValue  string     `json:"mean_value"`
}

// ForecastResponse is the body POST /v1/cost_forecasts answers with.
type ForecastResponse struct {
	AsOf                  time.Time        `json:"as_of"`
	Model                 string           `json:"model"`
	Estimated             bool             `json:"estimated"`
	HistoryPeriod         TimePeriod       `json:"history_period"`
	Total                 Metric           `json:"total"`
	ForecastResultsByTime []ForecastResult `json:"forecast_results_by_time"`
	Regions               []RegionStatus   `json:"regions,omitempty" doc:"The regions the history covers and how far each one's data is complete."`
	MissingRegions        []string         `json:"missing_regions,omitempty" doc:"Regions left out of an allow_partial forecast because they did not answer."`
}
