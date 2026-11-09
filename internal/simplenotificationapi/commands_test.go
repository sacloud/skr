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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/kong"
	v1 "github.com/sacloud/sacloud-sdk-go/api/simple-notification/apis/v1"
	"github.com/stretchr/testify/require"
)

// unsupportedCommonServiceItemOp は CommonServiceItemAPI のみを実装し、種別固有の
// 操作を持たないモックです。
type unsupportedCommonServiceItemOp struct {
	CommonServiceItemAPI
}

// commonServiceItemAPIMock は CommonServiceItemAPI の任意のメソッドを差し替えるモックです。
type commonServiceItemAPIMock struct {
	CommonServiceItemAPI
	create      func(context.Context, v1.PostCommonServiceItemRequest) (*v1.CreateCommonServiceItemCreated, error)
	update      func(context.Context, string, v1.PutCommonServiceItemRequest) (*v1.UpdateCommonServiceItemOK, error)
	sendMessage func(context.Context, string, v1.SendNotificationRequest) (*v1.SendNotificationResponse, error)
	readStatus  func(context.Context, string) (*v1.CheckDestinationStatusResponse, error)
	reorder     func(context.Context, v1.ReorderNotificationRoutingRequest) (*v1.ReorderNotificationRoutingAccepted, error)
	listSource  func(context.Context) (*v1.ListNotificationSourcesResponse, error)
}

func (m commonServiceItemAPIMock) Create(ctx context.Context, request v1.PostCommonServiceItemRequest) (*v1.CreateCommonServiceItemCreated, error) {
	return m.create(ctx, request)
}

func (m commonServiceItemAPIMock) Update(ctx context.Context, id string, request v1.PutCommonServiceItemRequest) (*v1.UpdateCommonServiceItemOK, error) {
	return m.update(ctx, id, request)
}

func (m commonServiceItemAPIMock) SendMessage(ctx context.Context, id string, request v1.SendNotificationRequest) (*v1.SendNotificationResponse, error) {
	return m.sendMessage(ctx, id, request)
}

func (m commonServiceItemAPIMock) ReadStatus(ctx context.Context, id string) (*v1.CheckDestinationStatusResponse, error) {
	return m.readStatus(ctx, id)
}

func (m commonServiceItemAPIMock) Reorder(ctx context.Context, request v1.ReorderNotificationRoutingRequest) (*v1.ReorderNotificationRoutingAccepted, error) {
	return m.reorder(ctx, request)
}

func (m commonServiceItemAPIMock) ListSource(ctx context.Context) (*v1.ListNotificationSourcesResponse, error) {
	return m.listSource(ctx)
}

func newTestRuntime(decode func(string, any) error) CommonServiceItemRuntime {
	return CommonServiceItemRuntime{
		DecodeRequest:                 decode,
		ValidateOutput:                func(*kong.Context) error { return nil },
		WriteOutput:                   func(*kong.Context, any) error { return nil },
		SetCreateRequestProviderInput: SetCreateRequestProviderInput,
	}
}

func TestParseProviderClassAcceptsKnownClassesOnly(t *testing.T) {
	for _, test := range []struct {
		value string
		want  v1.CommonServiceItemProviderClass
	}{
		{value: "saknoticedestination", want: v1.CommonServiceItemProviderClassSaknoticedestination},
		{value: "saknoticegroup", want: v1.CommonServiceItemProviderClassSaknoticegroup},
		{value: "saknoticerouting", want: v1.CommonServiceItemProviderClassSaknoticerouting},
	} {
		t.Run(test.value, func(t *testing.T) {
			class, err := parseProviderClass(test.value)
			require.NoError(t, err)
			require.Equal(t, test.want, class)
		})
	}
	t.Run("unknown class", func(t *testing.T) {
		_, err := parseProviderClass("unknown")
		require.Error(t, err)
		require.Contains(t, err.Error(), "saknoticedestination")
	})
}

