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

// Package serviceendpointgatewayapi はさくらのクラウド
// サービスエンドポイントゲートウェイのコマンド実装を提供します。
package serviceendpointgatewayapi

import (
	"context"
	"fmt"
	"math"

	"github.com/alecthomas/kong"
	seg "github.com/sacloud/sacloud-sdk-go/api/service-endpoint-gateway"
	v1 "github.com/sacloud/sacloud-sdk-go/api/service-endpoint-gateway/apis/v1"
	clientconfig "github.com/sacloud/skr/internal/saclient"
)

// Commands は service-endpoint-gateway-api のコマンド群です。
type Commands struct {
	Appliance ApplianceCommands `cmd:"" help:"サービスエンドポイントゲートウェイの作成・削除、接続先マネージドサービスの設定、設定反映、電源操作を行います。"`
}

// ApplianceCreateCommand はサービスエンドポイントゲートウェイの作成コマンドです。
// リクエストが単純なスカラー値 (スイッチ ID、ネットマスク長、ゲートウェイ IP アドレス) のみで
// 表現できるため、個別フラグと --request JSON の両経路を手書きで実装しています。
type ApplianceCreateCommand struct {
	Request        *string  `help:"API リクエスト JSON を直接または @path.json で指定します。個別フラグと併用できません。Appliance.Remark に Switch.ID、Network.NetworkMaskLen、Servers[].IPAddress を指定します。Class と Plan は自動的に設定されます。"`
	SwitchID       *string  `name:"switch-id" help:"必須: 接続先スイッチの ID。スイッチ単体が対象で、ルータ＋スイッチでは利用できません。iaas-api switch list で確認できます。"`
	NetworkMaskLen *int     `name:"network-mask-len" help:"必須: サービスエンドポイントゲートウェイに割り当てるサブネットマスク長。例: 24"`
	ServerIPs      []string `name:"server-ip" help:"任意: サービスエンドポイントゲートウェイに割り当てる IPv4 アドレス (ゲートウェイアドレス)。ネットマスク長とともにゲートウェイのサブネットを構成します。例: --server-ip 192.168.2.110"`
	factory        ApplianceAPIFactory
	runtime        ApplianceRuntime
}

func (c *ApplianceCreateCommand) Run(ctx *kong.Context) error {
	flagsSet := false
	flagsSet = flagsSet || c.SwitchID != nil
	flagsSet = flagsSet || c.NetworkMaskLen != nil
	flagsSet = flagsSet || len(c.ServerIPs) > 0
	if c.Request != nil && flagsSet {
		return fmt.Errorf("--request と個別フラグは併用できません")
	}
	var request v1.ModelsApplianceApplianceCreateRequest
	if c.Request != nil {
		if c.runtime.DecodeRequest == nil {
			return fmt.Errorf("API リクエスト処理が設定されていません")
		}
		var body createRequestBody
		if err := c.runtime.DecodeRequest(*c.Request, &body); err != nil {
			return err
		}
		request = v1.ModelsApplianceApplianceCreateRequest{
			Appliance: v1.ModelsApplianceApplianceCreateBody{Remark: body.Appliance.Remark},
		}
	} else {
		if !flagsSet {
			return fmt.Errorf("--request または個別フラグが必要です")
		}
		if c.SwitchID == nil {
			return fmt.Errorf("--switch-id が必要です")
		}
		if c.NetworkMaskLen == nil {
			return fmt.Errorf("--network-mask-len が必要です")
		}
		// サブネットマスク長は API のフィールド (int32) に収まる範囲で受けます
		if *c.NetworkMaskLen < 0 || *c.NetworkMaskLen > math.MaxInt32 {
			return fmt.Errorf("--network-mask-len は 0 から %d の範囲で指定してください", math.MaxInt32)
		}
		// Servers は API のリクエストで必須のため、未指定でも空配列を送ります。
		servers := make([]v1.ModelsRemarkServerRemark, 0, len(c.ServerIPs))
		for _, ip := range c.ServerIPs {
			servers = append(servers, v1.ModelsRemarkServerRemark{IPAddress: ip})
		}
		request = v1.ModelsApplianceApplianceCreateRequest{
			Appliance: v1.ModelsApplianceApplianceCreateBody{
				Remark: v1.ModelsRemarkApplianceCreateRemark{
					Switch:  v1.ModelsRemarkSwitchRemark{ID: *c.SwitchID},
					Network: v1.ModelsRemarkNetworkRemark{NetworkMaskLen: int32(*c.NetworkMaskLen)},
					Servers: servers,
				},
			},
		}
	}
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	result, err := op.Create(context.Background(), request)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

// createRequestBody は --request JSON のデコード先です。API リクエストの Class と
// Plan は SDK の Create が固定値を設定するため、利用者が指定する Remark だけを
// 受け付けます。Remark のデコードは Switch、Network、Servers の指定を検証します。
type createRequestBody struct {
	Appliance struct {
		Remark v1.ModelsRemarkApplianceCreateRemark `json:"Remark"`
	} `json:"Appliance"`
}

// NewApplianceAPI は Appliance コマンド用の API を構築します。
func NewApplianceAPI(trace bool) (ApplianceAPI, error) {
	client, err := clientconfig.New(trace)
	if err != nil {
		return nil, err
	}
	segClient, err := seg.NewClient(client)
	if err != nil {
		return nil, err
	}
	return seg.NewServiceEndpointGatewayOp(segClient), nil
}

func (r ApplianceRuntime) validateOutput(ctx *kong.Context) error {
	if r.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.ValidateOutput(ctx)
}

func (r ApplianceRuntime) writeOutput(ctx *kong.Context, value any) error {
	if r.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.WriteOutput(ctx, value)
}
