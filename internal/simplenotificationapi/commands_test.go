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
	"testing"

	"github.com/alecthomas/kong"
	v1 "github.com/sacloud/sacloud-sdk-go/api/simple-notification/apis/v1"
	"github.com/stretchr/testify/require"
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
			require.NoError(t, test.set(&request))
			require.Equal(t, test.providerClass, request.CommonServiceItem.Provider.Class)
			require.Equal(t, test.serviceClass, request.CommonServiceItem.ServiceClass)
		})
	}
}

func TestDestinationCreateCommandSetsProviderAndPreservesSettings(t *testing.T) {
	requestJSON := `{"CommonServiceItem":{"Name":"test-destination","Provider":{"Class":"saknoticegroup"},"Settings":{"Type":"email","Value":"example@example.test"}}}`
	requestFile := filepath.Join(t.TempDir(), "destination.json")
	require.NoError(t, os.WriteFile(requestFile, []byte(requestJSON), 0o600))
	requestInput := "@" + requestFile
	var gotRequest v1.PostCommonServiceItemRequest
	var output any
	command := DestinationCreateCommand{
		Request: &requestInput,
		runtime: DestinationRuntime{
			DecodeRequest:  DecodeDestinationRequest,
			ValidateOutput: func(*kong.Context) error { return nil },
			WriteOutput: func(_ *kong.Context, value any) error {
				output = value
				return nil
			},
		},
		factory: func() (DestinationAPI, error) {
			return destinationAPIMock{
				create: func(ctx context.Context, request v1.PostCommonServiceItemRequest) (*v1.CreateCommonServiceItemCreated, error) {
					require.NotNil(t, ctx, "Create received a nil context")
					gotRequest = request
					return &v1.CreateCommonServiceItemCreated{}, nil
				},
			}, nil
		},
	}
	require.NoError(t, command.Run(&kong.Context{}))
	require.Equal(t, v1.CommonServiceItemProviderClassSaknoticedestination, gotRequest.CommonServiceItem.Provider.Class)
	require.Equal(t, v1.CommonServiceItemServiceClassCloudSaknoticedestination2, gotRequest.CommonServiceItem.ServiceClass)
	settings, ok := gotRequest.CommonServiceItem.Settings.GetDestinationSettings()
	require.True(t, ok, "Settings is not a destination settings variant")
	require.Equal(t, v1.DestinationSettingsTypeEmail, settings.Type)
	require.Equal(t, "example@example.test", settings.Value)
	require.IsType(t, &v1.CreateCommonServiceItemCreated{}, output, "output should be a create response")
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
						require.NotNil(t, ctx)
						require.Equal(t, "group-123", id)
						require.Equal(t, message, request.Message)
						return &v1.SendNotificationMessageResponse{IsOk: true}, nil
					}}, nil
				},
			}
			require.NoError(t, command.Run(&kong.Context{}))
			require.True(t, called, "SendMessage was not called")
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
			require.Error(t, err)
			require.Contains(t, err.Error(), test.want)
		})
	}
}

