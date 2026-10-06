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

package simplemqapi

import (
	"encoding/json"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/simplemq/apis/v1/queue"
)

func TestConfigRequestPaths(t *testing.T) {
	visibilityTimeoutSeconds, expireSeconds := 30, 345600
	jsonInput := `{"CommonServiceItem":{"Settings":{"VisibilityTimeoutSeconds":30,"ExpireSeconds":345600}}}`
	decode := func(input string, destination any) error {
		data, err := requestData(input)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, destination)
	}

	flagRequest, err := configRequest(nil, &visibilityTimeoutSeconds, &expireSeconds, decode)
	if err != nil {
		t.Fatal(err)
	}
	jsonRequest, err := configRequest(&jsonInput, nil, nil, decode)
	if err != nil {
		t.Fatal(err)
	}
	if flagRequest.CommonServiceItem.Settings != jsonRequest.CommonServiceItem.Settings {
		t.Fatalf("flag settings %+v and JSON settings %+v differ",
			flagRequest.CommonServiceItem.Settings, jsonRequest.CommonServiceItem.Settings)
	}

	zero := 0
	requestWithZero, err := configRequest(nil, &zero, &expireSeconds, decode)
	if err != nil {
		t.Fatal(err)
	}
	if requestWithZero.CommonServiceItem.Settings.VisibilityTimeoutSeconds != 0 {
		t.Fatalf("explicit zero was not preserved: %+v", requestWithZero.CommonServiceItem.Settings)
	}

	for _, test := range []struct {
		name       string
		input      *string
		visibility *int
		expire     *int
	}{
		{name: "missing both flags"},
		{name: "missing expire flag", visibility: &visibilityTimeoutSeconds},
		{name: "missing visibility flag", expire: &expireSeconds},
		{name: "request with explicit zero flag", input: &jsonInput, visibility: &zero},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := configRequest(test.input, test.visibility, test.expire, decode); err == nil {
				t.Fatal("request succeeded, want input validation error")
			}
		})
	}
}

func TestDecodeCreateRequestAddsSimpleMQProvider(t *testing.T) {
	var request queue.CreateQueueRequest
	if err := decodeCreateRequest(`{"CommonServiceItem":{"Name":"sample-queue"}}`, &request); err != nil {
		t.Fatal(err)
	}
	if request.CommonServiceItem.Provider.Class != queue.CreateQueueRequestCommonServiceItemProviderClassSimplemq {
		t.Fatalf("provider class = %q, want %q",
			request.CommonServiceItem.Provider.Class,
			queue.CreateQueueRequestCommonServiceItemProviderClassSimplemq)
	}
}
