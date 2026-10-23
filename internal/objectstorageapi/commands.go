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

package objectstorageapi

import (
	"context"
	"fmt"
	"time"

	"github.com/alecthomas/kong"
	objectstorage "github.com/sacloud/sacloud-sdk-go/api/object-storage"
	v2 "github.com/sacloud/sacloud-sdk-go/api/object-storage/apis/v2"
)

type API struct {
	Site        objectstorage.SiteAPI
	Account     objectstorage.AccountAPI
	Bucket      objectstorage.BucketAPI
	BucketExtra func(bucket string) objectstorage.BucketExtraAPI
	Permission  objectstorage.PermissionsAPI
	SiteStatus  objectstorage.SiteStatusAPI
}

type APIFactory func(siteID string) (API, error)

type Runtime struct {
	DecodeRequest  func(string, any) error
	ValidateOutput func(*kong.Context) error
	WriteOutput    func(*kong.Context, any) error
}

type Commands struct {
	Site       SiteCommands       `cmd:"" help:"オブジェクトストレージのサイト、プラン、状態を確認します。"`
	Account    AccountCommands    `cmd:"" help:"サイトアカウントとアカウントアクセスキーを管理します。"`
	Bucket     BucketCommands     `cmd:"" help:"バケットを作成・一覧・削除し、暗号化やレプリケーションを管理します。"`
	Permission PermissionCommands `cmd:"" help:"バケットごとのアクセス権限とアクセスキーを管理します。"`
	factory    APIFactory
	runtime    Runtime
}

func NewCommands(runtime Runtime, factory APIFactory) Commands {
	commands := Commands{
		Site:       SiteCommands{factory: factory, runtime: runtime},
		Account:    AccountCommands{factory: factory, runtime: runtime},
		Bucket:     BucketCommands{factory: factory, runtime: runtime},
		Permission: PermissionCommands{factory: factory, runtime: runtime},
		factory:    factory,
		runtime:    runtime,
	}
	commands.initialize()
	return commands
}

type SiteCommands struct {
	List    SiteListCommand   `cmd:"" help:"利用可能なサイトを一覧表示します。"`
	Read    SiteReadCommand   `cmd:"" help:"サイトの情報を取得します。"`
	Plans   SitePlansCommand  `cmd:"" help:"サイトで選択できるバケットプランを一覧表示します。"`
	Status  SiteStatusCommand `cmd:"" help:"サイトの状態、クォータ、バケット利用量を確認します。"`
	factory APIFactory
	runtime Runtime
}

type SiteListCommand struct {
	factory APIFactory
	runtime Runtime
}

func (c *SiteListCommand) Run(ctx *kong.Context) error {
	api, err := c.open("")
	if err != nil {
		return err
	}
	result, err := api.Site.List(context.Background())
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type SiteReadCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"情報を取得するサイト ID。サイト ID は site list の結果から確認します。"`
	factory APIFactory
	runtime Runtime
}

func (c *SiteReadCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.Site.Read(context.Background(), c.SiteID)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type SitePlansCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"プランを確認するサイト ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *SitePlansCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.Site.ListPlans(context.Background())
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type SiteStatusCommand struct {
	Read           SiteStatusReadCommand           `cmd:"" help:"サイトの状態を取得します。"`
	Quota          SiteStatusQuotaCommand          `cmd:"" help:"サイトのクォータを取得します。"`
	BucketMetering SiteStatusBucketMeteringCommand `cmd:"" name:"bucket-metering" help:"期間を指定してバケットの利用量を取得します。"`
	factory        APIFactory
	runtime        Runtime
}

type SiteStatusReadCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"状態を取得するサイト ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *SiteStatusReadCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.SiteStatus.Read(context.Background())
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type SiteStatusQuotaCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"クォータを取得するサイト ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *SiteStatusQuotaCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.SiteStatus.ReadQuota(context.Background())
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type SiteStatusBucketMeteringCommand struct {
	SiteID     string `arg:"" name:"site-id" help:"利用量を確認するサイト ID。"`
	BucketName string `arg:"" name:"bucket-name" help:"利用量を確認するバケット名。"`
	From       string `name:"from" required:"" help:"必須: RFC3339 形式の開始時刻。"`
	To         string `name:"to" required:"" help:"必須: RFC3339 形式の終了時刻。"`
	factory    APIFactory
	runtime    Runtime
}

func (c *SiteStatusBucketMeteringCommand) Run(ctx *kong.Context) error {
	from, err := time.Parse(time.RFC3339, c.From)
	if err != nil {
		return fmt.Errorf("--from は RFC3339 形式で指定してください: %w", err)
	}
	to, err := time.Parse(time.RFC3339, c.To)
	if err != nil {
		return fmt.Errorf("--to は RFC3339 形式で指定してください: %w", err)
	}
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.SiteStatus.ReadBucketMetering(context.Background(), c.BucketName, from, to)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type AccountCommands struct {
	Create    AccountCreateCommand     `cmd:"" help:"サイトアカウントを作成します。"`
	Read      AccountReadCommand       `cmd:"" help:"サイトアカウントを取得します。"`
	Delete    AccountDeleteCommand     `cmd:"" help:"サイトアカウントを削除します。"`
	AccessKey AccountAccessKeyCommands `cmd:"" name:"access-key" help:"サイトアカウントのアクセスキーを管理します。"`
	factory   APIFactory
	runtime   Runtime
}

type AccountCreateCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"サイトアカウントを作成するサイト ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *AccountCreateCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.Account.Create(context.Background())
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type AccountReadCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"サイトアカウントを取得するサイト ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *AccountReadCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.Account.Read(context.Background())
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type AccountDeleteCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"サイトアカウントを削除するサイト ID。サイト内のバケットなどを先に削除してください。"`
	factory APIFactory
	runtime Runtime
}

