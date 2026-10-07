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

package apprundedicatedapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/alecthomas/kong"
	apprun_dedicated "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated"
	"github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/application"
	"github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/autoscalinggroup"
	"github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/certificate"
	"github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/cluster"
	"github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/loadbalancer"

	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	"github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/version"
	"github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/workernode"
	clientconfig "github.com/sacloud/skr/internal/saclient"
)

type Commands struct {
	Cluster          ClusterCommands          `cmd:"" help:"クラスタを管理します。"`
	Application      ApplicationCommands      `cmd:"" help:"アプリケーションを管理します。クラスタ作成後に登録します。"`
	Version          VersionCommands          `cmd:"" help:"アプリケーションのバージョンを管理します。アプリケーション ID を指定します。"`
	AutoScalingGroup AutoScalingGroupCommands `cmd:"" name:"auto-scaling-group" help:"クラスタのワーカノードグループを管理します。"`
	WorkerNode       WorkerNodeCommands       `cmd:"" name:"worker-node" help:"オートスケーリンググループのワーカノードを参照・更新します。"`
	LoadBalancer     LoadBalancerCommands     `cmd:"" name:"load-balancer" help:"オートスケーリンググループのロードバランサを管理します。"`
	Certificate      CertificateCommands      `cmd:"" help:"クラスタの証明書を管理します。"`
	ServiceClass     ServiceClassCommands     `cmd:"" name:"service-class" help:"利用可能なワーカ／ロードバランサのサービスクラスを取得します。"`
}

type Runtime struct {
	NewClient   func() (*v1.Client, error)
	OutputType  func(*kong.Context) (string, error)
	WriteOutput func(*kong.Context, string, any, ...[]string) error
}

func (c *Commands) SetRuntime(runtime Runtime) {
	c.Cluster.setRuntime(runtime)
	c.Application.setRuntime(runtime)
	c.Version.setRuntime(runtime)
	c.AutoScalingGroup.setRuntime(runtime)
	c.WorkerNode.setRuntime(runtime)
	c.LoadBalancer.setRuntime(runtime)
	c.Certificate.setRuntime(runtime)
	c.ServiceClass.setRuntime(runtime)
}

func NewClient(trace bool) (*v1.Client, error) {
	client, err := clientconfig.New(trace)
	if err != nil {
		return nil, err
	}
	return apprun_dedicated.NewClient(client)
}

type page[T any] struct {
	Items      []T `json:"items"`
	NextCursor any `json:"nextCursor,omitempty"`
}

func runValue[T any](ctx *kong.Context, runtime Runtime, call func(context.Context, *v1.Client) (T, error)) error {
	if runtime.OutputType == nil || runtime.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	format, err := runtime.OutputType(ctx)
	if err != nil {
		return err
	}
	if runtime.NewClient == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	client, err := runtime.NewClient()
	if err != nil {
		return err
	}
	result, err := call(context.Background(), client)
	if err != nil {
		return err
	}
	return runtime.WriteOutput(ctx, format, result)
}

func runNoOutput(ctx context.Context, runtime Runtime, call func(context.Context, *v1.Client) error) error {
	if runtime.NewClient == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	client, err := runtime.NewClient()
	if err != nil {
		return err
	}
	return call(ctx, client)
}

func readRequest(input string, sensitive bool) ([]byte, error) {
	if sensitive && !strings.HasPrefix(input, "@") && input != "-" {
		return nil, fmt.Errorf("秘密情報を含む可能性があるため、--request は @path.json または - (標準入力) で指定してください")
	}
	if input == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("標準入力からリクエストを読み込めません: %w", err)
		}
		return data, nil
	}
	if strings.HasPrefix(input, "@") {
		data, err := os.ReadFile(strings.TrimPrefix(input, "@"))
		if err != nil {
			return nil, fmt.Errorf("リクエストファイルを読み込めません: %w", err)
		}
		return data, nil
	}
	return []byte(input), nil
}

func decodeRequest(input string, sensitive bool, destination any) error {
	data, err := readRequest(input, sensitive)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("リクエスト JSON を読み込めません: %w", err)
	}
	return nil
}

func parseID[T any](value string) (T, error) {
	var id T
	data, err := json.Marshal(value)
	if err != nil {
		return id, fmt.Errorf("ID を読み込めません: %w", err)
	}
	if err := json.Unmarshal(data, &id); err != nil {
		return id, fmt.Errorf("ID %q の形式が正しくありません: %w", value, err)
	}
	return id, nil
}

func parseCursor[T any](value *string) (*T, error) {
	if value == nil {
		return nil, nil
	}
	cursor, err := parseID[T](*value)
	if err != nil {
		return nil, fmt.Errorf("カーソル: %w", err)
	}
	return &cursor, nil
}

func parseVersionNumber(value string) (v1.ApplicationVersionNumber, error) {
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("バージョン %q は32ビット整数で指定してください: %w", value, err)
	}
	return v1.ApplicationVersionNumber(parsed), nil
}

