// Copyright 2026 VEXXHOST, Inc.
// SPDX-License-Identifier: Apache-2.0

// Package costreports computes cost reports: POST /v1/cost_reports.
package costreports

import (
	"context"

	"github.com/gophercloud/gophercloud/v2"

	"github.com/vexxhost/cost-explorer-go/apiv1"
	"github.com/vexxhost/cost-explorer-go/internal/post"
)

// Create computes a report. An answer other than 200 is a *costexplorer.StatusError.
func Create(ctx context.Context, client *gophercloud.ServiceClient, r apiv1.Request) (apiv1.Response, error) {
	var out apiv1.Response
	return out, post.JSON(ctx, client, "v1/cost_reports", r, &out)
}
