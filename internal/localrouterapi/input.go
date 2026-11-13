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
	"fmt"
	"strings"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	localrouter "github.com/sacloud/sacloud-sdk-go/service/iaas/localrouter"
)

func createRequestFromFlags(command *CreateCommand) (*localrouter.CreateRequest, error) {
	request := &localrouter.CreateRequest{
		Name: *command.Name,
	}
	if command.Description != nil {
		request.Description = *command.Description
	}
	if command.Tags != nil {
		request.Tags = types.Tags(*command.Tags)
	}
	if command.IconID != nil {
		request.IconID = types.ID(*command.IconID)
	}
	sw, err := switchFlags{command.SwitchID, command.SwitchCategory, command.SwitchZoneID}.build()
	if err != nil {
		return nil, err
	}
	request.Switch = sw
	iface, err := interfaceFlags{
		command.InterfaceVirtualIPAddress, command.InterfaceIPAddress,
		command.InterfaceNetworkMaskLen, command.InterfaceVRID,
	}.build()
	if err != nil {
		return nil, err
	}
	request.Interface = iface
	routes, err := parseStaticRoutes(command.StaticRoutes, "--static-route")
	if err != nil {
		return nil, err
	}
	request.StaticRoutes = routes
	return request, nil
}

func updateRequestFromFlags(command *UpdateCommand) (*localrouter.UpdateRequest, error) {
	request := &localrouter.UpdateRequest{
		ID: types.ID(*command.ID),
	}
	if command.Name != nil {
		request.Name = command.Name
	}
	if command.Description != nil {
		request.Description = command.Description
	}
	if command.Tags != nil {
		tags := types.Tags(*command.Tags)
		request.Tags = &tags
	}
	if command.IconID != nil {
		iconID := types.ID(*command.IconID)
		request.IconID = &iconID
	}
	sw, err := switchFlags{command.SwitchID, command.SwitchCategory, command.SwitchZoneID}.build()
	if err != nil {
		return nil, err
	}
	request.Switch = sw
	iface, err := interfaceFlags{
		command.InterfaceVirtualIPAddress, command.InterfaceIPAddress,
		command.InterfaceNetworkMaskLen, command.InterfaceVRID,
	}.build()
	if err != nil {
		return nil, err
	}
	request.Interface = iface
	routes, err := parseStaticRoutes(command.StaticRoutes, "--static-route")
	if err != nil {
		return nil, err
	}
	if routes != nil {
		request.StaticRoutes = &routes
	}
	// 接続設定の片側だけの指定は service 層が現在値とマージするため、
	// ここでは検証しない
	return request, nil
}

func updateSettingsRequestFromFlags(command *UpdateSettingsCommand) (*UpdateSettingsRequest, error) {
	request := &UpdateSettingsRequest{
		ID: types.ID(*command.ID),
	}
	sw, err := switchFlags{command.SwitchID, command.SwitchCategory, command.SwitchZoneID}.build()
	if err != nil {
		return nil, err
	}
	request.Switch = sw
	iface, err := interfaceFlags{
		command.InterfaceVirtualIPAddress, command.InterfaceIPAddress,
		command.InterfaceNetworkMaskLen, command.InterfaceVRID,
	}.build()
	if err != nil {
		return nil, err
	}
	request.Interface = iface
	routes, err := parseStaticRoutes(command.StaticRoutes, "--static-route")
	if err != nil {
		return nil, err
	}
	if routes != nil {
		request.StaticRoutes = &routes
	}
	// 接続設定の片側だけの指定はアダプターが現在値とマージしたうえで
	// 対になる設定を確認するため、ここでは検証しない
	return request, nil
}

// finalizeCreateRequest は Create の接続設定を検証する。
// service 層の Create は不完全な接続設定の送信を黙って省略するため、
// 設定系の項目を指定した場合にスイッチとインターフェースの完全な設定を必須とする。
// flags が true の場合はエラーメッセージに個別フラグ名を含める。
func finalizeCreateRequest(request *localrouter.CreateRequest, flags bool) error {
	hasSettings := request.Switch != nil || request.Interface != nil ||
		len(request.Peers) > 0 || len(request.StaticRoutes) > 0
	if !hasSettings {
		return nil
	}
	return requireNetworkSettings(request.Switch, request.Interface, flags)
}

// requireNetworkSettings は service 層の Create が送信を省略しないための最小要件を検証する。
// スイッチの Code とインターフェースの仮想 IP、IP アドレス、マスク長が揃っている必要がある。
// flags が true の場合はエラーメッセージに個別フラグ名を含める。
func requireNetworkSettings(sw *iaas.LocalRouterSwitch, iface *iaas.LocalRouterInterface, flags bool) error {
	switchHint := " (API の Switch: Code)"
	interfaceHint := " (API の Interface: VirtualIPAddress、IPAddress、NetworkMaskLen)"
	if flags {
		switchHint = " (--switch-id)"
		interfaceHint = " (--interface-virtual-ip-address、--interface-ip-address、--interface-network-mask-len)"
	}
	if sw == nil || iface == nil {
		return fmt.Errorf("接続設定にはスイッチとインターフェースの両方が必要です%s%s", switchHint, interfaceHint)
	}
	if sw.Code == "" {
		return fmt.Errorf("接続するスイッチの ID が指定されていません%s", switchHint)
	}
	if iface.VirtualIPAddress == "" {
		return fmt.Errorf("インターフェースの仮想 IP アドレスが指定されていません%s", interfaceHint)
	}
	if len(iface.IPAddress) == 0 {
		return fmt.Errorf("インターフェースの IP アドレスが指定されていません%s", interfaceHint)
	}
	if iface.NetworkMaskLen == 0 {
		return fmt.Errorf("インターフェースのネットワークマスク長が指定されていません%s", interfaceHint)
	}
	return nil
}