type ClusterCommands struct {
	List   ClusterListCommand   `cmd:"" help:"クラスタを一覧表示します。JSON 出力には items と nextCursor が含まれます。"`
	Read   ClusterReadCommand   `cmd:"" help:"クラスタ ID を指定して詳細を取得します。"`
	Create ClusterCreateCommand `cmd:"" help:"クラスタを作成します。サービスプリンシパル ID とポート設定を含む JSON を指定します。"`
	Update ClusterUpdateCommand `cmd:"" help:"クラスタの設定を更新します。更新対象の JSON を指定します。"`
	Delete ClusterDeleteCommand `cmd:"" help:"クラスタを削除します。関連リソースがないことを確認してください。"`
}

func (c *ClusterCommands) setRuntime(runtime Runtime) {
	c.List.runtime, c.Read.runtime, c.Create.runtime, c.Update.runtime, c.Delete.runtime = runtime, runtime, runtime, runtime, runtime
}

type ClusterListCommand struct {
	MaxItems int64   `name:"max-items" default:"100" help:"1ページあたりの最大件数。"`
	Cursor   *string `help:"前ページの nextCursor。"`
	runtime  Runtime
}

func (c *ClusterListCommand) Run(ctx *kong.Context) error {
	cursor, err := parseCursor[v1.ClusterID](c.Cursor)
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (page[cluster.ClusterDetail], error) {
		items, next, err := apprun_dedicated.NewClusterOp(client).List(ctx, c.MaxItems, cursor)
		return page[cluster.ClusterDetail]{Items: items, NextCursor: next}, err
	})
}

type ClusterReadCommand struct {
	ID      string `arg:"" help:"参照するクラスタ ID。"`
	runtime Runtime
}

func (c *ClusterReadCommand) Run(ctx *kong.Context) error {
	id, err := parseID[v1.ClusterID](c.ID)
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*cluster.ClusterDetail, error) {
		return apprun_dedicated.NewClusterOp(client).Read(ctx, id)
	})
}

type ClusterCreateCommand struct {
	Request *string `required:"" help:"cluster.CreateParams JSON。直接 JSON または @path.json で指定します。必須項目は name と servicePrincipalID です。ports はロードバランサの待受ポート設定です。"`
	runtime Runtime
}

func (c *ClusterCreateCommand) Run(ctx *kong.Context) error {
	var request cluster.CreateParams
	if err := decodeRequest(*c.Request, false, &request); err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*v1.CreatedCluster, error) {
		return apprun_dedicated.NewClusterOp(client).Create(ctx, request)
	})
}

type ClusterUpdateCommand struct {
	ID      string  `arg:"" help:"更新するクラスタ ID。"`
	Request *string `required:"" help:"cluster.UpdateParams JSON。直接 JSON または @path.json で指定します。"`
	runtime Runtime
}

func (c *ClusterUpdateCommand) Run(ctx *kong.Context) error {
	id, err := parseID[v1.ClusterID](c.ID)
	if err != nil {
		return err
	}
	var request cluster.UpdateParams
	if err := decodeRequest(*c.Request, false, &request); err != nil {
		return err
	}
	return runNoOutput(context.Background(), c.runtime, func(ctx context.Context, client *v1.Client) error {
		return apprun_dedicated.NewClusterOp(client).Update(ctx, id, request)
	})
}

type ClusterDeleteCommand struct {
	ID      string `arg:"" help:"削除するクラスタ ID。"`
	runtime Runtime
}

func (c *ClusterDeleteCommand) Run(ctx *kong.Context) error {
	id, err := parseID[v1.ClusterID](c.ID)
	if err != nil {
		return err
	}
	return runNoOutput(context.Background(), c.runtime, func(ctx context.Context, client *v1.Client) error {
		return apprun_dedicated.NewClusterOp(client).Delete(ctx, id)
	})
}

type ApplicationCommands struct {
	List       ApplicationListCommand       `cmd:"" help:"アプリケーションを一覧表示します。"`
	Create     ApplicationCreateCommand     `cmd:"" help:"アプリケーションをクラスタに登録します。"`
	Read       ApplicationReadCommand       `cmd:"" help:"アプリケーション ID を指定して詳細を取得します。"`
	Update     ApplicationUpdateCommand     `cmd:"" help:"アクティブバージョンを切り替えるか、--deactivate で停止します。"`
	Delete     ApplicationDeleteCommand     `cmd:"" help:"アプリケーションを削除します。削除前に対象 ID を確認してください。"`
	Containers ApplicationContainersCommand `cmd:"" help:"アプリケーションのコンテナ配置と状態を取得します。"`
}

func (c *ApplicationCommands) setRuntime(runtime Runtime) {
	c.List.runtime, c.Create.runtime, c.Read.runtime = runtime, runtime, runtime
	c.Update.runtime, c.Delete.runtime, c.Containers.runtime = runtime, runtime, runtime
}

