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
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	localrouter "github.com/sacloud/sacloud-sdk-go/service/iaas/localrouter"
)

type API interface {
	FindWithContext(context.Context, *localrouter.FindRequest) ([]*iaas.LocalRouter, error)
	ReadWithContext(context.Context, *localrouter.ReadRequest) (*iaas.LocalRouter, error)
	CreateWithContext(context.Context, *localrouter.CreateRequest) (*iaas.LocalRouter, error)
	UpdateWithContext(context.Context, *localrouter.UpdateRequest) (*iaas.LocalRouter, error)
	UpdateSettingsWithContext(context.Context, *UpdateSettingsRequest) (*iaas.LocalRouter, error)
	DeleteWithContext(context.Context, *localrouter.DeleteRequest) error
	HealthWithContext(context.Context, *localrouter.HealthRequest) (*iaas.LocalRouterHealth, error)
	MonitorLocalRouterWithContext(context.Context, *localrouter.MonitorLocalRouterRequest) ([]*iaas.MonitorLocalRouterValue, error)
}

type APIFactory func() (API, error)

// UpdateSettingsRequest は update-settings の部分更新入力です。
// 指定しない設定は現在値が引き継がれ、Peers や StaticRoutes に空配列を指定すると一覧を消去します。
type UpdateSettingsRequest struct {
	ID           types.ID
	Switch       *iaas.LocalRouterSwitch
	Interface    *iaas.LocalRouterInterface
	Peers        *[]*iaas.LocalRouterPeer
	StaticRoutes *[]*iaas.LocalRouterStaticRoute
}

type Runtime struct {
	DecodeRequest          func(string, any) error
	DecodeProtectedRequest func(string, any) error
	ValidateOutput         func(*kong.Context) error
	WriteOutput            func(*kong.Context, any) error
}

type Commands struct {
	Find               FindCommand               `cmd:"" help:"ローカルルータを検索します。例: skr local-router-api find --name example"`
	Create             CreateCommand             `cmd:"" help:"ローカルルータを作成します。スイッチ接続は --switch-id とインターフェースのフラグ、スタティックルートは --static-route を繰り返して指定します。ピア設定など複雑な設定には --request JSON を使います。接続設定を指定する場合はスイッチとインターフェースの両方が必要です。"`
	Read               ReadCommand               `cmd:"" help:"指定した ID のローカルルータを読み取ります。例: skr local-router-api read --id 123456789012"`
	Update             UpdateCommand             `cmd:"" help:"ローカルルータの指定項目を更新します。省略した設定は変更しません。スイッチやインターフェースのフラグを指定するとその設定全体を置き換え、スタティックルートを指定すると一覧全体を置き換えます。"`
	UpdateSettings     UpdateSettingsCommand     `cmd:"" help:"ローカルルータの接続設定 (スイッチ、インターフェース、スタティックルート、ピア) のみを更新します。省略した設定は変更しません。スイッチとインターフェースの設定全体を組み立てるため、保持したい項目も再指定してください。スイッチ接続の解除は生の API 操作が必要です。"`
	Delete             DeleteCommand             `cmd:"" help:"指定した ID のローカルルータを削除します。"`
	HealthStatus       HealthStatusCommand       `cmd:"" help:"ローカルルータのピアの稼働状態と学習したルートを取得します。"`
	MonitorLocalRouter MonitorLocalRouterCommand `cmd:"" help:"ローカルルータのトラフィックモニタリングを取得します。"`
}

func NewCommands(runtime Runtime, factory APIFactory) Commands {
	return Commands{
		Find:               FindCommand{runtime: runtime, factory: factory},
		Create:             CreateCommand{runtime: runtime, factory: factory},
		Read:               ReadCommand{runtime: runtime, factory: factory},
		Update:             UpdateCommand{runtime: runtime, factory: factory},
		UpdateSettings:     UpdateSettingsCommand{runtime: runtime, factory: factory},
		Delete:             DeleteCommand{runtime: runtime, factory: factory},
		HealthStatus:       HealthStatusCommand{runtime: runtime, factory: factory},
		MonitorLocalRouter: MonitorLocalRouterCommand{runtime: runtime, factory: factory},
	}
}

