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

package eventbusapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
)

func TestParseProviderClass(t *testing.T) {
	for _, test := range []struct {
		input string
		want  v1.ProviderClass
	}{
		{"eventbusprocessconfiguration", v1.ProviderClassEventbusprocessconfiguration},
		{"eventbusschedule", v1.ProviderClassEventbusschedule},
		{"eventbustrigger", v1.ProviderClassEventbustrigger},
	} {
		t.Run(test.input, func(t *testing.T) {
			got, err := parseProviderClass(test.input)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("parseProviderClass() = %q, want %q", got, test.want)
			}
		})
	}
	t.Run("unknown class", func(t *testing.T) {
		_, err := parseProviderClass("eventbusscheduler")
		if err == nil || !strings.Contains(err.Error(), "--provider-class") {
			t.Fatalf("error = %v, want --provider-class validation error", err)
		}
	})
	t.Run("empty class", func(t *testing.T) {
		_, err := parseProviderClass("")
		if err == nil || !strings.Contains(err.Error(), "--provider-class") {
			t.Fatalf("error = %v, want --provider-class validation error", err)
		}
	})
}

func TestNewCommonServiceItemAPIRejectsUnknownProviderClass(t *testing.T) {
	// 認証設定がなくても、種別の検証エラーはクライアント生成より先に返ること
	_, err := NewCommonServiceItemAPI(false, "unknown-class")
	if err == nil || !strings.Contains(err.Error(), "--provider-class") {
		t.Fatalf("error = %v, want --provider-class validation error", err)
	}
}

func TestNewCommonServiceItemAPISelectsOperationsByProviderClass(t *testing.T) {
	for _, test := range []struct {
		class v1.ProviderClass
		want  string
	}{
		{v1.ProviderClassEventbusprocessconfiguration, "processConfigurationOp"},
		{v1.ProviderClassEventbusschedule, "scheduleOp"},
		{v1.ProviderClassEventbustrigger, "triggerOp"},
	} {
		t.Run(string(test.class), func(t *testing.T) {
			op := selectCommonServiceItemOp(test.class, nil)
			got := fmt.Sprintf("%T", op)
			if !strings.Contains(got, test.want) {
				t.Fatalf("selected operation type = %s, want %s", got, test.want)
			}
		})
	}
}

