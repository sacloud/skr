// Copyright 2022-2026 The sacloud/skr Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package licenseinfoapi

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
)

func TestFindRequestUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
		check     func(*testing.T, *FindRequest)
	}{
		{
			name:  "preserves large numeric scalar and array",
			input: `{"Filter":{"ID":1234567890123456789,"IDs":[1234567890123456789,1234567890123456790]}}`,
			check: func(t *testing.T, request *FindRequest) {
				t.Helper()
				if got := request.Filter["ID"]; got != json.Number("1234567890123456789") {
					t.Errorf("Filter[ID] = %#v, want exact json.Number", got)
				}
				values, ok := request.Filter["IDs"].([]any)
				if !ok || len(values) != 2 {
					t.Fatalf("Filter[IDs] = %#v, want two values", request.Filter["IDs"])
				}
				for i, want := range []json.Number{"1234567890123456789", "1234567890123456790"} {
					if got := values[i]; got != want {
						t.Errorf("Filter[IDs][%d] = %#v, want %s", i, got, want)
					}
				}
			},
		},
		{name: "omitted filter", input: `{}`, check: func(t *testing.T, request *FindRequest) {
			if request.Filter != nil {
				t.Errorf("Filter = %#v, want nil", request.Filter)
			}
		}},
		{name: "null filter", input: `{"Filter":null}`, check: func(t *testing.T, request *FindRequest) {
			if request.Filter != nil {
				t.Errorf("Filter = %#v, want nil", request.Filter)
			}
		}},
		{name: "allows unknown fields", input: `{"Unknown":"ignored"}`},
		{name: "rejects string filter", input: `{"Filter":"invalid"}`, wantError: true},
		{name: "rejects array filter", input: `{"Filter":["invalid"]}`, wantError: true},
		{name: "rejects numeric filter", input: `{"Filter":123}`, wantError: true},
		{name: "rejects trailing JSON", input: `{} {}`, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var request FindRequest
			err := json.Unmarshal([]byte(test.input), &request)
			if (err != nil) != test.wantError {
				t.Fatalf("json.Unmarshal() error = %v, wantError %v", err, test.wantError)
			}
			if err == nil && test.check != nil {
				test.check(t, &request)
			}
		})
	}
}

func TestFindRequestFindCondition(t *testing.T) {
	var request FindRequest
	if err := json.Unmarshal([]byte(`{
		"Count":50,
		"From":10,
		"Sort":["Name","-CreatedAt"],
		"Filter":{"Name":"linux","IDs":[1234567890123456789,2],"CPU>=":1234567890123456789},
		"Include":["ID","Name"],
		"Exclude":["Description"]
	}`), &request); err != nil {
		t.Fatal(err)
	}

	want := &iaas.FindCondition{
		Count:   50,
		From:    10,
		Include: []string{"ID", "Name"},
		Exclude: []string{"Description"},
		Sort: search.SortKeys{
			search.SortKeyAsc("Name"),
			search.SortKeyDesc("CreatedAt"),
		},
		Filter: search.Filter{
			search.Key("Name"): search.AndEqual("linux"),
			search.Key("IDs"):  search.OrEqual("1234567890123456789", "2"),
			search.KeyWithOp("CPU", search.OpGreaterEqual): "1234567890123456789",
		},
	}
	if got := request.findCondition(); !reflect.DeepEqual(got, want) {
		t.Errorf("findCondition() = %#v, want %#v", got, want)
	}
}
