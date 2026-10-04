# cost-explorer-go

Go types and a client for the [cost-explorer](https://github.com/vexxhost/cost-explorer) v1 API:
`POST /v1/cost_reports` and `POST /v1/cost_forecasts`, answered by a regional deployment or by the
aggregator that covers every region.

```sh
go get github.com/vexxhost/cost-explorer-go@latest
```

```go
import (
	"github.com/vexxhost/cost-explorer-go/apiv1"
	"github.com/vexxhost/cost-explorer-go/client"
)

c := &client.Client{BaseURL: "https://cost-explorer.example.com"}
report, err := c.Report(ctx, token, apiv1.Request{
	TimePeriod:  apiv1.TimePeriod{Start: start, End: end},
	Granularity: "monthly",
	Metrics:     []string{"cost"},
	GroupBy:     []string{}, // totals only
	Filter:      apiv1.Filter{ProjectIDs: []string{projectID}},
})
```

`apiv1` is the wire format, and cost-explorer's own server uses these types, so the two cannot
drift. Amounts are decimal strings. Optional lists are tagged `omitzero`: a nil list imposes no
restriction and is left out, and an explicitly empty one is sent as `[]` and keeps its meaning, a
filter that matches nothing or a `group_by` that asks for totals.

`client` sends one request and decodes one answer. Any status other than 200 is a
`*client.StatusError` carrying the problem detail. It follows no redirects by default, because the
request carries a Keystone token. Retries and how the token is obtained are the caller's: pass an
`http.Client` that retries, and a token you validated or minted.

The module depends only on the Go standard library.

## License

Copyright 2026 VEXXHOST, Inc. Licensed under the [Apache License, Version 2.0](LICENSE).
