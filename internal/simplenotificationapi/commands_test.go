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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	v1 "github.com/sacloud/sacloud-sdk-go/api/simple-notification/apis/v1"
)

type destinationAPIMock struct {
	DestinationAPI
	create func(context.Context, v1.PostCommonServiceItemRequest) (*v1.CreateCommonServiceItemCreated, error)
}

func (m destinationAPIMock) Create(ctx context.Context, request v1.PostCommonServiceItemRequest) (*v1.CreateCommonServiceItemCreated, error) {
	return m.create(ctx, request)
}

type groupAPIMock struct {
	GroupAPI
	sendMessage func(context.Context, string, v1.SendNotificationMessageRequest) (*v1.SendNotificationMessageResponse, error)
}

func (m groupAPIMock) SendMessage(ctx context.Context, id string, request v1.SendNotificationMessageRequest) (*v1.SendNotificationMessageResponse, error) {
	return m.sendMessage(ctx, id, request)
}

func TestCreateProviderCallbacks(t *testing.T) {
	for _, test := range []struct {
		name          string
		set           func(*v1.PostCommonServiceItemRequest) error
		providerClass v1.CommonServiceItemProviderClass
		serviceClass  v1.CommonServiceItemServiceClass
	}{
		{
			name:          "destination",
			set:           SetDestinationProvider,
			providerClass: v1.CommonServiceItemProviderClassSaknoticedestination,
			serviceClass:  v1.CommonServiceItemServiceClassCloudSaknoticedestination2,
		},
		{
			name:          "group",
			set:           SetGroupProvider,
			providerClass: v1.CommonServiceItemProviderClassSaknoticegroup,
			serviceClass:  v1.CommonServiceItemServiceClassCloudSaknoticegroup2,
		},
		{
			name:          "routing",
			set:           SetRoutingProvider,
			providerClass: v1.CommonServiceItemProviderClassSaknoticerouting,
			serviceClass:  v1.CommonServiceItemServiceClassCloudSaknoticerouting2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := v1.PostCommonServiceItemRequest{}
			if err := test.set(&request); err != nil {
				t.Fatal(err)
			}
			if got := request.CommonServiceItem.Provider.Class; got != test.providerClass {
				t.Errorf("Provider.Class = %q, want %q", got, test.providerClass)
			}
			if got := request.CommonServiceItem.ServiceClass; got != test.serviceClass {
				t.Errorf("ServiceClass = %q, want %q", got, test.serviceClass)
			}
		})
	}
}

