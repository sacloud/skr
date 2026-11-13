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

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	localrouter "github.com/sacloud/sacloud-sdk-go/service/iaas/localrouter"
	"github.com/sacloud/skr/internal/localrouterapi"
	"github.com/stretchr/testify/require"
)

type localRouterAPITestDouble struct {
	findRequest           *localrouter.FindRequest
	readRequest           *localrouter.ReadRequest
	createRequest         *localrouter.CreateRequest
	updateRequest         *localrouter.UpdateRequest
	updateSettingsRequest *localrouterapi.UpdateSettingsRequest
	deleteRequest         *localrouter.DeleteRequest
	healthRequest         *localrouter.HealthRequest
	monitorRequest        *localrouter.MonitorLocalRouterRequest
	err                   error
}

func (api *localRouterAPITestDouble) FindWithContext(_ context.Context, request *localrouter.FindRequest) ([]*iaas.LocalRouter, error) {
	api.findRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return []*iaas.LocalRouter{{ID: 123, Name: "test-localrouter"}}, nil
}

func (api *localRouterAPITestDouble) ReadWithContext(_ context.Context, request *localrouter.ReadRequest) (*iaas.LocalRouter, error) {
	api.readRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return localRouterFixture(), nil
}

func (api *localRouterAPITestDouble) CreateWithContext(_ context.Context, request *localrouter.CreateRequest) (*iaas.LocalRouter, error) {
	api.createRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return localRouterFixture(), nil
}

func (api *localRouterAPITestDouble) UpdateWithContext(_ context.Context, request *localrouter.UpdateRequest) (*iaas.LocalRouter, error) {
	api.updateRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return localRouterFixture(), nil
}

func (api *localRouterAPITestDouble) UpdateSettingsWithContext(_ context.Context, request *localrouterapi.UpdateSettingsRequest) (*iaas.LocalRouter, error) {
	api.updateSettingsRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return localRouterFixture(), nil
}

func (api *localRouterAPITestDouble) DeleteWithContext(_ context.Context, request *localrouter.DeleteRequest) error {
	api.deleteRequest = request
	return api.err
}

func (api *localRouterAPITestDouble) HealthWithContext(_ context.Context, request *localrouter.HealthRequest) (*iaas.LocalRouterHealth, error) {
	api.healthRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return &iaas.LocalRouterHealth{Peers: []*iaas.LocalRouterHealthPeer{
		{ID: 1, Status: types.EServerInstanceStatus("up"), Routes: []string{"10.0.0.0/24"}},
	}}, nil
}

func (api *localRouterAPITestDouble) MonitorLocalRouterWithContext(_ context.Context, request *localrouter.MonitorLocalRouterRequest) ([]*iaas.MonitorLocalRouterValue, error) {
	api.monitorRequest = request
	if api.err != nil {
		return nil, api.err
	}
	return []*iaas.MonitorLocalRouterValue{
		{Time: time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC), ReceiveBytesPerSec: 1.5, SendBytesPerSec: 2.5},
	}, nil
}

func localRouterFixture() *iaas.LocalRouter {
	return &iaas.LocalRouter{
		ID:           123,
		Name:         "test-localrouter",
		Switch:       &iaas.LocalRouterSwitch{Code: "123456789012", Category: "cloud", ZoneID: "is1b"},
		Interface:    &iaas.LocalRouterInterface{VirtualIPAddress: "192.0.2.1", IPAddress: []string{"192.0.2.2"}, NetworkMaskLen: 24, VRID: 100},
		StaticRoutes: []*iaas.LocalRouterStaticRoute{{Prefix: "10.0.0.0/24", NextHop: "192.0.2.254"}},
		SettingsHash: "hash1",
	}
}

func setLocalRouterAPIFactory(commandLine *cli, factory localrouterapi.APIFactory) {
	commandLine.LocalRouterAPI = localrouterapi.NewCommands(localrouterapi.Runtime{
		DecodeRequest:          decodeRequest,
		DecodeProtectedRequest: decodeProtectedRequest,
		ValidateOutput:         validateOutput,
		WriteOutput:            writeOutput,
	}, factory)
}

func TestRunLocalRouterAPIHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{[]string{"local-router-api", "--help"}, []string{"find", "read", "create", "update",
			"update-settings", "delete", "health-status", "monitor-local-router", "ローカルルータ", "グローバルリソース"}},
		{[]string{"local-router-api", "find", "--help"}, []string{"--name", "--tags", "--count", "--from",
			"部分一致検索", "@path.json"}},
		{[]string{"local-router-api", "create", "--help"}, []string{"--name", "--description", "--tags",
			"--switch-id", "--switch-category", "--switch-zone-id", "--interface-virtual-ip-address",
			"--interface-ip-address", "--interface-network-mask-len", "--interface-vrid",
			"prefix=CIDR,next-hop=IP", "--request", "@path.json", "標準入力", "秘密情報",
			"スイッチとインターフェースの両方が必要"}},
		{[]string{"local-router-api", "update", "--help"}, []string{"--id", "省略した設定は変更しません",
			"一覧全体を置き換えます", "--static-route"}},
		{[]string{"local-router-api", "update-settings", "--help"}, []string{"--id", "--switch-id",
			"--interface-virtual-ip-address", "--static-route", "生の API 操作"}},
		{[]string{"local-router-api", "delete", "--help"}, []string{"--id", "--fail-if-not-found"}},
		{[]string{"local-router-api", "health-status", "--help"}, []string{"--id", "ピア", "ルート"}},
		{[]string{"local-router-api", "monitor-local-router", "--help"}, []string{"--id", "--start", "--end", "RFC3339"}},
	} {
		t.Run(strings.Join(test.args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, want := range test.want {
				require.Contains(t, help, want, "help does not contain %q: %s", want, help)
			}
		})
	}
}

