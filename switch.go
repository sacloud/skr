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

package main

import (
	"context"
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/swytch"
)

type switchCommands struct {
	Find   switchFindCommand   `cmd:"" help:"スイッチを検索し JSON 配列で出力します。--zone all で全ゾーンを検索できます。例: skr iaas-api switch find --zone ZONE"`
	Read   switchReadCommand   `cmd:"" help:"スイッチを読み取り JSON で出力します。例: skr iaas-api switch read --zone ZONE --id 123456789012"`
	Create switchCreateCommand `cmd:"" help:"スイッチを作成し JSON で出力します。例: skr iaas-api switch create --zone ZONE --name example"`
	Update switchUpdateCommand `cmd:"" help:"指定項目だけを更新し JSON で出力します。例: skr iaas-api switch update --zone ZONE --id 123456789012 --name updated"`
	Delete switchDeleteCommand `cmd:"" help:"スイッチを削除します。成功時は出力しません。例: skr iaas-api switch delete --zone ZONE --id 123456789012"`
}

type switchAPI interface {
	FindWithContext(context.Context, *swytch.FindRequest) ([]*iaas.Switch, error)
	ReadWithContext(context.Context, *swytch.ReadRequest) (*iaas.Switch, error)
	CreateWithContext(context.Context, *swytch.CreateRequest) (*iaas.Switch, error)
	UpdateWithContext(context.Context, *swytch.UpdateRequest) (*iaas.Switch, error)
	DeleteWithContext(context.Context, *swytch.DeleteRequest) error
}

type switchAPIFactory func() (switchAPI, error)

func switchRequest[T any](input *string, flags bool, fromFlags func() (T, error)) (T, error) {
	var request T
	if input != nil {
		if flags {
			return request, fmt.Errorf("--request と個別フラグは併用できません")
		}
		err := decodeRequest(*input, &request)
		return request, err
	}
	return fromFlags()
}

func switchZone(zone *string) (string, error) {
	if zone == nil || *zone == "" {
		return "", fmt.Errorf("--zone が必要です (--request JSON も指定できます)")
	}
	return *zone, nil
}

func switchSingleZone(zone *string) (string, error) {
	name, err := switchZone(zone)
	if err != nil {
		return "", err
	}
	if err := validateSingleIaaSZone(name); err != nil {
		return "", err
	}
	return name, nil
}

func switchID(id *int64) (types.ID, error) {
	if id == nil || *id == 0 {
		return 0, fmt.Errorf("--id が必要です (--request JSON も指定できます)")
	}
	return types.ID(*id), nil
}

type switchFindCommand struct {
	Request     *string `help:"個別フラグと併用不可。JSON オブジェクトを直接または @path.json で指定します。必須: Zone (単一のゾーン名)。任意: Names (名前の文字列配列)、Tags (タグの文字列配列)、Sort (ソートキーの配列)、Count (取得件数の整数)、From (開始位置の整数)。Sort の要素は Key (API のソート対象フィールド名) と Order (0: 昇順、1: 降順) を持つオブジェクトです。有効な Key は対象 API の仕様で確認してください。例: --request='{\"Zone\":\"tk1v\",\"Names\":[\"example\"]}'"`
	Zone        *string `help:"必須: 対象ゾーン名。all を指定すると全ゾーンを検索します。例: --zone tk1v または --zone all"`
	Count       *int    `help:"任意: 取得件数 (整数)。"`
	From        *int    `help:"任意: 取得開始位置 (整数)。"`
	factory     switchAPIFactory
	zoneFactory iaasZoneAPIFactory
}