type ApplicationListCommand struct {
	MaxItems int64   `name:"max-items" default:"100" help:"1ページあたりの最大件数。"`
	Cursor   *string `help:"前ページの nextCursor。"`
	runtime  Runtime
}

func (c *ApplicationListCommand) Run(ctx *kong.Context) error {
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (page[v1.ReadApplicationDetail], error) {
		items, next, err := apprun_dedicated.NewApplicationOp(client).List(ctx, c.MaxItems, c.Cursor)
		return page[v1.ReadApplicationDetail]{Items: items, NextCursor: next}, err
	})
}

type ApplicationCreateCommand struct {
	Name      string `required:"" help:"アプリケーション名。"`
	ClusterID string `name:"cluster-id" required:"" help:"作成先クラスタの ID。"`
	runtime   Runtime
}

func (c *ApplicationCreateCommand) Run(ctx *kong.Context) error {
	clusterID, err := parseID[v1.ClusterID](c.ClusterID)
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*v1.CreatedApplication, error) {
		return apprun_dedicated.NewApplicationOp(client).Create(ctx, c.Name, clusterID)
	})
}

type ApplicationReadCommand struct {
	ID      string `arg:"" help:"参照するアプリケーション ID。"`
	runtime Runtime
}

func (c *ApplicationReadCommand) Run(ctx *kong.Context) error {
	id, err := parseID[v1.ApplicationID](c.ID)
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*application.ApplicationDetail, error) {
		return apprun_dedicated.NewApplicationOp(client).Read(ctx, id)
	})
}

type ApplicationUpdateCommand struct {
	ID            string `arg:"" help:"更新するアプリケーション ID。"`
	ActiveVersion *int32 `name:"active-version" help:"アクティブにするバージョン番号。"`
	Deactivate    bool   `help:"アクティブバージョンを解除します。--active-version とは併用できません。"`
	runtime       Runtime
}

func (c *ApplicationUpdateCommand) Run(ctx *kong.Context) error {
	if (c.ActiveVersion == nil) == !c.Deactivate {
		return fmt.Errorf("--active-version または --deactivate のいずれか一方を指定してください")
	}
	id, err := parseID[v1.ApplicationID](c.ID)
	if err != nil {
		return err
	}
	return runNoOutput(context.Background(), c.runtime, func(ctx context.Context, client *v1.Client) error {
		return apprun_dedicated.NewApplicationOp(client).Update(ctx, id, c.ActiveVersion)
	})
}

type ApplicationDeleteCommand struct {
	ID      string `arg:"" help:"削除するアプリケーション ID。"`
	runtime Runtime
}

func (c *ApplicationDeleteCommand) Run(ctx *kong.Context) error {
	id, err := parseID[v1.ApplicationID](c.ID)
	if err != nil {
		return err
	}
	return runNoOutput(context.Background(), c.runtime, func(ctx context.Context, client *v1.Client) error {
		return apprun_dedicated.NewApplicationOp(client).Delete(ctx, id)
	})
}

type ApplicationContainersCommand struct {
	ID      string `arg:"" help:"状態を取得するアプリケーション ID。"`
	runtime Runtime
}

func (c *ApplicationContainersCommand) Run(ctx *kong.Context) error {
	id, err := parseID[v1.ApplicationID](c.ID)
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) ([]application.Placement, error) {
		return apprun_dedicated.NewApplicationOp(client).Containers(ctx, id)
	})
}

type VersionCommands struct {
	List   VersionListCommand   `cmd:"" help:"アプリケーションのバージョンを一覧表示します。"`
	Create VersionCreateCommand `cmd:"" help:"アプリケーションバージョンを作成します。"`
	Read   VersionReadCommand   `cmd:"" help:"バージョンの詳細を取得します。"`
	Delete VersionDeleteCommand `cmd:"" help:"バージョンを削除します。"`
}

func (c *VersionCommands) setRuntime(runtime Runtime) {
	c.List.runtime, c.Create.runtime, c.Read.runtime, c.Delete.runtime = runtime, runtime, runtime, runtime
}

type VersionListCommand struct {
	ApplicationID string `name:"application-id" required:"" help:"対象アプリケーション ID。"`
	MaxItems      int64  `name:"max-items" default:"100" help:"1ページあたりの最大件数。"`
	Cursor        *int32 `help:"前ページの nextCursor。"`
	runtime       Runtime
}

func (c *VersionListCommand) Run(ctx *kong.Context) error {
	applicationID, err := parseID[v1.ApplicationID](c.ApplicationID)
	if err != nil {
		return err
	}
	var cursor *v1.ApplicationVersionNumber
	if c.Cursor != nil {
		value := v1.ApplicationVersionNumber(*c.Cursor)
		cursor = &value
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (page[v1.ApplicationVersionDeploymentStatus], error) {
		items, next, err := apprun_dedicated.NewVersionOp(client, applicationID).List(ctx, c.MaxItems, cursor)
		return page[v1.ApplicationVersionDeploymentStatus]{Items: items, NextCursor: next}, err
	})
}