func TestLocalRouterCommandOperations(t *testing.T) {
	api := &localRouterAPITestDouble{}
	commandLine := newCLI()
	setLocalRouterAPIFactory(commandLine, func() (localrouterapi.API, error) { return api, nil })

	run := func(args ...string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, runCLI(args, &stdout, &stderr, commandLine), "stderr: %s", stderr.String())
		require.Empty(t, stderr.String(), "run(%v) stderr=%q", args, stderr.String())
		return stdout.Bytes()
	}

	listed := run("local-router-api", "find", "--name", "test-localrouter", "--tags", "web,production",
		"--count", "10", "--from", "5", "--query", "map({ID,Name})")
	var projection []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(listed, &projection), "find output = %s", listed)
	require.Len(t, projection, 1)
	require.JSONEq(t, `"test-localrouter"`, string(projection[0]["Name"]))
	require.NotNil(t, api.findRequest, "find request = %+v", api.findRequest)
	require.Equal(t, []string{"test-localrouter"}, api.findRequest.Names)
	require.Equal(t, []string{"web", "production"}, api.findRequest.Tags)
	require.Equal(t, 10, api.findRequest.Count)
	require.Equal(t, 5, api.findRequest.From)

	created := run("local-router-api", "create", "--name", "test-localrouter",
		"--description", "desc", "--tags", "web", "--icon-id", "9",
		"--switch-id", "123456789012", "--switch-category", "cloud", "--switch-zone-id", "is1b",
		"--interface-virtual-ip-address", "192.0.2.1",
		"--interface-ip-address", "192.0.2.2,192.0.2.3",
		"--interface-network-mask-len", "24", "--interface-vrid", "100",
		"--static-route", "prefix=10.0.0.0/24,next-hop=192.0.2.254",
		"--static-route", "prefix=10.1.0.0/24,next-hop=192.0.2.253")
	var createResult iaas.LocalRouter
	require.NoError(t, json.Unmarshal(created, &createResult), "create output = %s", created)
	require.EqualValues(t, 123, createResult.ID)
	require.NotNil(t, api.createRequest, "create request = %+v", api.createRequest)
	require.Equal(t, "test-localrouter", api.createRequest.Name)
	require.Equal(t, "desc", api.createRequest.Description)
	require.Equal(t, []string{"web"}, []string(api.createRequest.Tags))
	require.EqualValues(t, 9, api.createRequest.IconID)
	require.Equal(t, &iaas.LocalRouterSwitch{Code: "123456789012", Category: "cloud", ZoneID: "is1b"}, api.createRequest.Switch)
	require.Equal(t, &iaas.LocalRouterInterface{
		VirtualIPAddress: "192.0.2.1",
		IPAddress:        []string{"192.0.2.2", "192.0.2.3"},
		NetworkMaskLen:   24,
		VRID:             100,
	}, api.createRequest.Interface)
	require.Len(t, api.createRequest.StaticRoutes, 2)
	require.Equal(t, &iaas.LocalRouterStaticRoute{Prefix: "10.0.0.0/24", NextHop: "192.0.2.254"}, api.createRequest.StaticRoutes[0])
	require.Equal(t, &iaas.LocalRouterStaticRoute{Prefix: "10.1.0.0/24", NextHop: "192.0.2.253"}, api.createRequest.StaticRoutes[1])

	run("local-router-api", "read", "--id", "123")
	require.NotNil(t, api.readRequest, "read request = %+v", api.readRequest)
	require.EqualValues(t, 123, api.readRequest.ID)

	run("local-router-api", "update", "--id", "123", "--name", "renamed", "--description", "")
	require.NotNil(t, api.updateRequest, "update request = %+v", api.updateRequest)
	require.EqualValues(t, 123, api.updateRequest.ID)
	require.NotNil(t, api.updateRequest.Name)
	require.Equal(t, "renamed", *api.updateRequest.Name)
	require.NotNil(t, api.updateRequest.Description)
	require.Empty(t, *api.updateRequest.Description)
	require.Nil(t, api.updateRequest.Tags, "omitted update fields were populated: %+v", api.updateRequest)
	require.Nil(t, api.updateRequest.IconID, "omitted update fields were populated: %+v", api.updateRequest)
	require.Nil(t, api.updateRequest.Switch, "omitted update fields were populated: %+v", api.updateRequest)
	require.Nil(t, api.updateRequest.Interface, "omitted update fields were populated: %+v", api.updateRequest)
	require.Nil(t, api.updateRequest.StaticRoutes, "omitted update fields were populated: %+v", api.updateRequest)

	run("local-router-api", "update-settings", "--id", "123",
		"--switch-id", "123456789013",
		"--interface-virtual-ip-address", "192.0.2.11",
		"--interface-ip-address", "192.0.2.12",
		"--interface-network-mask-len", "25", "--interface-vrid", "200",
		"--static-route", "prefix=10.2.0.0/24,next-hop=192.0.2.252")
	require.NotNil(t, api.updateSettingsRequest, "update-settings request = %+v", api.updateSettingsRequest)
	require.EqualValues(t, 123, api.updateSettingsRequest.ID)
	require.Equal(t, &iaas.LocalRouterSwitch{Code: "123456789013"}, api.updateSettingsRequest.Switch)
	require.Equal(t, &iaas.LocalRouterInterface{
		VirtualIPAddress: "192.0.2.11",
		IPAddress:        []string{"192.0.2.12"},
		NetworkMaskLen:   25,
		VRID:             200,
	}, api.updateSettingsRequest.Interface)
	require.Nil(t, api.updateSettingsRequest.Peers, "omitted settings were populated: %+v", api.updateSettingsRequest)
	require.NotNil(t, api.updateSettingsRequest.StaticRoutes)
	require.Len(t, *api.updateSettingsRequest.StaticRoutes, 1)
	require.Equal(t, "10.2.0.0/24", (*api.updateSettingsRequest.StaticRoutes)[0].Prefix)

	run("local-router-api", "delete", "--id", "123", "--fail-if-not-found")
	require.NotNil(t, api.deleteRequest, "delete request = %+v", api.deleteRequest)
	require.EqualValues(t, 123, api.deleteRequest.ID)
	require.True(t, api.deleteRequest.FailIfNotFound)

	health := run("local-router-api", "health-status", "--id", "123")
	var healthResult iaas.LocalRouterHealth
	require.NoError(t, json.Unmarshal(health, &healthResult), "health output = %s", health)
	require.Len(t, healthResult.Peers, 1)
	require.EqualValues(t, 1, healthResult.Peers[0].ID)
	require.Equal(t, types.EServerInstanceStatus("up"), healthResult.Peers[0].Status)
	require.Equal(t, []string{"10.0.0.0/24"}, healthResult.Peers[0].Routes)

	monitored := run("local-router-api", "monitor-local-router", "--id", "123",
		"--start", "2026-11-13T09:00:00Z", "--end", "2026-11-13T10:00:00Z")
	var monitorResult []*iaas.MonitorLocalRouterValue
	require.NoError(t, json.Unmarshal(monitored, &monitorResult), "monitor output = %s", monitored)
	require.Len(t, monitorResult, 1)
	require.InDelta(t, 1.5, monitorResult[0].ReceiveBytesPerSec, 0.0001)
	require.InDelta(t, 2.5, monitorResult[0].SendBytesPerSec, 0.0001)
	require.NotNil(t, api.monitorRequest, "monitor request = %+v", api.monitorRequest)
	require.Equal(t, "2026-11-13T09:00:00Z", api.monitorRequest.Start.UTC().Format("2006-01-02T15:04:05Z"))
	require.Equal(t, "2026-11-13T10:00:00Z", api.monitorRequest.End.UTC().Format("2006-01-02T15:04:05Z"))

	// --start/--end を省略すると SDK の既定値 (end の1時間前) が使われます
	run("local-router-api", "monitor-local-router", "--id", "123")
	require.True(t, api.monitorRequest.Start.IsZero(), "monitor request = %+v", api.monitorRequest)
	require.True(t, api.monitorRequest.End.IsZero(), "monitor request = %+v", api.monitorRequest)
}

