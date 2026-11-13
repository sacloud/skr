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

package localrouterapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	localrouter "github.com/sacloud/sacloud-sdk-go/service/iaas/localrouter"
	"github.com/stretchr/testify/require"
)

const connectedReadResponse = `{"CommonServiceItem":{"ID":123,"Name":"current","Description":"current-desc","Tags":["keep"],"SettingsHash":"hash1","Settings":{"LocalRouter":{"Switch":{"Code":"123456789010","Category":"cloud","ZoneID":"is1b"},"Interface":{"VirtualIPAddress":"192.0.2.1","IPAddress":["192.0.2.2","192.0.2.3"],"NetworkMaskLen":24,"VRID":100},"StaticRoutes":[{"Prefix":"10.0.0.0/24","NextHop":"192.0.2.254"}]}}}}`

const disconnectedReadResponse = `{"CommonServiceItem":{"ID":123,"Name":"current","Description":"current-desc","SettingsHash":"hash1","Settings":{"LocalRouter":{"StaticRoutes":[{"Prefix":"10.0.0.0/24","NextHop":"192.0.2.254"}]}}}}`

// apiCall は SDK が発行した 1 回の HTTP リクエストを記録する。
type apiCall struct {
	method string
	path   string
	body   any
}

// apiCaller は iaas.APICaller のテストダブル。HTTP 通信をせず、
// リクエストを記録してパスとメソッドに対応した応答を返す。
type apiCaller struct {
	calls      []apiCall
	readBody   string // GET /commonserviceitem/123 の応答。空なら接続設定を持つ現状値を返す
	createBody string // POST /commonserviceitem の応答。空なら既定値を返す
}

func (c *apiCaller) Do(_ context.Context, method, uri string, body any) ([]byte, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	c.calls = append(c.calls, apiCall{method: method, path: parsed.Path, body: body})

	switch {
	case strings.HasSuffix(parsed.Path, "/commonserviceitem") && method == "GET":
		return []byte(`{"Total":1,"Count":1,"CommonServiceItems":[{"ID":123,"Name":"current","Description":"current-desc","Tags":["keep"],"SettingsHash":"hash1","Settings":{"LocalRouter":{"Switch":{"Code":"123456789010","Category":"cloud","ZoneID":"is1b"},"Interface":{"VirtualIPAddress":"192.0.2.1","IPAddress":["192.0.2.2","192.0.2.3"],"NetworkMaskLen":24,"VRID":100},"StaticRoutes":[{"Prefix":"10.0.0.0/24","NextHop":"192.0.2.254"}]}}}]}`), nil
	case strings.HasSuffix(parsed.Path, "/commonserviceitem") && method == "POST":
		if c.createBody != "" {
			return []byte(c.createBody), nil
		}
		return []byte(`{"CommonServiceItem":{"ID":123,"Name":"created"}}`), nil
	case strings.HasSuffix(parsed.Path, "/commonserviceitem/123") && method == "GET":
		if c.readBody != "" {
			return []byte(c.readBody), nil
		}
		return []byte(connectedReadResponse), nil
	case strings.HasSuffix(parsed.Path, "/commonserviceitem/123") && method == "PUT":
		return []byte(`{"CommonServiceItem":{"ID":123,"Name":"current","Description":"current-desc","Tags":["keep"],"SettingsHash":"hash2","Settings":{"LocalRouter":{"Switch":{"Code":"123456789010","Category":"cloud","ZoneID":"is1b"},"Interface":{"VirtualIPAddress":"192.0.2.1","IPAddress":["192.0.2.2","192.0.2.3"],"NetworkMaskLen":24,"VRID":100},"StaticRoutes":[{"Prefix":"10.0.0.0/24","NextHop":"192.0.2.254"}]}}}}`), nil
	case strings.HasSuffix(parsed.Path, "/commonserviceitem/123") && method == "DELETE":
		return nil, nil
	case strings.HasSuffix(parsed.Path, "/commonserviceitem/123/health") && method == "GET":
		return []byte(`{"LocalRouter":{"Peers":[{"ID":1,"Status":"up","Routes":["10.0.0.0/24 via 192.0.2.254"]}]}}`), nil
	case strings.HasSuffix(parsed.Path, "/commonserviceitem/123/activity/localrouter/monitor") && method == "GET":
		return []byte(`{"Data":{"2026-11-13T10:00:00+09:00":{"receiveBytesPerSec":1.5,"sendBytesPerSec":2.5}}}`), nil
	default:
		return nil, fmt.Errorf("unexpected request %s %s", method, parsed.Path)
	}
}

