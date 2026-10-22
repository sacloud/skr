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

package diskplanapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
)

func TestSDKAPIFindUsesLowLevelOperationAndPreservesResponse(t *testing.T) {
	caller := &apiCaller{response: []byte(`{"Total":2,"From":1,"Count":1,"DiskPlans":[{"ID":"4","Name":"ssd"}]}`)}
	api := NewSDKAPI(caller)

	result, err := api.FindWithContext(context.Background(), &FindRequest{
		Zone:   "test-zone",
		Filter: map[string]any{"Name": "ssd"},
		Count:  1,
		From:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || result.From != 1 || result.Count != 1 ||
		len(result.DiskPlans) != 1 || result.DiskPlans[0].Name != "ssd" {
		t.Fatalf("Find result = %+v", result)
	}
	if len(caller.calls) != 1 || !strings.HasSuffix(caller.calls[0].uri, "/api/cloud/1.1/product/disk") {
		t.Fatalf("API caller requests = %+v", caller.calls)
	}
	var condition struct {
		Count  int
		From   int
		Filter map[string]any
	}
	if err := json.Unmarshal(caller.calls[0].body, &condition); err != nil {
		t.Fatal(err)
	}
	if condition.Count != 1 || condition.From != 1 || condition.Filter["Name"] == nil {
		t.Fatalf("Find condition = %+v", condition)
	}
}

func TestSDKAPIReadRejectsInvalidRequestBeforeCallingAPI(t *testing.T) {
	caller := &apiCaller{}
	api := NewSDKAPI(caller)
	_, err := api.ReadWithContext(context.Background(), &ReadRequest{Zone: "test-zone"})
	if err == nil {
		t.Fatal("Read accepted a missing ID")
	}
	if len(caller.calls) != 0 {
		t.Fatalf("invalid Read reached API caller: %+v", caller.calls)
	}
}

type apiCaller struct {
	calls    []apiCall
	response []byte
	err      error
}

type apiCall struct {
	method string
	uri    string
	body   []byte
}

func (caller *apiCaller) Do(_ context.Context, method, uri string, body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	caller.calls = append(caller.calls, apiCall{method: method, uri: uri, body: data})
	if caller.err != nil {
		return nil, caller.err
	}
	if caller.response == nil {
		return nil, errors.New("unexpected API call")
	}
	return caller.response, nil
}

var _ iaas.APICaller = (*apiCaller)(nil)