func TestDestinationCreateCommandSetsProviderAndPreservesSettings(t *testing.T) {
	requestJSON := `{"CommonServiceItem":{"Name":"test-destination","Provider":{"Class":"saknoticegroup"},"Settings":{"Type":"email","Value":"example@example.test"}}}`
	requestFile := filepath.Join(t.TempDir(), "destination.json")
	require.NoError(t, os.WriteFile(requestFile, []byte(requestJSON), 0o600))
	requestInput := "@" + requestFile
	var gotRequest v1.PostCommonServiceItemRequest
	var output any
	runtime := newTestRuntime(DecodeRequest)
	runtime.WriteOutput = func(_ *kong.Context, value any) error {
		output = value
		return nil
	}
	command := CommonServiceItemCreateCommand{
		ProviderClass: "saknoticedestination",
		Request:       &requestInput,
		runtime:       runtime,
		factory: func(string) (CommonServiceItemAPI, error) {
			return commonServiceItemAPIMock{
				create: func(ctx context.Context, request v1.PostCommonServiceItemRequest) (*v1.CreateCommonServiceItemCreated, error) {
					require.NotNil(t, ctx, "Create received a nil context")
					gotRequest = request
					return &v1.CreateCommonServiceItemCreated{}, nil
				},
			}, nil
		},
	}
	require.NoError(t, command.Run(&kong.Context{}))
	require.Equal(t, v1.PostCommonServiceItemRequestCommonServiceItemProviderClassSaknoticedestination, gotRequest.CommonServiceItem.Provider.Class)
	require.Equal(t, v1.PostCommonServiceItemRequestCommonServiceItemServiceClassCloudSaknoticedestination2, gotRequest.CommonServiceItem.ServiceClass)
	require.Equal(t, "", gotRequest.CommonServiceItem.Description, "omitted Description should be filled with the default")
	require.Equal(t, []string{}, gotRequest.CommonServiceItem.Tags, "omitted Tags should be filled with the default")
	require.True(t, gotRequest.CommonServiceItem.Icon.IsNull(), "omitted Icon should be represented as null")
	settings, ok := gotRequest.CommonServiceItem.Settings.GetCommonServiceItemDestinationSettings()
	require.True(t, ok, "Settings is not a destination settings variant")
	require.Equal(t, v1.CommonServiceItemDestinationSettingsTypeEmail, settings.Type)
	require.Equal(t, "example@example.test", settings.Value)
	require.IsType(t, &v1.CreateCommonServiceItemCreated{}, output, "output should be a create response")
}

func TestCreateCommandRejectsInlineDestinationRequest(t *testing.T) {
	input := `{"CommonServiceItem":{"Name":"test"}}`
	command := CommonServiceItemCreateCommand{
		ProviderClass: "saknoticedestination",
		Request:       &input,
		runtime:       newTestRuntime(DecodeRequest),
	}
	err := command.Run(&kong.Context{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "@path.json")
}

func TestSendMessageFlagAndJSONInputs(t *testing.T) {
	const message = "test notification"
	requestJSON := `{"Message":"test notification"}`
	flag := message
	for _, test := range []struct {
		name    string
		request *string
		flag    *string
	}{
		{name: "flag", flag: &flag},
		{name: "JSON", request: &requestJSON},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			command := CommonServiceItemSendMessageCommand{
				ID:      "group-123",
				Request: test.request,
				Message: test.flag,
				runtime: CommonServiceItemRuntime{
					DecodeRequest: func(input string, destination any) error {
						return json.Unmarshal([]byte(input), destination)
					},
					ValidateOutput: func(*kong.Context) error { return nil },
					WriteOutput:    func(*kong.Context, any) error { return nil },
				},
				factory: func(string) (CommonServiceItemAPI, error) {
					return commonServiceItemAPIMock{sendMessage: func(ctx context.Context, id string, request v1.SendNotificationRequest) (*v1.SendNotificationResponse, error) {
						called = true
						require.NotNil(t, ctx)
						require.Equal(t, "group-123", id)
						require.Equal(t, message, request.Message)
						return &v1.SendNotificationResponse{IsOk: true}, nil
					}}, nil
				},
			}
			require.NoError(t, command.Run(&kong.Context{}))
			require.True(t, called, "SendMessage was not called")
		})
	}
}