func TestLocalRouterJSONInputAndMutualExclusion(t *testing.T) {
	api := &localRouterAPITestDouble{}
	commandLine := newCLI()
	setLocalRouterAPIFactory(commandLine, func() (localrouterapi.API, error) { return api, nil })

	run := func(args ...string) (int, string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runCLI(args, &stdout, &stderr, commandLine)
		return code, stderr.String()
	}

	// Create / Update / UpdateSettings の JSON は Peers[].SecretKey など秘密情報を
	// 受け取れるため、保護したファイルまたは標準入力のみを受け付け、直接指定は拒否します
	code, stderr := run("local-router-api", "create", "--request",
		`{"Name":"json-lr","Switch":{"Code":"123456789012"},"Interface":{"VirtualIPAddress":"192.0.2.1","IPAddress":["192.0.2.2"],"NetworkMaskLen":24,"VRID":100}}`)
	require.Equal(t, 1, code, "inline JSON create code=%d stderr=%q", code, stderr)
	require.Contains(t, stderr, "--request は @path.json または -")
	// エラーには指定した値を含めない
	require.NotContains(t, stderr, "json-lr")
	require.Nil(t, api.createRequest, "inline JSON must not reach the API")

	code, stderr = run("local-router-api", "update", "--request", `{"ID":123}`)
	require.Equal(t, 1, code, "inline JSON update code=%d stderr=%q", code, stderr)
	require.Contains(t, stderr, "--request は @path.json または -")
	require.Nil(t, api.updateRequest, "inline JSON must not reach the API")

	code, stderr = run("local-router-api", "update-settings", "--id", "123", "--request", `{"ID":123}`)
	require.Equal(t, 1, code, "inline JSON update-settings code=%d stderr=%q", code, stderr)
	require.Contains(t, stderr, "--request は @path.json または -")
	require.Nil(t, api.updateSettingsRequest, "inline JSON must not reach the API")

	dir := t.TempDir()
	createPath := filepath.Join(dir, "create.json")
	createJSON := `{"Name":"json-lr","Switch":{"Code":"123456789012","Category":"cloud","ZoneID":"is1b"},"Interface":{"VirtualIPAddress":"192.0.2.1","IPAddress":["192.0.2.2"],"NetworkMaskLen":24,"VRID":100},"Peers":[{"ID":1,"SecretKey":"peer-secret","Enabled":true}]}`
	require.NoError(t, os.WriteFile(createPath, []byte(createJSON), 0o600))
	code, stderr = run("local-router-api", "create", "--request", "@"+createPath)
	require.Equal(t, 0, code, "JSON create code=%d stderr=%q", code, stderr)
	require.Empty(t, stderr)
	require.NotNil(t, api.createRequest, "JSON create request = %+v", api.createRequest)
	require.Equal(t, "json-lr", api.createRequest.Name)
	require.NotNil(t, api.createRequest.Switch)
	require.Len(t, api.createRequest.Peers, 1)
	require.Equal(t, "peer-secret", api.createRequest.Peers[0].SecretKey)

	updatePath := filepath.Join(dir, "update.json")
	require.NoError(t, os.WriteFile(updatePath, []byte(`{"ID":123,"Description":"json-updated","StaticRoutes":[{"Prefix":"10.0.0.0/24","NextHop":"192.0.2.254"}]}`), 0o600))
	code, stderr = run("local-router-api", "update", "--request", "@"+updatePath)
	require.Equal(t, 0, code, "JSON update code=%d stderr=%q", code, stderr)
	require.NotNil(t, api.updateRequest, "JSON update request = %+v", api.updateRequest)
	require.EqualValues(t, 123, api.updateRequest.ID)
	require.NotNil(t, api.updateRequest.Description)
	require.NotNil(t, api.updateRequest.StaticRoutes)

	// 空配列の指定は一覧の消去になります。--id は --request と併用できます
	settingsPath := filepath.Join(dir, "settings.json")
	require.NoError(t, os.WriteFile(settingsPath, []byte(`{"ID":123,"StaticRoutes":[],"Peers":[]}`), 0o600))
	code, stderr = run("local-router-api", "update-settings", "--id", "123", "--request", "@"+settingsPath)
	require.Equal(t, 0, code, "JSON update-settings code=%d stderr=%q", code, stderr)
	require.NotNil(t, api.updateSettingsRequest, "JSON update-settings request = %+v", api.updateSettingsRequest)
	require.EqualValues(t, 123, api.updateSettingsRequest.ID)
	require.NotNil(t, api.updateSettingsRequest.StaticRoutes)
	require.Empty(t, *api.updateSettingsRequest.StaticRoutes)
	require.NotNil(t, api.updateSettingsRequest.Peers)
	require.Empty(t, *api.updateSettingsRequest.Peers)

	// リクエスト JSON の ID と --id が不一致の場合はエラーになります
	sent := api.updateSettingsRequest
	code, stderr = run("local-router-api", "update-settings", "--id", "456", "--request", "@"+settingsPath)
	require.Equal(t, 1, code, "ID mismatch code=%d stderr=%q", code, stderr)
	require.Contains(t, stderr, "--id とリクエスト JSON の ID が一致しません")
	require.Same(t, sent, api.updateSettingsRequest, "mismatched ID must not reach the API")

	// --request と個別の設定フラグは併用できません (--id は併用できます)
	for _, args := range [][]string{
		{"local-router-api", "create", "--request", "@" + createPath, "--name", "x", "--request と個別フラグ"},
		{"local-router-api", "update", "--request", "@" + updatePath, "--description", "d", "--request と個別フラグ"},
		{"local-router-api", "update-settings", "--request", "@" + settingsPath, "--switch-id", "123456789012", "--request と個別フラグ"},
		{"local-router-api", "update-settings", "--request", "@" + settingsPath, "--switch-id", "123456789012", "--request と個別フラグ"},
		{"local-router-api", "find", "--request", `{"Names":["x"]}`, "--name", "x", "--request と個別フラグ"},
		{"local-router-api", "read", "--request", `{"ID":123}`, "--id", "123", "--request と --id"},
		{"local-router-api", "delete", "--request", `{"ID":123}`, "--id", "123", "--request と個別フラグ"},
		{"local-router-api", "health-status", "--request", `{"ID":123}`, "--id", "123", "--request と --id"},
		{"local-router-api", "monitor-local-router", "--request", `{"ID":123}`, "--start", "2026-11-13T09:00:00Z", "--request と個別フラグ"},
	} {
		code, stderr = run(args...)
		require.Equal(t, 1, code, "run(%v) = code %d, stderr %q", args, code, stderr)
		want := args[len(args)-1]
		require.Contains(t, stderr, want, "missing expected text %q in %q", want, stderr)
	}

	// 検索条件はすべて任意です。条件なしの find は空の FindRequest を渡して全件検索できます
	code, stderr = run("local-router-api", "find")
	require.Equal(t, 0, code, "find without conditions code=%d stderr=%q", code, stderr)
	require.NotNil(t, api.findRequest, "find without conditions must reach the API")
	require.Empty(t, api.findRequest.Names, "find without conditions should pass an empty FindRequest")
}

