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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alecthomas/kong"
	simplenotification "github.com/sacloud/sacloud-sdk-go/api/simple-notification"
	v1 "github.com/sacloud/sacloud-sdk-go/api/simple-notification/apis/v1"
	clientconfig "github.com/sacloud/skr/internal/saclient"
	"github.com/stretchr/testify/require"
)

func TestCommonServiceItemCommandsUseSDKRequestAndResponse(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "test-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "test-secret")

	var requests []string
	var createBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}

		requests = append(requests, request.Method+" "+request.URL.Path)
		switch request.Method + " " + request.URL.Path {
		case "POST /api/cloud/1.1/commonserviceitem":
			if err := json.Unmarshal(body, &createBody); err != nil {
				t.Errorf("decode create request: %v", err)
				http.Error(writer, "invalid request", http.StatusBadRequest)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprint(writer, `{"CommonServiceItem":{"ID":"123456789013","Name":"test-group","Description":"","Settings":{"Destinations":["123456789012"]},"Provider":{"Class":"saknoticegroup"},"ServiceClass":"cloud/saknoticegroup/2","Icon":null,"Tags":[],"CreatedAt":"2026-10-20T00:00:00Z","ModifiedAt":"2026-10-20T00:00:00Z"}}`)
		case "POST /api/cloud/1.1/commonserviceitem/123456789013/simplenotification/message":
			var messageRequest map[string]string
			if err := json.Unmarshal(body, &messageRequest); err != nil {
				t.Errorf("decode send request: %v", err)
			}
			if !reflect.DeepEqual(messageRequest, map[string]string{"Message": "test notification"}) {
				t.Errorf("send body = %#v", messageRequest)
			}
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusAccepted)
			_, _ = fmt.Fprint(writer, `{"is_ok":true}`)
		default:
			t.Errorf("unexpected SDK request: %s %s", request.Method, request.URL.Path)
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	client, err := clientconfig.New(false)
	if err != nil {
		t.Fatal(err)
	}
	apiClient, err := simplenotification.NewClientWithAPIRootURL(client, server.URL+"/api/cloud/1.1/")
	if err != nil {
		t.Fatal(err)
	}

	requestJSON := `{"CommonServiceItem":{"Name":"test-group","Settings":{"Destinations":["123456789012"]}}}`
	var createOutput any
	createCommand := CommonServiceItemCreateCommand{
		ProviderClass: "saknoticegroup",
		Request:       &requestJSON,
		runtime: CommonServiceItemRuntime{
			DecodeRequest:  DecodeRequest,
			ValidateOutput: func(*kong.Context) error { return nil },
			WriteOutput: func(_ *kong.Context, value any) error {
				createOutput = value
				return nil
			},
			SetCreateRequestProviderInput: SetCreateRequestProviderInput,
		},
		factory: func(string) (CommonServiceItemAPI, error) {
			return simplenotification.NewGroupOp(apiClient), nil
		},
	}
	if err := createCommand.Run(&kong.Context{}); err != nil {
		t.Fatal(err)
	}
	item, ok := createBody["CommonServiceItem"].(map[string]any)
	if !ok {
		t.Fatalf("create body has no CommonServiceItem: %#v", createBody)
	}
	wantItem := map[string]any{
		"Name":        "test-group",
		"Description": "",
		"Settings": map[string]any{
			"Destinations": []any{"123456789012"},
		},
		"Provider":     map[string]any{"Class": "saknoticegroup", "ServiceClass": "cloud/saknotice"},
		"ServiceClass": "cloud/saknoticegroup/2",
		"Icon":         nil,
		"Tags":         []any{},
	}
	if !reflect.DeepEqual(item, wantItem) {
		t.Errorf("create body item = %#v, want %#v", item, wantItem)
	}
	if _, ok := createOutput.(*v1.CreateCommonServiceItemCreated); !ok {
		t.Fatalf("create output type = %T", createOutput)
	}

	message := "test notification"
	var sendOutput any
	sendCommand := CommonServiceItemSendMessageCommand{
		ID:      "123456789013",
		Message: &message,
		runtime: CommonServiceItemRuntime{
			ValidateOutput: func(*kong.Context) error { return nil },
			WriteOutput: func(_ *kong.Context, value any) error {
				sendOutput = value
				return nil
			},
		},
		factory: func(string) (CommonServiceItemAPI, error) {
			return simplenotification.NewGroupOp(apiClient), nil
		},
	}
	if err := sendCommand.Run(&kong.Context{}); err != nil {
		t.Fatal(err)
	}
	response, ok := sendOutput.(*v1.SendNotificationResponse)
	if !ok || !response.IsOk {
		t.Fatalf("send output = %#v, want successful response", sendOutput)
	}
	wantRequests := []string{
		"POST /api/cloud/1.1/commonserviceitem",
		"POST /api/cloud/1.1/commonserviceitem/123456789013/simplenotification/message",
	}
	if !reflect.DeepEqual(requests, wantRequests) {
		t.Errorf("SDK requests = %#v, want %#v", requests, wantRequests)
	}
}