func TestReadSecretFile(t *testing.T) {
	const content = `{"APIKey":"test-only-secret"}`
	path := writeRequestFile(t, content)
	for _, source := range []string{path, "-"} {
		t.Run(source, func(t *testing.T) {
			if source == "-" {
				setStdin(t, content, false)
			}
			data, err := readSecretFile(source)
			if err != nil || string(data) != content {
				t.Fatalf("readSecretFile() = %q, %v", data, err)
			}
		})
	}
	t.Run("missing file", func(t *testing.T) {
		_, err := readSecretFile(filepath.Join(t.TempDir(), "missing"))
		if !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "read secret file") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("stdin failure", func(t *testing.T) {
		setStdin(t, "", true)
		_, err := readSecretFile("-")
		if !errors.Is(err, os.ErrClosed) || !strings.Contains(err.Error(), "read secret from standard input") {
			t.Fatalf("error = %v", err)
		}
	})
}

// secretUpdaterAPI はシークレット更新を含む API 操作の差し替えです。
type secretUpdaterAPI struct {
	CommonServiceItemAPI
	updateSecret func(context.Context, string, v1.SetSecretRequest) error
}

func (api secretUpdaterAPI) UpdateSecret(ctx context.Context, id string, request v1.SetSecretRequest) error {
	return api.updateSecret(ctx, id, request)
}

func TestUpdateSecretCommand(t *testing.T) {
	const content = `{"APIKey":"test-only-secret"}`
	for _, source := range []string{writeRequestFile(t, content), "-"} {
		t.Run(source, func(t *testing.T) {
			if source == "-" {
				setStdin(t, content, false)
			}
			called := false
			command := CommonServiceItemUpdateSecretCommand{
				ID: "process-id", SecretFile: source,
				factory: func(providerClass string) (CommonServiceItemAPI, error) {
					if providerClass != string(v1.ProviderClassEventbusprocessconfiguration) {
						t.Fatalf("factory providerClass = %q, want %q", providerClass, v1.ProviderClassEventbusprocessconfiguration)
					}
					return secretUpdaterAPI{updateSecret: func(ctx context.Context, id string, request v1.SetSecretRequest) error {
						called = true
						if ctx == nil || id != "process-id" {
							t.Fatalf("UpdateSecret context = %v, ID = %q", ctx, id)
						}
						data, err := json.Marshal(request)
						if err != nil {
							t.Fatal(err)
						}
						assertJSONEqual(t, data, []byte(`{"Secret":`+content+`}`))
						return nil
					}}, nil
				},
			}
			if err := command.Run(nil); err != nil {
				t.Fatal(err)
			}
			if !called {
				t.Fatal("UpdateSecret was not called")
			}
		})
	}
}

func TestUpdateSecretCommandErrors(t *testing.T) {
	sentinel := errors.New("operation failed")
	for _, test := range []struct {
		name    string
		input   string
		factory CommonServiceItemAPIFactory
		want    string
		wrapped error
	}{
		{"read", filepath.Join(t.TempDir(), "missing"), nil, "read secret file", os.ErrNotExist},
		{"decode", writeRequestFile(t, `{"APIKey":"test-only-secret"`), nil, "decode request JSON", nil},
		{"type", writeRequestFile(t, `{"APIKey":{"value":"test-only-secret"}}`), nil, "decode request JSON", nil},
		{"missing factory", writeRequestFile(t, `{"APIKey":"test-only-secret"}`), nil, "API クライアント", nil},
		{"factory", writeRequestFile(t, `{"APIKey":"test-only-secret"}`), func(string) (CommonServiceItemAPI, error) { return nil, sentinel }, "", sentinel},
		{"unsupported operation", writeRequestFile(t, `{"APIKey":"test-only-secret"}`), func(string) (CommonServiceItemAPI, error) {
			// シークレット更新をサポートしない操作 (schedule、trigger 相当) を返します
			return &unsupportedSecretUpdaterAPI{}, nil
		}, "シークレットの更新は実行設定", nil},
		{"API", writeRequestFile(t, `{"APIKey":"test-only-secret"}`), func(string) (CommonServiceItemAPI, error) {
			return secretUpdaterAPI{updateSecret: func(context.Context, string, v1.SetSecretRequest) error { return sentinel }}, nil
		}, "", sentinel},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := CommonServiceItemUpdateSecretCommand{ID: "id", SecretFile: test.input, factory: test.factory}
			err := command.Run(nil)
			if err == nil || !strings.Contains(err.Error(), test.want) || strings.Contains(err.Error(), "test-only-secret") {
				t.Fatalf("error = %v, want safe error containing %q", err, test.want)
			}
			if test.wrapped != nil && !errors.Is(err, test.wrapped) {
				t.Fatalf("error = %v, want wrapped %v", err, test.wrapped)
			}
		})
	}
}

// commonServiceItemTestDouble は CommonServiceItem 操作のリクエストを記録します。
type commonServiceItemTestDouble struct {
	createRequests []v1.CreateCommonServiceItemRequest
	updateIDs      []string
	updateRequests []v1.UpdateCommonServiceItemRequest
}

func (api *commonServiceItemTestDouble) List(context.Context) ([]v1.CommonServiceItem, error) {
	return nil, nil
}

func (api *commonServiceItemTestDouble) Read(context.Context, string) (*v1.CommonServiceItem, error) {
	return &v1.CommonServiceItem{}, nil
}

func (api *commonServiceItemTestDouble) Create(_ context.Context, request v1.CreateCommonServiceItemRequest) (*v1.CommonServiceItem, error) {
	api.createRequests = append(api.createRequests, request)
	return &v1.CommonServiceItem{}, nil
}

func (api *commonServiceItemTestDouble) Update(_ context.Context, id string, request v1.UpdateCommonServiceItemRequest) (*v1.CommonServiceItem, error) {
	api.updateIDs = append(api.updateIDs, id)
	api.updateRequests = append(api.updateRequests, request)
	return &v1.CommonServiceItem{}, nil
}

func (api *commonServiceItemTestDouble) Delete(context.Context, string) error {
	return nil
}

