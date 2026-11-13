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
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	localrouter "github.com/sacloud/sacloud-sdk-go/service/iaas/localrouter"
	"github.com/stretchr/testify/require"
)

func strPtr(v string) *string { return &v }
func intPtr(v int) *int       { return &v }
func int64Ptr(v int64) *int64 { return &v }

func TestParseStaticRoute(t *testing.T) {
	t.Run("prefix と next-hop を解析する", func(t *testing.T) {
		route, err := parseStaticRoute("prefix=10.0.0.0/24,next-hop=192.0.2.254")
		require.NoError(t, err)
		require.Equal(t, &iaas.LocalRouterStaticRoute{Prefix: "10.0.0.0/24", NextHop: "192.0.2.254"}, route)
	})

	t.Run("順序を入れ替えても解析できる", func(t *testing.T) {
		route, err := parseStaticRoute("next-hop=192.0.2.254,prefix=10.0.0.0/24")
		require.NoError(t, err)
		require.Equal(t, &iaas.LocalRouterStaticRoute{Prefix: "10.0.0.0/24", NextHop: "192.0.2.254"}, route)
	})

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"unknown key", "prefix=10.0.0.0/24,next-hop=192.0.2.254,unknown=1", `unknown key "unknown"`},
		{"duplicate key", "prefix=10.0.0.0/24,prefix=10.1.0.0/24", `duplicate key "prefix"`},
		{"missing prefix", "next-hop=192.0.2.254", "prefix is required"},
		{"missing next-hop", "prefix=10.0.0.0/24", "next-hop is required"},
		{"empty value", "prefix=,next-hop=192.0.2.254", "expected non-empty key=value entries"},
		{"missing equal", "prefix", "expected non-empty key=value entries"},
		{"empty input", "", "expected non-empty key=value entries"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseStaticRoute(test.input)
			require.Error(t, err)
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestParseStaticRoutes(t *testing.T) {
	t.Run("未指定は nil を返す", func(t *testing.T) {
		routes, err := parseStaticRoutes(nil, "--static-route")
		require.NoError(t, err)
		require.Nil(t, routes)
	})

	t.Run("空文字列などの入力は拒否する", func(t *testing.T) {
		_, err := parseStaticRoutes([]string{""}, "--static-route")
		require.Error(t, err)
		require.ErrorContains(t, err, "--static-route 1: expected non-empty key=value entries")
	})

	t.Run("複数の指定を解析する", func(t *testing.T) {
		routes, err := parseStaticRoutes([]string{
			"prefix=10.0.0.0/24,next-hop=192.0.2.254",
			"prefix=10.1.0.0/24,next-hop=192.0.2.253",
		}, "--static-route")
		require.NoError(t, err)
		require.Equal(t, []*iaas.LocalRouterStaticRoute{
			{Prefix: "10.0.0.0/24", NextHop: "192.0.2.254"},
			{Prefix: "10.1.0.0/24", NextHop: "192.0.2.253"},
		}, routes)
	})

	t.Run("エラーには指定位置が含まれる", func(t *testing.T) {
		_, err := parseStaticRoutes([]string{
			"prefix=10.0.0.0/24,next-hop=192.0.2.254",
			"invalid",
		}, "--static-route")
		require.Error(t, err)
		require.ErrorContains(t, err, "--static-route 2: expected non-empty key=value entries")
	})
}

func TestSwitchFlagsBuild(t *testing.T) {
	t.Run("未指定は nil を返す", func(t *testing.T) {
		sw, err := switchFlags{}.build()
		require.NoError(t, err)
		require.Nil(t, sw)
	})

	t.Run("ID だけでも組み立てられる", func(t *testing.T) {
		sw, err := switchFlags{id: strPtr("123456789012")}.build()
		require.NoError(t, err)
		require.Equal(t, &iaas.LocalRouterSwitch{Code: "123456789012"}, sw)
	})

	t.Run("カテゴリとゾーンも設定される", func(t *testing.T) {
		sw, err := switchFlags{
			id:       strPtr("123456789012"),
			category: strPtr("cloud"),
			zoneID:   strPtr("is1b"),
		}.build()
		require.NoError(t, err)
		require.Equal(t, &iaas.LocalRouterSwitch{Code: "123456789012", Category: "cloud", ZoneID: "is1b"}, sw)
	})

	t.Run("カテゴリだけでは指定できない", func(t *testing.T) {
		_, err := switchFlags{category: strPtr("cloud")}.build()
		require.Error(t, err)
		require.ErrorContains(t, err, "--switch-id が必要です")
	})
}

func TestInterfaceFlagsBuild(t *testing.T) {
	full := interfaceFlags{
		virtualIPAddress: strPtr("192.0.2.1"),
		ipAddress:        []string{"192.0.2.2", "192.0.2.3"},
		networkMaskLen:   intPtr(24),
		vrid:             intPtr(100),
	}

	t.Run("未指定は nil を返す", func(t *testing.T) {
		iface, err := interfaceFlags{}.build()
		require.NoError(t, err)
		require.Nil(t, iface)
	})

	t.Run("すべて指定すると組み立てられる", func(t *testing.T) {
		iface, err := full.build()
		require.NoError(t, err)
		require.Equal(t, &iaas.LocalRouterInterface{
			VirtualIPAddress: "192.0.2.1",
			IPAddress:        []string{"192.0.2.2", "192.0.2.3"},
			NetworkMaskLen:   24,
			VRID:             100,
		}, iface)
	})

	tests := []struct {
		name    string
		iface   interfaceFlags
		wantErr string
	}{
		{"仮想 IP なし", interfaceFlags{ipAddress: []string{"192.0.2.2"}, networkMaskLen: intPtr(24), vrid: intPtr(100)}, "--interface-virtual-ip-address が必要です"},
		{"IP アドレスなし", interfaceFlags{virtualIPAddress: strPtr("192.0.2.1"), networkMaskLen: intPtr(24), vrid: intPtr(100)}, "--interface-ip-address が必要です"},
		{"マスク長なし", interfaceFlags{virtualIPAddress: strPtr("192.0.2.1"), ipAddress: []string{"192.0.2.2"}, vrid: intPtr(100)}, "--interface-network-mask-len が必要です"},
		{"VRID なし", interfaceFlags{virtualIPAddress: strPtr("192.0.2.1"), ipAddress: []string{"192.0.2.2"}, networkMaskLen: intPtr(24)}, "--interface-vrid が必要です"},
		{"空の IP アドレスを含む", interfaceFlags{virtualIPAddress: strPtr("192.0.2.1"), ipAddress: []string{"", "192.0.2.2"}, networkMaskLen: intPtr(24), vrid: intPtr(100)}, "--interface-ip-address に空の IP アドレスが含まれています"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.iface.build()
			require.Error(t, err)
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

func TestFinalizeCreateRequest(t *testing.T) {
	t.Run("設定なしは有効", func(t *testing.T) {
		err := finalizeCreateRequest(&localrouter.CreateRequest{}, false)
		require.NoError(t, err)
	})

	t.Run("完全な接続設定は有効", func(t *testing.T) {
		err := finalizeCreateRequest(localrouterCreateRequestFixture(), false)
		require.NoError(t, err)
	})

	t.Run("スイッチだけは無効", func(t *testing.T) {
		request := localrouterCreateRequestFixture()
		request.Interface = nil
		err := finalizeCreateRequest(request, false)
		require.Error(t, err)
		require.ErrorContains(t, err, "接続設定にはスイッチとインターフェースの両方が必要です")
		require.ErrorContains(t, err, "API の Switch: Code")
	})

	t.Run("仮想 IP のないインターフェースは無効", func(t *testing.T) {
		request := localrouterCreateRequestFixture()
		request.Interface.VirtualIPAddress = ""
		err := finalizeCreateRequest(request, false)
		require.Error(t, err)
		require.ErrorContains(t, err, "インターフェースの仮想 IP アドレスが指定されていません")
	})

	t.Run("フラグ経路のエラーにはフラグ名を含める", func(t *testing.T) {
		request := localrouterCreateRequestFixture()
		request.Interface = nil
		err := finalizeCreateRequest(request, true)
		require.Error(t, err)
		require.ErrorContains(t, err, "接続設定にはスイッチとインターフェースの両方が必要です (--switch-id) (--interface-virtual-ip-address、--interface-ip-address、--interface-network-mask-len)")
	})
}

func localrouterCreateRequestFixture() *localrouter.CreateRequest {
	return &localrouter.CreateRequest{
		Switch: &iaas.LocalRouterSwitch{Code: "123456789012"},
		Interface: &iaas.LocalRouterInterface{
			VirtualIPAddress: "192.0.2.1",
			IPAddress:        []string{"192.0.2.2"},
			NetworkMaskLen:   24,
			VRID:             100,
		},
	}
}

func TestCreateRequestFromFlags(t *testing.T) {
	command := &CreateCommand{
		Name:                      strPtr("lr"),
		Description:               strPtr("desc"),
		Tags:                      &[]string{"web", "production"},
		IconID:                    int64Ptr(9),
		SwitchID:                  strPtr("123456789012"),
		SwitchCategory:            strPtr("cloud"),
		SwitchZoneID:              strPtr("is1b"),
		InterfaceVirtualIPAddress: strPtr("192.0.2.1"),
		InterfaceIPAddress:        []string{"192.0.2.2", "192.0.2.3"},
		InterfaceNetworkMaskLen:   intPtr(24),
		InterfaceVRID:             intPtr(100),
		StaticRoutes: []string{
			"prefix=10.0.0.0/24,next-hop=192.0.2.254",
			"prefix=10.1.0.0/24,next-hop=192.0.2.253",
		},
	}
	request, err := createRequestFromFlags(command)
	require.NoError(t, err)
	require.Equal(t, "lr", request.Name)
	require.Equal(t, "desc", request.Description)
	require.Equal(t, []string{"web", "production"}, []string(request.Tags))
	require.EqualValues(t, 9, request.IconID)
	require.Equal(t, &iaas.LocalRouterSwitch{Code: "123456789012", Category: "cloud", ZoneID: "is1b"}, request.Switch)
	require.Equal(t, &iaas.LocalRouterInterface{
		VirtualIPAddress: "192.0.2.1",
		IPAddress:        []string{"192.0.2.2", "192.0.2.3"},
		NetworkMaskLen:   24,
		VRID:             100,
	}, request.Interface)
	require.Len(t, request.StaticRoutes, 2)
	require.Equal(t, "10.0.0.0/24", request.StaticRoutes[0].Prefix)
	require.Equal(t, "192.0.2.253", request.StaticRoutes[1].NextHop)
}

func TestUpdateRequestFromFlags(t *testing.T) {
	t.Run("メタデータのみ", func(t *testing.T) {
		command := &UpdateCommand{
			ID:          int64Ptr(123),
			Name:        strPtr("renamed"),
			Description: strPtr("desc"),
			Tags:        &[]string{"web"},
			IconID:      int64Ptr(9),
		}
		request, err := updateRequestFromFlags(command)
		require.NoError(t, err)
		require.EqualValues(t, 123, request.ID)
		require.NotNil(t, request.Name)
		require.Equal(t, "renamed", *request.Name)
		require.NotNil(t, request.Description)
		require.NotNil(t, request.Tags)
		require.Equal(t, []string{"web"}, []string(*request.Tags))
		require.NotNil(t, request.IconID)
		require.EqualValues(t, 9, *request.IconID)
		require.Nil(t, request.Switch)
		require.Nil(t, request.Interface)
		require.Nil(t, request.StaticRoutes)
	})

	t.Run("接続設定とスタティックルート", func(t *testing.T) {
		command := &UpdateCommand{
			ID:                        int64Ptr(123),
			SwitchID:                  strPtr("123456789012"),
			InterfaceVirtualIPAddress: strPtr("192.0.2.1"),
			InterfaceIPAddress:        []string{"192.0.2.2"},
			InterfaceNetworkMaskLen:   intPtr(24),
			InterfaceVRID:             intPtr(100),
			StaticRoutes:              []string{"prefix=10.0.0.0/24,next-hop=192.0.2.254"},
		}
		request, err := updateRequestFromFlags(command)
		require.NoError(t, err)
		require.NotNil(t, request.Switch)
		require.NotNil(t, request.Interface)
		require.NotNil(t, request.StaticRoutes)
		require.Len(t, *request.StaticRoutes, 1)
		require.Equal(t, "10.0.0.0/24", (*request.StaticRoutes)[0].Prefix)
	})

	t.Run("接続設定の片側だけの指定は service 層が現在値とマージする", func(t *testing.T) {
		command := &UpdateCommand{
			ID:       int64Ptr(123),
			SwitchID: strPtr("123456789012"),
		}
		request, err := updateRequestFromFlags(command)
		require.NoError(t, err)
		require.NotNil(t, request.Switch)
		require.Nil(t, request.Interface)
	})

	t.Run("スタティックルートの一覧置き換え", func(t *testing.T) {
		command := &UpdateCommand{
			ID:           int64Ptr(123),
			StaticRoutes: []string{"prefix=10.0.0.0/24,next-hop=192.0.2.254"},
		}
		request, err := updateRequestFromFlags(command)
		require.NoError(t, err)
		require.NotNil(t, request.StaticRoutes)
		require.Len(t, *request.StaticRoutes, 1)
	})
}

func TestUpdateSettingsRequestFromFlags(t *testing.T) {
	t.Run("ID だけの入力は有効", func(t *testing.T) {
		command := &UpdateSettingsCommand{ID: int64Ptr(123)}
		request, err := updateSettingsRequestFromFlags(command)
		require.NoError(t, err)
		require.EqualValues(t, 123, request.ID)
		require.Nil(t, request.Switch)
		require.Nil(t, request.Interface)
		require.Nil(t, request.StaticRoutes)
	})

	t.Run("接続設定の片側だけの指定はアダプターがマージ後に確認する", func(t *testing.T) {
		command := &UpdateSettingsCommand{
			ID:                        int64Ptr(123),
			InterfaceVirtualIPAddress: strPtr("192.0.2.1"),
			InterfaceIPAddress:        []string{"192.0.2.2"},
			InterfaceNetworkMaskLen:   intPtr(24),
			InterfaceVRID:             intPtr(100),
		}
		request, err := updateSettingsRequestFromFlags(command)
		require.NoError(t, err)
		require.Nil(t, request.Switch)
		require.NotNil(t, request.Interface)
	})

	t.Run("スイッチとインターフェースとスタティックルート", func(t *testing.T) {
		command := &UpdateSettingsCommand{
			ID:                        int64Ptr(123),
			SwitchID:                  strPtr("123456789012"),
			InterfaceVirtualIPAddress: strPtr("192.0.2.1"),
			InterfaceIPAddress:        []string{"192.0.2.2"},
			InterfaceNetworkMaskLen:   intPtr(24),
			InterfaceVRID:             intPtr(100),
			StaticRoutes:              []string{"prefix=10.0.0.0/24,next-hop=192.0.2.254"},
		}
		request, err := updateSettingsRequestFromFlags(command)
		require.NoError(t, err)
		require.NotNil(t, request.Switch)
		require.NotNil(t, request.Interface)
		require.NotNil(t, request.StaticRoutes)
	})
}

func TestBuildMonitorLocalRouterCondition(t *testing.T) {
	t.Run("未指定は既定値に任せる", func(t *testing.T) {
		command := &MonitorLocalRouterCommand{}
		request := &localrouter.MonitorLocalRouterRequest{}
		err := buildMonitorLocalRouterCondition(command, request)
		require.NoError(t, err)
		require.True(t, request.Start.IsZero())
		require.True(t, request.End.IsZero())
	})

	t.Run("指定した時刻を設定する", func(t *testing.T) {
		command := &MonitorLocalRouterCommand{
			Start: strPtr("2026-11-13T09:00:00Z"),
			End:   strPtr("2026-11-13T10:00:00Z"),
		}
		request := &localrouter.MonitorLocalRouterRequest{}
		err := buildMonitorLocalRouterCondition(command, request)
		require.NoError(t, err)
		require.Equal(t, time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC), request.Start)
		require.Equal(t, time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC), request.End)
	})

	t.Run("RFC3339 以外は拒否する", func(t *testing.T) {
		command := &MonitorLocalRouterCommand{Start: strPtr("2026-11-13 09:00:00")}
		request := &localrouter.MonitorLocalRouterRequest{}
		err := buildMonitorLocalRouterCondition(command, request)
		require.Error(t, err)
		require.ErrorContains(t, err, "--start: RFC3339 形式で指定してください")
	})
}