func TestDestinationCreateCommandSetsProviderAndPreservesSettings(t *testing.T) {
	requestJSON := `{"CommonServiceItem":{"Name":"test-destination","Provider":{"Class":"saknoticegroup"},"Settings":{"Type":"email","Value":"example@example.test"}}}`
	requestFile := filepath.Join(t.TempDir(), "destination.json")
	if err := os.WriteFile(requestFile, []byte(requestJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	requestInput := "@" + requestFile
	var gotRequest v1.PostCommonServiceItemRequest
	var output any
	command := DestinationCreateCommand{
		Request: &requestInput,
		runtime: DestinationRuntime{
			DecodeRequest: func(input string, destination any) error {
				return decodeDestinationRequest(input, destination, func(input string, destination any) error {
					return json.Unmarshal([]byte(input), destination)
				}, v1.CommonServiceItemProviderClassSaknoticedestination, v1.CommonServiceItemServiceClassCloudSaknoticedestination2)
			},
			ValidateOutput: func(*kong.Context) error { return nil },
			WriteOutput: func(_ *kong.Context, value any) error {
				output = value
				return nil
			},
		},
		factory: func() (DestinationAPI, error) {
			return destinationAPIMock{
				create: func(ctx context.Context, request v1.PostCommonServiceItemRequest) (*v1.CreateCommonServiceItemCreated, error) {
					if ctx == nil {
						t.Fatal("Create received a nil context")
					}
					gotRequest = request
					return &v1.CreateCommonServiceItemCreated{}, nil
				},
			}, nil
		},
	}
	if err := command.Run(&kong.Context{}); err != nil {
		t.Fatal(err)
	}
	if gotRequest.CommonServiceItem.Provider.Class != v1.CommonServiceItemProviderClassSaknoticedestination {
		t.Errorf("Provider.Class = %q", gotRequest.CommonServiceItem.Provider.Class)
	}
	if gotRequest.CommonServiceItem.ServiceClass != v1.CommonServiceItemServiceClassCloudSaknoticedestination2 {
		t.Errorf("ServiceClass = %q", gotRequest.CommonServiceItem.ServiceClass)
	}
	settings, ok := gotRequest.CommonServiceItem.Settings.GetDestinationSettings()
	if !ok {
		t.Fatal("Settings is not a destination settings variant")
	}
	if settings.Type != v1.DestinationSettingsTypeEmail || settings.Value != "example@example.test" {
		t.Errorf("Settings = %+v", settings)
	}
	if _, ok := output.(*v1.CreateCommonServiceItemCreated); !ok {
		t.Errorf("output has type %T, want create response", output)
	}
}

func TestSendMessageFlagAndJSONInputs(t *testing.T) {
	const message = "test notification"
	requestJSON := `{"Message":"test notification"}`
	for _, test := range []struct {
		name    string
		request *string
		flag    *string
	}{
		{name: "flag", flag: stringPointer(message)},
		{name: "JSON", request: &requestJSON},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			command := GroupSendMessageCommand{
				ID:      "group-123",
				Request: test.request,
				Message: test.flag,
				runtime: GroupRuntime{
					DecodeRequest: func(input string, destination any) error {
						return json.Unmarshal([]byte(input), destination)
					},
					ValidateOutput: func(*kong.Context) error { return nil },
					WriteOutput:    func(*kong.Context, any) error { return nil },
				},
				factory: func() (GroupAPI, error) {
					return groupAPIMock{sendMessage: func(ctx context.Context, id string, request v1.SendNotificationMessageRequest) (*v1.SendNotificationMessageResponse, error) {
						called = true
						if ctx == nil || id != "group-123" || request.Message != message {
							t.Fatalf("SendMessage(%v, %q, %+v)", ctx, id, request)
						}
						return &v1.SendNotificationMessageResponse{IsOk: true}, nil
					}}, nil
				},
			}
			if err := command.Run(&kong.Context{}); err != nil {
				t.Fatal(err)
			}
			if !called {
				t.Fatal("SendMessage was not called")
			}
		})
	}
}