type VersionCreateCommand struct {
	ApplicationID string  `name:"application-id" required:"" help:"対象アプリケーション ID。"`
	Request       *string `required:"" help:"version.CreateParams JSON。イメージ、CPU、メモリ、スケーリング設定を指定します。秘密情報が含まれる場合は @path.json または - (標準入力) を使います。"`
	runtime       Runtime
}

func (c *VersionCreateCommand) Run(ctx *kong.Context) error {
	applicationID, err := parseID[v1.ApplicationID](c.ApplicationID)
	if err != nil {
		return err
	}
	var request version.CreateParams
	if err := decodeRequest(*c.Request, true, &request); err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*v1.ReadApplicationVersionSummary, error) {
		return apprun_dedicated.NewVersionOp(client, applicationID).Create(ctx, request)
	})
}

type VersionReadCommand struct {
	ApplicationID string `name:"application-id" required:"" help:"対象アプリケーション ID。"`
	Version       string `arg:"" help:"参照するバージョン番号。"`
	runtime       Runtime
}

func (c *VersionReadCommand) Run(ctx *kong.Context) error {
	applicationID, err := parseID[v1.ApplicationID](c.ApplicationID)
	if err != nil {
		return err
	}
	versionNumber, err := parseVersionNumber(c.Version)
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*version.VersionDetail, error) {
		return apprun_dedicated.NewVersionOp(client, applicationID).Read(ctx, versionNumber)
	})
}

type VersionDeleteCommand struct {
	ApplicationID string `name:"application-id" required:"" help:"対象アプリケーション ID。"`
	Version       string `arg:"" help:"削除するバージョン番号。"`
	runtime       Runtime
}

func (c *VersionDeleteCommand) Run(ctx *kong.Context) error {
	applicationID, err := parseID[v1.ApplicationID](c.ApplicationID)
	if err != nil {
		return err
	}
	versionNumber, err := parseVersionNumber(c.Version)
	if err != nil {
		return err
	}
	return runNoOutput(context.Background(), c.runtime, func(ctx context.Context, client *v1.Client) error {
		return apprun_dedicated.NewVersionOp(client, applicationID).Delete(ctx, versionNumber)
	})
}

type AutoScalingGroupCommands struct {
	List   AutoScalingGroupListCommand   `cmd:"" help:"クラスタ内のグループを一覧表示します。"`
	Create AutoScalingGroupCreateCommand `cmd:"" help:"クラスタにオートスケーリンググループを作成します。"`
	Read   AutoScalingGroupReadCommand   `cmd:"" help:"グループの詳細を取得します。"`
	Delete AutoScalingGroupDeleteCommand `cmd:"" help:"グループを削除します。"`
}

func (c *AutoScalingGroupCommands) setRuntime(runtime Runtime) {
	c.List.runtime, c.Create.runtime, c.Read.runtime, c.Delete.runtime = runtime, runtime, runtime, runtime
}

type AutoScalingGroupListCommand struct {
	ClusterID string  `name:"cluster-id" required:"" help:"対象クラスタ ID。"`
	MaxItems  int64   `name:"max-items" default:"100" help:"1ページあたりの最大件数。"`
	Cursor    *string `help:"前ページの nextCursor。"`
	runtime   Runtime
}

func (c *AutoScalingGroupListCommand) Run(ctx *kong.Context) error {
	clusterID, err := parseID[v1.ClusterID](c.ClusterID)
	if err != nil {
		return err
	}
	cursor, err := parseCursor[v1.AutoScalingGroupID](c.Cursor)
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (page[v1.ReadAutoScalingGroupDetail], error) {
		items, next, err := apprun_dedicated.NewAutoScalingGroupOp(client, clusterID).List(ctx, c.MaxItems, cursor)
		return page[v1.ReadAutoScalingGroupDetail]{Items: items, NextCursor: next}, err
	})
}

type AutoScalingGroupCreateCommand struct {
	ClusterID string  `name:"cluster-id" required:"" help:"作成先クラスタ ID。"`
	Request   *string `required:"" help:"autoscalinggroup.CreateParams JSON。zone、workerServiceClassPath、ノード数、ネットワーク interfaces を指定します。複雑な配列を含むため JSON で指定してください。"`
	runtime   Runtime
}

func (c *AutoScalingGroupCreateCommand) Run(ctx *kong.Context) error {
	clusterID, err := parseID[v1.ClusterID](c.ClusterID)
	if err != nil {
		return err
	}
	var request autoscalinggroup.CreateParams
	if err := decodeRequest(*c.Request, false, &request); err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*v1.CreatedAutoScalingGroup, error) {
		return apprun_dedicated.NewAutoScalingGroupOp(client, clusterID).Create(ctx, request)
	})
}

type AutoScalingGroupReadCommand struct {
	ClusterID          string `name:"cluster-id" required:"" help:"対象クラスタ ID。"`
	AutoScalingGroupID string `name:"auto-scaling-group-id" required:"" help:"参照するグループ ID。"`
	runtime            Runtime
}

