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
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
)

func TestSDKAPIFindUsesLicenseInfoOperation(t *testing.T) {
	caller := &apiCaller{response: []byte(`{"Total":1,"From":0,"Count":1,"LicenseInfo":[{"ID":"42","Name":"license","TermsOfUse":"terms"}]}`)}
	api := NewSDKAPI(caller)
	result, err := api.FindWithContext(context.Background(), &FindRequest{
		Count: 1, Filter: map[string]any{"Name": "license"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Count != 1 || len(result.LicenseInfo) != 1 ||
		result.LicenseInfo[0].ID != 42 || result.LicenseInfo[0].TermsOfUse != "terms" {
		t.Fatalf("Find result = %+v", result)
	}
	if len(caller.calls) != 1 || !strings.HasSuffix(caller.calls[0].uri, "/api/cloud/1.1/product/license") {
		t.Fatalf("API caller requests = %+v", caller.calls)
	}
	var body struct {
		Count  int
		Filter map[string]json.RawMessage
	}
	if err := json.Unmarshal(caller.calls[0].body, &body); err != nil {
		t.Fatal(err)
	}
	if body.Count != 1 || string(body.Filter["Name"]) != `"license"` {
		t.Fatalf("Find request body = %s", caller.calls[0].body)
	}
}

func TestSDKAPIFindPropagatesAPIError(t *testing.T) {
	api := NewSDKAPI(&apiCaller{err: errors.New("request failed")})
	_, err := api.FindWithContext(context.Background(), &FindRequest{})
	if err == nil || !strings.Contains(err.Error(), "request failed") {
		t.Fatalf("Find error = %v, want API error", err)
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