func (c *AccountDeleteCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	return api.Account.Delete(context.Background())
}

type AccountAccessKeyCommands struct {
	List    AccountAccessKeyListCommand   `cmd:"" help:"サイトアカウントのアクセスキーを一覧表示します。"`
	Create  AccountAccessKeyCreateCommand `cmd:"" help:"アクセスキーを発行します。シークレットは作成時の応答にのみ含まれます。"`
	Read    AccountAccessKeyReadCommand   `cmd:"" help:"アクセスキーの情報を取得します。"`
	Delete  AccountAccessKeyDeleteCommand `cmd:"" help:"アクセスキーを削除します。"`
	factory APIFactory
	runtime Runtime
}

type AccountAccessKeyListCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"アクセスキーを一覧表示するサイト ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *AccountAccessKeyListCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.Account.ListAccessKeys(context.Background())
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type AccountAccessKeyCreateCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"アクセスキーを発行するサイト ID。シークレットは作成時に一度だけ返されます。"`
	factory APIFactory
	runtime Runtime
}

func (c *AccountAccessKeyCreateCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.Account.CreateAccessKey(context.Background())
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type AccountAccessKeyReadCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"アクセスキーが属するサイト ID。"`
	KeyID   string `arg:"" name:"key-id" help:"情報を取得するアクセスキー ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *AccountAccessKeyReadCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.Account.ReadAccessKey(context.Background(), c.KeyID)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type AccountAccessKeyDeleteCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"アクセスキーが属するサイト ID。"`
	KeyID   string `arg:"" name:"key-id" help:"削除するアクセスキー ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *AccountAccessKeyDeleteCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	return api.Account.DeleteAccessKey(context.Background(), c.KeyID)
}

type BucketCommands struct {
	List        BucketListCommand         `cmd:"" help:"サイト内のバケットを一覧表示します。"`
	Create      BucketCreateCommand       `cmd:"" help:"バケットを作成します。現在利用できるサイトは isk01、tky01、arc02 です。arc02 では --plan を指定します。作成完了後からバケットの基本料金が発生します。"`
	Delete      BucketDeleteCommand       `cmd:"" help:"バケットを削除します。バケット内のオブジェクトを事前に削除してください。"`
	Encryption  BucketEncryptionCommands  `cmd:"" help:"バケット暗号化を管理します。"`
	Replication BucketReplicationCommands `cmd:"" help:"バケットのリージョン間レプリケーションを管理します。"`
	Penalty     BucketPenaltyCommand      `cmd:"" help:"バケットのペナルティ情報を取得します。"`
	Usage       BucketUsageCommand        `cmd:"" help:"バケットの利用量を取得します。"`
	Quota       BucketQuotaCommand        `cmd:"" help:"バケットのクォータを取得します。"`
	factory     APIFactory
	runtime     Runtime
}

type BucketListCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"一覧を取得するサイト ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *BucketListCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.Bucket.List(context.Background())
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type BucketCreateCommand struct {
	SiteID  string  `arg:"" name:"site-id" help:"バケットを作成するサイト ID。"`
	Bucket  string  `arg:"" name:"bucket-name" help:"作成する一意のバケット名。半角小文字英字・数字・ハイフンを使用できます。"`
	Plan    *string `name:"plan" help:"アーカイブプラン用サイト arc02 で必須です。site plans の service_class_path の末尾にあるプラン値を指定します。"`
	factory APIFactory
	runtime Runtime
}

func (c *BucketCreateCommand) Run(ctx *kong.Context) error {
	switch c.SiteID {
	case "isk01", "tky01":
		if c.Plan != nil {
			return fmt.Errorf("--plan はアーカイブプラン用サイト arc02 でのみ指定できます")
		}
	case "arc02":
		if c.Plan == nil {
			return fmt.Errorf("--site-id arc02 では --plan が必要です")
		}
	default:
		return fmt.Errorf("バケット作成に対応するサイト ID は isk01、tky01、arc02 です")
	}
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	plan := ""
	if c.Plan != nil {
		plan = *c.Plan
	}
	result, err := api.Bucket.Create(context.Background(), &objectstorage.BucketCreateParams{
		Bucket: c.Bucket,
		SiteId: c.SiteID,
		Plan:   plan,
	})
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type BucketDeleteCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"バケットが属するサイト ID。"`
	Bucket  string `arg:"" name:"bucket-name" help:"削除するバケット名。バケット内のオブジェクトを先に削除してください。"`
	factory APIFactory
	runtime Runtime
}

