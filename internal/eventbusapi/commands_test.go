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
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
)

func TestDecodeAndValidateCreateRequest(t *testing.T) {
	for _, test := range []struct {
		name     string
		provider v1.ProviderClass
		settings string
	}{
		{"setProcessConfigurationProvider", v1.ProviderClassEventbusprocessconfiguration, `{"Destination":"simplemq","Parameters":"{\"queue_name\":\"test\",\"content\":\"message\"}"}`},
		{"setScheduleProvider", v1.ProviderClassEventbusschedule, `{"ProcessConfigurationID":"123","StartsAt":1893456000000,"RecurringStep":1,"RecurringUnit":"day"}`},
		{"setTriggerProvider", v1.ProviderClassEventbustrigger, `{"ProcessConfigurationID":"123","Source":"source","Types":["type"]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, provider := range []string{"", `,"Provider":{"Class":"eventbustrigger"}`} {
				input := `{"CommonServiceItem":{"Name":"test","Settings":` + test.settings + provider + `}}`
				for _, source := range []string{input, "@" + writeRequestFile(t, input)} {
					var request v1.CreateCommonServiceItemRequest
					if err := DecodeRequest(source, &request); err != nil {
						t.Fatal(err)
					}
					if request.CommonServiceItem.Provider.Class != v1.ProviderClassEventbusprocessconfiguration {
						t.Fatalf("decoded Provider = %v", request.CommonServiceItem.Provider)
					}
					before, err := json.Marshal(request.CommonServiceItem.Settings)
					if err != nil {
						t.Fatal(err)
					}
					if err := ValidateRequest(test.name, &request); err != nil {
						t.Fatal(err)
					}
					if request.CommonServiceItem.Provider.Class != test.provider || request.CommonServiceItem.Name != "test" {
						t.Fatalf("validated item = %#v", request.CommonServiceItem)
					}
					after, err := json.Marshal(request.CommonServiceItem.Settings)
					if err != nil {
						t.Fatal(err)
					}
					assertJSONEqual(t, before, []byte(test.settings))
					assertJSONEqual(t, after, before)
				}
			}
		})
	}
}

func TestWithProviderPreservesItemFields(t *testing.T) {
	input := `{"CommonServiceItem":{"Name":"test","Description":"description","Tags":["tag"],"Settings":{"nested":{"values":[1,true,null]},"custom":"value"},"Provider":{"Class":"eventbustrigger"}}}`
	for _, provider := range []v1.ProviderClass{
		v1.ProviderClassEventbusprocessconfiguration,
		v1.ProviderClassEventbusschedule,
		v1.ProviderClassEventbustrigger,
	} {
		data, err := withProvider([]byte(input), provider)
		if err != nil {
			t.Fatal(err)
		}
		var got, want map[string]map[string]json.RawMessage
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(input), &want); err != nil {
			t.Fatal(err)
		}
		assertJSONEqual(t, got["CommonServiceItem"]["Provider"], []byte(`{"Class":"`+string(provider)+`"}`))
		delete(got["CommonServiceItem"], "Provider")
		delete(want["CommonServiceItem"], "Provider")
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("item fields changed: got %s", data)
		}
	}
}

func TestDecodeRequestErrors(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		want  string
	}{
		{"malformed", `{"CommonServiceItem":`, "decode request JSON"},
		{"trailing", `{"CommonServiceItem":{}} {}`, "decode request JSON"},
		{"missing item", `{}`, "decode CommonServiceItem"},
		{"null item", `{"CommonServiceItem":null}`, "expected JSON object"},
		{"string item", `{"CommonServiceItem":"item"}`, "decode CommonServiceItem"},
		{"array item", `{"CommonServiceItem":[]}`, "decode CommonServiceItem"},
		{"number item", `{"CommonServiceItem":1}`, "decode CommonServiceItem"},
		{"wrong field type", `{"CommonServiceItem":{"Name":123}}`, "decode request JSON"},
		{"missing file", "@" + filepath.Join(t.TempDir(), "missing.json"), "read request file"},
		{"malformed file", "@" + writeRequestFile(t, `{`), "decode request JSON"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var request v1.CreateCommonServiceItemRequest
			err := DecodeRequest(test.input, &request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if test.name == "missing file" && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("error = %v, want os.ErrNotExist", err)
			}
		})
	}
}

func TestDecodeUpdateRequestDoesNotSetProvider(t *testing.T) {
	var request v1.UpdateCommonServiceItemRequest
	if err := DecodeRequest(`{"CommonServiceItem":{"Name":"updated"}}`, &request); err != nil {
		t.Fatal(err)
	}
	if request.CommonServiceItem.Name.Value != "updated" {
		t.Fatalf("Name = %#v", request.CommonServiceItem.Name)
	}
}

func TestValidateRequestErrors(t *testing.T) {
	if err := ValidateRequest("unknown", &v1.CreateCommonServiceItemRequest{}); err == nil || !strings.Contains(err.Error(), "unknown EventBus request validator") {
		t.Fatalf("unknown validator error = %v", err)
	}
	for _, name := range []string{"setProcessConfigurationProvider", "setScheduleProvider", "setTriggerProvider"} {
		for _, request := range []any{nil, v1.CreateCommonServiceItemRequest{}, &v1.UpdateCommonServiceItemRequest{}} {
			if err := ValidateRequest(name, request); err == nil || !strings.Contains(err.Error(), "create requests") {
				t.Fatalf("ValidateRequest(%q, %T) error = %v", name, request, err)
			}
		}
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

type secretAPI struct {
	ProcessConfigurationAPI
	updateSecret func(context.Context, string, v1.SetSecretRequest) error
}

func (api secretAPI) UpdateSecret(ctx context.Context, id string, request v1.SetSecretRequest) error {
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
			command := ProcessConfigurationUpdateSecretCommand{
				ID: "process-id", SecretFile: source,
				factory: func() (ProcessConfigurationAPI, error) {
					return secretAPI{updateSecret: func(ctx context.Context, id string, request v1.SetSecretRequest) error {
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
		factory ProcessConfigurationAPIFactory
		want    string
		wrapped error
	}{
		{"read", filepath.Join(t.TempDir(), "missing"), nil, "read secret file", os.ErrNotExist},
		{"decode", writeRequestFile(t, `{"APIKey":"test-only-secret"`), nil, "decode request JSON", nil},
		{"type", writeRequestFile(t, `{"APIKey":{"value":"test-only-secret"}}`), nil, "decode request JSON", nil},
		{"missing factory", writeRequestFile(t, `{"APIKey":"test-only-secret"}`), nil, "API クライアント", nil},
		{"factory", writeRequestFile(t, `{"APIKey":"test-only-secret"}`), func() (ProcessConfigurationAPI, error) { return nil, sentinel }, "", sentinel},
		{"API", writeRequestFile(t, `{"APIKey":"test-only-secret"}`), func() (ProcessConfigurationAPI, error) {
			return secretAPI{updateSecret: func(context.Context, string, v1.SetSecretRequest) error { return sentinel }}, nil
		}, "", sentinel},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := ProcessConfigurationUpdateSecretCommand{ID: "id", SecretFile: test.input, factory: test.factory}
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
