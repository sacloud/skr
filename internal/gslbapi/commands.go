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

package gslbapi

import (
	"context"
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/gslb"
)

type API interface {
	FindWithContext(context.Context, *gslb.FindRequest) ([]*iaas.GSLB, error)
	ReadWithContext(context.Context, *gslb.ReadRequest) (*iaas.GSLB, error)
	CreateWithContext(context.Context, *gslb.CreateRequest) (*iaas.GSLB, error)
	UpdateWithContext(context.Context, *gslb.UpdateRequest) (*iaas.GSLB, error)
	DeleteWithContext(context.Context, *gslb.DeleteRequest) error
}

type APIFactory func() (API, error)

type Runtime struct {
	DecodeRequest  func(string, any) error
	ValidateOutput func(*kong.Context) error
	WriteOutput    func(*kong.Context, any) error
}

type Commands struct {
	Find   FindCommand   `cmd:"" help:"GSLB を検索します。Name は完全一致です。例: skr gslb-api find --name example"`
	Read   ReadCommand   `cmd:"" help:"指定した ID の GSLB を読み取ります。例: skr gslb-api read --id 123456789012"`
	Create CreateCommand `cmd:"" help:"GSLB を作成します。監視方法には --health-check-protocol を指定します。実サーバーは --destination-server を繰り返して指定できます。複雑な設定には --request JSON を使います。"`
	Update UpdateCommand `cmd:"" help:"GSLB の指定項目を更新します。省略した設定は変更しません。ヘルスチェックのフラグを指定すると監視設定全体を置き換えるため、保持したい監視項目も再指定してください。実サーバーを指定すると一覧全体を置き換えます。"`
	Delete DeleteCommand `cmd:"" help:"指定した ID の GSLB を削除します。"`
}

func NewCommands(runtime Runtime, factory APIFactory) Commands {
	return Commands{
		Find:   FindCommand{runtime: runtime, factory: factory},
		Read:   ReadCommand{runtime: runtime, factory: factory},
		Create: CreateCommand{runtime: runtime, factory: factory},
		Update: UpdateCommand{runtime: runtime, factory: factory},
		Delete: DeleteCommand{runtime: runtime, factory: factory},
	}
}

func (c *FindCommand) Run(ctx *kong.Context) error {
	request := &gslb.FindRequest{}
	if c.Request != nil {
		if err := decodeRequest(c.runtime, *c.Request, request); err != nil {
			return err
		}
		if c.hasFlags() {
			return fmt.Errorf("--request と個別フラグは併用できません")
		}
	} else {
		if c.Name != nil {
			request.Names = []string{*c.Name}
		}
		if c.Tags != nil {
			request.Tags = *c.Tags
		}
		if c.Count != nil {
			request.Count = *c.Count
		}
		if c.From != nil {
			request.From = *c.From
		}
	}

	if err := validateOutput(c.runtime, ctx); err != nil {
		return err
	}
	api, err := c.newAPI()
	if err != nil {
		return err
	}
	result, err := api.FindWithContext(context.Background(), request)
	if err != nil {
		return err
	}
	return writeOutput(c.runtime, ctx, result)
}

type FindCommand struct {
	Request *string   `help:"個別フラグと併用できません。FindRequest JSON を直接または @path.json で指定します。Names と Tags は複数条件を JSON で指定できます。"`
	Name    *string   `name:"name" help:"任意: 名前の完全一致検索。複数名は --request JSON の Names 配列で指定します。"`
	Tags    *[]string `name:"tags" sep:"," help:"任意: タグによる絞り込み。カンマ区切りで指定します。"`
	Count   *int      `name:"count" help:"任意: 取得件数。"`
	From    *int      `name:"from" help:"任意: 取得開始位置。"`
	factory APIFactory
	runtime Runtime
}

func (c *FindCommand) hasFlags() bool {
	return c.Name != nil || c.Tags != nil || c.Count != nil || c.From != nil
}

func (c *FindCommand) newAPI() (API, error) {
	if c.factory == nil {
		return nil, fmt.Errorf("GSLB API クライアントが設定されていません")
	}
	return c.factory()
}

func (c *ReadCommand) Run(ctx *kong.Context) error {
	request := &gslb.ReadRequest{}
	if c.Request != nil {
		if err := decodeRequest(c.runtime, *c.Request, request); err != nil {
			return err
		}
		if c.ID != nil {
			return fmt.Errorf("--request と --id は併用できません")
		}
	} else {
		if c.ID == nil {
			return fmt.Errorf("--id または --request が必要です")
		}
		request.ID = types.ID(*c.ID)
	}

	if err := validateOutput(c.runtime, ctx); err != nil {
		return err
	}
	api, err := c.newAPI()
	if err != nil {
		return err
	}
	result, err := api.ReadWithContext(context.Background(), request)
	if err != nil {
		return err
	}
	return writeOutput(c.runtime, ctx, result)
}

