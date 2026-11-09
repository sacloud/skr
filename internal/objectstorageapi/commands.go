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

// Commands はオブジェクトストレージ管理 API の手書きコマンド集合です。
// operationId 派生名への移行 (ADR 0074) に合わせ、site / account / bucket / permission の
// グループ階層を Kong の embed で展開して 1 階層のフラット構造にしています。
// サブコマンド名は OpenAPI の operationId から機械派生した名前です。
type Commands struct {
	Site       SiteCommands       `embed:""`
	Account    AccountCommands    `embed:""`
	Bucket     BucketCommands     `embed:""`
	Permission PermissionCommands `embed:""`
}

func NewCommands(runtime Runtime, factory APIFactory) Commands {
	return Commands{
		Site:       newSiteCommands(runtime, factory),
		Account:    newAccountCommands(runtime, factory),
		Bucket:     newBucketCommands(runtime, factory),
		Permission: newPermissionCommands(runtime, factory),
	}
}

type SiteCommands struct {
	GetClusters       SiteGetClustersCommand       `cmd:"" name:"get-clusters" help:"利用可能なサイトを一覧表示します。"`
	GetCluster        SiteGetClusterCommand        `cmd:"" name:"get-cluster" help:"サイトの情報を取得します。"`
	GetPlans          SiteGetPlansCommand          `cmd:"" name:"get-plans" help:"サイトで選択できるバケットプランを一覧表示します。"`
	GetStatus         SiteGetStatusCommand         `cmd:"" name:"get-status" help:"サイトの状態を取得します。"`
	GetQuota          SiteGetQuotaCommand          `cmd:"" name:"get-quota" help:"サイトのクォータを取得します。"`
	GetBucketMetering SiteGetBucketMeteringCommand `cmd:"" name:"get-bucket-metering" help:"期間を指定してバケットの利用量を取得します。"`
}

func newSiteCommands(runtime Runtime, factory APIFactory) SiteCommands {
	commands := SiteCommands{
		GetClusters:       SiteGetClustersCommand{factory: factory, runtime: runtime},
		GetCluster:        SiteGetClusterCommand{factory: factory, runtime: runtime},
		GetPlans:          SiteGetPlansCommand{factory: factory, runtime: runtime},
		GetStatus:         SiteGetStatusCommand{factory: factory, runtime: runtime},
		GetQuota:          SiteGetQuotaCommand{factory: factory, runtime: runtime},
		GetBucketMetering: SiteGetBucketMeteringCommand{factory: factory, runtime: runtime},
	}
	return commands
}

type SiteGetClustersCommand struct {
	factory APIFactory
	runtime Runtime
}