func TestSendMessageRejectsMissingOrMixedInput(t *testing.T) {
	const message = "test"
	requestJSON := `{"Message":"test"}`
	flag := message
	for _, test := range []struct {
		name    string
		request *string
		message *string
		want    string
	}{
		{name: "missing input", want: "--request または個別フラグが必要です"},
		{name: "mixed input", request: &requestJSON, message: &flag, want: "--request と個別フラグは併用できません"},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := CommonServiceItemSendMessageCommand{Request: test.request, Message: test.message}
			err := command.Run(&kong.Context{})
			require.Error(t, err)
			require.Contains(t, err.Error(), test.want)
		})
	}
}

func TestResourceSpecificCommandsRejectOpsWithoutSupport(t *testing.T) {
	unsupported := unsupportedCommonServiceItemOp{}
	for _, test := range []struct {
		name    string
		command func() error
		want    string
	}{
		{
			name: "read status",
			command: func() error {
				command := CommonServiceItemReadStatusCommand{
					ID:      "id",
					runtime: newTestRuntime(nil),
					factory: func(string) (CommonServiceItemAPI, error) { return unsupported, nil },
				}
				return command.Run(&kong.Context{})
			},
			want: "ステータスの確認は通知先",
		},
		{
			name: "send message",
			command: func() error {
				message := "test"
				command := CommonServiceItemSendMessageCommand{
					ID:      "id",
					Message: &message,
					runtime: newTestRuntime(nil),
					factory: func(string) (CommonServiceItemAPI, error) { return unsupported, nil },
				}
				return command.Run(&kong.Context{})
			},
			want: "メッセージの送信は通知先グループ",
		},
		{
			name: "reorder",
			command: func() error {
				requestJSON := `{"Orders":[{"PriorityRank":2,"RoutingID":"123456789013"}]}`
				command := CommonServiceItemReorderCommand{
					Request: &requestJSON,
					runtime: CommonServiceItemRuntime{
						DecodeRequest:  func(input string, destination any) error { return json.Unmarshal([]byte(input), destination) },
						ValidateOutput: func(*kong.Context) error { return nil },
						WriteOutput:    func(*kong.Context, any) error { return nil },
					},
					factory: func(string) (CommonServiceItemAPI, error) { return unsupported, nil },
				}
				return command.Run(&kong.Context{})
			},
			want: "順序の変更はルーティング設定",
		},
		{
			name: "list source",
			command: func() error {
				command := CommonServiceItemListSourceCommand{
					runtime: newTestRuntime(nil),
					factory: func(string) (CommonServiceItemAPI, error) { return unsupported, nil },
				}
				return command.Run(&kong.Context{})
			},
			want: "通知元の一覧はルーティング設定",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.command()
			require.Error(t, err)
			require.Contains(t, err.Error(), test.want)
		})
	}
}

func TestResourceSpecificCommandsCallOps(t *testing.T) {
	statusCalled := false
	reorderCalled := false
	sourceCalled := false
	factory := func(string) (CommonServiceItemAPI, error) {
		return commonServiceItemAPIMock{
			readStatus: func(ctx context.Context, id string) (*v1.CheckDestinationStatusResponse, error) {
				statusCalled = true
				require.Equal(t, "destination-id", id)
				return &v1.CheckDestinationStatusResponse{}, nil
			},
			reorder: func(ctx context.Context, request v1.ReorderNotificationRoutingRequest) (*v1.ReorderNotificationRoutingAccepted, error) {
				reorderCalled = true
				return &v1.ReorderNotificationRoutingAccepted{}, nil
			},
			listSource: func(ctx context.Context) (*v1.ListNotificationSourcesResponse, error) {
				sourceCalled = true
				return &v1.ListNotificationSourcesResponse{}, nil
			},
		}, nil
	}
	decodeRuntime := CommonServiceItemRuntime{
		DecodeRequest:  func(input string, destination any) error { return json.Unmarshal([]byte(input), destination) },
		ValidateOutput: func(*kong.Context) error { return nil },
		WriteOutput:    func(*kong.Context, any) error { return nil },
	}
	requestJSON := `{"Orders":[{"PriorityRank":2,"RoutingID":"123456789013"},{"PriorityRank":1,"RoutingID":"123456789012"}]}`
	readStatus := CommonServiceItemReadStatusCommand{ID: "destination-id", runtime: decodeRuntime, factory: factory}
	reorder := CommonServiceItemReorderCommand{Request: &requestJSON, runtime: decodeRuntime, factory: factory}
	listSource := CommonServiceItemListSourceCommand{runtime: decodeRuntime, factory: factory}
	require.NoError(t, readStatus.Run(&kong.Context{}))
	require.NoError(t, reorder.Run(&kong.Context{}))
	require.NoError(t, listSource.Run(&kong.Context{}))
	require.True(t, statusCalled, "ReadStatus was not called")
	require.True(t, reorderCalled, "Reorder was not called")
	require.True(t, sourceCalled, "ListSource was not called")
}