func (c *switchFindCommand) Run(ctx *kong.Context) error {
	request, err := switchRequest(c.Request, c.Zone != nil || c.Count != nil || c.From != nil, func() (swytch.FindRequest, error) {
		zone, err := switchZone(c.Zone)
		if err != nil {
			return swytch.FindRequest{}, err
		}
		result := swytch.FindRequest{Zone: zone}
		if c.Count != nil {
			result.Count = *c.Count
		}
		if c.From != nil {
			result.From = *c.From
		}
		return result, nil
	})
	if err != nil {
		return err
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	var switches []*iaas.Switch
	if c.Request == nil && c.Zone != nil && *c.Zone == allIaaSZones {
		switches, err = findInAllIaaSZones(context.Background(), c.zoneFactory, func(ctx context.Context, zone string) ([]*iaas.Switch, error) {
			zoneRequest := request
			zoneRequest.Zone = zone
			return op.FindWithContext(ctx, &zoneRequest)
		})
	} else {
		switches, err = op.FindWithContext(context.Background(), &request)
	}
	if err != nil {
		return err
	}
	return writeJSON(ctx, switches)
}

type switchReadCommand struct {
	Request *string `help:"個別フラグと併用不可。JSON オブジェクトを直接または @path.json で指定します。必須: Zone (ゾーン名)、ID (作成結果にある数値)。例: --request='{\"Zone\":\"tk1v\",\"ID\":123456789012}'。ID は実際の値に置き換えます。"`
	Zone    *string `help:"必須: 対象ゾーン名。"`
	ID      *int64  `help:"必須: 作成結果の ID (数値)。"`
	factory switchAPIFactory
}

func (c *switchReadCommand) Run(ctx *kong.Context) error {
	request, err := switchRequest(c.Request, c.Zone != nil || c.ID != nil, func() (swytch.ReadRequest, error) {
		zone, err := switchSingleZone(c.Zone)
		if err != nil {
			return swytch.ReadRequest{}, err
		}
		id, err := switchID(c.ID)
		return swytch.ReadRequest{Zone: zone, ID: id}, err
	})
	if err != nil {
		return err
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	item, err := op.ReadWithContext(context.Background(), &request)
	if err != nil {
		return err
	}
	return writeJSON(ctx, item)
}

type switchCreateCommand struct {
	Request        *string `help:"個別フラグと併用不可。JSON オブジェクトを直接または @path.json で指定します。必須: Zone (ゾーン名)、Name (名前)。任意: Description (説明、最大512文字)、Tags (タグの文字列配列、フラグでは指定不可)、IconID (数値)、NetworkMaskLen (整数)、DefaultRoute (文字列)。例: --request='{\"Zone\":\"tk1v\",\"Name\":\"example\",\"Tags\":[\"test\"]}'"`
	Zone           *string `help:"必須: 作成先のゾーン名。"`
	Name           *string `help:"必須: スイッチの名前。"`
	Description    *string `help:"任意: 説明 (最大512文字)。"`
	IconID         *int64  `name:"icon-id" help:"任意: アイコンの ID (数値)。"`
	NetworkMaskLen *int    `help:"任意: ネットワークマスク長 (整数)。"`
	DefaultRoute   *string `help:"任意: デフォルトルート (文字列)。"`
	factory        switchAPIFactory
}

func (c *switchCreateCommand) Run(ctx *kong.Context) error {
	flags := c.Zone != nil || c.Name != nil || c.Description != nil || c.IconID != nil || c.NetworkMaskLen != nil || c.DefaultRoute != nil
	request, err := switchRequest(c.Request, flags, func() (swytch.CreateRequest, error) {
		zone, err := switchSingleZone(c.Zone)
		if err != nil {
			return swytch.CreateRequest{}, err
		}
		if c.Name == nil || *c.Name == "" {
			return swytch.CreateRequest{}, fmt.Errorf("--name が必要です")
		}
		result := swytch.CreateRequest{Zone: zone, Name: *c.Name}
		if c.Description != nil {
			result.Description = *c.Description
		}
		if c.IconID != nil {
			result.IconID = types.ID(*c.IconID)
		}
		if c.NetworkMaskLen != nil {
			result.NetworkMaskLen = *c.NetworkMaskLen
		}
		if c.DefaultRoute != nil {
			result.DefaultRoute = *c.DefaultRoute
		}
		return result, nil
	})
	if err != nil {
		return err
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	item, err := op.CreateWithContext(context.Background(), &request)
	if err != nil {
		return err
	}
	return writeJSON(ctx, item)
}

type switchUpdateCommand struct {
	Request        *string `help:"個別フラグと併用不可。JSON オブジェクトを直接または @path.json で指定します。必須: Zone (ゾーン名)、ID (数値)。任意: Name (名前)、Description (説明、最大512文字)、Tags (タグの文字列配列、フラグでは指定不可)、IconID (数値)、NetworkMaskLen (整数)、DefaultRoute (文字列)。省略した項目は変更しません。例: --request='{\"Zone\":\"tk1v\",\"ID\":123456789012,\"Tags\":[\"test\"]}'。ID は実際の値に置き換えます。"`
	Zone           *string `help:"必須: 対象ゾーン名。"`
	ID             *int64  `help:"必須: 更新対象の ID (数値)。"`
	Name           *string `help:"任意: 新しい名前。未指定なら変更しません。"`
	Description    *string `help:"任意: 新しい説明 (最大512文字)。空文字列も指定できます。"`
	IconID         *int64  `name:"icon-id" help:"任意: アイコンの ID (数値)。"`
	NetworkMaskLen *int    `help:"任意: ネットワークマスク長 (整数)。0 も明示できます。"`
	DefaultRoute   *string `help:"任意: デフォルトルート (文字列)。空文字列も指定できます。"`
	factory        switchAPIFactory
}

func (c *switchUpdateCommand) Run(ctx *kong.Context) error {
	flags := c.Zone != nil || c.ID != nil || c.Name != nil || c.Description != nil || c.IconID != nil || c.NetworkMaskLen != nil || c.DefaultRoute != nil
	request, err := switchRequest(c.Request, flags, func() (swytch.UpdateRequest, error) {
		zone, err := switchSingleZone(c.Zone)
		if err != nil {
			return swytch.UpdateRequest{}, err
		}
		id, err := switchID(c.ID)
		if err != nil {
			return swytch.UpdateRequest{}, err
		}
		result := swytch.UpdateRequest{Zone: zone, ID: id, Name: c.Name, Description: c.Description, NetworkMaskLen: c.NetworkMaskLen, DefaultRoute: c.DefaultRoute}
		if c.IconID != nil {
			iconID := types.ID(*c.IconID)
			result.IconID = &iconID
		}
		return result, nil
	})
	if err != nil {
		return err
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	item, err := op.UpdateWithContext(context.Background(), &request)
	if err != nil {
		return err
	}
	return writeJSON(ctx, item)
}

type switchDeleteCommand struct {
	Request               *string `help:"個別フラグと併用不可。JSON オブジェクトを直接または @path.json で指定します。必須: Zone (ゾーン名)、ID (数値)。任意: FailIfNotFound (真偽値)、WaitForRelease (真偽値)、WaitForReleaseTimeout (秒の整数)、WaitForReleaseTick (秒の整数)。例: --request='{\"Zone\":\"tk1v\",\"ID\":123456789012,\"FailIfNotFound\":true}'。ID は実際の値に置き換えます。"`
	Zone                  *string `help:"必須: 対象ゾーン名。"`
	ID                    *int64  `help:"必須: 削除対象の ID (数値)。"`
	FailIfNotFound        *bool   `help:"任意: 対象が見つからない場合にエラーにします (真偽値)。"`
	WaitForRelease        *bool   `help:"任意: 他リソースから参照されている間は削除を待ちます (真偽値)。"`
	WaitForReleaseTimeout *int    `help:"任意: 待機のタイムアウト (秒の整数、既定3600秒)。"`
	WaitForReleaseTick    *int    `help:"任意: 待機時の確認間隔 (秒の整数、既定5秒)。"`
	factory               switchAPIFactory
}

func (c *switchDeleteCommand) Run(_ *kong.Context) error {
	flags := c.Zone != nil || c.ID != nil || c.FailIfNotFound != nil || c.WaitForRelease != nil || c.WaitForReleaseTimeout != nil || c.WaitForReleaseTick != nil
	request, err := switchRequest(c.Request, flags, func() (swytch.DeleteRequest, error) {
		zone, err := switchSingleZone(c.Zone)
		if err != nil {
			return swytch.DeleteRequest{}, err
		}
		id, err := switchID(c.ID)
		if err != nil {
			return swytch.DeleteRequest{}, err
		}
		result := swytch.DeleteRequest{Zone: zone, ID: id}
		if c.FailIfNotFound != nil {
			result.FailIfNotFound = *c.FailIfNotFound
		}
		if c.WaitForRelease != nil {
			result.WaitForRelease = *c.WaitForRelease
		}
		if c.WaitForReleaseTimeout != nil {
			result.WaitForReleaseTimeout = *c.WaitForReleaseTimeout
		}
		if c.WaitForReleaseTick != nil {
			result.WaitForReleaseTick = *c.WaitForReleaseTick
		}
		return result, nil
	})
	if err != nil {
		return err
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	return op.DeleteWithContext(context.Background(), &request)
}

func (c *switchCommands) setFactory(factory switchAPIFactory) {
	c.Find.factory = factory
	c.Read.factory = factory
	c.Create.factory = factory
	c.Update.factory = factory
	c.Delete.factory = factory
}

func (c *switchCommands) setZoneFactory(factory iaasZoneAPIFactory) {
	c.Find.zoneFactory = factory
}

func newSwitchAPI() (switchAPI, error) {
	client, err := newIaaSClient()
	if err != nil {
		return nil, err
	}
	return swytch.New(client), nil
}