// requirePairedConnection はマージ後の接続設定がスイッチとインターフェースの
// 片側だけにならないことを確認する。API 層の UpdateSettings は設定全体を置き換えるため、
// 片側だけの設定は既存の対になる設定を失わせる可能性がある。
func requirePairedConnection(sw *iaas.LocalRouterSwitch, iface *iaas.LocalRouterInterface) error {
	if (sw == nil) == (iface == nil) {
		return nil
	}
	return fmt.Errorf("スイッチとインターフェースは両方指定するか、既存の接続設定に合わせて指定してください (API の Switch と Interface)")
}

type switchFlags struct {
	id       *string
	category *string
	zoneID   *string
}

func (f switchFlags) hasFlags() bool {
	return f.id != nil || f.category != nil || f.zoneID != nil
}

// build はスイッチのフラグからスイッチ設定全体を組み立てる。
// 既定値の補完は行わず、指定された値だけを送る。
func (f switchFlags) build() (*iaas.LocalRouterSwitch, error) {
	if !f.hasFlags() {
		return nil, nil
	}
	if f.id == nil {
		return nil, fmt.Errorf("--switch-id が必要です")
	}
	sw := &iaas.LocalRouterSwitch{Code: *f.id}
	if f.category != nil {
		sw.Category = *f.category
	}
	if f.zoneID != nil {
		sw.ZoneID = *f.zoneID
	}
	return sw, nil
}

type interfaceFlags struct {
	virtualIPAddress *string
	ipAddress        []string
	networkMaskLen   *int
	vrid             *int
}

func (f interfaceFlags) hasFlags() bool {
	return f.virtualIPAddress != nil || len(f.ipAddress) > 0 || f.networkMaskLen != nil || f.vrid != nil
}

// build はインターフェースのフラグからインターフェース設定全体を組み立てる。
// 部分的な設定は API 動作が不明なため、4 項目すべてを必須とする。
func (f interfaceFlags) build() (*iaas.LocalRouterInterface, error) {
	if !f.hasFlags() {
		return nil, nil
	}
	if f.virtualIPAddress == nil {
		return nil, fmt.Errorf("--interface-virtual-ip-address が必要です")
	}
	if len(f.ipAddress) == 0 {
		return nil, fmt.Errorf("--interface-ip-address が必要です")
	}
	for _, ip := range f.ipAddress {
		if ip == "" {
			return nil, fmt.Errorf("--interface-ip-address に空の IP アドレスが含まれています")
		}
	}
	if f.networkMaskLen == nil {
		return nil, fmt.Errorf("--interface-network-mask-len が必要です")
	}
	if f.vrid == nil {
		return nil, fmt.Errorf("--interface-vrid が必要です")
	}
	return &iaas.LocalRouterInterface{
		VirtualIPAddress: *f.virtualIPAddress,
		IPAddress:        f.ipAddress,
		NetworkMaskLen:   *f.networkMaskLen,
		VRID:             *f.vrid,
	}, nil
}

// parseStaticRoutes は反復可能な --static-route の値を解析する。
// 指定するとスタティックルート一覧全体を置き換えるため、この構文は
// ローカルルータ専用であり他のサービスや配列へ適用しない。
func parseStaticRoutes(inputs []string, flag string) ([]*iaas.LocalRouterStaticRoute, error) {
	if inputs == nil {
		return nil, nil
	}
	result := make([]*iaas.LocalRouterStaticRoute, 0, len(inputs))
	for i, input := range inputs {
		route, err := parseStaticRoute(input)
		if err != nil {
			return nil, fmt.Errorf("%s %d: %w", flag, i+1, err)
		}
		result = append(result, route)
	}
	return result, nil
}

func parseStaticRoute(input string) (*iaas.LocalRouterStaticRoute, error) {
	fields := strings.Split(input, ",")
	values := make(map[string]string, len(fields))
	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok || key == "" || value == "" {
			return nil, fmt.Errorf("expected non-empty key=value entries")
		}
		switch key {
		case "prefix", "next-hop":
		default:
			return nil, fmt.Errorf("unknown key %q", key)
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("duplicate key %q", key)
		}
		values[key] = value
	}
	prefix, ok := values["prefix"]
	if !ok {
		return nil, fmt.Errorf("prefix is required")
	}
	nextHop, ok := values["next-hop"]
	if !ok {
		return nil, fmt.Errorf("next-hop is required")
	}
	return &iaas.LocalRouterStaticRoute{Prefix: prefix, NextHop: nextHop}, nil
}

// buildMonitorLocalRouterCondition は --start と --end を解析して設定する。
// 未指定時の既定値 (終了時刻が現在、開始時刻がその1時間前) は SDK が適用します。
func buildMonitorLocalRouterCondition(command *MonitorLocalRouterCommand, request *localrouter.MonitorLocalRouterRequest) error {
	start, err := parseTime(command.Start, "--start")
	if err != nil {
		return err
	}
	if start != nil {
		request.Start = *start
	}
	end, err := parseTime(command.End, "--end")
	if err != nil {
		return err
	}
	if end != nil {
		request.End = *end
	}
	return nil
}

func parseTime(value *string, flag string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return nil, fmt.Errorf("%s: RFC3339 形式で指定してください: %w", flag, err)
	}
	return &parsed, nil
}
