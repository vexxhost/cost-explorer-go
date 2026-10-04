// Copyright 2026 VEXXHOST, Inc.
// SPDX-License-Identifier: Apache-2.0

// Package post sends one JSON request through a gophercloud service client and decodes a
// bounded answer.
package post

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gophercloud/gophercloud/v2"

	costexplorer "github.com/vexxhost/cost-explorer-go"
)

// MaxResponseBytes bounds an answer. A report at cost-explorer's max_groups is tens of MB.
const MaxResponseBytes = 256 << 20

// JSON posts in to the service's path and decodes the answer into out.
func JSON(ctx context.Context, client *gophercloud.ServiceClient, path string, in, out any) error {
	resp, err := client.Post(ctx, client.ServiceURL(path), in, nil, &gophercloud.RequestOpts{
		OkCodes:          []int{http.StatusOK},
		KeepResponseBody: true,
	})
	if err != nil {
		return costexplorer.AsStatusError(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > MaxResponseBytes {
		return fmt.Errorf("response exceeds %d bytes", MaxResponseBytes)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
