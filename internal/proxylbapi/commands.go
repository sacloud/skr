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

package proxylbapi

import (
	"context"
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/proxylb"
)

type API interface {
	FindWithContext(context.Context, *proxylb.FindRequest) ([]*iaas.ProxyLB, error)
	ReadWithContext(context.Context, *proxylb.ReadRequest) (*iaas.ProxyLB, error)
	CreateWithContext(context.Context, *proxylb.CreateRequest) (*iaas.ProxyLB, error)
	UpdateWithContext(context.Context, *proxylb.UpdateRequest) (*iaas.ProxyLB, error)
	DeleteWithContext(context.Context, *proxylb.DeleteRequest) error
	GetCertificatesWithContext(context.Context, *proxylb.GetCertificatesRequest) (*iaas.ProxyLBCertificates, error)
	SetCertificatesWithContext(context.Context, *proxylb.SetCertificatesRequest) error
	DeleteCertificatesWithContext(context.Context, *proxylb.DeleteCertificatesRequest) error
	RenewLetsEncryptCertWithContext(context.Context, *proxylb.RenewLetsEncryptCertRequest) error
	HealthStatusWithContext(context.Context, *proxylb.HealthStatusRequest) (*iaas.ProxyLBHealth, error)
	MonitorConnectionWithContext(context.Context, *proxylb.MonitorConnectionRequest) ([]*iaas.MonitorConnectionValue, error)
}

type APIFactory func() (API, error)

type Runtime struct {
	DecodeRequest          func(string, any) error
	DecodeProtectedRequest func(string, any) error
	ValidateOutput         func(*kong.Context) error
	WriteOutput            func(*kong.Context, any) error
}

type Commands struct {
	Find                 FindCommand                 `cmd:"" help:"エンハンスドロードバランサを検索します。例: skr proxylb-api find --name example"`
	Read                 ReadCommand                 `cmd:"" help:"指定した ID のエンハンスドロードバランサを読み取ります。例: skr proxylb-api read --id 123456789012"`
	Create               CreateCommand               `cmd:"" help:"エンハンスドロードバランサを作成します。リッスンポートは --bind-port、配下サーバーは --server を繰り返して指定します。複雑な設定には --request JSON を使います。"`
	Update               UpdateCommand               `cmd:"" help:"エンハンスドロードバランサの指定項目を更新します。省略した設定は変更しません。ヘルスチェックのフラグを指定するとヘルスチェック設定全体を置き換えるため、保持したいヘルスチェック項目も再指定してください。リッスンポートや配下サーバーを指定すると一覧全体を置き換えます。"`
	Delete               DeleteCommand               `cmd:"" help:"指定した ID のエンハンスドロードバランサを削除します。"`
	GetCertificates      GetCertificatesCommand      `cmd:"" help:"エンハンスドロードバランサに登録された証明書を取得します。"`
	SetCertificates      SetCertificatesCommand      `cmd:"" help:"エンハンスドロードバランサの証明書を置き換えます。秘密鍵を含むため証明書は保護したファイルまたは標準入力から渡してください。API は証明書一式を置き換えるため、追加証明書だけの差し替えでも主証明書の再指定が必要です。"`
	DeleteCertificates   DeleteCertificatesCommand   `cmd:"" help:"エンハンスドロードバランサから証明書を削除します。"`
	RenewLetsEncryptCert RenewLetsEncryptCertCommand `cmd:"" help:"Let's Encrypt 設定の証明書を更新します。Let's Encrypt による自動更新には --lets-encrypt-enabled での設定が必要です。"`
	HealthStatus         HealthStatusCommand         `cmd:"" help:"エンハンスドロードバランサの配下サーバーの稼働状態を取得します。"`
	MonitorConnection    MonitorConnectionCommand    `cmd:"" help:"エンハンスドロードバランサの接続数モニタリングを取得します。"`
}

