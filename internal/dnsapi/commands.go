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

package dnsapi

import (
	"context"
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/dns"
)

// API は DNS の操作を公開するサービスのインターフェースです。
type API interface {
	FindWithContext(context.Context, *dns.FindRequest) ([]*iaas.DNS, error)
	ReadWithContext(context.Context, *dns.ReadRequest) (*iaas.DNS, error)
	CreateWithContext(context.Context, *dns.CreateRequest) (*iaas.DNS, error)
	UpdateWithContext(context.Context, *dns.UpdateRequest) (*iaas.DNS, error)
	DeleteWithContext(context.Context, *dns.DeleteRequest) error
}

// APIFactory は API を構築する関数です。
type APIFactory func() (API, error)

// Runtime は CLI 共通のリクエスト処理と出力処理を受け取ります。
type Runtime struct {
	DecodeRequest  func(string, any) error
	ValidateOutput func(*kong.Context) error
	WriteOutput    func(*kong.Context, any) error
}

type Commands struct {
	Find   FindCommand   `cmd:"" help:"DNS ゾーンを検索します。--name はゾーン名の部分一致検索です。例: skr dns-api find --name example"`
	Read   ReadCommand   `cmd:"" help:"指定した ID の DNS ゾーンを読み取ります。レコード一覧と権威 DNS サーバーも返します。例: skr dns-api read --id 123456789012"`
	Create CreateCommand `cmd:"" help:"DNS ゾーンを作成します。レコードは --record を繰り返して指定します。ゾーン名の変更はできないため、既存の DNS ゾーン名と重複しない名前を指定してください。カンマを含むレコード値など、フラグで表現できない値は --request JSON を使います。"`
	Update UpdateCommand `cmd:"" help:"DNS ゾーンの指定項目を更新します。省略した設定は変更しません。--record を指定するとレコード一覧全体を置き換えるため、保持したいレコードも再指定してください。ゾーン名 (Name) は変更できません。"`
	Delete DeleteCommand `cmd:"" help:"指定した ID の DNS ゾーンを削除します。"`
}

// NewCommands は各コマンドに Runtime と APIFactory を注入します。
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
	request := &dns.FindRequest{}
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
	Name    *string   `name:"name" help:"任意: ゾーン名の部分一致検索。値を分割せず1要素の Names に変換します。複数名は --request JSON の Names 配列で指定します。"`
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
		return nil, fmt.Errorf("DNS API クライアントが設定されていません")
	}
	return c.factory()
}

