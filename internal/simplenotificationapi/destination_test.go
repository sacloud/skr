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

package simplenotificationapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	simplenotification "github.com/sacloud/sacloud-sdk-go/api/simple-notification"
	v1 "github.com/sacloud/sacloud-sdk-go/api/simple-notification/apis/v1"
	clientconfig "github.com/sacloud/skr/internal/saclient"
)

func TestDestinationAdapterProtectsAllSDKErrorsAndPreservesClassification(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "test-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "test-secret")
	const webhookURL = "https://example.invalid/private-webhook-value"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error_code":"not_found","error_msg":"` + webhookURL + `","is_fatal":true}`))
	}))
	t.Cleanup(server.Close)
	client, err := clientconfig.New(false)
	if err != nil {
		t.Fatal(err)
	}
	apiClient, err := simplenotification.NewClientWithAPIRootURL(client, server.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	adapter := destinationAdapter{simplenotification.NewDestinationOp(apiClient)}
	var create v1.PostCommonServiceItemRequest
	if err := json.Unmarshal([]byte(`{"CommonServiceItem":{"Name":"test","Description":"","Tags":[],"Icon":null,"Provider":{"Class":"saknoticedestination"},"ServiceClass":"cloud/saknoticedestination/2","Settings":{"Type":"webhook","Value":"`+webhookURL+`"}}}`), &create); err != nil {
		t.Fatal(err)
	}
	var update v1.PutCommonServiceItemRequest
	if err := json.Unmarshal([]byte(`{"CommonServiceItem":{"Name":"test","Description":"","Tags":[],"Icon":null,"Settings":{"Type":"webhook","Value":"`+webhookURL+`"}}}`), &update); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, test := range []struct {
		name string
		call func() error
	}{
		{"list", func() error { _, err := adapter.List(ctx); return err }},
		{"read", func() error { _, err := adapter.Read(ctx, "123456789012"); return err }},
		{"create", func() error { _, err := adapter.Create(ctx, create); return err }},
		{"update", func() error { _, err := adapter.Update(ctx, "123456789012", update); return err }},
		{"delete", func() error { return adapter.Delete(ctx, "123456789012") }},
		{"read-status", func() error { _, err := adapter.ReadStatus(ctx, "123456789012"); return err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			if err == nil || strings.Contains(err.Error(), webhookURL) {
				t.Fatalf("%s did not return a safe error", test.name)
			}
			var sdkError *simplenotification.Error
			if !errors.As(err, &sdkError) {
				t.Fatalf("SDK error classification lost: %T", err)
			}
			var statusError *v1.ErrorStatusCode
			if !errors.As(err, &statusError) || statusError.StatusCode != http.StatusNotFound {
				t.Fatalf("API status classification lost: %v", err)
			}
		})
	}
}

func TestDestinationRequestProtectsDecodeErrorsAndPreservesCause(t *testing.T) {
	cause := errors.New("invalid value: https://example.invalid/private")
	err := decodeDestinationRequest("@request.json", &v1.PutCommonServiceItemRequest{}, func(string, any) error {
		return cause
	}, v1.CommonServiceItemProviderClassSaknoticedestination, v1.CommonServiceItemServiceClassCloudSaknoticedestination2)
	if err == nil || strings.Contains(err.Error(), "https://") || !errors.Is(err, cause) {
		t.Fatal("decode error is not protected or its cause was lost")
	}
}