func NewCommands(runtime Runtime, factory APIFactory) Commands {
	return Commands{
		Find:                 FindCommand{runtime: runtime, factory: factory},
		Read:                 ReadCommand{runtime: runtime, factory: factory},
		Create:               CreateCommand{runtime: runtime, factory: factory},
		Update:               UpdateCommand{runtime: runtime, factory: factory},
		Delete:               DeleteCommand{runtime: runtime, factory: factory},
		GetCertificates:      GetCertificatesCommand{runtime: runtime, factory: factory},
		SetCertificates:      SetCertificatesCommand{runtime: runtime, factory: factory},
		DeleteCertificates:   DeleteCertificatesCommand{runtime: runtime, factory: factory},
		RenewLetsEncryptCert: RenewLetsEncryptCertCommand{runtime: runtime, factory: factory},
		HealthStatus:         HealthStatusCommand{runtime: runtime, factory: factory},
		MonitorConnection:    MonitorConnectionCommand{runtime: runtime, factory: factory},
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
	request := &proxylb.FindRequest{}
	if c.Request != nil {
		if err := decodeRequest(c.runtime, *c.Request, request); err != nil {
			return err
		}
		if c.hasFlags() {
			return fmt.Errorf("--request と個別フラグは併用できません")
		}
	} else {
		if !c.hasFlags() {
			return fmt.Errorf("--request または個別フラグが必要です")
		}
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
	ID      *int64  `name:"id" help:"必須: エンハンスドロードバランサ ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *ReadCommand) Run(ctx *kong.Context) error {
	request := &proxylb.ReadRequest{}
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
	Request                    *string   `help:"個別フラグと併用できません。CreateRequest JSON を直接または @path.json で指定します。Rules、OriginGuard など、専用フラグのない API フィールドは JSON で指定します。"`
	Name                       *string   `name:"name" help:"必須: エンハンスドロードバランサ名。"`
	Description                *string   `name:"description" help:"任意: エンハンスドロードバランサの説明。"`
	Tags                       *[]string `name:"tags" sep:"," help:"任意: API の Tags 配列。カンマ区切りで指定します。"`
	IconID                     *int64    `name:"icon-id" help:"任意: アイコン ID。"`
	Plan                       *int      `name:"plan" help:"必須: プラン (CPS)。100, 500, 1000, 5000, 10000, 50000, 100000, 400000 のいずれかです。"`
	Region                     *string   `name:"region" help:"任意: 設置先リージョン (tk1, is1, anycast)。"`
	UseVIPFailover             *bool     `name:"use-vip-failover" help:"任意: VIP フェイルオーバーを有効にします。"`
	HealthCheckProtocol        *string   `name:"health-check-protocol" help:"必須: 監視プロトコル (http, tcp)。ヘルスチェックのフラグを指定するとヘルスチェック設定全体を構成します。"`
	HealthCheckDelayLoop       *int      `name:"health-check-delay-loop" help:"必須: 監視間隔 (秒)。"`
	HealthCheckHost            *string   `name:"health-check-host" help:"任意: 監視の Host ヘッダー (http のみ)。"`
	HealthCheckPath            *string   `name:"health-check-path" help:"必須 (http のみ): 監視パス。"`
	SorryServerIP              *string   `name:"sorry-server-ip" help:"任意: 全配下サーバーがダウンした場合の誘導先 IP アドレス。"`
	SorryServerPort            *int      `name:"sorry-server-port" help:"任意: 誘導先ポート。ソーリーサーバーのフラグを指定した場合は必須です。"`
	BindPorts                  []string  `name:"bind-port" sep:"none" help:"任意: リッスンポートを追加します。形式: proxy-mode=http|https|tcp,port=ポート番号[,redirect-to-https=true|false][,support-http2=true|false][,ssl-policy=ポリシー名]。redirect-to-https は http のみ、support-http2 と ssl-policy は https のみ指定できます。AddResponseHeader は JSON で指定します。繰り返し指定できます。"`
	Servers                    []string  `name:"server" sep:"none" help:"任意: 配下サーバーを追加します。形式: ip-address=IP,port=ポート番号[,server-group=グループ名][,enabled=true|false][,proxy-ssl=true|false]。enabled を省略すると無効で登録されます。繰り返し指定できます。"`
	LetsEncryptEnabled         *bool     `name:"lets-encrypt-enabled" help:"任意: Let's Encrypt による証明書の自動更新を有効にします。"`
	LetsEncryptCommonName      *string   `name:"lets-encrypt-common-name" help:"任意: Let's Encrypt で取得する証明書の共通名 (FQDN)。Let's Encrypt のフラグを指定した場合は必須です。"`
	LetsEncryptSubjectAltNames *[]string `name:"lets-encrypt-subject-alt-names" sep:"," help:"任意: 証明書の SAN。カンマ区切りで指定します。"`
	StickySession              *bool     `name:"sticky-session" help:"任意: セッション維持 (Sticky session) を有効にします。方式は cookie になります。"`
	Gzip                       *bool     `name:"gzip" help:"任意: レスポンスの Gzip 圧縮を有効にします。"`
	BackendHTTPKeepAlive       *string   `name:"backend-http-keep-alive" help:"任意: 実サーバーとの HTTP 持続接続 (safe, aggressive)。"`
	ProxyProtocol              *bool     `name:"proxy-protocol" help:"任意: Proxy Protocol v2 を有効にします。"`
	SyslogServer               *string   `name:"syslog-server" help:"任意: ログの送信先 syslog サーバー。"`
	SyslogPort                 *int      `name:"syslog-port" help:"任意: syslog のポート番号。syslog のフラグを指定した場合は必須です。"`
	TimeoutInactiveSec         *int      `name:"timeout-inactive-sec" help:"任意: アイドル接続のタイムアウト (秒)。未指定時は 10 が設定されます。"`
	MonitoringSuiteLogEnabled  *bool     `name:"monitoring-suite-log-enabled" help:"任意: モニタリングスイートへのログ連携を有効にします。"`
	StrictRule                 *bool     `name:"strict-rule" help:"任意: Strict ルールを有効にします。有効にすると Rules の許可されていないリクエストを拒否します。"`
	factory                    APIFactory
	runtime                    Runtime
}

func (c *CreateCommand) hasFlags() bool {
	return c.Name != nil || c.Description != nil || c.Tags != nil || c.IconID != nil ||
		c.Plan != nil || c.Region != nil || c.UseVIPFailover != nil ||
		c.HealthCheckProtocol != nil || c.HealthCheckDelayLoop != nil || c.HealthCheckHost != nil || c.HealthCheckPath != nil ||
		c.SorryServerIP != nil || c.SorryServerPort != nil || c.BindPorts != nil || c.Servers != nil ||
		c.LetsEncryptEnabled != nil || c.LetsEncryptCommonName != nil || c.LetsEncryptSubjectAltNames != nil ||
		c.StickySession != nil || c.Gzip != nil || c.BackendHTTPKeepAlive != nil || c.ProxyProtocol != nil ||
		c.SyslogServer != nil || c.SyslogPort != nil || c.TimeoutInactiveSec != nil ||
		c.MonitoringSuiteLogEnabled != nil || c.StrictRule != nil
}

func (c *CreateCommand) Run(ctx *kong.Context) error {
	request := &proxylb.CreateRequest{}
	if c.Request != nil {
		if err := decodeRequest(c.runtime, *c.Request, request); err != nil {
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
		if c.Plan == nil {
			return fmt.Errorf("--plan または --request が必要です")
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
	Request                    *string   `help:"個別フラグと併用できません。UpdateRequest JSON を直接または @path.json で指定します。Rules、OriginGuard など、専用フラグのない API フィールドは JSON で指定します。"`
	ID                         *int64    `name:"id" help:"必須: 更新するエンハンスドロードバランサ ID。"`
	Name                       *string   `name:"name" help:"任意: エンハンスドロードバランサ名。"`
	Description                *string   `name:"description" help:"任意: エンハンスドロードバランサの説明。空文字列を指定すると説明を空にします。"`
	Tags                       *[]string `name:"tags" sep:"," help:"任意: API の Tags 配列。カンマ区切りで指定します。"`
	IconID                     *int64    `name:"icon-id" help:"任意: アイコン ID。"`
	Plan                       *int      `name:"plan" help:"任意: プラン (CPS)。変更するとプラン変更 API が呼ばれます。100, 500, 1000, 5000, 10000, 50000, 100000, 400000 のいずれかです。"`
	HealthCheckProtocol        *string   `name:"health-check-protocol" help:"必須: 監視プロトコル (http, tcp)。ヘルスチェック設定全体を置き換えます。API がヘルスチェックの設定を常に要求するため、更新時に再指定が必要です。"`
	HealthCheckDelayLoop       *int      `name:"health-check-delay-loop" help:"必須: 監視間隔 (秒)。ヘルスチェックのフラグを指定した場合は必須です。"`
	HealthCheckHost            *string   `name:"health-check-host" help:"任意: 監視の Host ヘッダー (http のみ)。"`
	HealthCheckPath            *string   `name:"health-check-path" help:"必須 (http のみ): 監視パス。"`
	SorryServerIP              *string   `name:"sorry-server-ip" help:"任意: 全配下サーバーがダウンした場合の誘導先 IP アドレス。"`
	SorryServerPort            *int      `name:"sorry-server-port" help:"任意: 誘導先ポート。ソーリーサーバーのフラグを指定した場合は必須です。"`
	BindPorts                  []string  `name:"bind-port" sep:"none" help:"任意: リッスンポート一覧全体を置き換えます。形式: proxy-mode=http|https|tcp,port=ポート番号[,redirect-to-https=true|false][,support-http2=true|false][,ssl-policy=ポリシー名]。redirect-to-https は http のみ、support-http2 と ssl-policy は https のみ指定できます。AddResponseHeader は JSON で指定します。繰り返し指定できます。"`
	Servers                    []string  `name:"server" sep:"none" help:"任意: 配下サーバー一覧全体を置き換えます。形式: ip-address=IP,port=ポート番号[,server-group=グループ名][,enabled=true|false][,proxy-ssl=true|false]。繰り返し指定できます。"`
	LetsEncryptEnabled         *bool     `name:"lets-encrypt-enabled" help:"任意: Let's Encrypt による証明書の自動更新の有効／無効。Let's Encrypt のフラグを指定すると Let's Encrypt 設定全体を置き換えます。"`
	LetsEncryptCommonName      *string   `name:"lets-encrypt-common-name" help:"任意: Let's Encrypt で取得する証明書の共通名 (FQDN)。Let's Encrypt のフラグを指定した場合は必須です。"`
	LetsEncryptSubjectAltNames *[]string `name:"lets-encrypt-subject-alt-names" sep:"," help:"任意: 証明書の SAN。カンマ区切りで指定します。"`
	StickySession              *bool     `name:"sticky-session" help:"任意: セッション維持 (Sticky session) の有効／無効。方式は cookie になります。"`
	Gzip                       *bool     `name:"gzip" help:"任意: レスポンスの Gzip 圧縮の有効／無効。"`
	BackendHTTPKeepAlive       *string   `name:"backend-http-keep-alive" help:"任意: 実サーバーとの HTTP 持続接続 (safe, aggressive)。"`
	ProxyProtocol              *bool     `name:"proxy-protocol" help:"任意: Proxy Protocol v2 の有効／無効。"`
	SyslogServer               *string   `name:"syslog-server" help:"任意: ログの送信先 syslog サーバー。"`
	SyslogPort                 *int      `name:"syslog-port" help:"任意: syslog のポート番号。syslog のフラグを指定した場合は必須です。"`
	TimeoutInactiveSec         *int      `name:"timeout-inactive-sec" help:"任意: アイドル接続のタイムアウト (秒)。未指定時は 10 が設定されます。"`
	MonitoringSuiteLogEnabled  *bool     `name:"monitoring-suite-log-enabled" help:"任意: モニタリングスイートへのログ連携の有効／無効。"`
	StrictRule                 *bool     `name:"strict-rule" help:"任意: Strict ルールの有効／無効。有効にすると Rules の許可されていないリクエストを拒否します。"`
	factory                    APIFactory
	runtime                    Runtime
}

func (c *UpdateCommand) hasFlags() bool {
	return c.ID != nil || c.Name != nil || c.Description != nil || c.Tags != nil || c.IconID != nil ||
		c.Plan != nil ||
		c.HealthCheckProtocol != nil || c.HealthCheckDelayLoop != nil || c.HealthCheckHost != nil || c.HealthCheckPath != nil ||
		c.SorryServerIP != nil || c.SorryServerPort != nil || c.BindPorts != nil || c.Servers != nil ||
		c.LetsEncryptEnabled != nil || c.LetsEncryptCommonName != nil || c.LetsEncryptSubjectAltNames != nil ||
		c.StickySession != nil || c.Gzip != nil || c.BackendHTTPKeepAlive != nil || c.ProxyProtocol != nil ||
		c.SyslogServer != nil || c.SyslogPort != nil || c.TimeoutInactiveSec != nil ||
		c.MonitoringSuiteLogEnabled != nil || c.StrictRule != nil
}

func (c *UpdateCommand) hasUpdateFields() bool {
	return c.Name != nil || c.Description != nil || c.Tags != nil || c.IconID != nil ||
		c.Plan != nil ||
		c.HealthCheckProtocol != nil || c.HealthCheckDelayLoop != nil || c.HealthCheckHost != nil || c.HealthCheckPath != nil ||
		c.SorryServerIP != nil || c.SorryServerPort != nil || c.BindPorts != nil || c.Servers != nil ||
		c.LetsEncryptEnabled != nil || c.LetsEncryptCommonName != nil || c.LetsEncryptSubjectAltNames != nil ||
		c.StickySession != nil || c.Gzip != nil || c.BackendHTTPKeepAlive != nil || c.ProxyProtocol != nil ||
		c.SyslogServer != nil || c.SyslogPort != nil || c.TimeoutInactiveSec != nil ||
		c.MonitoringSuiteLogEnabled != nil || c.StrictRule != nil
}

func (c *UpdateCommand) Run(ctx *kong.Context) error {
	request := &proxylb.UpdateRequest{}
	if c.Request != nil {
		if err := decodeRequest(c.runtime, *c.Request, request); err != nil {
			return err
		}
		if c.hasFlags() {
			return fmt.Errorf("--request と個別フラグは併用できません")
		}
		if err := finalizeUpdateRequest(request, false); err != nil {
			return err
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
		if err := finalizeUpdateRequest(request, true); err != nil {
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

type DeleteCommand struct {
	Request        *string `help:"個別フラグと併用できません。DeleteRequest JSON を直接または @path.json で指定します。"`
	ID             *int64  `name:"id" help:"必須: 削除するエンハンスドロードバランサ ID。"`
	FailIfNotFound *bool   `name:"fail-if-not-found" help:"対象が存在しない場合にエラーにします。"`
	factory        APIFactory
	runtime        Runtime
}

func (c *DeleteCommand) Run(_ *kong.Context) error {
	request := &proxylb.DeleteRequest{}
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

type GetCertificatesCommand struct {
	Request *string `help:"個別フラグと併用できません。GetCertificatesRequest JSON を直接または @path.json で指定します。"`
	ID      *int64  `name:"id" help:"必須: エンハンスドロードバランサ ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *GetCertificatesCommand) Run(ctx *kong.Context) error {
	request := &proxylb.GetCertificatesRequest{}
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
	result, err := api.GetCertificatesWithContext(context.Background(), request)
	if err != nil {
		return err
	}
	return writeOutput(c.runtime, ctx, result)
}

type SetCertificatesCommand struct {
	Request             *string `help:"--primary-cert、--primary-key と併用できません。SetCertificatesRequest JSON (PrimaryCerts、AdditionalCerts) を保護したファイルまたは -（標準入力）で指定します。秘密鍵を含むため直接指定は受け付けません。"`
	ID                  *int64  `name:"id" help:"必須: エンハンスドロードバランサ ID。"`
	PrimaryCert         *string `name:"primary-cert" help:"主証明書 (サーバー証明書) の PEM。--primary-key と併せて必須です。@path または -（標準入力）で指定します。"`
	PrimaryIntermediate *string `name:"primary-intermediate" help:"主証明書の中間証明書の PEM。@path または -（標準入力）で指定します。"`
	PrimaryKey          *string `name:"primary-key" help:"主証明書の秘密鍵の PEM。--primary-cert と併せて必須です。@path または -（標準入力）で指定します。"`
	AdditionalCerts     *string `name:"additional-certs" help:"追加証明書の JSON 配列。各要素は ServerCertificate、IntermediateCertificate、PrivateKey を持ちます。@path または -（標準入力）で指定します。秘密鍵を含むため直接指定は受け付けません。"`
	factory             APIFactory
	runtime             Runtime
}

func (c *SetCertificatesCommand) hasFlags() bool {
	return c.PrimaryCert != nil || c.PrimaryIntermediate != nil || c.PrimaryKey != nil || c.AdditionalCerts != nil
}

func (c *SetCertificatesCommand) Run(ctx *kong.Context) error {
	request := &proxylb.SetCertificatesRequest{}
	if c.Request != nil {
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
		request.ID = types.ID(*c.ID)
		var err error
		request, err = setCertificatesRequestFromFlags(c)
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
	return api.SetCertificatesWithContext(context.Background(), request)
}

type DeleteCertificatesCommand struct {
	Request *string `help:"個別フラグと併用できません。DeleteCertificatesRequest JSON を直接または @path.json で指定します。"`
	ID      *int64  `name:"id" help:"必須: エンハンスドロードバランサ ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *DeleteCertificatesCommand) Run(_ *kong.Context) error {
	request := &proxylb.DeleteCertificatesRequest{}
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
	api, err := c.newAPI()
	if err != nil {
		return err
	}
	return api.DeleteCertificatesWithContext(context.Background(), request)
}

type RenewLetsEncryptCertCommand struct {
	Request *string `help:"個別フラグと併用できません。RenewLetsEncryptCertRequest JSON を直接または @path.json で指定します。"`
	ID      *int64  `name:"id" help:"必須: エンハンスドロードバランサ ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *RenewLetsEncryptCertCommand) Run(_ *kong.Context) error {
	request := &proxylb.RenewLetsEncryptCertRequest{}
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
	api, err := c.newAPI()
	if err != nil {
		return err
	}
	return api.RenewLetsEncryptCertWithContext(context.Background(), request)
}

type HealthStatusCommand struct {
	Request *string `help:"個別フラグと併用できません。HealthStatusRequest JSON を直接または @path.json で指定します。"`
	ID      *int64  `name:"id" help:"必須: エンハンスドロードバランサ ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *HealthStatusCommand) Run(ctx *kong.Context) error {
	request := &proxylb.HealthStatusRequest{}
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
	result, err := api.HealthStatusWithContext(context.Background(), request)
	if err != nil {
		return err
	}
	return writeOutput(c.runtime, ctx, result)
}

type MonitorConnectionCommand struct {
	Request *string `help:"個別フラグと併用できません。MonitorConnectionRequest JSON を直接または @path.json で指定します。"`
	ID      *int64  `name:"id" help:"必須: エンハンスドロードバランサ ID。"`
	Start   *string `name:"start" help:"任意: 取得開始時刻 (RFC3339 形式)。未指定時は end の1時間前です。"`
	End     *string `name:"end" help:"任意: 取得終了時刻 (RFC3339 形式)。未指定時は現在時刻です。"`
	factory APIFactory
	runtime Runtime
}

func (c *MonitorConnectionCommand) hasFlags() bool {
	return c.Start != nil || c.End != nil
}

func (c *MonitorConnectionCommand) Run(ctx *kong.Context) error {
	request := &proxylb.MonitorConnectionRequest{}
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
		if err := buildMonitorConnectionCondition(c, request); err != nil {
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
	result, err := api.MonitorConnectionWithContext(context.Background(), request)
	if err != nil {
		return err
	}
	return writeOutput(c.runtime, ctx, result)
}

func (c *FindCommand) newAPI() (API, error) {
	return newAPI(c.factory, "エンハンスドロードバランサ")
}

func (c *ReadCommand) newAPI() (API, error) {
	return newAPI(c.factory, "エンハンスドロードバランサ")
}

func (c *CreateCommand) newAPI() (API, error) {
	return newAPI(c.factory, "エンハンスドロードバランサ")
}

func (c *UpdateCommand) newAPI() (API, error) {
	return newAPI(c.factory, "エンハンスドロードバランサ")
}

func (c *DeleteCommand) newAPI() (API, error) {
	return newAPI(c.factory, "エンハンスドロードバランサ")
}

func (c *GetCertificatesCommand) newAPI() (API, error) {
	return newAPI(c.factory, "エンハンスドロードバランサ")
}

func (c *SetCertificatesCommand) newAPI() (API, error) {
	return newAPI(c.factory, "エンハンスドロードバランサ")
}

func (c *DeleteCertificatesCommand) newAPI() (API, error) {
	return newAPI(c.factory, "エンハンスドロードバランサ")
}

func (c *RenewLetsEncryptCertCommand) newAPI() (API, error) {
	return newAPI(c.factory, "エンハンスドロードバランサ")
}

func (c *HealthStatusCommand) newAPI() (API, error) {
	return newAPI(c.factory, "エンハンスドロードバランサ")
}

func (c *MonitorConnectionCommand) newAPI() (API, error) {
	return newAPI(c.factory, "エンハンスドロードバランサ")
}

func newAPI(factory APIFactory, label string) (API, error) {
	if factory == nil {
		return nil, fmt.Errorf("%s API クライアントが設定されていません", label)
	}
	return factory()
}

func decodeRequest(runtime Runtime, input string, destination any) error {
	if runtime.DecodeRequest == nil {
		return fmt.Errorf("エンハンスドロードバランサ リクエスト処理が設定されていません")
	}
	return runtime.DecodeRequest(input, destination)
}

func decodeProtectedRequest(runtime Runtime, input string, destination any) error {
	if runtime.DecodeProtectedRequest == nil {
		return fmt.Errorf("エンハンスドロードバランサ リクエスト処理が設定されていません")
	}
	return runtime.DecodeProtectedRequest(input, destination)
}

func validateOutput(runtime Runtime, ctx *kong.Context) error {
	if runtime.ValidateOutput == nil {
		return fmt.Errorf("エンハンスドロードバランサ API 出力処理が設定されていません")
	}
	return runtime.ValidateOutput(ctx)
}

func writeOutput(runtime Runtime, ctx *kong.Context, result any) error {
	if runtime.WriteOutput == nil {
		return fmt.Errorf("エンハンスドロードバランサ API 出力処理が設定されていません")
	}
	return runtime.WriteOutput(ctx, result)
}