type ReadCommand struct {
	Request *string `help:"個別フラグと併用できません。ReadRequest JSON を直接または @path.json で指定します。"`
	ID      *int64  `name:"id" help:"必須: GSLB ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *ReadCommand) newAPI() (API, error) {
	if c.factory == nil {
		return nil, fmt.Errorf("GSLB API クライアントが設定されていません")
	}
	return c.factory()
}

type CreateCommand struct {
	Request                 *string   `help:"個別フラグと併用できません。CreateRequest JSON を直接または @path.json で指定します。MonitoringSuiteLog など、専用フラグのない API フィールドは JSON で指定します。"`
	Name                    *string   `name:"name" help:"必須: GSLB 名。"`
	Description             *string   `name:"description" help:"任意: GSLB の説明。"`
	Tags                    *[]string `name:"tags" sep:"," help:"任意: API の Tags 配列。カンマ区切りで指定します。"`
	IconID                  *int64    `name:"icon-id" help:"任意: アイコン ID。"`
	DelayLoop               *int      `name:"delay-loop" help:"任意: 実サーバーのチェック間隔 (秒)。10～60 の整数です。"`
	Weighted                *bool     `name:"weighted" help:"任意: 重み付け応答を有効にします。"`
	SorryServer             *string   `name:"sorry-server" help:"任意: 全実サーバーがダウンした場合の誘導先 IP アドレス。"`
	HealthCheckProtocol     *string   `name:"health-check-protocol" help:"必須: 監視プロトコル (http, https, tcp, ping)。"`
	HealthCheckHostHeader   *string   `name:"health-check-host-header" help:"任意: HealthCheck.HostHeader。"`
	HealthCheckPath         *string   `name:"health-check-path" help:"任意: HealthCheck.Path。"`
	HealthCheckResponseCode *int      `name:"health-check-response-code" help:"任意: HealthCheck.ResponseCode。"`
	HealthCheckPort         *int      `name:"health-check-port" help:"任意: HealthCheck.Port。"`
	DestinationServers      []string  `name:"destination-server" sep:"none" help:"任意: 実サーバーを追加します。形式: ip-address=IP[,enabled=true|false][,weight=1..10000]。繰り返し指定できます。"`
	factory                 APIFactory
	runtime                 Runtime
}

func (c *CreateCommand) hasFlags() bool {
	return c.Name != nil || c.Description != nil || c.Tags != nil || c.IconID != nil ||
		c.DelayLoop != nil || c.Weighted != nil || c.SorryServer != nil ||
		c.HealthCheckProtocol != nil || c.HealthCheckHostHeader != nil || c.HealthCheckPath != nil ||
		c.HealthCheckResponseCode != nil || c.HealthCheckPort != nil || c.DestinationServers != nil
}

func (c *CreateCommand) Run(ctx *kong.Context) error {
	request := &gslb.CreateRequest{}
	if c.Request != nil {
		if err := decodeRequest(c.runtime, *c.Request, request); err != nil {
			return err
		}
		if c.hasFlags() {
			return fmt.Errorf("--request と個別フラグは併用できません")
		}
	} else {
		if c.Name == nil {
			return fmt.Errorf("--name または --request が必要です")
		}
		if c.HealthCheckProtocol == nil {
			return fmt.Errorf("--health-check-protocol が必要です")
		}
		var err error
		request, err = createRequestFromFlags(c)
		if err != nil {
			return err
		}
	}

	if err := validateOutput(c.runtime, ctx); err != nil {
		return err
	}
	api, err := c.newAPI()
	if err != nil {
		return err
	}
	result, err := api.CreateWithContext(context.Background(), request)
	if err != nil {
		return err
	}
	return writeOutput(c.runtime, ctx, result)
}

func (c *CreateCommand) newAPI() (API, error) {
	if c.factory == nil {
		return nil, fmt.Errorf("GSLB API クライアントが設定されていません")
	}
	return c.factory()
}

type UpdateCommand struct {
	Request                   *string   `help:"個別フラグと併用できません。UpdateRequest JSON を直接または @path.json で指定します。MonitoringSuiteLog など、専用フラグのない API フィールドは JSON で指定します。"`
	ID                        *int64    `name:"id" help:"必須: 更新する GSLB ID。"`
	Name                      *string   `name:"name" help:"任意: GSLB 名。"`
	Description               *string   `name:"description" help:"任意: GSLB の説明。空文字列を指定すると説明を空にします。"`
	Tags                      *[]string `name:"tags" sep:"," help:"任意: API の Tags 配列。カンマ区切りで指定します。"`
	IconID                    *int64    `name:"icon-id" help:"任意: アイコン ID。"`
	DelayLoop                 *int      `name:"delay-loop" help:"任意: 実サーバーのチェック間隔 (秒)。10～60 の整数です。"`
	Weighted                  *bool     `name:"weighted" help:"任意: 重み付け応答の有効／無効。"`
	SorryServer               *string   `name:"sorry-server" help:"任意: 全実サーバーがダウンした場合の誘導先 IP アドレス。"`
	HealthCheckProtocol       *string   `name:"health-check-protocol" help:"HealthCheck を指定する場合は必須。監視プロトコル (http, https, tcp, ping)。"`
	HealthCheckHostHeader     *string   `name:"health-check-host-header" help:"任意: HealthCheck.HostHeader。"`
	HealthCheckPath           *string   `name:"health-check-path" help:"任意: HealthCheck.Path。"`
	HealthCheckResponseCode   *int      `name:"health-check-response-code" help:"任意: HealthCheck.ResponseCode。"`
	HealthCheckPort           *int      `name:"health-check-port" help:"任意: HealthCheck.Port。"`
	DestinationServers        []string  `name:"destination-server" sep:"none" help:"任意: 実サーバー一覧全体を置き換えます。形式: ip-address=IP[,enabled=true|false][,weight=1..10000]。繰り返し指定できます。"`
	MonitoringSuiteLogEnabled *bool     `name:"monitoring-suite-log-enabled" help:"任意: モニタリングスイートへのログ連携を有効／無効にします。"`
	factory                   APIFactory
	runtime                   Runtime
}

func (c *UpdateCommand) hasFlags() bool {
	return c.ID != nil || c.Name != nil || c.Description != nil || c.Tags != nil || c.IconID != nil ||
		c.DelayLoop != nil || c.Weighted != nil || c.SorryServer != nil ||
		c.HealthCheckProtocol != nil || c.HealthCheckHostHeader != nil || c.HealthCheckPath != nil ||
		c.HealthCheckResponseCode != nil || c.HealthCheckPort != nil || c.DestinationServers != nil ||
		c.MonitoringSuiteLogEnabled != nil
}

func (c *UpdateCommand) Run(ctx *kong.Context) error {
	request := &gslb.UpdateRequest{}
	if c.Request != nil {
		if err := decodeRequest(c.runtime, *c.Request, request); err != nil {
			return err
		}
		if c.hasFlags() {
			return fmt.Errorf("--request と個別フラグは併用できません")
		}
	} else {
		if c.ID == nil {
			return fmt.Errorf("--id または --request が必要です")
		}
		request.ID = types.ID(*c.ID)
		var err error
		request, err = updateRequestFromFlags(c)
		if err != nil {
			return err
		}
		if !c.hasUpdateFields() {
			return fmt.Errorf("更新するフラグが必要です")
		}
	}

	if err := validateOutput(c.runtime, ctx); err != nil {
		return err
	}
	api, err := c.newAPI()
	if err != nil {
		return err
	}
	result, err := api.UpdateWithContext(context.Background(), request)
	if err != nil {
		return err
	}
	return writeOutput(c.runtime, ctx, result)
}

func (c *UpdateCommand) hasUpdateFields() bool {
	return c.Name != nil || c.Description != nil || c.Tags != nil || c.IconID != nil ||
		c.DelayLoop != nil || c.Weighted != nil || c.SorryServer != nil ||
		c.HealthCheckProtocol != nil || c.HealthCheckHostHeader != nil || c.HealthCheckPath != nil ||
		c.HealthCheckResponseCode != nil || c.HealthCheckPort != nil || c.DestinationServers != nil ||
		c.MonitoringSuiteLogEnabled != nil
}

func (c *UpdateCommand) newAPI() (API, error) {
	if c.factory == nil {
		return nil, fmt.Errorf("GSLB API クライアントが設定されていません")
	}
	return c.factory()
}

type DeleteCommand struct {
	Request        *string `help:"個別フラグと併用できません。DeleteRequest JSON を直接または @path.json で指定します。"`
	ID             *int64  `name:"id" help:"必須: 削除する GSLB ID。"`
	FailIfNotFound *bool   `name:"fail-if-not-found" help:"対象が存在しない場合にエラーにします。"`
	factory        APIFactory
	runtime        Runtime
}

func (c *DeleteCommand) Run(_ *kong.Context) error {
	request := &gslb.DeleteRequest{}
	if c.Request != nil {
		if err := decodeRequest(c.runtime, *c.Request, request); err != nil {
			return err
		}
		if c.ID != nil || c.FailIfNotFound != nil {
			return fmt.Errorf("--request と個別フラグは併用できません")
		}
	} else {
		if c.ID == nil {
			return fmt.Errorf("--id または --request が必要です")
		}
		request.ID = types.ID(*c.ID)
		if c.FailIfNotFound != nil {
			request.FailIfNotFound = *c.FailIfNotFound
		}
	}
	if c.factory == nil {
		return fmt.Errorf("GSLB API クライアントが設定されていません")
	}
	api, err := c.factory()
	if err != nil {
		return err
	}
	return api.DeleteWithContext(context.Background(), request)
}

func decodeRequest(runtime Runtime, input string, destination any) error {
	if runtime.DecodeRequest == nil {
		return fmt.Errorf("GSLB リクエスト処理が設定されていません")
	}
	return runtime.DecodeRequest(input, destination)
}

func validateOutput(runtime Runtime, ctx *kong.Context) error {
	if runtime.ValidateOutput == nil {
		return fmt.Errorf("GSLB API 出力処理が設定されていません")
	}
	return runtime.ValidateOutput(ctx)
}

func writeOutput(runtime Runtime, ctx *kong.Context, result any) error {
	if runtime.WriteOutput == nil {
		return fmt.Errorf("GSLB API 出力処理が設定されていません")
	}
	return runtime.WriteOutput(ctx, result)
}