// requestBody は記録したリクエスト本文を JSON にして返す。
func requestBody(t *testing.T, call apiCall) map[string]any {
	t.Helper()
	body, err := json.Marshal(call.body)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(body, &decoded), "body: %s", body)
	return decoded
}

// lookupPath は map から "." 区切りのパスで値を取り出す。パスが存在しない場合は失敗する。
func lookupPath(t *testing.T, m map[string]any, path string) any {
	t.Helper()
	current, ok := lookupOpt(m, path)
	require.True(t, ok, "path %q not found in %v", path, m)
	return current
}

// lookupOpt は map から "." 区切りのパスで値を取り出す。配列の添字も数値キーで扱える。
// パスが存在しない場合は false を返す。
func lookupOpt(m map[string]any, path string) (any, bool) {
	current := any(m)
	for _, key := range strings.Split(path, ".") {
		var ok bool
		switch node := current.(type) {
		case map[string]any:
			current, ok = node[key]
		case []any:
			index, err := strconv.Atoi(key)
			if err != nil || index < 0 || index >= len(node) {
				return nil, false
			}
			current, ok = node[index], true
		default:
			return nil, false
		}
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func TestSDKAPIFindAndRead(t *testing.T) {
	caller := &apiCaller{}
	api := NewSDKAPI(caller)
	ctx := context.Background()

	found, err := api.FindWithContext(ctx, &localrouter.FindRequest{Names: []string{"current"}})
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.EqualValues(t, 123, found[0].ID)
	require.Equal(t, "current", found[0].Name)
	require.Len(t, caller.calls, 1)
	require.Equal(t, "GET", caller.calls[0].method)
	require.True(t, strings.HasSuffix(caller.calls[0].path, "/commonserviceitem"))

	read, err := api.ReadWithContext(ctx, &localrouter.ReadRequest{ID: 123})
	require.NoError(t, err)
	require.EqualValues(t, 123, read.ID)
	require.Equal(t, "hash1", read.SettingsHash)
	require.NotNil(t, read.Switch)
	require.Equal(t, "123456789010", read.Switch.Code)
	require.NotNil(t, read.Interface)
	require.Equal(t, "192.0.2.1", read.Interface.VirtualIPAddress)
	require.Len(t, read.StaticRoutes, 1)
	require.Equal(t, "10.0.0.0/24", read.StaticRoutes[0].Prefix)
}

func TestSDKAPICreateWithConnectionFlags(t *testing.T) {
	caller := &apiCaller{}
	api := NewSDKAPI(caller)
	ctx := context.Background()

	request := &localrouter.CreateRequest{
		Name:        "lr",
		Description: "desc",
		Tags:        []string{"web"},
		Switch:      &iaas.LocalRouterSwitch{Code: "999", Category: "cloud", ZoneID: "is1b"},
		Interface: &iaas.LocalRouterInterface{
			VirtualIPAddress: "192.0.2.1",
			IPAddress:        []string{"192.0.2.2"},
			NetworkMaskLen:   24,
			VRID:             100,
		},
		StaticRoutes: []*iaas.LocalRouterStaticRoute{{Prefix: "10.1.0.0/24", NextHop: "192.0.2.254"}},
	}
	created, err := api.CreateWithContext(ctx, request)
	require.NoError(t, err)
	require.EqualValues(t, 123, created.ID)

	// サービス層の Create は作成 (POST) と接続設定の更新 (PUT) をまとめて行う
	require.Len(t, caller.calls, 2)
	require.Equal(t, "POST", caller.calls[0].method)
	require.Equal(t, "PUT", caller.calls[1].method)

	createBody := requestBody(t, caller.calls[0])
	require.Equal(t, "lr", lookupPath(t, createBody, "CommonServiceItem.Name"))
	require.Equal(t, "desc", lookupPath(t, createBody, "CommonServiceItem.Description"))
	require.Equal(t, "web", lookupPath(t, createBody, "CommonServiceItem.Tags.0"))

	settingsBody := requestBody(t, caller.calls[1])
	require.Equal(t, "999", lookupPath(t, settingsBody, "CommonServiceItem.Settings.LocalRouter.Switch.Code"))
	require.Equal(t, "cloud", lookupPath(t, settingsBody, "CommonServiceItem.Settings.LocalRouter.Switch.Category"))
	require.Equal(t, "is1b", lookupPath(t, settingsBody, "CommonServiceItem.Settings.LocalRouter.Switch.ZoneID"))
	require.Equal(t, "192.0.2.1", lookupPath(t, settingsBody, "CommonServiceItem.Settings.LocalRouter.Interface.VirtualIPAddress"))
	require.Equal(t, "10.1.0.0/24", lookupPath(t, settingsBody, "CommonServiceItem.Settings.LocalRouter.StaticRoutes.0.Prefix"))
	require.Equal(t, "192.0.2.254", lookupPath(t, settingsBody, "CommonServiceItem.Settings.LocalRouter.StaticRoutes.0.NextHop"))
	// 接続設定のみの更新には SettingsHash を含めない
	_, ok := lookupOpt(settingsBody, "CommonServiceItem.SettingsHash")
	require.False(t, ok, "settings update body should not contain SettingsHash: %v", settingsBody)
}

func TestSDKAPIUpdateMetadataOnly(t *testing.T) {
	caller := &apiCaller{}
	api := NewSDKAPI(caller)
	ctx := context.Background()

	name := "renamed"
	description := "new-desc"
	updated, err := api.UpdateWithContext(ctx, &localrouter.UpdateRequest{
		ID:          123,
		Name:        &name,
		Description: &description,
	})
	require.NoError(t, err)
	require.EqualValues(t, 123, updated.ID)

	// サービス層の Update は BuilderFromResource の Read と更新用の Read、
	// そして PUT をまとめて行う
	require.Len(t, caller.calls, 3)
	require.Equal(t, "GET", caller.calls[0].method)
	require.Equal(t, "GET", caller.calls[1].method)
	require.Equal(t, "PUT", caller.calls[2].method)

	body := requestBody(t, caller.calls[2])
	require.Equal(t, "renamed", lookupPath(t, body, "CommonServiceItem.Name"))
	require.Equal(t, "new-desc", lookupPath(t, body, "CommonServiceItem.Description"))
	require.Equal(t, "keep", lookupPath(t, body, "CommonServiceItem.Tags.0"))
	// サービス層の Update は現在の接続設定を引き継ぐため、メタデータだけの
	// 更新でも Switch と Interface は現在値がそのまま送られる
	require.Equal(t, "123456789010", lookupPath(t, body, "CommonServiceItem.Settings.LocalRouter.Switch.Code"))
	require.Equal(t, "192.0.2.1", lookupPath(t, body, "CommonServiceItem.Settings.LocalRouter.Interface.VirtualIPAddress"))
	require.Equal(t, "10.0.0.0/24", lookupPath(t, body, "CommonServiceItem.Settings.LocalRouter.StaticRoutes.0.Prefix"))
	_, ok := lookupOpt(body, "CommonServiceItem.SettingsHash")
	require.False(t, ok, "update body should not contain SettingsHash: %v", body)
}

func TestSDKAPIUpdateWithConnection(t *testing.T) {
	caller := &apiCaller{}
	api := NewSDKAPI(caller)
	ctx := context.Background()

	updated, err := api.UpdateWithContext(ctx, &localrouter.UpdateRequest{
		ID:     123,
		Switch: &iaas.LocalRouterSwitch{Code: "999", Category: "cloud", ZoneID: "is1b"},
		Interface: &iaas.LocalRouterInterface{
			VirtualIPAddress: "192.0.2.11",
			IPAddress:        []string{"192.0.2.12"},
			NetworkMaskLen:   24,
			VRID:             200,
		},
		StaticRoutes: &[]*iaas.LocalRouterStaticRoute{{Prefix: "10.2.0.0/24", NextHop: "192.0.2.253"}},
	})
	require.NoError(t, err)
	require.EqualValues(t, 123, updated.ID)

	require.Len(t, caller.calls, 3)
	body := requestBody(t, caller.calls[2])
	require.Equal(t, "999", lookupPath(t, body, "CommonServiceItem.Settings.LocalRouter.Switch.Code"))
	require.Equal(t, "192.0.2.11", lookupPath(t, body, "CommonServiceItem.Settings.LocalRouter.Interface.VirtualIPAddress"))
	require.Equal(t, "10.2.0.0/24", lookupPath(t, body, "CommonServiceItem.Settings.LocalRouter.StaticRoutes.0.Prefix"))
	_, ok := lookupOpt(body, "CommonServiceItem.Peers")
	require.False(t, ok, "update without peers should not resend Peers: %v", body)
}

func TestSDKAPIUpdateSettingsMerge(t *testing.T) {
	ctx := context.Background()

	t.Run("指定したスタティックルートだけを置き換え、残りは現在値を維持する", func(t *testing.T) {
		caller := &apiCaller{}
		api := NewSDKAPI(caller)

		updated, err := api.UpdateSettingsWithContext(ctx, &UpdateSettingsRequest{
			ID:           123,
			StaticRoutes: &[]*iaas.LocalRouterStaticRoute{{Prefix: "10.1.0.0/24", NextHop: "192.0.2.9"}},
		})
		require.NoError(t, err)
		require.EqualValues(t, 123, updated.ID)

		// マージアダプターは現在値の Read (GET) と設定更新 (PUT) を行う
		require.Len(t, caller.calls, 2)
		require.Equal(t, "GET", caller.calls[0].method)
		require.Equal(t, "PUT", caller.calls[1].method)

		body := requestBody(t, caller.calls[1])
		require.Equal(t, "hash1", lookupPath(t, body, "CommonServiceItem.SettingsHash"))
		require.Equal(t, "123456789010", lookupPath(t, body, "CommonServiceItem.Settings.LocalRouter.Switch.Code"))
		require.Equal(t, "192.0.2.1", lookupPath(t, body, "CommonServiceItem.Settings.LocalRouter.Interface.VirtualIPAddress"))
		require.Equal(t, "10.1.0.0/24", lookupPath(t, body, "CommonServiceItem.Settings.LocalRouter.StaticRoutes.0.Prefix"))
		require.Equal(t, "192.0.2.9", lookupPath(t, body, "CommonServiceItem.Settings.LocalRouter.StaticRoutes.0.NextHop"))
	})

	t.Run("未指定の設定は現在値がそのまま送られる", func(t *testing.T) {
		caller := &apiCaller{}
		api := NewSDKAPI(caller)

		_, err := api.UpdateSettingsWithContext(ctx, &UpdateSettingsRequest{ID: 123})
		require.NoError(t, err)

		require.Len(t, caller.calls, 2)
		body := requestBody(t, caller.calls[1])
		require.Equal(t, "123456789010", lookupPath(t, body, "CommonServiceItem.Settings.LocalRouter.Switch.Code"))
		require.Equal(t, "192.0.2.1", lookupPath(t, body, "CommonServiceItem.Settings.LocalRouter.Interface.VirtualIPAddress"))
		require.Equal(t, "10.0.0.0/24", lookupPath(t, body, "CommonServiceItem.Settings.LocalRouter.StaticRoutes.0.Prefix"))
	})

	t.Run("空配列を指定するとスタティックルートが消える", func(t *testing.T) {
		caller := &apiCaller{}
		api := NewSDKAPI(caller)

		_, err := api.UpdateSettingsWithContext(ctx, &UpdateSettingsRequest{
			ID:           123,
			StaticRoutes: &[]*iaas.LocalRouterStaticRoute{},
		})
		require.NoError(t, err)

		require.Len(t, caller.calls, 2)
		body, err := json.Marshal(caller.calls[1].body)
		require.NoError(t, err)
		// 空配列は API へ null として送られ、設定の置き換えでスタティックルートが消える
		require.Contains(t, string(body), `"StaticRoutes":null`,
			"空配列は [] ではなく null として送る: %s", body)
	})

	t.Run("接続設定の片側だけを指定すると既存設定とマージされる", func(t *testing.T) {
		caller := &apiCaller{}
		api := NewSDKAPI(caller)

		_, err := api.UpdateSettingsWithContext(ctx, &UpdateSettingsRequest{
			ID: 123,
			Interface: &iaas.LocalRouterInterface{
				VirtualIPAddress: "192.0.2.21",
				IPAddress:        []string{"192.0.2.22"},
				NetworkMaskLen:   24,
				VRID:             300,
			},
		})
		require.NoError(t, err)

		require.Len(t, caller.calls, 2)
		body := requestBody(t, caller.calls[1])
		// スイッチは現在値が維持され、インターフェースは指定した値に置き換わる
		require.Equal(t, "123456789010", lookupPath(t, body, "CommonServiceItem.Settings.LocalRouter.Switch.Code"))
		require.Equal(t, "192.0.2.21", lookupPath(t, body, "CommonServiceItem.Settings.LocalRouter.Interface.VirtualIPAddress"))
	})
}

func TestSDKAPIUpdateSettingsRejectsOneSidedConnection(t *testing.T) {
	// 接続設定を持たないローカルルータに対してスイッチだけを指定した更新は
	// 既存の対になる設定を失わせるため拒否する
	caller := &apiCaller{readBody: disconnectedReadResponse}
	api := NewSDKAPI(caller)

	_, err := api.UpdateSettingsWithContext(context.Background(), &UpdateSettingsRequest{
		ID: 123,
		Switch: &iaas.LocalRouterSwitch{
			Code:     "999",
			Category: "cloud",
			ZoneID:   "is1b",
		},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "スイッチとインターフェースは両方指定するか、既存の接続設定に合わせて指定してください")
}

func TestSDKAPIDelete(t *testing.T) {
	caller := &apiCaller{}
	api := NewSDKAPI(caller)

	err := api.DeleteWithContext(context.Background(), &localrouter.DeleteRequest{ID: 123, FailIfNotFound: true})
	require.NoError(t, err)
	require.Len(t, caller.calls, 1)
	require.Equal(t, "DELETE", caller.calls[0].method)
	require.True(t, strings.HasSuffix(caller.calls[0].path, "/commonserviceitem/123"))
}

func TestSDKAPIHealth(t *testing.T) {
	caller := &apiCaller{}
	api := NewSDKAPI(caller)

	health, err := api.HealthWithContext(context.Background(), &localrouter.HealthRequest{ID: 123})
	require.NoError(t, err)
	require.NotNil(t, health)
	require.Len(t, health.Peers, 1)
	require.EqualValues(t, 1, health.Peers[0].ID)
	require.Equal(t, types.EServerInstanceStatus("up"), health.Peers[0].Status)
	require.Equal(t, []string{"10.0.0.0/24 via 192.0.2.254"}, health.Peers[0].Routes)

	require.Len(t, caller.calls, 1)
	require.Equal(t, "GET", caller.calls[0].method)
	require.True(t, strings.HasSuffix(caller.calls[0].path, "/commonserviceitem/123/health"))
}

func TestSDKAPIMonitorLocalRouter(t *testing.T) {
	caller := &apiCaller{}
	api := NewSDKAPI(caller)

	start := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	end := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	values, err := api.MonitorLocalRouterWithContext(context.Background(), &localrouter.MonitorLocalRouterRequest{
		ID:    123,
		Start: start,
		End:   end,
	})
	require.NoError(t, err)
	require.Len(t, values, 1)
	require.InDelta(t, 1.5, values[0].ReceiveBytesPerSec, 0.001)
	require.InDelta(t, 2.5, values[0].SendBytesPerSec, 0.001)

	require.Len(t, caller.calls, 1)
	require.Equal(t, "GET", caller.calls[0].method)
	require.True(t, strings.HasSuffix(caller.calls[0].path, "/commonserviceitem/123/activity/localrouter/monitor"))
	body := requestBody(t, caller.calls[0])
	require.Equal(t, start.Format(time.RFC3339), lookupPath(t, body, "Start"))
	require.Equal(t, end.Format(time.RFC3339), lookupPath(t, body, "End"))
}