type FindCommand struct {
	Request *string   `help:"個別フラグと併用できません。FindRequest JSON を直接または @path.json で指定します。Sort は複数条件を JSON で指定できます。"`
	Name    *string   `name:"name" help:"任意: 名前の部分一致検索。値を分割せず1要素の Names に変換します。複数名は --request JSON の Names 配列で指定します。"`
	Tags    *[]string `name:"tags" sep:"," help:"任意: タグによる絞り込み。カンマ区切りで指定します。"`
	Count   *int      `name:"count" help:"任意: 取得件数。"`
	From    *int      `name:"from" help:"任意: 取得開始位置。"`
	factory APIFactory
	runtime Runtime
}

func (c *FindCommand) hasFlags() bool {
	return c.Name != nil || c.Tags != nil || c.Count != nil || c.From != nil
}

func (c *FindCommand) Run(ctx *kong.Context) error {
	request := &localrouter.FindRequest{}
	if c.Request != nil {
		if err := decodeRequest(c.runtime, *c.Request, request); err != nil {
			return err
		}
		if c.hasFlags() {
			return fmt.Errorf("--request と個別フラグは併用できません")
		}
	} else {
		// 検索条件はすべて任意です。空の FindRequest は有効で、条件なしの全件検索を受け付けます
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

type ReadCommand struct {
	Request *string `help:"個別フラグと併用できません。ReadRequest JSON を直接または @path.json で指定します。"`
	ID      *int64  `name:"id" help:"必須: ローカルルータ ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *ReadCommand) Run(ctx *kong.Context) error {
	request := &localrouter.ReadRequest{}
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

type CreateCommand struct {
	Request                   *string   `help:"個別フラグと併用できません。CreateRequest JSON を @path.json または -（標準入力）で指定します。Peers[].SecretKey など秘密情報を受け取れる API フィールドがあるため直接指定は受け付けません。接続設定を指定する場合はスイッチとインターフェースの完全な設定が必要です。"`
	Name                      *string   `name:"name" help:"必須: ローカルルータ名。"`
	Description               *string   `name:"description" help:"任意: ローカルルータの説明。"`
	Tags                      *[]string `name:"tags" sep:"," help:"任意: API の Tags 配列。カンマ区切りで指定します。"`
	IconID                    *int64    `name:"icon-id" help:"任意: アイコン ID。"`
	SwitchID                  *string   `name:"switch-id" help:"任意: 接続するスイッチの ID (API の Switch.Code)。スイッチのフラグを指定した場合は必須です。"`
	SwitchCategory            *string   `name:"switch-category" help:"任意: スイッチのカテゴリ (API の Switch.Category)。さくらのクラウドのスイッチは cloud、さくらの VPS などは vps です。"`
	SwitchZoneID              *string   `name:"switch-zone-id" help:"任意: スイッチのゾーン (API の Switch.ZoneID)。is1b など、さくらのクラウドのスイッチではゾーン名です。"`
	InterfaceVirtualIPAddress *string   `name:"interface-virtual-ip-address" help:"任意: VRRP の仮想 IP アドレス (API の Interface.VirtualIPAddress)。"`
	InterfaceIPAddress        []string  `name:"interface-ip-address" sep:"," help:"任意: インターフェースの IP アドレス (API の Interface.IPAddress)。カンマ区切りまたは繰り返しで指定します。"`
	InterfaceNetworkMaskLen   *int      `name:"interface-network-mask-len" help:"任意: ネットワークマスク長 (API の Interface.NetworkMaskLen)。"`
	InterfaceVRID             *int      `name:"interface-vrid" help:"任意: VRRP のグループ ID (API の Interface.VRID)。"`
	StaticRoutes              []string  `name:"static-route" sep:"none" help:"任意: スタティックルートを追加します。形式: prefix=CIDR,next-hop=IP。prefix と next-hop は必須です。繰り返し指定できます。"`
	factory                   APIFactory
	runtime                   Runtime
}

func (c *CreateCommand) hasFlags() bool {
	return c.Name != nil || c.Description != nil || c.Tags != nil || c.IconID != nil ||
		c.hasSwitchFlags() || c.hasInterfaceFlags() || c.StaticRoutes != nil
}

func (c *CreateCommand) hasSwitchFlags() bool {
	return c.SwitchID != nil || c.SwitchCategory != nil || c.SwitchZoneID != nil
}

func (c *CreateCommand) hasInterfaceFlags() bool {
	return c.InterfaceVirtualIPAddress != nil || c.InterfaceIPAddress != nil ||
		c.InterfaceNetworkMaskLen != nil || c.InterfaceVRID != nil
}

func (c *CreateCommand) Run(ctx *kong.Context) error {
	request := &localrouter.CreateRequest{}
	if c.Request != nil {
		// Peers[].SecretKey など秘密情報を受け取れるため、保護したファイルまたは標準入力に限定します
		if err := decodeProtectedRequest(c.runtime, *c.Request, request); err != nil {
			return err
		}
		if c.hasFlags() {
			return fmt.Errorf("--request と個別フラグは併用できません")
		}
		if err := finalizeCreateRequest(request, false); err != nil {
			return err
		}
	} else {
		if c.Name == nil {
			return fmt.Errorf("--name または --request が必要です")
		}
		var err error
		request, err = createRequestFromFlags(c)
		if err != nil {
			return err
		}
		if err := finalizeCreateRequest(request, true); err != nil {
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

type UpdateCommand struct {
	Request                   *string   `help:"個別フラグと併用できません。UpdateRequest JSON を @path.json または -（標準入力）で指定します。Peers[].SecretKey など秘密情報を受け取れる API フィールドがあるため直接指定は受け付けません。未指定の設定は現状維持で、StaticRoutes や Peers に空配列を指定すると一覧を消去します。"`
	ID                        *int64    `name:"id" help:"必須: 更新するローカルルータ ID。"`
	Name                      *string   `name:"name" help:"任意: ローカルルータ名。"`
	Description               *string   `name:"description" help:"任意: ローカルルータの説明。空文字列を指定すると説明を空にします。"`
	Tags                      *[]string `name:"tags" sep:"," help:"任意: API の Tags 配列。カンマ区切りで指定します。"`
	IconID                    *int64    `name:"icon-id" help:"任意: アイコン ID。"`
	SwitchID                  *string   `name:"switch-id" help:"任意: 接続するスイッチの ID (API の Switch.Code)。スイッチのフラグを指定するとスイッチ設定全体を置き換えます。"`
	SwitchCategory            *string   `name:"switch-category" help:"任意: スイッチのカテゴリ (API の Switch.Category)。さくらのクラウドのスイッチは cloud、さくらの VPS などは vps です。"`
	SwitchZoneID              *string   `name:"switch-zone-id" help:"任意: スイッチのゾーン (API の Switch.ZoneID)。is1b など、さくらのクラウドのスイッチではゾーン名です。"`
	InterfaceVirtualIPAddress *string   `name:"interface-virtual-ip-address" help:"任意: VRRP の仮想 IP アドレス (API の Interface.VirtualIPAddress)。"`
	InterfaceIPAddress        []string  `name:"interface-ip-address" sep:"," help:"任意: インターフェースの IP アドレス (API の Interface.IPAddress)。カンマ区切りまたは繰り返しで指定します。"`
	InterfaceNetworkMaskLen   *int      `name:"interface-network-mask-len" help:"任意: ネットワークマスク長 (API の Interface.NetworkMaskLen)。"`
	InterfaceVRID             *int      `name:"interface-vrid" help:"任意: VRRP のグループ ID (API の Interface.VRID)。"`
	StaticRoutes              []string  `name:"static-route" sep:"none" help:"任意: スタティックルート一覧全体を置き換えます。形式: prefix=CIDR,next-hop=IP。prefix と next-hop は必須です。繰り返し指定できます。"`
	factory                   APIFactory
	runtime                   Runtime
}

func (c *UpdateCommand) hasFlags() bool {
	return c.ID != nil || c.hasUpdateFields()
}

func (c *UpdateCommand) hasUpdateFields() bool {
	return c.Name != nil || c.Description != nil || c.Tags != nil || c.IconID != nil ||
		c.hasSwitchFlags() || c.hasInterfaceFlags() || c.StaticRoutes != nil
}

func (c *UpdateCommand) hasSwitchFlags() bool {
	return c.SwitchID != nil || c.SwitchCategory != nil || c.SwitchZoneID != nil
}

func (c *UpdateCommand) hasInterfaceFlags() bool {
	return c.InterfaceVirtualIPAddress != nil || c.InterfaceIPAddress != nil ||
		c.InterfaceNetworkMaskLen != nil || c.InterfaceVRID != nil
}

func (c *UpdateCommand) Run(ctx *kong.Context) error {
	request := &localrouter.UpdateRequest{}
	if c.Request != nil {
		// Peers[].SecretKey など秘密情報を受け取れるため、保護したファイルまたは標準入力に限定します
		if err := decodeProtectedRequest(c.runtime, *c.Request, request); err != nil {
			return err
		}
		if c.hasFlags() {
			return fmt.Errorf("--request と個別フラグは併用できません")
		}
	} else {
		if c.ID == nil {
			return fmt.Errorf("--id または --request が必要です")
		}
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

type UpdateSettingsCommand struct {
	Request                   *string  `help:"個別の設定フラグと併用できません。--id とは併用できます。UpdateSettingsRequest JSON を @path.json または -（標準入力）で指定します。Peers[].SecretKey など秘密情報を受け取れる API フィールドがあるため直接指定は受け付けません。未指定の設定は現状維持で、Peers や StaticRoutes に空配列を指定すると一覧を消去します。"`
	ID                        *int64   `name:"id" help:"必須: 更新するローカルルータ ID。--request と併用できます。"`
	SwitchID                  *string  `name:"switch-id" help:"任意: 接続するスイッチの ID (API の Switch.Code)。スイッチのフラグを指定するとスイッチ設定全体を置き換えます。"`
	SwitchCategory            *string  `name:"switch-category" help:"任意: スイッチのカテゴリ (API の Switch.Category)。さくらのクラウドのスイッチは cloud、さくらの VPS などは vps です。"`
	SwitchZoneID              *string  `name:"switch-zone-id" help:"任意: スイッチのゾーン (API の Switch.ZoneID)。is1b など、さくらのクラウドのスイッチではゾーン名です。"`
	InterfaceVirtualIPAddress *string  `name:"interface-virtual-ip-address" help:"任意: VRRP の仮想 IP アドレス (API の Interface.VirtualIPAddress)。"`
	InterfaceIPAddress        []string `name:"interface-ip-address" sep:"," help:"任意: インターフェースの IP アドレス (API の Interface.IPAddress)。カンマ区切りまたは繰り返しで指定します。"`
	InterfaceNetworkMaskLen   *int     `name:"interface-network-mask-len" help:"任意: ネットワークマスク長 (API の Interface.NetworkMaskLen)。"`
	InterfaceVRID             *int     `name:"interface-vrid" help:"任意: VRRP のグループ ID (API の Interface.VRID)。"`
	StaticRoutes              []string `name:"static-route" sep:"none" help:"任意: スタティックルート一覧全体を置き換えます。形式: prefix=CIDR,next-hop=IP。prefix と next-hop は必須です。繰り返し指定できます。"`
	factory                   APIFactory
	runtime                   Runtime
}

func (c *UpdateSettingsCommand) hasFlags() bool {
	return c.hasSwitchFlags() || c.hasInterfaceFlags() || c.StaticRoutes != nil
}

func (c *UpdateSettingsCommand) hasSwitchFlags() bool {
	return c.SwitchID != nil || c.SwitchCategory != nil || c.SwitchZoneID != nil
}

func (c *UpdateSettingsCommand) hasInterfaceFlags() bool {
	return c.InterfaceVirtualIPAddress != nil || c.InterfaceIPAddress != nil ||
		c.InterfaceNetworkMaskLen != nil || c.InterfaceVRID != nil
}

func (c *UpdateSettingsCommand) Run(ctx *kong.Context) error {
	// --id は対象リソースの指定であり設定フラグではないため、--request とも併用できます
	if c.ID == nil {
		return fmt.Errorf("--id が必要です")
	}
	request := &UpdateSettingsRequest{}
	if c.Request != nil {
		// Peers[].SecretKey など秘密情報を受け取れるため、保護したファイルまたは標準入力に限定します
		if err := decodeProtectedRequest(c.runtime, *c.Request, request); err != nil {
			return err
		}
		if c.hasFlags() {
			return fmt.Errorf("--request と個別フラグは併用できません")
		}
		// リクエスト JSON に ID がある場合は --id と一致する必要があります
		if request.ID != 0 && request.ID != types.ID(*c.ID) {
			return fmt.Errorf("--id とリクエスト JSON の ID が一致しません")
		}
		request.ID = types.ID(*c.ID)
	} else {
		var err error
		request, err = updateSettingsRequestFromFlags(c)
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
	result, err := api.UpdateSettingsWithContext(context.Background(), request)
	if err != nil {
		return err
	}
	return writeOutput(c.runtime, ctx, result)
}

type DeleteCommand struct {
	Request        *string `help:"個別フラグと併用できません。DeleteRequest JSON を直接または @path.json で指定します。"`
	ID             *int64  `name:"id" help:"必須: 削除するローカルルータ ID。"`
	FailIfNotFound *bool   `name:"fail-if-not-found" help:"対象が存在しない場合にエラーにします。"`
	factory        APIFactory
	runtime        Runtime
}

func (c *DeleteCommand) Run(_ *kong.Context) error {
	request := &localrouter.DeleteRequest{}
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
	api, err := c.newAPI()
	if err != nil {
		return err
	}
	return api.DeleteWithContext(context.Background(), request)
}

type HealthStatusCommand struct {
	Request *string `help:"個別フラグと併用できません。HealthRequest JSON を直接または @path.json で指定します。"`
	ID      *int64  `name:"id" help:"必須: ローカルルータ ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *HealthStatusCommand) Run(ctx *kong.Context) error {
	request := &localrouter.HealthRequest{}
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
	result, err := api.HealthWithContext(context.Background(), request)
	if err != nil {
		return err
	}
	return writeOutput(c.runtime, ctx, result)
}

type MonitorLocalRouterCommand struct {
	Request *string `help:"個別フラグと併用できません。MonitorLocalRouterRequest JSON を直接または @path.json で指定します。"`
	ID      *int64  `name:"id" help:"必須: ローカルルータ ID。"`
	Start   *string `name:"start" help:"任意: 取得開始時刻 (RFC3339 形式)。未指定時は end の1時間前です。"`
	End     *string `name:"end" help:"任意: 取得終了時刻 (RFC3339 形式)。未指定時は現在時刻です。"`
	factory APIFactory
	runtime Runtime
}

func (c *MonitorLocalRouterCommand) hasFlags() bool {
	return c.Start != nil || c.End != nil
}

func (c *MonitorLocalRouterCommand) Run(ctx *kong.Context) error {
	request := &localrouter.MonitorLocalRouterRequest{}
	if c.Request != nil {
		if err := decodeRequest(c.runtime, *c.Request, request); err != nil {
			return err
		}
		if c.ID != nil || c.hasFlags() {
			return fmt.Errorf("--request と個別フラグは併用できません")
		}
	} else {
		if c.ID == nil {
			return fmt.Errorf("--id または --request が必要です")
		}
		request.ID = types.ID(*c.ID)
		if err := buildMonitorLocalRouterCondition(c, request); err != nil {
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
	result, err := api.MonitorLocalRouterWithContext(context.Background(), request)
	if err != nil {
		return err
	}
	return writeOutput(c.runtime, ctx, result)
}

func (c *FindCommand) newAPI() (API, error)   { return newAPI(c.factory, "local-router-api find") }
func (c *ReadCommand) newAPI() (API, error)   { return newAPI(c.factory, "local-router-api read") }
func (c *CreateCommand) newAPI() (API, error) { return newAPI(c.factory, "local-router-api create") }
func (c *UpdateCommand) newAPI() (API, error) { return newAPI(c.factory, "local-router-api update") }
func (c *UpdateSettingsCommand) newAPI() (API, error) {
	return newAPI(c.factory, "local-router-api update-settings")
}
func (c *DeleteCommand) newAPI() (API, error) { return newAPI(c.factory, "local-router-api delete") }
func (c *HealthStatusCommand) newAPI() (API, error) {
	return newAPI(c.factory, "local-router-api health-status")
}
func (c *MonitorLocalRouterCommand) newAPI() (API, error) {
	return newAPI(c.factory, "local-router-api monitor-local-router")
}

func newAPI(factory APIFactory, label string) (API, error) {
	if factory == nil {
		return nil, fmt.Errorf("%s は利用できません", label)
	}
	return factory()
}

func decodeRequest(runtime Runtime, input string, destination any) error {
	if runtime.DecodeRequest == nil {
		return fmt.Errorf("リクエストのデコードが設定されていません")
	}
	return runtime.DecodeRequest(input, destination)
}

func decodeProtectedRequest(runtime Runtime, input string, destination any) error {
	if runtime.DecodeProtectedRequest == nil {
		return fmt.Errorf("リクエストのデコードが設定されていません")
	}
	return runtime.DecodeProtectedRequest(input, destination)
}

func validateOutput(runtime Runtime, ctx *kong.Context) error {
	if runtime.ValidateOutput == nil {
		return nil
	}
	return runtime.ValidateOutput(ctx)
}

func writeOutput(runtime Runtime, ctx *kong.Context, result any) error {
	if runtime.WriteOutput == nil {
		return nil
	}
	return runtime.WriteOutput(ctx, result)
}