func TestLocalRouterFlagValidationErrors(t *testing.T) {
	api := &localRouterAPITestDouble{}
	commandLine := newCLI()
	setLocalRouterAPIFactory(commandLine, func() (localrouterapi.API, error) { return api, nil })

	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"local-router-api", "create", "--switch-id", "123456789012"}, "--name または --request が必要です"},
		{[]string{"local-router-api", "create", "--name", "x", "--switch-category", "cloud"}, "--switch-id が必要です"},
		{[]string{"local-router-api", "create", "--name", "x", "--switch-id", "123456789012"},
			"接続設定にはスイッチとインターフェースの両方が必要です"},
		{[]string{"local-router-api", "create", "--name", "x", "--switch-id", "123456789012",
			"--interface-virtual-ip-address", "192.0.2.1", "--interface-ip-address", "192.0.2.2",
			"--interface-network-mask-len", "24"}, "--interface-vrid が必要です"},
		{[]string{"local-router-api", "create", "--name", "x", "--static-route", "prefix=10.0.0.0/24"},
			"next-hop is required"},
		{[]string{"local-router-api", "create", "--name", "x", "--static-route", "unknown=1"},
			"unknown key"},
		{[]string{"local-router-api", "update", "--id", "123"}, "更新するフラグが必要です"},
		{[]string{"local-router-api", "read"}, "--id または --request が必要です"},
		{[]string{"local-router-api", "update-settings", "--switch-id", "123456789012"}, "--id が必要です"},
		{[]string{"local-router-api", "update-settings"}, "--id が必要です"},
		{[]string{"local-router-api", "update-settings", "--request", "{\"Peers\":[]}"}, "--id が必要です"},
		{[]string{"local-router-api", "monitor-local-router", "--id", "123", "--start", "invalid"}, "RFC3339"},
	} {
		t.Run(strings.Join(test.args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runCLI(test.args, &stdout, &stderr, commandLine)
			require.Equal(t, 1, code, "run(%v) = code %d, stdout %q, stderr %q", test.args, code, stdout.String(), stderr.String())
			require.Empty(t, stdout.String(), "run(%v) stdout=%q", test.args, stdout.String())
			require.Contains(t, stderr.String(), test.want, "missing expected text %q in %q", test.want, stderr.String())
			// エラーには秘密情報になり得る値を含めない
			require.NotContains(t, stderr.String(), "peer-secret")
		})
	}
	require.Nil(t, api.createRequest, "validation errors must not reach the API")
	require.Nil(t, api.updateRequest, "validation errors must not reach the API")
}