func (c *BucketDeleteCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	return api.Bucket.Delete(context.Background(), c.Bucket)
}

type BucketEncryptionCommands struct {
	Read    BucketEncryptionReadCommand    `cmd:"" help:"暗号化設定を取得します。"`
	Enable  BucketEncryptionEnableCommand  `cmd:"" help:"KMS キーを指定して暗号化を有効にします。"`
	Disable BucketEncryptionDisableCommand `cmd:"" help:"バケット暗号化を無効にします。"`
	factory APIFactory
	runtime Runtime
}

type BucketEncryptionReadCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"バケットが属するサイト ID。"`
	Bucket  string `arg:"" name:"bucket-name" help:"設定を取得するバケット名。"`
	factory APIFactory
	runtime Runtime
}

func (c *BucketEncryptionReadCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.BucketExtra(c.Bucket).ReadEncryption(context.Background())
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type BucketEncryptionEnableCommand struct {
	SiteID   string `arg:"" name:"site-id" help:"バケットが属するサイト ID。"`
	Bucket   string `arg:"" name:"bucket-name" help:"暗号化するバケット名。"`
	KMSKeyID string `name:"kms-key-id" required:"" help:"必須: 暗号化に使用する KMS キー ID。"`
	factory  APIFactory
	runtime  Runtime
}

func (c *BucketEncryptionEnableCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	return api.BucketExtra(c.Bucket).EnableEncryption(context.Background(), c.KMSKeyID)
}

type BucketEncryptionDisableCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"バケットが属するサイト ID。"`
	Bucket  string `arg:"" name:"bucket-name" help:"暗号化を無効にするバケット名。"`
	factory APIFactory
	runtime Runtime
}

func (c *BucketEncryptionDisableCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	return api.BucketExtra(c.Bucket).DisableEncryption(context.Background())
}

type BucketReplicationCommands struct {
	Read    BucketReplicationReadCommand    `cmd:"" help:"レプリケーション設定を取得します。"`
	Enable  BucketReplicationEnableCommand  `cmd:"" help:"別サイトのバケットを宛先にしてレプリケーションを有効にします。"`
	Disable BucketReplicationDisableCommand `cmd:"" help:"レプリケーションを無効にします。"`
	factory APIFactory
	runtime Runtime
}

type BucketReplicationReadCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"バケットが属するサイト ID。"`
	Bucket  string `arg:"" name:"bucket-name" help:"設定を取得するバケット名。"`
	factory APIFactory
	runtime Runtime
}

func (c *BucketReplicationReadCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.BucketExtra(c.Bucket).ReadReplication(context.Background())
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type BucketReplicationEnableCommand struct {
	SiteID       string `arg:"" name:"site-id" help:"レプリケーション元バケットのサイト ID。"`
	Bucket       string `arg:"" name:"bucket-name" help:"レプリケーション元バケット名。"`
	TargetBucket string `name:"target-bucket" required:"" help:"必須: レプリケーション先バケット名。"`
	factory      APIFactory
	runtime      Runtime
}

func (c *BucketReplicationEnableCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.BucketExtra(c.Bucket).EnableReplication(context.Background(), c.TargetBucket)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type BucketReplicationDisableCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"バケットが属するサイト ID。"`
	Bucket  string `arg:"" name:"bucket-name" help:"レプリケーションを無効にするバケット名。"`
	factory APIFactory
	runtime Runtime
}

func (c *BucketReplicationDisableCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	return api.BucketExtra(c.Bucket).DisableReplication(context.Background())
}

type BucketPenaltyCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"バケットが属するサイト ID。"`
	Bucket  string `arg:"" name:"bucket-name" help:"ペナルティ情報を取得するバケット名。"`
	factory APIFactory
	runtime Runtime
}

func (c *BucketPenaltyCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.BucketExtra(c.Bucket).ReadPenalty(context.Background())
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type BucketUsageCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"バケットが属するサイト ID。"`
	Bucket  string `arg:"" name:"bucket-name" help:"利用量を取得するバケット名。"`
	factory APIFactory
	runtime Runtime
}

func (c *BucketUsageCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.BucketExtra(c.Bucket).ReadUsage(context.Background())
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type BucketQuotaCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"バケットが属するサイト ID。"`
	Bucket  string `arg:"" name:"bucket-name" help:"クォータを取得するバケット名。"`
	factory APIFactory
	runtime Runtime
}

func (c *BucketQuotaCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.BucketExtra(c.Bucket).ReadQuota(context.Background())
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type PermissionCommands struct {
	List      PermissionListCommand       `cmd:"" help:"サイトのパーミッションを一覧表示します。"`
	Create    PermissionCreateCommand     `cmd:"" help:"バケット制御を指定してパーミッションを作成します。"`
	Read      PermissionReadCommand       `cmd:"" help:"パーミッションを取得します。"`
	Update    PermissionUpdateCommand     `cmd:"" help:"パーミッション名とバケット制御を更新します。"`
	Delete    PermissionDeleteCommand     `cmd:"" help:"パーミッションを削除します。"`
	AccessKey PermissionAccessKeyCommands `cmd:"" name:"access-key" help:"パーミッションのアクセスキーを管理します。シークレットは作成時の応答にのみ含まれます。"`
	factory   APIFactory
	runtime   Runtime
}

type PermissionListCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"一覧を取得するサイト ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *PermissionListCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.Permission.List(context.Background())
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type PermissionCreateCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"パーミッションを作成するサイト ID。"`
	Request string `name:"request" required:"" help:"必須: PermissionBucketControlsBody JSON を直接または @path.json で指定します。display_name と bucket_controls を指定してください。例: --request @permission.json"`
	factory APIFactory
	runtime Runtime
}