func (c *SiteGetClustersCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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

type SiteGetClusterCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"情報を取得するサイト ID。サイト ID は get-clusters の結果から確認します。"`
	factory APIFactory
	runtime Runtime
}

func (c *SiteGetClusterCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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

type SiteGetPlansCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"プランを確認するサイト ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *SiteGetPlansCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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

type SiteGetStatusCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"状態を取得するサイト ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *SiteGetStatusCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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

type SiteGetQuotaCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"クォータを取得するサイト ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *SiteGetQuotaCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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

type SiteGetBucketMeteringCommand struct {
	SiteID     string `arg:"" name:"site-id" help:"利用量を確認するサイト ID。"`
	BucketName string `arg:"" name:"bucket-name" help:"利用量を確認するバケット名。"`
	From       string `name:"from" required:"" help:"必須: RFC3339 形式の開始時刻。"`
	To         string `name:"to" required:"" help:"必須: RFC3339 形式の終了時刻。"`
	factory    APIFactory
	runtime    Runtime
}

func (c *SiteGetBucketMeteringCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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
	CreateAccount    AccountCreateCommand          `cmd:"" name:"create-account" help:"サイトアカウントを作成します。"`
	GetAccount       AccountReadCommand            `cmd:"" name:"get-account" help:"サイトアカウントを取得します。"`
	DeleteAccount    AccountDeleteCommand          `cmd:"" name:"delete-account" help:"サイトアカウントを削除します。サイト内のバケットなどを先に削除してください。"`
	GetAccountKeys   AccountAccessKeyListCommand   `cmd:"" name:"get-account-keys" help:"サイトアカウントのアクセスキーを一覧表示します。"`
	CreateAccountKey AccountAccessKeyCreateCommand `cmd:"" name:"create-account-key" help:"アクセスキーを発行します。シークレットは作成時の応答にのみ含まれます。"`
	GetAccountKey    AccountAccessKeyReadCommand   `cmd:"" name:"get-account-key" help:"アクセスキーの情報を取得します。"`
	DeleteAccountKey AccountAccessKeyDeleteCommand `cmd:"" name:"delete-account-key" help:"アクセスキーを削除します。"`
}

func newAccountCommands(runtime Runtime, factory APIFactory) AccountCommands {
	return AccountCommands{
		CreateAccount:    AccountCreateCommand{factory: factory, runtime: runtime},
		GetAccount:       AccountReadCommand{factory: factory, runtime: runtime},
		DeleteAccount:    AccountDeleteCommand{factory: factory, runtime: runtime},
		GetAccountKeys:   AccountAccessKeyListCommand{factory: factory, runtime: runtime},
		CreateAccountKey: AccountAccessKeyCreateCommand{factory: factory, runtime: runtime},
		GetAccountKey:    AccountAccessKeyReadCommand{factory: factory, runtime: runtime},
		DeleteAccountKey: AccountAccessKeyDeleteCommand{factory: factory, runtime: runtime},
	}
}

type AccountCreateCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"サイトアカウントを作成するサイト ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *AccountCreateCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	return api.Account.Delete(context.Background())
}

type AccountAccessKeyListCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"アクセスキーを一覧表示するサイト ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *AccountAccessKeyListCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	return api.Account.DeleteAccessKey(context.Background(), c.KeyID)
}

type BucketCommands struct {
	ListBuckets             BucketListCommand               `cmd:"" name:"list-buckets" help:"サイト内のバケットを一覧表示します。"`
	CreateBucket            BucketCreateCommand             `cmd:"" name:"create-bucket" help:"バケットを作成します。現在利用できるサイトは isk01、tky01、arc02 です。arc02 では --plan を指定します。作成完了後からバケットの基本料金が発生します。"`
	DeleteBucket            BucketDeleteCommand             `cmd:"" name:"delete-bucket" help:"指定サイトに所属するバケットを削除します。所属を確認できない場合は拒否します。バケット内のオブジェクトを事前に削除してください。"`
	GetBucketEncryption     BucketEncryptionReadCommand     `cmd:"" name:"get-bucket-encryption" help:"暗号化設定を取得します。"`
	PutBucketEncryption     BucketEncryptionEnableCommand   `cmd:"" name:"put-bucket-encryption" help:"KMS キーを指定して暗号化を有効にします。"`
	DeleteBucketEncryption  BucketEncryptionDisableCommand  `cmd:"" name:"delete-bucket-encryption" help:"バケット暗号化を無効にします。"`
	GetBucketReplication    BucketReplicationReadCommand    `cmd:"" name:"get-bucket-replication" help:"レプリケーション設定を取得します。"`
	PostBucketReplication   BucketReplicationEnableCommand  `cmd:"" name:"post-bucket-replication" help:"別サイトのバケットを宛先にしてレプリケーションを有効にします。"`
	DeleteBucketReplication BucketReplicationDisableCommand `cmd:"" name:"delete-bucket-replication" help:"レプリケーションを無効にします。"`
	GetBucketPenalty        BucketPenaltyCommand            `cmd:"" name:"get-bucket-penalty" help:"バケットのペナルティ情報を取得します。"`
	GetBucketUsage          BucketUsageCommand              `cmd:"" name:"get-bucket-usage" help:"バケットの利用量を取得します。"`
	GetBucketQuota          BucketQuotaCommand              `cmd:"" name:"get-bucket-quota" help:"バケットのクォータを取得します。"`
}

func newBucketCommands(runtime Runtime, factory APIFactory) BucketCommands {
	commands := BucketCommands{
		ListBuckets:             BucketListCommand{factory: factory, runtime: runtime},
		CreateBucket:            BucketCreateCommand{factory: factory, runtime: runtime},
		DeleteBucket:            BucketDeleteCommand{factory: factory, runtime: runtime},
		GetBucketEncryption:     BucketEncryptionReadCommand{factory: factory, runtime: runtime},
		PutBucketEncryption:     BucketEncryptionEnableCommand{factory: factory, runtime: runtime},
		DeleteBucketEncryption:  BucketEncryptionDisableCommand{factory: factory, runtime: runtime},
		GetBucketReplication:    BucketReplicationReadCommand{factory: factory, runtime: runtime},
		PostBucketReplication:   BucketReplicationEnableCommand{factory: factory, runtime: runtime},
		DeleteBucketReplication: BucketReplicationDisableCommand{factory: factory, runtime: runtime},
		GetBucketPenalty:        BucketPenaltyCommand{factory: factory, runtime: runtime},
		GetBucketUsage:          BucketUsageCommand{factory: factory, runtime: runtime},
		GetBucketQuota:          BucketQuotaCommand{factory: factory, runtime: runtime},
	}
	return commands
}

type BucketListCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"一覧を取得するサイト ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *BucketListCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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
	Plan    *string `name:"plan" help:"アーカイブプラン用サイト arc02 で必須です。get-plans の service_class_path の末尾にあるプラン値を指定します。"`
	factory APIFactory
	runtime Runtime
}

func (c *BucketCreateCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	if err := requireBucketInSite(context.Background(), api, c.SiteID, c.Bucket); err != nil {
		return err
	}
	return api.Bucket.Delete(context.Background(), c.Bucket)
}

type BucketEncryptionReadCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"バケットが属するサイト ID。"`
	Bucket  string `arg:"" name:"bucket-name" help:"設定を取得するバケット名。"`
	factory APIFactory
	runtime Runtime
}