func (c *AutoScalingGroupReadCommand) Run(ctx *kong.Context) error {
	clusterID, err := parseID[v1.ClusterID](c.ClusterID)
	if err != nil {
		return err
	}
	groupID, err := parseID[v1.AutoScalingGroupID](c.AutoScalingGroupID)
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*autoscalinggroup.AutoScalingGroupDetail, error) {
		return apprun_dedicated.NewAutoScalingGroupOp(client, clusterID).Read(ctx, groupID)
	})
}

type AutoScalingGroupDeleteCommand struct {
	ClusterID          string `name:"cluster-id" required:"" help:"対象クラスタ ID。"`
	AutoScalingGroupID string `name:"auto-scaling-group-id" required:"" help:"削除するグループ ID。"`
	runtime            Runtime
}

func (c *AutoScalingGroupDeleteCommand) Run(ctx *kong.Context) error {
	clusterID, err := parseID[v1.ClusterID](c.ClusterID)
	if err != nil {
		return err
	}
	groupID, err := parseID[v1.AutoScalingGroupID](c.AutoScalingGroupID)
	if err != nil {
		return err
	}
	return runNoOutput(context.Background(), c.runtime, func(ctx context.Context, client *v1.Client) error {
		return apprun_dedicated.NewAutoScalingGroupOp(client, clusterID).Delete(ctx, groupID)
	})
}

type WorkerNodeCommands struct {
	List   WorkerNodeListCommand   `cmd:"" help:"グループ内のワーカノードを一覧表示します。"`
	Read   WorkerNodeReadCommand   `cmd:"" help:"ワーカノードの詳細を取得します。"`
	Update WorkerNodeUpdateCommand `cmd:"" help:"ワーカノードの draining 状態を変更します。"`
}

func (c *WorkerNodeCommands) setRuntime(runtime Runtime) {
	c.List.runtime, c.Read.runtime, c.Update.runtime = runtime, runtime, runtime
}

type WorkerNodeListCommand struct {
	ClusterID          string  `name:"cluster-id" required:"" help:"対象クラスタ ID。"`
	AutoScalingGroupID string  `name:"auto-scaling-group-id" required:"" help:"対象グループ ID。"`
	MaxItems           int64   `name:"max-items" default:"100" help:"1ページあたりの最大件数。"`
	Cursor             *string `help:"前ページの nextCursor。"`
	runtime            Runtime
}

func (c *WorkerNodeListCommand) Run(ctx *kong.Context) error {
	clusterID, err := parseID[v1.ClusterID](c.ClusterID)
	if err != nil {
		return err
	}
	groupID, err := parseID[v1.AutoScalingGroupID](c.AutoScalingGroupID)
	if err != nil {
		return err
	}
	cursor, err := parseCursor[v1.WorkerNodeID](c.Cursor)
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (page[workernode.WorkerNodeDetail], error) {
		items, next, err := apprun_dedicated.NewWorkerNodeOp(client, clusterID, groupID).List(ctx, c.MaxItems, cursor)
		return page[workernode.WorkerNodeDetail]{Items: items, NextCursor: next}, err
	})
}

type WorkerNodeReadCommand struct {
	ClusterID          string `name:"cluster-id" required:"" help:"対象クラスタ ID。"`
	AutoScalingGroupID string `name:"auto-scaling-group-id" required:"" help:"対象グループ ID。"`
	WorkerNodeID       string `name:"worker-node-id" required:"" help:"参照するワーカノード ID。"`
	runtime            Runtime
}

func (c *WorkerNodeReadCommand) Run(ctx *kong.Context) error {
	clusterID, err := parseID[v1.ClusterID](c.ClusterID)
	if err != nil {
		return err
	}
	groupID, err := parseID[v1.AutoScalingGroupID](c.AutoScalingGroupID)
	if err != nil {
		return err
	}
	nodeID, err := parseID[v1.WorkerNodeID](c.WorkerNodeID)
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*workernode.WorkerNodeDetail, error) {
		return apprun_dedicated.NewWorkerNodeOp(client, clusterID, groupID).Read(ctx, nodeID)
	})
}

type WorkerNodeUpdateCommand struct {
	ClusterID          string `name:"cluster-id" required:"" help:"対象クラスタ ID。"`
	AutoScalingGroupID string `name:"auto-scaling-group-id" required:"" help:"対象グループ ID。"`
	WorkerNodeID       string `name:"worker-node-id" required:"" help:"更新するワーカノード ID。"`
	Draining           string `name:"draining" enum:"true,false" required:"" help:"draining 状態。true で draining、false で解除します。"`
	runtime            Runtime
}