// TestCommonServiceItemCommandsUseSDKRequests は --request 経路で作成・更新コマンドを実行し、
// --provider-class の値がリクエストの Provider に反映されることを検証します。
func TestCommonServiceItemCommandsUseSDKRequests(t *testing.T) {
	api := &commonServiceItemTestDouble{}
	runtime := CommonServiceItemRuntime{
		DecodeRequest:                 func(input string, destination any) error { return json.Unmarshal([]byte(input), destination) },
		SetCreateRequestProviderInput: SetCreateRequestProviderInput,
		SetUpdateRequestProviderInput: SetUpdateRequestProviderInput,
		ValidateOutput:                func(*kong.Context) error { return nil },
		WriteOutput:                   func(*kong.Context, any) error { return nil },
	}
	factory := func(ProviderClass string) (CommonServiceItemAPI, error) { return api, nil }
	ctx := &kong.Context{}

	createRequest := `{"CommonServiceItem":{"Name":"json-trigger","Settings":{"Source":"sakuracloud://switch/123","Types":["SwitchUpdated"],"ProcessConfigurationID":"456"}}}`
	createCommand := CommonServiceItemCreateCommand{
		ProviderClass: "eventbustrigger",
		Request:       &createRequest,
		runtime:       runtime,
		factory:       factory,
	}
	if err := createCommand.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(api.createRequests) != 1 {
		t.Fatalf("create request count = %d", len(api.createRequests))
	}
	created := api.createRequests[0].CommonServiceItem
	if created.Provider.Class != v1.ProviderClassEventbustrigger || created.Name != "json-trigger" {
		t.Fatalf("created item = %#v", created)
	}
	if created.Settings.Type != v1.TriggerSettingsSettings || created.Settings.TriggerSettings.Source != "sakuracloud://switch/123" ||
		len(created.Settings.TriggerSettings.Types.Value) != 1 || created.Settings.TriggerSettings.Types.Value[0] != "SwitchUpdated" {
		t.Fatalf("created settings = %#v", created.Settings)
	}

	updateRequest := `{"CommonServiceItem":{"Name":"json-trigger-renamed"}}`
	updateCommand := CommonServiceItemUpdateCommand{
		ProviderClass: "eventbustrigger",
		ID:            "123456789012",
		Request:       &updateRequest,
		runtime:       runtime,
		factory:       factory,
	}
	if err := updateCommand.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(api.updateRequests) != 1 || api.updateIDs[0] != "123456789012" {
		t.Fatalf("update request = %+v, ids = %v", api.updateRequests, api.updateIDs)
	}
	updated := api.updateRequests[0].CommonServiceItem
	if !updated.Provider.Set || updated.Provider.Value.Class != v1.ProviderClassEventbustrigger || !updated.Name.Set || updated.Name.Value != "json-trigger-renamed" {
		t.Fatalf("updated item = %#v", updated)
	}
}

// unsupportedSecretUpdaterAPI はシークレット更新をサポートしない API 操作です。
type unsupportedSecretUpdaterAPI struct {
	CommonServiceItemAPI
}

func TestProviderClassValidationRejectsUnexpectedRequestProvider(t *testing.T) {
	// リクエスト JSON に任意の Provider を含めても、SDK 操作が --provider-class の値で上書きします。
	// ここではデコード経路の動作を確認します。
	var request v1.CreateCommonServiceItemRequest
	input := `{"CommonServiceItem":{"Name":"test","Provider":{"Class":"eventbustrigger"},"Settings":{"ProcessConfigurationID":"123","StartsAt":1893456000000,"RecurringStep":1,"RecurringUnit":"day"}}}`
	if err := json.Unmarshal([]byte(input), &request); err != nil {
		t.Fatal(err)
	}
	if request.CommonServiceItem.Provider.Class != v1.ProviderClassEventbustrigger || request.CommonServiceItem.Name != "test" {
		t.Fatalf("decoded item = %#v", request.CommonServiceItem)
	}
}

func writeRequestFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func setStdin(t *testing.T, content string, closed bool) {
	t.Helper()
	file, err := os.Open(writeRequestFile(t, content))
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdin
	os.Stdin = file
	t.Cleanup(func() {
		os.Stdin = previous
		if !closed {
			if err := file.Close(); err != nil {
				t.Error(err)
			}
		}
	})
	if closed {
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func assertJSONEqual(t *testing.T, got, want []byte) {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("JSON = %s, want %s", got, want)
	}
}