func (c *BucketEncryptionReadCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	return api.BucketExtra(c.Bucket).DisableEncryption(context.Background())
}

type BucketReplicationReadCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"バケットが属するサイト ID。"`
	Bucket  string `arg:"" name:"bucket-name" help:"設定を取得するバケット名。"`
	factory APIFactory
	runtime Runtime
}

func (c *BucketReplicationReadCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	if err := requireBucketInSite(context.Background(), api, c.SiteID, c.Bucket); err != nil {
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
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	if err := requireBucketInSite(context.Background(), api, c.SiteID, c.Bucket); err != nil {
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
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	if err := requireBucketInSite(context.Background(), api, c.SiteID, c.Bucket); err != nil {
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
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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
	GetPermissions      PermissionListCommand            `cmd:"" name:"get-permissions" help:"サイトのパーミッションを一覧表示します。"`
	CreatePermission    PermissionCreateCommand          `cmd:"" name:"create-permission" help:"バケット制御を指定してパーミッションを作成します。"`
	GetPermission       PermissionReadCommand            `cmd:"" name:"get-permission" help:"パーミッションを取得します。"`
	UpdatePermission    PermissionUpdateCommand          `cmd:"" name:"update-permission" help:"パーミッション名とバケット制御を更新します。"`
	DeletePermission    PermissionDeleteCommand          `cmd:"" name:"delete-permission" help:"パーミッションを削除します。"`
	GetPermissionKeys   PermissionAccessKeyListCommand   `cmd:"" name:"get-permission-keys" help:"パーミッションのアクセスキーを一覧表示します。"`
	CreatePermissionKey PermissionAccessKeyCreateCommand `cmd:"" name:"create-permission-key" help:"パーミッションのアクセスキーを発行します。シークレットは作成時の応答にのみ含まれます。"`
	GetPermissionKey    PermissionAccessKeyReadCommand   `cmd:"" name:"get-permission-key" help:"パーミッションのアクセスキーの情報を取得します。"`
	DeletePermissionKey PermissionAccessKeyDeleteCommand `cmd:"" name:"delete-permission-key" help:"パーミッションのアクセスキーを削除します。"`
}

func newPermissionCommands(runtime Runtime, factory APIFactory) PermissionCommands {
	return PermissionCommands{
		GetPermissions:      PermissionListCommand{factory: factory, runtime: runtime},
		CreatePermission:    PermissionCreateCommand{factory: factory, runtime: runtime},
		GetPermission:       PermissionReadCommand{factory: factory, runtime: runtime},
		UpdatePermission:    PermissionUpdateCommand{factory: factory, runtime: runtime},
		DeletePermission:    PermissionDeleteCommand{factory: factory, runtime: runtime},
		GetPermissionKeys:   PermissionAccessKeyListCommand{factory: factory, runtime: runtime},
		CreatePermissionKey: PermissionAccessKeyCreateCommand{factory: factory, runtime: runtime},
		GetPermissionKey:    PermissionAccessKeyReadCommand{factory: factory, runtime: runtime},
		DeletePermissionKey: PermissionAccessKeyDeleteCommand{factory: factory, runtime: runtime},
	}
}

type PermissionListCommand struct {
	SiteID  string `arg:"" name:"site-id" help:"一覧を取得するサイト ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *PermissionListCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	return api.Permission.Delete(context.Background(), c.PermissionID)
}

