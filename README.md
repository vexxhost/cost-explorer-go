# cost-explorer-go

A Go client for the [cost-explorer](https://github.com/vexxhost/cost-explorer) v1 API, generated
with [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) from the schema cost-explorer
serves at `/openapi-3.0.yaml`, and authenticated through
[gophercloud](https://github.com/gophercloud/gophercloud).

```sh
go get github.com/vexxhost/cost-explorer-go@latest
```

```go
import (
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/utils/v2/openstack/clientconfig"

	costexplorer "github.com/vexxhost/cost-explorer-go"
)

// Credentials from clouds.yaml or OS_* variables, as the openstack CLI reads them.
provider, err := clientconfig.AuthenticatedClient(ctx, &clientconfig.ClientOpts{Cloud: "mycloud"})

client, err := costexplorer.New(provider, costexplorer.Options{
	EndpointOpts: gophercloud.EndpointOpts{Region: "ca-east-1"}, // found in the catalog
	Retries:      2,
})

out, err := client.CreateCostReportWithResponse(ctx, costexplorer.Request{
	TimePeriod:  costexplorer.TimePeriod{Start: start, End: end},
	Granularity: costexplorer.RequestGranularityMonthly,
	Metrics:     &[]costexplorer.RequestMetrics{costexplorer.RequestMetricsCost},
	GroupBy:     &[]costexplorer.RequestGroupBy{}, // totals only
	Filter:      &costexplorer.Filter{ProjectIds: &[]string{projectID}},
})
if err == nil {
	err = costexplorer.Problem(out.HTTPResponse, out.Body) // nil for a 200
}
report := out.JSON200
```

`New` finds the `cost-explorer` endpoint in the provider's service catalog, or uses
`Options.Endpoint`. Each request carries the provider's token. With `AllowReauth` on the provider,
a token Keystone revokes before it expires is replaced and the request repeated. `Retries` repeats
a request after a connection failure, 408, 429 or a 5xx other than 501, with jittered backoff that
honours `Retry-After`. Redirects are never followed, because Go would forward `X-Auth-Token` to the
redirect's target.

Optional lists are pointers: nil is left out and imposes no restriction, while a pointer to an
empty list is sent as `[]` and keeps its meaning, a filter that matches nothing or a `group_by`
that asks for totals. Amounts are decimal strings.

## Regenerating

`costexplorer.gen.go` is generated and committed; the schema is not. `SpecVersion` names the
cost-explorer build it came from.

```sh
make generate SPEC_URL=https://cost-explorer.example.com/openapi-3.0.yaml
```

The `regenerate` workflow does this daily against the repository variable `SPEC_URL` and opens a
pull request when the served schema changed. Everything outside `*.gen.go` is hand-written:
`gophercloud.go` and its tests.

## License

Copyright 2026 VEXXHOST, Inc. Licensed under the [Apache License, Version 2.0](LICENSE).