func TestCreateCommandPropagatesDecodeAndAPIErrors(t *testing.T) {
	requestJSON := `{"CommonServiceItem":{"Name":"test","Settings":{"Type":"email","Value":"example@example.test"}}}`
	requestFile := filepath.Join(t.TempDir(), "destination.json")
	require.NoError(t, os.WriteFile(requestFile, []byte(requestJSON), 0o600))
	requestInput := "@" + requestFile
	wantErr := errors.New("provider API failed")
	command := DestinationCreateCommand{
		Request: &requestInput,
		runtime: DestinationRuntime{
			DecodeRequest:  func(string, any) error { return errors.New("decode failed") },
			ValidateOutput: func(*kong.Context) error { return nil },
		},
	}
	err := command.Run(&kong.Context{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "decode failed")

	command.runtime.DecodeRequest = DecodeDestinationRequest
	command.factory = func() (DestinationAPI, error) {
		return destinationAPIMock{create: func(context.Context, v1.PostCommonServiceItemRequest) (*v1.CreateCommonServiceItemCreated, error) {
			return nil, wantErr
		}}, nil
	}
	require.ErrorIs(t, command.Run(&kong.Context{}), wantErr)
}

func TestDestinationRequestRequiresFileForAddressSettings(t *testing.T) {
	const webhookURL = "https://example.test/secret"
	for _, test := range []struct {
		name        string
		destination any
		input       string
		wantError   bool
		wantMessage string
	}{
		{
			name:        "create",
			destination: &v1.PostCommonServiceItemRequest{},
			input:       `{"CommonServiceItem":{"Name":"destination","Settings":{"Type":"webhook","Value":"` + webhookURL + `"}}}`,
			wantError:   true,
			wantMessage: "@path.json",
		},
		{
			name:        "update settings",
			destination: &v1.PutCommonServiceItemRequest{},
			input:       `{"CommonServiceItem":{"Name":"destination","Description":"","Tags":[],"Icon":null,"Settings":{"Type":"webhook","Value":"` + webhookURL + `"}}}`,
			wantError:   true,
			wantMessage: "@path.json",
		},
		{
			name:        "metadata update",
			destination: &v1.PutCommonServiceItemRequest{},
			input:       `{"CommonServiceItem":{"Name":"destination","Description":"","Tags":[],"Icon":null}}`,
		},
		{
			name:        "malformed update cannot bypass file requirement",
			destination: &v1.PutCommonServiceItemRequest{},
			input:       `{"CommonServiceItem":{"Name":"destination","Settings":{"Type":"webhook","Value":"` + webhookURL + `"}},"commonserviceitem":0}`,
			wantError:   true,
			wantMessage: "通知先リクエストを読み込めません",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := DecodeDestinationRequest(test.input, test.destination)
			if test.wantError {
				require.Error(t, err)
				require.Contains(t, err.Error(), test.wantMessage)
				require.NotContains(t, err.Error(), webhookURL, "error must not contain the protected address")
				return
			}
			require.NoError(t, err)
			request, ok := test.destination.(*v1.PutCommonServiceItemRequest)
			require.True(t, ok)
			require.Equal(t, "destination", request.CommonServiceItem.Name, "metadata request should be decoded")
		})
	}
}

func TestDecodeResourceCreateRequestSupportsFileAndPreservesFields(t *testing.T) {
	input := `{"CommonServiceItem":{"Name":"test-group","Description":"kept description","Tags":["kept-tag"],"Provider":{"Class":"wrong"},"ServiceClass":"wrong","Settings":{"Destinations":["123456789012"]}}}`
	path := filepath.Join(t.TempDir(), "request.json")
	require.NoError(t, os.WriteFile(path, []byte(input), 0o600))

	var request v1.PostCommonServiceItemRequest
	require.NoError(t, DecodeGroupRequest("@"+path, &request))
	item := request.CommonServiceItem
	require.Equal(t, "test-group", item.Name)
	require.Equal(t, "kept description", item.Description)
	require.Equal(t, v1.CommonServiceItemProviderClassSaknoticegroup, item.Provider.Class)
	require.Equal(t, v1.CommonServiceItemServiceClassCloudSaknoticegroup2, item.ServiceClass)
	require.Equal(t, []string{"kept-tag"}, item.Tags)
	require.True(t, item.Icon.IsNull(), "omitted Icon should be represented as null")
	settings, ok := item.Settings.GetGroupSettings()
	require.True(t, ok)
	require.Equal(t, []string{"123456789012"}, settings.Destinations)
}

func TestDecodeResourceCreateRequestErrors(t *testing.T) {
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
			err := DecodeGroupRequest(test.input, &request)
			require.Error(t, err)
			require.Contains(t, err.Error(), test.want)
		})
	}
}

func stringPointer(value string) *string {
	return &value
}