func TestCreateCommandPropagatesDecodeAndAPIErrors(t *testing.T) {
	requestJSON := `{"CommonServiceItem":{"Name":"test","Description":"","Tags":[],"Icon":null,"Provider":{"Class":"saknoticedestination"},"ServiceClass":"cloud/saknoticedestination/2","Settings":{"Type":"email","Value":"example@example.test"}}}`
	requestFile := filepath.Join(t.TempDir(), "destination.json")
	require.NoError(t, os.WriteFile(requestFile, []byte(requestJSON), 0o600))
	requestInput := "@" + requestFile
	wantErr := errors.New("provider API failed")
	command := CommonServiceItemCreateCommand{
		ProviderClass: "saknoticedestination",
		Request:       &requestInput,
		runtime: CommonServiceItemRuntime{
			DecodeRequest:  func(string, any) error { return errors.New("decode failed") },
			ValidateOutput: func(*kong.Context) error { return nil },
			WriteOutput:    func(*kong.Context, any) error { return nil },
			SetCreateRequestProviderInput: func(*CommonServiceItemCreateCommand, string) (string, error) {
				return requestJSON, nil
			},
		},
	}
	err := command.Run(&kong.Context{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "decode failed")

	command.runtime.DecodeRequest = DecodeRequest
	command.factory = func(string) (CommonServiceItemAPI, error) {
		return commonServiceItemAPIMock{create: func(context.Context, v1.PostCommonServiceItemRequest) (*v1.CreateCommonServiceItemCreated, error) {
			return nil, wantErr
		}}, nil
	}
	require.ErrorIs(t, command.Run(&kong.Context{}), wantErr)
}

func TestDestinationUpdateRequiresFileForSettings(t *testing.T) {
	const webhookURL = "https://example.test/secret"
	for _, test := range []struct {
		name        string
		input       string
		wantError   bool
		wantMessage string
	}{
		{
			name:        "settings update",
			input:       `{"CommonServiceItem":{"Name":"destination","Description":"","Tags":[],"Icon":null,"Settings":{"Type":"webhook","Value":"` + webhookURL + `"}}}`,
			wantError:   true,
			wantMessage: "@path.json",
		},
		{
			name:      "metadata update",
			input:     `{"CommonServiceItem":{"Name":"destination","Description":"","Tags":[],"Icon":null}}`,
			wantError: false,
		},
		{
			name:        "malformed update reports decode error without values",
			input:       `{"CommonServiceItem":{"Name":"destination","Settings":{"Type":"webhook","Value":"` + webhookURL + `"}}`,
			wantError:   true,
			wantMessage: "decode request JSON",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := CommonServiceItemUpdateCommand{
				ProviderClass: "saknoticedestination",
				ID:            "123456789012",
				Request:       &test.input,
				runtime:       newTestRuntime(DecodeRequest),
				factory: func(string) (CommonServiceItemAPI, error) {
					return commonServiceItemAPIMock{update: func(ctx context.Context, id string, request v1.PutCommonServiceItemRequest) (*v1.UpdateCommonServiceItemOK, error) {
						require.Equal(t, "123456789012", id)
						require.Equal(t, "destination", request.CommonServiceItem.Name)
						return &v1.UpdateCommonServiceItemOK{}, nil
					}}, nil
				},
			}
			err := command.Run(&kong.Context{})
			if test.wantError {
				require.Error(t, err)
				require.Contains(t, err.Error(), test.wantMessage)
				require.NotContains(t, err.Error(), webhookURL, "error must not contain the protected address")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDestinationUpdateProtectsDecodeErrorsAndPreservesCause(t *testing.T) {
	// Settings を持つ通知先更新は @path.json に限定されるため、デコードエラーが発生するのは
	// Settings を含むファイル入力か、Settings を含まないメタデータ更新のみです。
	// メタデータ更新の入力にはメールアドレスや Webhook URL が含まれないため、
	// デコードエラーはそのまま返します。
	input := `{"CommonServiceItem":{"Name":123}}`
	command := CommonServiceItemUpdateCommand{
		ProviderClass: "saknoticedestination",
		ID:            "123456789012",
		Request:       &input,
		runtime:       newTestRuntime(DecodeRequest),
	}
	err := command.Run(&kong.Context{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "decode request JSON")

	// Settings を含むファイル入力のデコードエラーは、入力値を含まないメッセージへ変換します。
	settingsInput := `{"CommonServiceItem":{"Name":"destination","Settings":{"Type":"webhook","Value":"https://example.test/secret"}}}`
	settingsFile := filepath.Join(t.TempDir(), "destination.json")
	require.NoError(t, os.WriteFile(settingsFile, []byte(settingsInput), 0o600))
	fileInput := "@" + settingsFile
	command.Request = &fileInput
	err = command.Run(&kong.Context{})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "https://", "error must not contain request values")
	require.Contains(t, err.Error(), "通知先リクエストを読み込めません")
	require.NotNil(t, errors.Unwrap(err), "decode error cause should be preserved")
}

func TestUpdateCommandPassesInputThrough(t *testing.T) {
	input := `{"CommonServiceItem":{"Name":"updated-group","Description":"kept description","Tags":["kept-tag"],"Icon":null,"Settings":{"Destinations":["123456789012"]}}}`
	var gotRequest v1.PutCommonServiceItemRequest
	command := CommonServiceItemUpdateCommand{
		ProviderClass: "saknoticegroup",
		ID:            "123456789013",
		Request:       &input,
		runtime:       newTestRuntime(DecodeRequest),
		factory: func(string) (CommonServiceItemAPI, error) {
			return commonServiceItemAPIMock{update: func(ctx context.Context, id string, request v1.PutCommonServiceItemRequest) (*v1.UpdateCommonServiceItemOK, error) {
				require.Equal(t, "123456789013", id)
				gotRequest = request
				return &v1.UpdateCommonServiceItemOK{}, nil
			}}, nil
		},
	}
	require.NoError(t, command.Run(&kong.Context{}))
	require.Equal(t, "updated-group", gotRequest.CommonServiceItem.Name)
	require.Equal(t, []string{"kept-tag"}, gotRequest.CommonServiceItem.Tags)
	require.True(t, gotRequest.CommonServiceItem.Icon.IsNull())
	settings, ok := gotRequest.CommonServiceItem.Settings.Get()
	require.True(t, ok)
	groupSettings, ok := settings.GetCommonServiceItemGroupSettings()
	require.True(t, ok)
	require.Equal(t, []string{"123456789012"}, groupSettings.Destinations)
}

func TestCreateCommandSupportsFileAndPreservesFields(t *testing.T) {
	input := `{"CommonServiceItem":{"Name":"test-group","Description":"kept description","Tags":["kept-tag"],"Provider":{"Class":"wrong"},"ServiceClass":"wrong","Settings":{"Destinations":["123456789012"]}}}`
	path := filepath.Join(t.TempDir(), "request.json")
	require.NoError(t, os.WriteFile(path, []byte(input), 0o600))
	fileInput := "@" + path

	var request v1.PostCommonServiceItemRequest
	command := CommonServiceItemCreateCommand{
		ProviderClass: "saknoticegroup",
		Request:       &fileInput,
		runtime:       newTestRuntime(DecodeRequest),
	}
	// デコード結果を確認するため、変換後の入力を同じ経路でデコードします。
	adjusted, err := SetCreateRequestProviderInput(&command, fileInput)
	require.NoError(t, err)
	require.NoError(t, DecodeRequest(adjusted, &request))
	item := request.CommonServiceItem
	require.Equal(t, "test-group", item.Name)
	require.Equal(t, "kept description", item.Description)
	require.Equal(t, v1.PostCommonServiceItemRequestCommonServiceItemProviderClassSaknoticegroup, item.Provider.Class)
	require.Equal(t, v1.PostCommonServiceItemRequestCommonServiceItemServiceClassCloudSaknoticegroup2, item.ServiceClass)
	require.Equal(t, []string{"kept-tag"}, item.Tags)
	require.True(t, item.Icon.IsNull(), "omitted Icon should be represented as null")
	settings, ok := item.Settings.GetCommonServiceItemGroupSettings()
	require.True(t, ok)
	require.Equal(t, []string{"123456789012"}, settings.Destinations)
}

func TestCreateCommandInputErrors(t *testing.T) {
	for _, test := range []struct {
		name          string
		providerClass string
		input         string
		want          string
	}{
		{name: "malformed", providerClass: "saknoticegroup", input: `{"CommonServiceItem":`, want: "decode request JSON"},
		{name: "trailing data", providerClass: "saknoticegroup", input: `{"CommonServiceItem":{}} {}`, want: "decode request JSON"},
		{name: "missing item", providerClass: "saknoticegroup", input: `{}`, want: "decode CommonServiceItem"},
		{name: "null item", providerClass: "saknoticegroup", input: `{"CommonServiceItem":null}`, want: "expected JSON object"},
		{name: "missing file", providerClass: "saknoticegroup", input: "@" + filepath.Join(t.TempDir(), "missing.json"), want: "read request file"},
		{name: "unknown provider class", providerClass: "unknown", input: `{}`, want: "--provider-class"},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := CommonServiceItemCreateCommand{
				ProviderClass: test.providerClass,
				Request:       &test.input,
				runtime:       newTestRuntime(DecodeRequest),
			}
			err := command.Run(&kong.Context{})
			require.Error(t, err)
			require.Contains(t, err.Error(), test.want)
		})
	}
}

func TestReadStatusCommandReportsMissingFactory(t *testing.T) {
	command := CommonServiceItemReadStatusCommand{ID: "id", runtime: newTestRuntime(nil)}
	err := command.Run(&kong.Context{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "API クライアントが設定されていません")
}

func TestReorderCommandRequiresRequest(t *testing.T) {
	command := CommonServiceItemReorderCommand{runtime: newTestRuntime(nil)}
	err := command.Run(&kong.Context{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--request が必要です")
}

func TestWriteOutputIsConfigured(t *testing.T) {
	command := CommonServiceItemListSourceCommand{
		factory: func(string) (CommonServiceItemAPI, error) {
			return commonServiceItemAPIMock{listSource: func(context.Context) (*v1.ListNotificationSourcesResponse, error) {
				return &v1.ListNotificationSourcesResponse{}, nil
			}}, nil
		},
	}
	err := command.Run(&kong.Context{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "API 出力処理が設定されていません")
}

func TestDecodeRequestDispatchesDestinationProtection(t *testing.T) {
	data, err := json.Marshal(map[string]any{
		"CommonServiceItem": map[string]any{
			"Name":     "test",
			"Provider": map[string]any{"Class": "saknoticedestination"},
			// 不正な型でデコードエラーを起こし、エラー保護の適用を確認します。
			"Settings": []string{"unexpected"},
		},
	})
	require.NoError(t, err)
	err = DecodeRequest(string(data), &v1.PostCommonServiceItemRequest{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "通知先リクエストを読み込めません")

	data, err = json.Marshal(map[string]any{
		"CommonServiceItem": map[string]any{
			"Name":     "test",
			"Provider": map[string]any{"Class": "saknoticegroup"},
			"Settings": []string{"unexpected"},
		},
	})
	require.NoError(t, err)
	err = DecodeRequest(string(data), &v1.PostCommonServiceItemRequest{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "decode request JSON")
}

func TestRequestInputReadsFileAndStdin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "request.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"CommonServiceItem":{"Name":"file"}}`), 0o600))
	data, err := requestInput("@" + path)
	require.NoError(t, err)
	require.Contains(t, string(data), "file")

	_, err = requestInput("@" + filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "read request file")

	_, err = requestInput(fmt.Sprintf("@%s/..\x00", t.TempDir()))
	require.Error(t, err)
}