// 繰り返し指定した --interface-ip-address は kong で累積されることを確認する
// (*[]string だと最後の指定のみ有効になるため、意図的に []string を使っている)
func TestLocalRouterInterfaceIPAddressRepeatable(t *testing.T) {
	api := &localRouterAPITestDouble{}
	commandLine := newCLI()
	setLocalRouterAPIFactory(commandLine, func() (localrouterapi.API, error) { return api, nil })

	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"local-router-api", "create", "--name", "test-localrouter",
		"--switch-id", "123456789012", "--switch-category", "cloud", "--switch-zone-id", "is1b",
		"--interface-virtual-ip-address", "192.0.2.1",
		"--interface-ip-address", "192.0.2.2", "--interface-ip-address", "192.0.2.3",
		"--interface-network-mask-len", "24", "--interface-vrid", "100"},
		&stdout, &stderr, commandLine)
	require.Equal(t, 0, code, "stdout %q, stderr %q", stdout.String(), stderr.String())
	require.NotNil(t, api.createRequest.Interface)
	require.Equal(t, []string{"192.0.2.2", "192.0.2.3"}, api.createRequest.Interface.IPAddress)
}

type localRouterAPICall struct {
	method string
	uri    string
	body   map[string]any
}

type localRouterAPICaller struct {
	calls []localRouterAPICall
}

