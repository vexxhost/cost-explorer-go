// Copyright 2026 VEXXHOST, Inc.
// SPDX-License-Identifier: Apache-2.0

package apiv1

import (
	"encoding/json"
	"strings"
	"testing"
)

// A nil list imposes no restriction and is left out; an empty one keeps its meaning and is
// sent. Losing the difference turns "matches nothing" into "everything".
func TestEmptyAndAbsentListsStayDifferent(t *testing.T) {
	encode := func(r Request) string {
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	absent := encode(Request{Granularity: "monthly"})
	empty := encode(Request{Granularity: "monthly", GroupBy: []string{}, Filter: Filter{ProjectIDs: []string{}}})
	for _, want := range []string{`"group_by":[]`, `"project_ids":[]`} {
		if !strings.Contains(empty, want) {
			t.Errorf("%s lacks %s", empty, want)
		}
		if strings.Contains(absent, want[:len(want)-3]) {
			t.Errorf("%s should leave out %s", absent, want[:len(want)-3])
		}
	}

	var decoded Request
	if err := json.Unmarshal([]byte(`{"filter":{"project_ids":[]}}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Filter.ProjectIDs == nil || decoded.Filter.Regions != nil {
		t.Errorf("decoded %+v", decoded.Filter)
	}
}
