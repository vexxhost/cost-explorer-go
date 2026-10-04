# cost-explorer-go

Go types and a [gophercloud](https://github.com/gophercloud/gophercloud) client for the
[cost-explorer](https://github.com/vexxhost/cost-explorer) v1 API: `POST /v1/cost_reports` and
`POST /v1/cost_forecasts`, answered by a regional deployment or by the aggregator that covers every
region.

```sh
go get github.com/vexxhost/cost-explorer-go@latest
```

```go
import (
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"

	costexplorer "github.com/vexxhost/cost-explorer-go"
	"github.com/vexxhost/cost-explorer-go/apiv1"
	"github.com/vexxhost/cost-explorer-go/costreports"
)

provider, err := openstack.NewClient(keystoneURL)
provider.HTTPClient = http.Client{CheckRedirect: costexplorer.NoRedirects}
provider.RetryFunc = costexplorer.Retry(3)
err = openstack.Authenticate(ctx, provider, gophercloud.AuthOptions{
	IdentityEndpoint:            keystoneURL,
	ApplicationCredentialID:     id,
	ApplicationCredentialSecret: secret,
	AllowReauth:                 true,
})

client := costexplorer.NewServiceClient(provider, "https://cost-explorer.example.com")
report, err := costreports.Create(ctx, client, apiv1.Request{
	TimePeriod:  apiv1.TimePeriod{Start: start, End: end},
	Granularity: "monthly",
	Metrics:     []string{"cost"},
	GroupBy:     []string{}, // totals only
	Filter:      apiv1.Filter{ProjectIDs: []string{projectID}},
})
```

| package | what it is |
| --- | --- |
| `apiv1` | The wire format. cost-explorer's own server uses these types, so the two cannot drift. It imports only the standard library. |
| `costreports`, `costforecasts` | `Create` for each endpoint. An answer other than 200 is a `*costexplorer.StatusError` carrying the problem detail. |
| `costexplorer` | `NewServiceClient`, the `Retry` policy and `NoRedirects`. |

Authentication and re-authentication are gophercloud's: with `AllowReauth`, a token Keystone revokes
before it expires is replaced and the request repeated. `Retry(n)` is a gophercloud `RetryFunc` that
repeats a request up to `n` times on a connection failure, 408, 429 or a 5xx other than 501, with
jittered exponential backoff that honours `Retry-After`; refusals such as 400 and 403 fail at once.
Leave `RetryFunc` unset to fail fast, as the aggregator does when it forwards a caller's token.

Set `NoRedirects` on the provider's HTTP client. Requests carry the token in `X-Auth-Token`, which Go
forwards to a redirect's target.

Amounts are decimal strings. Optional lists are tagged `omitzero`: a nil list imposes no restriction
and is left out, and an explicitly empty one is sent as `[]` and keeps its meaning, a filter that
matches nothing or a `group_by` that asks for totals.

## License

Copyright 2026 VEXXHOST, Inc. Licensed under the [Apache License, Version 2.0](LICENSE).