func (c *WorkerNodeUpdateCommand) Run(ctx *kong.Context) error {
	clusterID, err := parseID[v1.ClusterID](c.ClusterID)
	if err != nil {
		return err
	}
	groupID, err := parseID[v1.AutoScalingGroupID](c.AutoScalingGroupID)
	if err != nil {
		return err
	}
	nodeID, err := parseID[v1.WorkerNodeID](c.WorkerNodeID)
	if err != nil {
		return err
	}
	draining, err := strconv.ParseBool(c.Draining)
	if err != nil {
		return fmt.Errorf("--draining は true または false で指定してください: %w", err)
	}
	return runNoOutput(context.Background(), c.runtime, func(ctx context.Context, client *v1.Client) error {
		return apprun_dedicated.NewWorkerNodeOp(client, clusterID, groupID).Update(ctx, nodeID, draining)
	})
}

type LoadBalancerCommands struct {
	List   LoadBalancerListCommand   `cmd:"" help:"グループ内のロードバランサを一覧表示します。"`
	Create LoadBalancerCreateCommand `cmd:"" help:"ロードバランサを作成します。"`
	Read   LoadBalancerReadCommand   `cmd:"" help:"ロードバランサの詳細を取得します。"`
	Delete LoadBalancerDeleteCommand `cmd:"" help:"ロードバランサを削除します。"`
	Node   LoadBalancerNodeCommands  `cmd:"" help:"ロードバランサノードの状態を参照します。"`
}

func (c *LoadBalancerCommands) setRuntime(runtime Runtime) {
	c.List.runtime, c.Create.runtime, c.Read.runtime, c.Delete.runtime = runtime, runtime, runtime, runtime
	c.Node.setRuntime(runtime)
}

type LoadBalancerListCommand struct {
	ClusterID          string  `name:"cluster-id" required:"" help:"対象クラスタ ID。"`
	AutoScalingGroupID string  `name:"auto-scaling-group-id" required:"" help:"対象グループ ID。"`
	MaxItems           int64   `name:"max-items" default:"100" help:"1ページあたりの最大件数。"`
	Cursor             *string `help:"前ページの nextCursor。"`
	runtime            Runtime
}

func (c *LoadBalancerListCommand) Run(ctx *kong.Context) error {
	clusterID, err := parseID[v1.ClusterID](c.ClusterID)
	if err != nil {
		return err
	}
	groupID, err := parseID[v1.AutoScalingGroupID](c.AutoScalingGroupID)
	if err != nil {
		return err
	}
	cursor, err := parseCursor[v1.LoadBalancerID](c.Cursor)
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (page[v1.ReadLoadBalancerSummary], error) {
		items, next, err := apprun_dedicated.NewLoadBalancerOp(client, clusterID, groupID).List(ctx, c.MaxItems, cursor)
		return page[v1.ReadLoadBalancerSummary]{Items: items, NextCursor: next}, err
	})
}

type LoadBalancerCreateCommand struct {
	ClusterID          string  `name:"cluster-id" required:"" help:"対象クラスタ ID。"`
	AutoScalingGroupID string  `name:"auto-scaling-group-id" required:"" help:"作成先グループ ID。"`
	Request            *string `required:"" help:"loadbalancer.CreateParams JSON。name、serviceClassPath、interfaces 等を指定します。"`
	runtime            Runtime
}

func (c *LoadBalancerCreateCommand) Run(ctx *kong.Context) error {
	clusterID, err := parseID[v1.ClusterID](c.ClusterID)
	if err != nil {
		return err
	}
	groupID, err := parseID[v1.AutoScalingGroupID](c.AutoScalingGroupID)
	if err != nil {
		return err
	}
	var request loadbalancer.CreateParams
	if err := decodeRequest(*c.Request, false, &request); err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*v1.CreatedLoadBalancer, error) {
		return apprun_dedicated.NewLoadBalancerOp(client, clusterID, groupID).Create(ctx, request)
	})
}

type LoadBalancerReadCommand struct {
	ClusterID          string `name:"cluster-id" required:"" help:"対象クラスタ ID。"`
	AutoScalingGroupID string `name:"auto-scaling-group-id" required:"" help:"対象グループ ID。"`
	LoadBalancerID     string `name:"load-balancer-id" required:"" help:"参照するロードバランサ ID。"`
	runtime            Runtime
}

func (c *LoadBalancerReadCommand) Run(ctx *kong.Context) error {
	clusterID, err := parseID[v1.ClusterID](c.ClusterID)
	if err != nil {
		return err
	}
	groupID, err := parseID[v1.AutoScalingGroupID](c.AutoScalingGroupID)
	if err != nil {
		return err
	}
	lbID, err := parseID[v1.LoadBalancerID](c.LoadBalancerID)
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*loadbalancer.LoadBalancerDetail, error) {
		return apprun_dedicated.NewLoadBalancerOp(client, clusterID, groupID).Read(ctx, lbID)
	})
}

type LoadBalancerDeleteCommand struct {
	ClusterID          string `name:"cluster-id" required:"" help:"対象クラスタ ID。"`
	AutoScalingGroupID string `name:"auto-scaling-group-id" required:"" help:"対象グループ ID。"`
	LoadBalancerID     string `name:"load-balancer-id" required:"" help:"削除するロードバランサ ID。"`
	runtime            Runtime
}