func (caller *localRouterAPICaller) Do(_ context.Context, method, uri string, body any) ([]byte, error) {
	call := localRouterAPICall{method: method, uri: uri}
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &call.body); err != nil {
			return nil, err
		}
	}
	caller.calls = append(caller.calls, call)
	return []byte(`{"CommonServiceItem":{"ID":321,"Name":"sdk-created","SettingsHash":"hash1","Settings":{"LocalRouter":{"Switch":{"Code":"123456789012","Category":"cloud","ZoneID":"is1b"},"Interface":{"VirtualIPAddress":"192.0.2.1","IPAddress":["192.0.2.2"],"NetworkMaskLen":24,"VRID":100},"StaticRoutes":[{"Prefix":"10.0.0.0/24","NextHop":"192.0.2.254"}]}},"Status":{"SecretKeys":[]}}}`), nil
}

func TestLocalRouterCLIUsesSDKRequestConversion(t *testing.T) {
	caller := &localRouterAPICaller{}
	commandLine := newCLI()
	setLocalRouterAPIFactory(commandLine, func() (localrouterapi.API, error) {
		return localrouterapi.NewSDKAPI(caller), nil
	})

	var stdout, stderr bytes.Buffer
	args := []string{
		"local-router-api", "create", "--name", "sdk-created",
		"--switch-id", "123456789012", "--switch-category", "cloud", "--switch-zone-id", "is1b",
		"--interface-virtual-ip-address", "192.0.2.1",
		"--interface-ip-address", "192.0.2.2",
		"--interface-network-mask-len", "24", "--interface-vrid", "100",
		"--static-route", "prefix=10.0.0.0/24,next-hop=192.0.2.254",
	}
	require.Equal(t, 0, runCLI(args, &stdout, &stderr, commandLine), "stderr: %s", stderr.String())
	// service 層の Create は POST と設定更新の PUT を送ります
	// service 層の Create は POST (名前などのメタデータ) と設定更新の PUT を送ります
	require.Len(t, caller.calls, 2, "SDK calls = %#v", caller.calls)
	first, last := caller.calls[0], caller.calls[1]
	require.Equal(t, "POST", first.method, "first call = %s %s %s", first.method, first.uri, first.body)
	require.Equal(t, "PUT", last.method, "last call = %s %s %s", last.method, last.uri, last.body)
	require.Contains(t, first.uri, "/commonserviceitem", "missing expected text %q", "/commonserviceitem")
	firstItem, ok := first.body["CommonServiceItem"].(map[string]any)
	require.True(t, ok, "first SDK request body = %#v", first.body)
	require.Equal(t, "sdk-created", firstItem["Name"])
	item, ok := last.body["CommonServiceItem"].(map[string]any)
	require.True(t, ok, "last SDK request body = %#v", last.body)
	settings := item["Settings"].(map[string]any)
	localRouterSettings := settings["LocalRouter"].(map[string]any)
	sw := localRouterSettings["Switch"].(map[string]any)
	iface := localRouterSettings["Interface"].(map[string]any)
	routes := localRouterSettings["StaticRoutes"].([]any)
	require.Equal(t, "123456789012", sw["Code"])
	require.Equal(t, "192.0.2.1", iface["VirtualIPAddress"])
	require.Len(t, routes, 1)
	require.Equal(t, "10.0.0.0/24", routes[0].(map[string]any)["Prefix"])

	var result iaas.LocalRouter
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result), "decoded SDK result = %s", stdout.String())
	require.EqualValues(t, 321, result.ID)
	require.Equal(t, "sdk-created", result.Name)
}