func (c *ReadCommand) Run(ctx *kong.Context) error {
	request := &dns.ReadRequest{}
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
	ID      *int64  `name:"id" help:"必須: DNS ゾーン ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *ReadCommand) newAPI() (API, error) {
	if c.factory == nil {
		return nil, fmt.Errorf("DNS API クライアントが設定されていません")
	}
	return c.factory()
}

type CreateCommand struct {
	Request                   *string   `help:"個別フラグと併用できません。CreateRequest JSON を直接または @path.json で指定します。レコード値にカンマを含める場合は JSON で指定します。"`
	Name                      *string   `name:"name" help:"必須: DNS ゾーン名 (例: example.com)。作成後に変更できません。"`
	Description               *string   `name:"description" help:"任意: DNS ゾーンの説明。"`
	Tags                      *[]string `name:"tags" sep:"," help:"任意: API の Tags 配列。カンマ区切りで指定します。"`
	IconID                    *int64    `name:"icon-id" help:"任意: アイコン ID。"`
	Records                   []string  `name:"record" sep:"none" help:"任意: 作成時に登録するレコード。形式: name=レコード名,type=レコード種別,rdata=レコード値,ttl=秒。繰り返し指定できます。type には A, AAAA, ALIAS, CNAME, NS, MX, TXT, SRV, CAA, HTTPS, SVCB, PTR を指定できます。"`
	MonitoringSuiteLogEnabled *bool     `name:"monitoring-suite-log-enabled" help:"任意: モニタリングスイートへのログ連携を有効にします。"`
	factory                   APIFactory
	runtime                   Runtime
}

func (c *CreateCommand) hasFlags() bool {
	return c.Name != nil || c.Description != nil || c.Tags != nil || c.IconID != nil ||
		c.Records != nil || c.MonitoringSuiteLogEnabled != nil
}

func (c *CreateCommand) Run(ctx *kong.Context) error {
	request := &dns.CreateRequest{}
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
		return nil, fmt.Errorf("DNS API クライアントが設定されていません")
	}
	return c.factory()
}

type UpdateCommand struct {
	Request                   *string   `help:"個別フラグと併用できません。UpdateRequest JSON を直接または @path.json で指定します。SettingsHash は SDK が現在値から設定します。"`
	ID                        *int64    `name:"id" help:"必須: 更新する DNS ゾーン ID。"`
	Description               *string   `name:"description" help:"任意: DNS ゾーンの説明。空文字列を指定すると説明を空にします。"`
	Tags                      *[]string `name:"tags" sep:"," help:"任意: API の Tags 配列。カンマ区切りで指定します。"`
	IconID                    *int64    `name:"icon-id" help:"任意: アイコン ID。"`
	Records                   []string  `name:"record" sep:"none" help:"任意: レコード一覧全体を置き換えます。形式: name=レコード名,type=レコード種別,rdata=レコード値,ttl=秒。繰り返し指定できます。指定しない場合は既存のレコードを維持します。"`
	MonitoringSuiteLogEnabled *bool     `name:"monitoring-suite-log-enabled" help:"任意: モニタリングスイートへのログ連携を有効／無効にします。"`
	factory                   APIFactory
	runtime                   Runtime
}

func (c *UpdateCommand) hasFlags() bool {
	return c.ID != nil || c.Description != nil || c.Tags != nil || c.IconID != nil ||
		c.Records != nil || c.MonitoringSuiteLogEnabled != nil
}

func (c *UpdateCommand) hasUpdateFields() bool {
	return c.Description != nil || c.Tags != nil || c.IconID != nil ||
		c.Records != nil || c.MonitoringSuiteLogEnabled != nil
}

func (c *UpdateCommand) Run(ctx *kong.Context) error {
	request := &dns.UpdateRequest{}
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
		if !c.hasUpdateFields() {
			return fmt.Errorf("更新するフラグが必要です")
		}
		request.ID = types.ID(*c.ID)
		var err error
		request, err = updateRequestFromFlags(c)
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
	result, err := api.UpdateWithContext(context.Background(), request)
	if err != nil {
		return err
	}
	return writeOutput(c.runtime, ctx, result)
}

func (c *UpdateCommand) newAPI() (API, error) {
	if c.factory == nil {
		return nil, fmt.Errorf("DNS API クライアントが設定されていません")
	}
	return c.factory()
}

type DeleteCommand struct {
	Request        *string `help:"個別フラグと併用できません。DeleteRequest JSON を直接または @path.json で指定します。"`
	ID             *int64  `name:"id" help:"必須: 削除する DNS ゾーン ID。"`
	FailIfNotFound *bool   `name:"fail-if-not-found" help:"対象が存在しない場合にエラーにします。"`
	factory        APIFactory
	runtime        Runtime
}

func (c *DeleteCommand) Run(_ *kong.Context) error {
	request := &dns.DeleteRequest{}
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
		return fmt.Errorf("DNS API クライアントが設定されていません")
	}
	api, err := c.factory()
	if err != nil {
		return err
	}
	return api.DeleteWithContext(context.Background(), request)
}

func decodeRequest(runtime Runtime, input string, destination any) error {
	if runtime.DecodeRequest == nil {
		return fmt.Errorf("DNS リクエスト処理が設定されていません")
	}
	return runtime.DecodeRequest(input, destination)
}

func validateOutput(runtime Runtime, ctx *kong.Context) error {
	if runtime.ValidateOutput == nil {
		return fmt.Errorf("DNS API 出力処理が設定されていません")
	}
	return runtime.ValidateOutput(ctx)
}

func writeOutput(runtime Runtime, ctx *kong.Context, result any) error {
	if runtime.WriteOutput == nil {
		return fmt.Errorf("DNS API 出力処理が設定されていません")
	}
	return runtime.WriteOutput(ctx, result)
}