func (c *LoadBalancerDeleteCommand) Run(ctx *kong.Context) error {
	clusterID, err := parseID[v1.ClusterID](c.ClusterID)
	if err != nil {
		return err
	}
	groupID, err := parseID[v1.AutoScalingGroupID](c.AutoScalingGroupID)
	if err != nil {
		return err
	}
	lbID, err := parseID[v1.LoadBalancerID](c.LoadBalancerID)
	if err != nil {
		return err
	}
	return runNoOutput(context.Background(), c.runtime, func(ctx context.Context, client *v1.Client) error {
		return apprun_dedicated.NewLoadBalancerOp(client, clusterID, groupID).Delete(ctx, lbID)
	})
}

type LoadBalancerNodeCommands struct {
	List LoadBalancerNodeListCommand `cmd:"" help:"ロードバランサのノード一覧を取得します。"`
	Read LoadBalancerNodeReadCommand `cmd:"" help:"ロードバランサノードの詳細を取得します。"`
}

func (c *LoadBalancerNodeCommands) setRuntime(runtime Runtime) {
	c.List.runtime, c.Read.runtime = runtime, runtime
}

type LoadBalancerNodeListCommand struct {
	ClusterID          string `name:"cluster-id" required:"" help:"対象クラスタ ID。"`
	AutoScalingGroupID string `name:"auto-scaling-group-id" required:"" help:"対象グループ ID。"`
	LoadBalancerID     string `name:"load-balancer-id" required:"" help:"対象ロードバランサ ID。"`
	MaxItems           int64  `name:"max-items" default:"100" help:"1ページあたりの最大件数。"`
	runtime            Runtime
}

func (c *LoadBalancerNodeListCommand) Run(ctx *kong.Context) error {
	clusterID, err := parseID[v1.ClusterID](c.ClusterID)
	if err != nil {
		return err
	}
	groupID, err := parseID[v1.AutoScalingGroupID](c.AutoScalingGroupID)
	if err != nil {
		return err
	}
	lbID, err := parseID[v1.LoadBalancerID](c.LoadBalancerID)
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (page[v1.ReadLoadBalancerNodeSummary], error) {
		items, err := apprun_dedicated.NewLoadBalancerOp(client, clusterID, groupID).ListNodes(ctx, lbID, c.MaxItems, nil)
		return page[v1.ReadLoadBalancerNodeSummary]{Items: items}, err
	})
}

type LoadBalancerNodeReadCommand struct {
	ClusterID          string `name:"cluster-id" required:"" help:"対象クラスタ ID。"`
	AutoScalingGroupID string `name:"auto-scaling-group-id" required:"" help:"対象グループ ID。"`
	LoadBalancerID     string `name:"load-balancer-id" required:"" help:"対象ロードバランサ ID。"`
	NodeID             string `name:"node-id" required:"" help:"参照するロードバランサノード ID。"`
	runtime            Runtime
}

func (c *LoadBalancerNodeReadCommand) Run(ctx *kong.Context) error {
	clusterID, err := parseID[v1.ClusterID](c.ClusterID)
	if err != nil {
		return err
	}
	groupID, err := parseID[v1.AutoScalingGroupID](c.AutoScalingGroupID)
	if err != nil {
		return err
	}
	lbID, err := parseID[v1.LoadBalancerID](c.LoadBalancerID)
	if err != nil {
		return err
	}
	nodeID, err := parseID[v1.LoadBalancerNodeID](c.NodeID)
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*loadbalancer.LoadBalancerNodeDetail, error) {
		return apprun_dedicated.NewLoadBalancerOp(client, clusterID, groupID).ReadNode(ctx, lbID, nodeID)
	})
}

type CertificateCommands struct {
	List   CertificateListCommand   `cmd:"" help:"クラスタの証明書一覧を取得します。"`
	Create CertificateCreateCommand `cmd:"" help:"証明書を作成します。PEM を含む JSON は保護したファイルまたは標準入力から渡してください。"`
	Read   CertificateReadCommand   `cmd:"" help:"証明書の詳細を取得します。"`
	Update CertificateUpdateCommand `cmd:"" help:"証明書を更新します。PEM を含む JSON は保護したファイルまたは標準入力から渡してください。"`
	Delete CertificateDeleteCommand `cmd:"" help:"証明書を削除します。"`
}

func (c *CertificateCommands) setRuntime(runtime Runtime) {
	c.List.runtime, c.Create.runtime, c.Read.runtime = runtime, runtime, runtime
	c.Update.runtime, c.Delete.runtime = runtime, runtime
}

type CertificateListCommand struct {
	ClusterID string  `name:"cluster-id" required:"" help:"対象クラスタ ID。"`
	MaxItems  int64   `name:"max-items" default:"100" help:"1ページあたりの最大件数。"`
	Cursor    *string `help:"前ページの nextCursor。"`
	runtime   Runtime
}