func TestSendMessageRejectsMissingOrMixedInput(t *testing.T) {
	requestJSON := `{"Message":"test"}`
	for _, test := range []struct {
		name    string
		request *string
		message *string
		want    string
	}{
		{name: "missing input", want: "--request または個別フラグが必要です"},
		{name: "mixed input", request: &requestJSON, message: stringPointer("test"), want: "--request と個別フラグは併用できません"},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := GroupSendMessageCommand{Request: test.request, Message: test.message}
			err := command.Run(&kong.Context{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCreateCommandPropagatesDecodeAndAPIErrors(t *testing.T) {
	requestJSON := `{"CommonServiceItem":{"Name":"test","Settings":{"Type":"email","Value":"example@example.test"}}}`
	requestFile := filepath.Join(t.TempDir(), "destination.json")
	if err := os.WriteFile(requestFile, []byte(requestJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	requestInput := "@" + requestFile
	wantErr := errors.New("provider API failed")
	command := DestinationCreateCommand{
		Request: &requestInput,
		runtime: DestinationRuntime{
			DecodeRequest:  func(string, any) error { return errors.New("decode failed") },
			ValidateOutput: func(*kong.Context) error { return nil },
		},
	}
	if err := command.Run(&kong.Context{}); err == nil || !strings.Contains(err.Error(), "decode failed") {
		t.Fatalf("decode error = %v", err)
	}

	command.runtime.DecodeRequest = func(input string, destination any) error {
		return decodeDestinationRequest(input, destination, func(input string, destination any) error {
			return json.Unmarshal([]byte(input), destination)
		}, v1.CommonServiceItemProviderClassSaknoticedestination, v1.CommonServiceItemServiceClassCloudSaknoticedestination2)
	}
	command.factory = func() (DestinationAPI, error) {
		return destinationAPIMock{create: func(context.Context, v1.PostCommonServiceItemRequest) (*v1.CreateCommonServiceItemCreated, error) {
			return nil, wantErr
		}}, nil
	}
	if err := command.Run(&kong.Context{}); !errors.Is(err, wantErr) {
		t.Fatalf("API error = %v, want %v", err, wantErr)
	}
}

func TestDestinationRequestRequiresFileForAddressSettings(t *testing.T) {
	for _, test := range []struct {
		name        string
		destination any
		input       string
		wantError   bool
	}{
		{
			name:        "create",
			destination: &v1.PostCommonServiceItemRequest{},
			input:       `{"CommonServiceItem":{"Name":"destination","Settings":{"Type":"webhook","Value":"https://example.test/secret"}}}`,
			wantError:   true,
		},
		{
			name:        "update settings",
			destination: &v1.PutCommonServiceItemRequest{},
			input:       `{"CommonServiceItem":{"Name":"destination","Description":"","Tags":[],"Icon":null,"Settings":{"Type":"webhook","Value":"https://example.test/secret"}}}`,
			wantError:   true,
		},
		{
			name:        "metadata update",
			destination: &v1.PutCommonServiceItemRequest{},
			input:       `{"CommonServiceItem":{"Name":"destination","Description":"","Tags":[],"Icon":null}}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			err := decodeDestinationRequest(test.input, test.destination, func(string, any) error {
				called = true
				return nil
			}, v1.CommonServiceItemProviderClassSaknoticedestination, v1.CommonServiceItemServiceClassCloudSaknoticedestination2)
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), "@path.json") || strings.Contains(err.Error(), "https://example.test/secret") {
					t.Fatalf("error = %v, want safe @path.json instruction", err)
				}
				if called {
					t.Fatal("inline address was passed to the JSON decoder")
				}
			} else if err != nil || !called {
				t.Fatalf("metadata request error = %v, decoder called = %v", err, called)
			}
		})
	}
}

func TestDecodeResourceCreateRequestSupportsFileAndPreservesFields(t *testing.T) {
	input := `{"CommonServiceItem":{"Name":"test-group","Description":"kept description","Tags":["kept-tag"],"Provider":{"Class":"wrong"},"ServiceClass":"wrong","Settings":{"Destinations":["123456789012"]}}}`
	path := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}

	var request v1.PostCommonServiceItemRequest
	err := decodeResourceRequest("@"+path, &request, func(input string, destination any) error {
		return json.Unmarshal([]byte(input), destination)
	}, v1.CommonServiceItemProviderClassSaknoticegroup, v1.CommonServiceItemServiceClassCloudSaknoticegroup2)
	if err != nil {
		t.Fatal(err)
	}
	item := request.CommonServiceItem
	if item.Name != "test-group" || item.Description != "kept description" {
		t.Errorf("Name/Description = %q/%q", item.Name, item.Description)
	}
	if item.Provider.Class != v1.CommonServiceItemProviderClassSaknoticegroup ||
		item.ServiceClass != v1.CommonServiceItemServiceClassCloudSaknoticegroup2 {
		t.Errorf("provider/service class = %q/%q", item.Provider.Class, item.ServiceClass)
	}
	if len(item.Tags) != 1 || item.Tags[0] != "kept-tag" {
		t.Errorf("Tags = %#v", item.Tags)
	}
	if !item.Icon.IsNull() {
		t.Errorf("omitted Icon should be represented as null, got %#v", item.Icon)
	}
	if settings, ok := item.Settings.GetGroupSettings(); !ok || len(settings.Destinations) != 1 {
		t.Errorf("Settings = %#v, want one group destination", item.Settings)
	}
}

func TestDecodeResourceCreateRequestErrors(t *testing.T) {
	decode := func(input string, destination any) error {
		return decodeResourceRequest(input, destination, func(input string, destination any) error {
			return json.Unmarshal([]byte(input), destination)
		}, v1.CommonServiceItemProviderClassSaknoticegroup, v1.CommonServiceItemServiceClassCloudSaknoticegroup2)
	}
	for _, test := range []struct {
		name  string
		input string
		want  string
	}{
		{name: "malformed", input: `{"CommonServiceItem":`, want: "decode request JSON"},
		{name: "trailing data", input: `{"CommonServiceItem":{}} {}`, want: "decode request JSON"},
		{name: "missing item", input: `{}`, want: "decode CommonServiceItem"},
		{name: "null item", input: `{"CommonServiceItem":null}`, want: "expected JSON object"},
		{name: "missing file", input: "@" + filepath.Join(t.TempDir(), "missing.json"), want: "read request file"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var request v1.PostCommonServiceItemRequest
			err := decode(test.input, &request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func stringPointer(value string) *string {
	return &value
}