type PermissionAccessKeyListCommand struct {
	SiteID       string `arg:"" name:"site-id" help:"パーミッションが属するサイト ID。"`
	PermissionID string `arg:"" name:"permission-id" help:"アクセスキーを一覧表示するパーミッション ID。"`
	factory      APIFactory
	runtime      Runtime
}

func (c *PermissionAccessKeyListCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
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
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	api, err := c.open(c.SiteID)
	if err != nil {
		return err
	}
	return api.Permission.DeleteAccessKey(context.Background(), c.PermissionID, c.KeyID)
}

func (c *SiteGetClustersCommand) open(siteID string) (API, error) { return openAPI(c.factory, siteID) }
func (c *SiteGetClusterCommand) open(siteID string) (API, error)  { return openAPI(c.factory, siteID) }
func (c *SiteGetPlansCommand) open(siteID string) (API, error)    { return openAPI(c.factory, siteID) }
func (c *SiteGetStatusCommand) open(siteID string) (API, error)   { return openAPI(c.factory, siteID) }
func (c *SiteGetQuotaCommand) open(siteID string) (API, error)    { return openAPI(c.factory, siteID) }
func (c *SiteGetBucketMeteringCommand) open(siteID string) (API, error) {
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

func requireBucketInSite(ctx context.Context, api API, siteID, name string) error {
	buckets, err := api.Bucket.List(ctx)
	if err != nil {
		return fmt.Errorf("サイト %q のバケット所属確認に失敗しました: %w", siteID, err)
	}
	matches := 0
	for _, bucket := range buckets {
		if string(bucket.Name) == name {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("サイト %q でバケット %q を一意に確認できないため、操作を拒否しました", siteID, name)
	}
	return nil
}

func (r Runtime) decodeRequest(input string, destination any) error {
	if r.DecodeRequest == nil {
		return fmt.Errorf("API リクエスト入力が設定されていません")
	}
	return r.DecodeRequest(input, destination)
}

func (r Runtime) validateOutput(ctx *kong.Context) error {
	if r.ValidateOutput == nil || r.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.ValidateOutput(ctx)
}

func (r Runtime) writeOutput(ctx *kong.Context, value any) error {
	return r.WriteOutput(ctx, value)
}