func (c *CertificateListCommand) Run(ctx *kong.Context) error {
	clusterID, err := parseID[v1.ClusterID](c.ClusterID)
	if err != nil {
		return err
	}
	cursor, err := parseCursor[v1.CertificateID](c.Cursor)
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (page[v1.ReadCertificate], error) {
		items, next, err := apprun_dedicated.NewCertificateOp(client, clusterID).List(ctx, c.MaxItems, cursor)
		return page[v1.ReadCertificate]{Items: items, NextCursor: next}, err
	})
}

type CertificateCreateCommand struct {
	ClusterID string  `name:"cluster-id" required:"" help:"対象クラスタ ID。"`
	Request   *string `required:"" help:"certificate.CreateParams JSON。name、certificatePem、privatekeyPem を指定します。リクエストは @path.json または - (標準入力) で渡してください。"`
	runtime   Runtime
}

func (c *CertificateCreateCommand) Run(ctx *kong.Context) error {
	clusterID, err := parseID[v1.ClusterID](c.ClusterID)
	if err != nil {
		return err
	}
	var request certificate.CreateParams
	if err := decodeRequest(*c.Request, true, &request); err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*v1.CreatedCertificate, error) {
		return apprun_dedicated.NewCertificateOp(client, clusterID).Create(ctx, request)
	})
}

type CertificateReadCommand struct {
	ClusterID     string `name:"cluster-id" required:"" help:"対象クラスタ ID。"`
	CertificateID string `name:"certificate-id" required:"" help:"参照する証明書 ID。"`
	runtime       Runtime
}

func (c *CertificateReadCommand) Run(ctx *kong.Context) error {
	clusterID, err := parseID[v1.ClusterID](c.ClusterID)
	if err != nil {
		return err
	}
	certificateID, err := parseID[v1.CertificateID](c.CertificateID)
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*v1.ReadCertificate, error) {
		return apprun_dedicated.NewCertificateOp(client, clusterID).Read(ctx, certificateID)
	})
}

type CertificateUpdateCommand struct {
	ClusterID     string  `name:"cluster-id" required:"" help:"対象クラスタ ID。"`
	CertificateID string  `name:"certificate-id" required:"" help:"更新する証明書 ID。"`
	Request       *string `required:"" help:"certificate.UpdateParams JSON。リクエストは @path.json または - (標準入力) で渡してください。"`
	runtime       Runtime
}

func (c *CertificateUpdateCommand) Run(ctx *kong.Context) error {
	clusterID, err := parseID[v1.ClusterID](c.ClusterID)
	if err != nil {
		return err
	}
	certificateID, err := parseID[v1.CertificateID](c.CertificateID)
	if err != nil {
		return err
	}
	var request certificate.UpdateParams
	if err := decodeRequest(*c.Request, true, &request); err != nil {
		return err
	}
	return runNoOutput(context.Background(), c.runtime, func(ctx context.Context, client *v1.Client) error {
		return apprun_dedicated.NewCertificateOp(client, clusterID).Update(ctx, certificateID, request)
	})
}

type CertificateDeleteCommand struct {
	ClusterID     string `name:"cluster-id" required:"" help:"対象クラスタ ID。"`
	CertificateID string `name:"certificate-id" required:"" help:"削除する証明書 ID。"`
	runtime       Runtime
}

func (c *CertificateDeleteCommand) Run(ctx *kong.Context) error {
	clusterID, err := parseID[v1.ClusterID](c.ClusterID)
	if err != nil {
		return err
	}
	certificateID, err := parseID[v1.CertificateID](c.CertificateID)
	if err != nil {
		return err
	}
	return runNoOutput(context.Background(), c.runtime, func(ctx context.Context, client *v1.Client) error {
		return apprun_dedicated.NewCertificateOp(client, clusterID).Delete(ctx, certificateID)
	})
}

type ServiceClassCommands struct {
	ListLB     ServiceClassListLBCommand     `cmd:"" name:"list-lb" help:"ロードバランサのサービスクラス一覧を取得します。"`
	ListWorker ServiceClassListWorkerCommand `cmd:"" name:"list-worker" help:"ワーカノードのサービスクラス一覧を取得します。"`
}

func (c *ServiceClassCommands) setRuntime(runtime Runtime) {
	c.ListLB.runtime, c.ListWorker.runtime = runtime, runtime
}

type ServiceClassListLBCommand struct{ runtime Runtime }

func (c *ServiceClassListLBCommand) Run(ctx *kong.Context) error {
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) ([]v1.ReadLbServiceClass, error) {
		return apprun_dedicated.NewServiceClassOp(client).ListLB(ctx)
	})
}

type ServiceClassListWorkerCommand struct{ runtime Runtime }

func (c *ServiceClassListWorkerCommand) Run(ctx *kong.Context) error {
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) ([]v1.ReadWorkerServiceClass, error) {
		return apprun_dedicated.NewServiceClassOp(client).ListWorker(ctx)
	})
}
