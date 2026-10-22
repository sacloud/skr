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

package privatehostplanapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
)

func TestSDKAPIFindUsesPrivateHostPlanOperation(t *testing.T) {
	caller := &apiCaller{response: []byte(`{"Total":1,"From":0,"Count":1,"PrivateHostPlans":[{"ID":"8","Name":"private-host","Class":"standard","CPU":20,"CPUModel":"model","Dedicated":true,"MemoryMB":229376,"Availability":"available"}]}`)}
	api := NewSDKAPI(caller)
	result, err := api.FindWithContext(context.Background(), &FindRequest{
		Zone: "is1b", Count: 1, Filter: map[string]any{"Name": "private-host"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Count != 1 || len(result.PrivateHostPlans) != 1 ||
		result.PrivateHostPlans[0].ID != 8 || result.PrivateHostPlans[0].MemoryMB != 229376 {
		t.Fatalf("Find result = %+v", result)
	}
	if len(caller.calls) != 1 || !strings.HasSuffix(caller.calls[0].uri, "/api/cloud/1.1/product/privatehost") {
		t.Fatalf("API caller requests = %+v", caller.calls)
	}
	var body struct {
		Count  int
		Filter map[string]json.RawMessage
	}
	if err := json.Unmarshal(caller.calls[0].body, &body); err != nil {
		t.Fatal(err)
	}
	if body.Count != 1 || string(body.Filter["Name"]) != `"private-host"` {
		t.Fatalf("Find request body = %s", caller.calls[0].body)
	}
}

func TestSDKAPIReadAndRejectsMissingID(t *testing.T) {
	caller := &apiCaller{response: []byte(`{"PrivateHostPlan":{"ID":"8","Name":"private-host","Class":"standard","CPU":20,"MemoryMB":229376}}`)}
	api := NewSDKAPI(caller)
	plan, err := api.ReadWithContext(context.Background(), &ReadRequest{Zone: "is1b", ID: 8})
	if err != nil {
		t.Fatal(err)
	}
	if plan.ID != 8 || plan.Name != "private-host" || plan.MemoryMB != 229376 {
		t.Fatalf("Read result = %+v", plan)
	}
	if len(caller.calls) != 1 || !strings.HasSuffix(caller.calls[0].uri, "/api/cloud/1.1/product/privatehost/8") {
		t.Fatalf("API caller requests = %+v", caller.calls)
	}

	caller.calls = nil
	if _, err := api.ReadWithContext(context.Background(), &ReadRequest{Zone: "is1b"}); err == nil {
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
