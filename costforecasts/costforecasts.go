// Copyright 2026 VEXXHOST, Inc.
// SPDX-License-Identifier: Apache-2.0

// Package costforecasts computes cost forecasts: POST /v1/cost_forecasts.
package costforecasts

import (
	"context"

	"github.com/gophercloud/gophercloud/v2"

	"github.com/vexxhost/cost-explorer-go/apiv1"
	"github.com/vexxhost/cost-explorer-go/internal/post"
)

// Create computes a forecast. An answer other than 200 is a *costexplorer.StatusError.
func Create(ctx context.Context, client *gophercloud.ServiceClient, r apiv1.ForecastRequest) (apiv1.ForecastResponse, error) {
	var out apiv1.ForecastResponse
	return out, post.JSON(ctx, client, "v1/cost_forecasts", r, &out)
}