func TestCommonServiceItemUpdateSettingsThroughSDK(t *testing.T) {
	for _, test := range []struct {
		name     string
		settings string
		want     map[string]any
	}{
		{name: "omitted settings"},
		{
			name:     "omitted disabled",
			settings: `,"Settings":{"Destinations":["123456789012"]}`,
			want:     map[string]any{"Destinations": []any{"123456789012"}},
		},
		{
			name:     "preserved disabled",
			settings: `,"Settings":{"Destinations":["123456789012"],"Disabled":true}`,
			want:     map[string]any{"Destinations": []any{"123456789012"}, "Disabled": true},
		},
		{
			name:     "explicit enabled",
			settings: `,"Settings":{"Destinations":["123456789012"],"Disabled":false}`,
			want:     map[string]any{"Destinations": []any{"123456789012"}, "Disabled": false},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
			t.Setenv("SAKURA_ACCESS_TOKEN", "test-token")
			t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "test-secret")
			var bodies []map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodPut || request.URL.Path != "/api/cloud/1.1/commonserviceitem/123456789013" {
					t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
					http.Error(writer, "unexpected request", http.StatusBadRequest)
					return
				}
				var body map[string]any
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Errorf("decode update body: %v", err)
					http.Error(writer, "invalid request", http.StatusBadRequest)
					return
				}
				bodies = append(bodies, body)
				writer.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(writer, `{"CommonServiceItem":{"ID":"123456789013","Name":"updated-group","Description":"kept description","Settings":{"Destinations":["123456789012"],"Disabled":true},"Provider":{"Class":"saknoticegroup"},"Icon":null,"Tags":["kept-tag"],"CreatedAt":"2026-10-20T00:00:00Z","ModifiedAt":"2026-10-20T00:00:00Z"}}`)
			}))
			t.Cleanup(server.Close)
			client, err := clientconfig.New(false)
			if err != nil {
				t.Fatal(err)
			}
			apiClient, err := simplenotification.NewClientWithAPIRootURL(client, server.URL+"/api/cloud/1.1/")
			if err != nil {
				t.Fatal(err)
			}
			input := `{"CommonServiceItem":{"Name":"updated-group","Description":"kept description","Tags":["kept-tag"],"Icon":null` + test.settings + `}}`
			var output any
			command := CommonServiceItemUpdateCommand{
				ID:            "123456789013",
				ProviderClass: "saknoticegroup",
				Request:       &input,
				runtime: CommonServiceItemRuntime{
					DecodeRequest:  DecodeRequest,
					ValidateOutput: func(*kong.Context) error { return nil },
					WriteOutput: func(_ *kong.Context, value any) error {
						output = value
						return nil
					},
				},
				factory: func(string) (CommonServiceItemAPI, error) {
					return simplenotification.NewGroupOp(apiClient), nil
				},
			}
			if err := command.Run(&kong.Context{}); err != nil {
				t.Fatal(err)
			}
			wantItem := map[string]any{
				"Name": "updated-group", "Description": "kept description", "Tags": []any{"kept-tag"}, "Icon": nil,
			}
			if test.want != nil {
				wantItem["Settings"] = test.want
			}
			wantBodies := []map[string]any{{"CommonServiceItem": wantItem}}
			if !reflect.DeepEqual(bodies, wantBodies) {
				t.Errorf("update requests = %#v, want %#v", bodies, wantBodies)
			}
			response, ok := output.(*v1.UpdateCommonServiceItemOK)
			if !ok || response.CommonServiceItem.ID != "123456789013" {
				t.Fatalf("update output = %#v", output)
			}
		})
	}
}

func TestCommonServiceItemCreateThroughDestinationOpSetsServiceClass(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "test-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "test-secret")

	var createBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/cloud/1.1/commonserviceitem" {
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
			http.Error(writer, "unexpected request", http.StatusBadRequest)
			return
		}
		if err := json.NewDecoder(request.Body).Decode(&createBody); err != nil {
			t.Errorf("decode create request: %v", err)
			http.Error(writer, "invalid request", http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(writer, `{"CommonServiceItem":{"ID":"123456789012","Name":"test-destination","Description":"","Tags":[],"Icon":null,"Settings":{"Type":"email","Value":"example@example.test"},"Provider":{"Class":"saknoticedestination"},"ServiceClass":"cloud/saknoticedestination/2","CreatedAt":"2026-10-20T00:00:00Z","ModifiedAt":"2026-10-20T00:00:00Z"}}`)
	}))
	t.Cleanup(server.Close)

	client, err := clientconfig.New(false)
	if err != nil {
		t.Fatal(err)
	}
	apiClient, err := simplenotification.NewClientWithAPIRootURL(client, server.URL+"/api/cloud/1.1/")
	if err != nil {
		t.Fatal(err)
	}

	requestJSON := `{"CommonServiceItem":{"Name":"test-destination","Settings":{"Type":"email","Value":"example@example.test"}}}`
	requestFile := filepath.Join(t.TempDir(), "destination.json")
	require.NoError(t, os.WriteFile(requestFile, []byte(requestJSON), 0o600))
	requestInput := "@" + requestFile
	var output any
	command := CommonServiceItemCreateCommand{
		ProviderClass: "saknoticedestination",
		Request:       &requestInput,
		runtime: CommonServiceItemRuntime{
			DecodeRequest:  DecodeRequest,
			ValidateOutput: func(*kong.Context) error { return nil },
			WriteOutput: func(_ *kong.Context, value any) error {
				output = value
				return nil
			},
			SetCreateRequestProviderInput: SetCreateRequestProviderInput,
		},
		factory: func(string) (CommonServiceItemAPI, error) {
			return destinationAdapter{simplenotification.NewDestinationOp(apiClient)}, nil
		},
	}
	require.NoError(t, command.Run(&kong.Context{}))
	item, ok := createBody["CommonServiceItem"].(map[string]any)
	require.True(t, ok, "create body has no CommonServiceItem: %#v", createBody)
	require.Equal(t, "saknoticedestination", item["Provider"].(map[string]any)["Class"])
	require.Equal(t, "cloud/saknoticedestination/2", item["ServiceClass"])
	settings, ok := item["Settings"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "email", settings["Type"])
	require.Equal(t, "example@example.test", settings["Value"])
	require.IsType(t, &v1.CreateCommonServiceItemCreated{}, output)
}