func (c *PermissionCreateCommand) Run(ctx *kong.Context) error {
	var request v2.PermissionBucketControlsBody
	if err := c.runtime.decodeRequest(c.Request, &request); err != nil {
		return err
	}
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.Permission.Create(context.Background(), string(request.DisplayName.Value), request.BucketControls)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type PermissionReadCommand struct {
	SiteID       string `arg:"" name:"site-id" help:"パーミッションが属するサイト ID。"`
	PermissionID string `arg:"" name:"permission-id" help:"参照するパーミッション ID。"`
	factory      APIFactory
	runtime      Runtime
}

func (c *PermissionReadCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.Permission.Read(context.Background(), c.PermissionID)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type PermissionUpdateCommand struct {
	SiteID       string `arg:"" name:"site-id" help:"パーミッションが属するサイト ID。"`
	PermissionID string `arg:"" name:"permission-id" help:"更新するパーミッション ID。"`
	Request      string `name:"request" required:"" help:"必須: PermissionBucketControlsBody JSON を直接または @path.json で指定します。display_name と bucket_controls を指定してください。省略したバケット制御を維持するかは指定した全項目で確認してください。例: --request @permission-update.json"`
	factory      APIFactory
	runtime      Runtime
}

func (c *PermissionUpdateCommand) Run(ctx *kong.Context) error {
	var request v2.PermissionBucketControlsBody
	if err := c.runtime.decodeRequest(c.Request, &request); err != nil {
		return err
	}
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.Permission.Update(context.Background(), c.PermissionID, string(request.DisplayName.Value), request.BucketControls)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type PermissionDeleteCommand struct {
	SiteID       string `arg:"" name:"site-id" help:"パーミッションが属するサイト ID。"`
	PermissionID string `arg:"" name:"permission-id" help:"削除するパーミッション ID。"`
	factory      APIFactory
	runtime      Runtime
}

func (c *PermissionDeleteCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	return api.Permission.Delete(context.Background(), c.PermissionID)
}

type PermissionAccessKeyCommands struct {
	List    PermissionAccessKeyListCommand   `cmd:"" help:"アクセスキーを一覧表示します。"`
	Create  PermissionAccessKeyCreateCommand `cmd:"" help:"アクセスキーを発行します。シークレットは作成時の応答にのみ含まれます。"`
	Read    PermissionAccessKeyReadCommand   `cmd:"" help:"アクセスキーの情報を取得します。"`
	Delete  PermissionAccessKeyDeleteCommand `cmd:"" help:"アクセスキーを削除します。"`
	factory APIFactory
	runtime Runtime
}

type PermissionAccessKeyListCommand struct {
	SiteID       string `arg:"" name:"site-id" help:"パーミッションが属するサイト ID。"`
	PermissionID string `arg:"" name:"permission-id" help:"アクセスキーを一覧表示するパーミッション ID。"`
	factory      APIFactory
	runtime      Runtime
}

func (c *PermissionAccessKeyListCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.Permission.ListAccessKeys(context.Background(), c.PermissionID)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type PermissionAccessKeyCreateCommand struct {
	SiteID       string `arg:"" name:"site-id" help:"パーミッションが属するサイト ID。"`
	PermissionID string `arg:"" name:"permission-id" help:"アクセスキーを発行するパーミッション ID。"`
	factory      APIFactory
	runtime      Runtime
}

func (c *PermissionAccessKeyCreateCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.Permission.CreateAccessKey(context.Background(), c.PermissionID)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type PermissionAccessKeyReadCommand struct {
	SiteID       string `arg:"" name:"site-id" help:"パーミッションが属するサイト ID。"`
	PermissionID string `arg:"" name:"permission-id" help:"アクセスキーが属するパーミッション ID。"`
	KeyID        string `arg:"" name:"key-id" help:"情報を取得するアクセスキー ID。"`
	factory      APIFactory
	runtime      Runtime
}

func (c *PermissionAccessKeyReadCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	result, err := api.Permission.ReadAccessKey(context.Background(), c.PermissionID, c.KeyID)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type PermissionAccessKeyDeleteCommand struct {
	SiteID       string `arg:"" name:"site-id" help:"パーミッションが属するサイト ID。"`
	PermissionID string `arg:"" name:"permission-id" help:"アクセスキーが属するパーミッション ID。"`
	KeyID        string `arg:"" name:"key-id" help:"削除するアクセスキー ID。"`
	factory      APIFactory
	runtime      Runtime
}

func (c *PermissionAccessKeyDeleteCommand) Run(ctx *kong.Context) error {
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	return api.Permission.DeleteAccessKey(context.Background(), c.PermissionID, c.KeyID)
}

func (c *SiteStatusCommand) setChildren() {
	c.Read.factory = c.factory
	c.Read.runtime = c.runtime
	c.Quota.factory = c.factory
	c.Quota.runtime = c.runtime
	c.BucketMetering.factory = c.factory
	c.BucketMetering.runtime = c.runtime
}

func (c *AccountCommands) setChildren() {
	c.AccessKey.factory = c.factory
	c.AccessKey.runtime = c.runtime
	c.AccessKey.List.factory = c.factory
	c.AccessKey.List.runtime = c.runtime
	c.AccessKey.Create.factory = c.factory
	c.AccessKey.Create.runtime = c.runtime
	c.AccessKey.Read.factory = c.factory
	c.AccessKey.Read.runtime = c.runtime
	c.AccessKey.Delete.factory = c.factory
	c.AccessKey.Delete.runtime = c.runtime
}

func (c *BucketCommands) setChildren() {
	c.Encryption.factory = c.factory
	c.Encryption.runtime = c.runtime
	c.Encryption.Read.factory = c.factory
	c.Encryption.Read.runtime = c.runtime
	c.Encryption.Enable.factory = c.factory
	c.Encryption.Enable.runtime = c.runtime
	c.Encryption.Disable.factory = c.factory
	c.Encryption.Disable.runtime = c.runtime
	c.Replication.factory = c.factory
	c.Replication.runtime = c.runtime
	c.Replication.Read.factory = c.factory
	c.Replication.Read.runtime = c.runtime
	c.Replication.Enable.factory = c.factory
	c.Replication.Enable.runtime = c.runtime
	c.Replication.Disable.factory = c.factory
	c.Replication.Disable.runtime = c.runtime
}

func (c *PermissionCommands) setChildren() {
	c.AccessKey.factory = c.factory
	c.AccessKey.runtime = c.runtime
	c.AccessKey.List.factory = c.factory
	c.AccessKey.List.runtime = c.runtime
	c.AccessKey.Create.factory = c.factory
	c.AccessKey.Create.runtime = c.runtime
	c.AccessKey.Read.factory = c.factory
	c.AccessKey.Read.runtime = c.runtime
	c.AccessKey.Delete.factory = c.factory
	c.AccessKey.Delete.runtime = c.runtime
}

func (c *Commands) initialize() {
	c.Site.initialize()
	c.Account.initialize()
	c.Bucket.initialize()
	c.Permission.initialize()
}

func (c *SiteListCommand) open(siteID string) (API, error)        { return openAPI(c.factory, siteID) }
func (c *SiteReadCommand) open(siteID string) (API, error)        { return openAPI(c.factory, siteID) }
func (c *SitePlansCommand) open(siteID string) (API, error)       { return openAPI(c.factory, siteID) }
func (c *SiteStatusReadCommand) open(siteID string) (API, error)  { return openAPI(c.factory, siteID) }
func (c *SiteStatusQuotaCommand) open(siteID string) (API, error) { return openAPI(c.factory, siteID) }
func (c *SiteStatusBucketMeteringCommand) open(siteID string) (API, error) {
	return openAPI(c.factory, siteID)
}
func (c *AccountCreateCommand) open(siteID string) (API, error) { return openAPI(c.factory, siteID) }
func (c *AccountReadCommand) open(siteID string) (API, error)   { return openAPI(c.factory, siteID) }
func (c *AccountDeleteCommand) open(siteID string) (API, error) { return openAPI(c.factory, siteID) }
func (c *AccountAccessKeyListCommand) open(siteID string) (API, error) {
	return openAPI(c.factory, siteID)
}
func (c *AccountAccessKeyCreateCommand) open(siteID string) (API, error) {
	return openAPI(c.factory, siteID)
}
func (c *AccountAccessKeyReadCommand) open(siteID string) (API, error) {
	return openAPI(c.factory, siteID)
}
func (c *AccountAccessKeyDeleteCommand) open(siteID string) (API, error) {
	return openAPI(c.factory, siteID)
}
func (c *BucketListCommand) open(siteID string) (API, error)   { return openAPI(c.factory, siteID) }
func (c *BucketCreateCommand) open(siteID string) (API, error) { return openAPI(c.factory, siteID) }
func (c *BucketDeleteCommand) open(siteID string) (API, error) { return openAPI(c.factory, siteID) }
func (c *BucketEncryptionReadCommand) open(siteID string) (API, error) {
	return openAPI(c.factory, siteID)
}
func (c *BucketEncryptionEnableCommand) open(siteID string) (API, error) {
	return openAPI(c.factory, siteID)
}
func (c *BucketEncryptionDisableCommand) open(siteID string) (API, error) {
	return openAPI(c.factory, siteID)
}
func (c *BucketReplicationReadCommand) open(siteID string) (API, error) {
	return openAPI(c.factory, siteID)
}
func (c *BucketReplicationEnableCommand) open(siteID string) (API, error) {
	return openAPI(c.factory, siteID)
}
func (c *BucketReplicationDisableCommand) open(siteID string) (API, error) {
	return openAPI(c.factory, siteID)
}
func (c *BucketPenaltyCommand) open(siteID string) (API, error) { return openAPI(c.factory, siteID) }
func (c *BucketUsageCommand) open(siteID string) (API, error)   { return openAPI(c.factory, siteID) }
func (c *BucketQuotaCommand) open(siteID string) (API, error)   { return openAPI(c.factory, siteID) }
func (c *PermissionListCommand) open(siteID string) (API, error) {
	return openAPI(c.factory, siteID)
}
func (c *PermissionCreateCommand) open(siteID string) (API, error) {
	return openAPI(c.factory, siteID)
}
func (c *PermissionReadCommand) open(siteID string) (API, error) {
	return openAPI(c.factory, siteID)
}
func (c *PermissionUpdateCommand) open(siteID string) (API, error) {
	return openAPI(c.factory, siteID)
}
func (c *PermissionDeleteCommand) open(siteID string) (API, error) {
	return openAPI(c.factory, siteID)
}
func (c *PermissionAccessKeyListCommand) open(siteID string) (API, error) {
	return openAPI(c.factory, siteID)
}
func (c *PermissionAccessKeyCreateCommand) open(siteID string) (API, error) {
	return openAPI(c.factory, siteID)
}
func (c *PermissionAccessKeyReadCommand) open(siteID string) (API, error) {
	return openAPI(c.factory, siteID)
}
func (c *PermissionAccessKeyDeleteCommand) open(siteID string) (API, error) {
	return openAPI(c.factory, siteID)
}

func openAPI(factory APIFactory, siteID string) (API, error) {
	if factory == nil {
		return API{}, fmt.Errorf("API クライアントが設定されていません")
	}
	return factory(siteID)
}

func (r Runtime) decodeRequest(input string, destination any) error {
	if r.DecodeRequest == nil {
		return fmt.Errorf("API リクエスト入力が設定されていません")
	}
	return r.DecodeRequest(input, destination)
}

func (r Runtime) writeOutput(ctx *kong.Context, value any) error {
	if r.ValidateOutput == nil || r.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	if err := r.ValidateOutput(ctx); err != nil {
		return err
	}
	return r.WriteOutput(ctx, value)
}

func (c *SiteCommands) initialize() {
	c.List.factory = c.factory
	c.List.runtime = c.runtime
	c.Read.factory = c.factory
	c.Read.runtime = c.runtime
	c.Plans.factory = c.factory
	c.Plans.runtime = c.runtime
	c.Status.factory = c.factory
	c.Status.runtime = c.runtime
	c.Status.setChildren()
}

func (c *AccountCommands) initialize() {
	c.Create.factory = c.factory
	c.Create.runtime = c.runtime
	c.Read.factory = c.factory
	c.Read.runtime = c.runtime
	c.Delete.factory = c.factory
	c.Delete.runtime = c.runtime
	c.setChildren()
}

func (c *BucketCommands) initialize() {
	c.List.factory = c.factory
	c.List.runtime = c.runtime
	c.Create.factory = c.factory
	c.Create.runtime = c.runtime
	c.Delete.factory = c.factory
	c.Delete.runtime = c.runtime
	c.Penalty.factory = c.factory
	c.Penalty.runtime = c.runtime
	c.Usage.factory = c.factory
	c.Usage.runtime = c.runtime
	c.Quota.factory = c.factory
	c.Quota.runtime = c.runtime
	c.setChildren()
}

func (c *PermissionCommands) initialize() {
	c.List.factory = c.factory
	c.List.runtime = c.runtime
	c.Create.factory = c.factory
	c.Create.runtime = c.runtime
	c.Read.factory = c.factory
	c.Read.runtime = c.runtime
	c.Update.factory = c.factory
	c.Update.runtime = c.runtime
	c.Delete.factory = c.factory
	c.Delete.runtime = c.runtime
	c.setChildren()
}
