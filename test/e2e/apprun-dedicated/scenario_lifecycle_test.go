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

// このファイルは apprun-dedicated 高レベルコマンドのライフサイクル
// (deploy、status、versions、containers、init、rollback、deactivate、delete) の
// ライブ E2E シナリオです。クラスタとオートスケーリンググループの作成を伴うため、
// 実行にはワーカノード (最小構成 1vCPU/2GB x 1 台) の時間割料金が発生します。
// 承認は [#59](https://github.com/sacloud/skr/issues/59) で管理します。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	apprun_dedicated "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated"
	application "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/application"
	autoscalinggroup "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/autoscalinggroup"
	cluster "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/cluster"
	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	workernode "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/workernode"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"
	skrsaclient "github.com/sacloud/skr/internal/saclient"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	// envServicePrincipalID はクラスタ作成に使うサービスプリンシパル ID の環境変数です。
	// サービスプリンシパルはプロジェクト固有のリソースのため、テストコードには
	// 埋め込まず、実行時に環境変数で渡します。クイックスタートのとおり、対象の
	// サービスプリンシパルには「さくらのクラウド > 作成・削除」のロールが必要です。
	// https://manual.sakura.ad.jp/cloud/apprun-dedicated/getting_started.html
	envServicePrincipalID = "SKR_E2E_APPRUN_DEDICATED_SERVICE_PRINCIPAL_ID"
	// apprunDedicatedZone はオートスケーリンググループを作成するゾーンです。
	apprunDedicatedZone = "tk1a"
	// workerServiceClassPath はテストで使う最小構成のワーカノードのサービスクラスです。
	workerServiceClassPath = "cloud/apprun/dedicated/worker/1vcpu_2gb"
	// tutorialImage は公開サンプルのコンテナイメージです。公開イメージのため
	// registryUsername と registryPassword は指定しません。
	tutorialImage = "ghcr.io/sacloud/apprun-samples/tutorial:latest"
	// appTargetPort はサンプルアプリケーションが待ち受けるポートです。
	appTargetPort = 8080
	// listPageSize は一覧取得のページサイズです。実 API は大きなページサイズを
	// 400 で拒否するため、service 層と同じ値を使います。
	listPageSize = 20
	// workerNodeWaitLimit はワーカノードが healthy になるまでの待機期限です。
	workerNodeWaitLimit = 20 * time.Minute
	// statePolling はワーカノードの状態を再確認する間隔です。
	statePolling = 15 * time.Second
	// deployWaitTimeout は deploy と rollback の反映待ちに渡すタイムアウトです。
	deployWaitTimeout = 10 * time.Minute
	// deployCommandLimit は deploy と rollback のコマンド実行の期限です。
	// 反映待ちのタイムアウトに起動と応答の待ち時間を加えた値です。
	deployCommandLimit = 15 * time.Minute
	// createCommandLimit はクラスタとオートスケーリンググループの作成コマンドの
	// 実行期限です。
	createCommandLimit = 15 * time.Minute
	// deleteAbsentLimit は削除後の不在確認の待機期限です。削除は非同期に
	// 完了するため、不在になるまで再確認します。
	deleteAbsentLimit = 10 * time.Minute
)

// deployResult は deploy と rollback の JSON 出力を解析するための項目です。
type deployResult struct {
	Cluster            string                      `json:"cluster"`
	Application        string                      `json:"application"`
	ApplicationID      v1.ApplicationID            `json:"applicationID"`
	Version            v1.ApplicationVersionNumber `json:"version"`
	ApplicationCreated bool                        `json:"applicationCreated"`
}

// statusOutput は status の JSON 出力を解析するための項目です。
type statusOutput struct {
	Cluster             string `json:"cluster"`
	ApplicationStatus   `json:"application"`
	ActiveVersionDetail *activeVersionSummary `json:"activeVersionDetail"`
}

// ApplicationStatus は status の出力のうちアプリケーションの状態です。
type ApplicationStatus struct {
	ID            v1.ApplicationID `json:"applicationID"`
	Name          string           `json:"name"`
	ActiveVersion *int32           `json:"activeVersion"`
	DesiredCount  *int32           `json:"desiredCount"`
}

// activeVersionSummary は status の出力のうちアクティブバージョンの内容です。
type activeVersionSummary struct {
	Version         v1.ApplicationVersionNumber `json:"version"`
	Image           string                      `json:"image"`
	CPU             int64                       `json:"cpu"`
	Memory          int64                       `json:"memory"`
	ScalingMode     string                      `json:"scalingMode"`
	ActiveNodeCount int64                       `json:"activeNodeCount"`
}

// versionEntry は versions の JSON 出力の要素を解析するための項目です。
type versionEntry struct {
	Version v1.ApplicationVersionNumber `json:"version"`
	Image   string                      `json:"image"`
	Active  bool                        `json:"active"`
	Created time.Time                   `json:"created"`
}

// placementOutput は containers の JSON 出力の要素を解析するための項目です。
type placementOutput struct {
	NodeID          v1.WorkerNodeID `json:"nodeID"`
	ContainersStats struct {
		Containers []struct {
			ID                 string                      `json:"id"`
			Image              string                      `json:"image"`
			State              string                      `json:"state"`
			ApplicationVersion v1.ApplicationVersionNumber `json:"applicationVersion"`
		} `json:"containers"`
	} `json:"containersStats"`
	Desired struct {
		Containers []struct {
			ApplicationVersion v1.ApplicationVersionNumber `json:"applicationVersion"`
			CpuMillis          int64                       `json:"cpuMillis"`
			MemoryMB           int64                       `json:"memoryMB"`
			Image              string                      `json:"image"`
		} `json:"containers"`
	} `json:"desired"`
}

// TestScenarioAppRunDedicatedLifecycle は apprun-dedicated 高レベルコマンドの
// ライブ E2E です。クラスタとオートスケーリンググループを作成し、deploy で
// アプリケーションを作成してから、status、versions、containers、init での参照、
// 2 回目の deploy と rollback、deactivate、delete までの一連の流れを確認します。
// テスト対象の操作は skr CLI で実行し、事前確認・反映待ち・削除後の不在確認は
// SDK で静かに実行します (ADR 0064)。
func TestScenarioAppRunDedicatedLifecycle(t *testing.T) {
	h := helper.New(t, "apprun-dedicated",
		"AppRun 専有型の高レベルコマンドのライフサイクル (deploy、status、versions、containers、init、rollback、deactivate、delete) を確認する。クラスタとワーカノード 1 台のオートスケーリンググループを作成するため、ワーカノードの時間割料金が発生する")

	// サービスプリンシパルはプロジェクト固有のため、環境変数で渡します。
	// 未設定のときは依存不足としてスキップします。
	servicePrincipalID := os.Getenv(envServicePrincipalID)
	if servicePrincipalID == "" {
		t.Skipf("%s not set, skipping E2E test", envServicePrincipalID)
	}

	// 事前確認と後片付けの確認は SDK で静かに実行します。
	// skr CLI と同じ認証設定 (プロファイル) を使います。
	sdkClient := newAppRunDedicatedClient(t)
	clusters := apprun_dedicated.NewClusterOp(sdkClient)
	applications := apprun_dedicated.NewApplicationOp(sdkClient)

	suffix := helper.RandomSuffix(t)
	// 名前の上限は 20 文字のため、「skr-」+ 16 文字のサフィックスで構成します。
	clusterName := "skr-" + suffix
	asgName := "skr-" + suffix
	appName := "skr-" + suffix

	// 作成前に同名リソースが存在しないことを確認する。
	// 衝突が見つかった場合は自動削除せず、手動確認を求めて失敗します。
	requireNoClusterNamed(t, clusters, clusterName)
	requireNoApplicationNamed(t, applications, appName)

	// クラスタを作成する。ロードバランサの待受ポートと Let's Encrypt は使わない。
	clusterRequest := h.WriteRequestJSONFile(t, "create-cluster", map[string]any{
		"name":               clusterName,
		"ports":              []any{},
		"servicePrincipalID": servicePrincipalID,
	})
	data := h.CallWithTimeout(t, createCommandLimit, "クラスタを作成する",
		"apprun-dedicated-api", "create-cluster", "--request", "@"+clusterRequest)
	clusterID := parseCreateClusterOutput(t, data)
	require.NotEqual(t, uuid.Nil, uuid.UUID(clusterID), "created cluster should have an ID")

	// 作成直後に SDK で読み戻し、ID と名前が一致することを確認する。
	verifyCreatedCluster(t, clusters, clusterID, clusterName)

	// 後片付け: クラスタを削除する。defer は LIFO のため、最後に実行されます。
	defer func() {
		cleanupCluster(t, h, clusters, clusterID, clusterName)
	}()

	// オートスケーリンググループを作成する。最小構成のワーカノードを 1 台に固定し、
	// 共有セグメントに接続します (パケットフィルタなしはクイックスタートの既定です)。
	asgRequest := h.WriteRequestJSONFile(t, "create-auto-scaling-group", map[string]any{
		"name":                   asgName,
		"zone":                   apprunDedicatedZone,
		"workerServiceClassPath": workerServiceClassPath,
		"minNodes":               1,
		"maxNodes":               1,
		"interfaces": []any{map[string]any{
			"interfaceIndex": 0,
			"upstream":       "shared",
			"connectsToLB":   false,
		}},
	})
	data = h.CallWithTimeout(t, createCommandLimit, "オートスケーリンググループを作成する (ワーカノード 1vCPU/2GB x 1 台、共有セグメント)",
		"apprun-dedicated-api", "create-auto-scaling-group", "--cluster-id", uuid.UUID(clusterID).String(), "--request", "@"+asgRequest)
	asgID := parseCreateAutoScalingGroupOutput(t, data)
	require.NotEqual(t, uuid.Nil, uuid.UUID(asgID), "created auto-scaling group should have an ID")

	// 作成直後に SDK で読み戻し、ID と指定内容が一致することを確認する。
	asgs := apprun_dedicated.NewAutoScalingGroupOp(sdkClient, clusterID)
	verifyCreatedAutoScalingGroup(t, asgs, asgID, asgName)

	// 後片付け: オートスケーリンググループを削除する。
	defer func() {
		cleanupAutoScalingGroup(t, h, asgs, clusterID, asgID, asgName)
	}()

	// ワーカノードが healthy になるのを待つ。コンテナの実行にはノードが必要です。
	waitForHealthyWorkerNode(t, apprun_dedicated.NewWorkerNodeOp(sdkClient, clusterID, asgID))

	// デプロイする定義を書き出す。公開イメージのためレジストリ認証は指定しません。
	definitionV1 := map[string]any{
		"cluster":     clusterName,
		"name":        appName,
		"cpu":         100,
		"memory":      128,
		"scalingMode": "manual",
		"fixedScale":  1,
		"image":       tutorialImage,
		"exposedPorts": []any{
			map[string]any{"targetPort": appTargetPort},
		},
	}
	pathV1 := h.WriteRequestJSONFile(t, "deploy-v1-definition", definitionV1)

	// 後片付け: deploy はアプリ作成後の反映待ちで失敗することがあるため、
	// deploy の成否にかかわらず、再検索で作成済みを確認できたアプリケーションを
	// 削除します。deploy より前に登録するのは、失敗時も後片付けを実行するためです。
	defer func() {
		cleanupApplicationAfterDeploy(t, h, applications, clusterName, appName)
	}()

	// deploy でアプリケーションを作成し、バージョン 1 を有効化して反映を待つ。
	data = h.CallWithTimeout(t, deployCommandLimit, "deploy でアプリケーションを作成し、バージョン 1 を有効化する",
		"apprun-dedicated", "deploy", "--app", pathV1, "--wait-timeout", deployWaitTimeout.String())
	deployedV1 := parseDeployResult(t, data)
	require.Equal(t, clusterName, deployedV1.Cluster, "deploy result should contain the cluster name")
	require.Equal(t, appName, deployedV1.Application, "deploy result should contain the application name")
	require.NotEqual(t, uuid.Nil, uuid.UUID(deployedV1.ApplicationID), "deploy result should contain the application ID")
	require.EqualValues(t, 1, deployedV1.Version, "first deploy should activate version 1")
	require.True(t, deployedV1.ApplicationCreated, "first deploy should create the application")

	// status でアクティブバージョンとコンテナの状態を確認する。
	data = h.Call(t, "status でアクティブバージョンとコンテナの状態を確認する",
		"apprun-dedicated", "status", "--app", pathV1)
	statusV1 := parseStatusOutput(t, data)
	require.Equal(t, clusterName, statusV1.Cluster)
	require.Equal(t, appName, statusV1.ApplicationStatus.Name)
	require.NotNil(t, statusV1.ApplicationStatus.ActiveVersion, "active version should be set after deploy")
	require.EqualValues(t, 1, *statusV1.ApplicationStatus.ActiveVersion)
	require.NotNil(t, statusV1.ApplicationStatus.DesiredCount, "desired count should be set after deploy")
	require.EqualValues(t, 1, *statusV1.ApplicationStatus.DesiredCount)
	require.NotNil(t, statusV1.ActiveVersionDetail, "active version detail should be set after deploy")
	require.EqualValues(t, 1, statusV1.ActiveVersionDetail.Version)
	require.Equal(t, tutorialImage, statusV1.ActiveVersionDetail.Image)
	require.EqualValues(t, 100, statusV1.ActiveVersionDetail.CPU)
	require.EqualValues(t, 128, statusV1.ActiveVersionDetail.Memory)
	require.Equal(t, "manual", statusV1.ActiveVersionDetail.ScalingMode)
	require.GreaterOrEqual(t, statusV1.ActiveVersionDetail.ActiveNodeCount, int64(1),
		"アクティブバージョンの稼働ノード数は 1 以上であること")

	// versions でバージョンの一覧を確認する。
	data = h.Call(t, "versions でバージョンの一覧を確認する",
		"apprun-dedicated", "versions", "--app", pathV1)
	versions := parseVersionEntries(t, data)
	require.Len(t, versions, 1, "versions should contain exactly one entry after the first deploy")
	require.EqualValues(t, 1, versions[0].Version)
	require.Equal(t, tutorialImage, versions[0].Image)
	require.True(t, versions[0].Active, "version 1 should be active after the first deploy")

	// containers でコンテナの配置を確認する。
	data = h.Call(t, "containers でコンテナの配置を確認する",
		"apprun-dedicated", "containers", "--app", pathV1)
	placements := parsePlacements(t, data)
	require.NotEmpty(t, placements, "containers should contain at least one placement")
	requireContainerPlacements(t, placements, 1, tutorialImage)

	// init でアクティブバージョンから定義を復元する。
	initFile := filepath.Join(t.TempDir(), "init-definition.json")
	h.Call(t, "init でアクティブバージョンから定義を復元する",
		"apprun-dedicated", "init", clusterName, appName, "--output-file", initFile)
	requireInitDefinition(t, initFile, clusterName, appName)

	// 2 回目の deploy では、環境変数を追加した定義でバージョン 2 を作成して有効化する。
	definitionV2 := map[string]any{
		"cluster":     clusterName,
		"name":        appName,
		"cpu":         100,
		"memory":      128,
		"scalingMode": "manual",
		"fixedScale":  1,
		"image":       tutorialImage,
		"exposedPorts": []any{
			map[string]any{"targetPort": appTargetPort},
		},
		"env": []any{
			map[string]any{"key": "GREETING", "value": "hello-skr-e2e-" + suffix, "secret": false},
		},
	}
	pathV2 := h.WriteRequestJSONFile(t, "deploy-v2-definition", definitionV2)
	data = h.CallWithTimeout(t, deployCommandLimit, "deploy で環境変数を追加したバージョン 2 を作成して有効化する",
		"apprun-dedicated", "deploy", "--app", pathV2, "--wait-timeout", deployWaitTimeout.String())
	deployedV2 := parseDeployResult(t, data)
	require.Equal(t, deployedV1.ApplicationID, deployedV2.ApplicationID,
		"second deploy should reuse the existing application")
	require.EqualValues(t, 2, deployedV2.Version, "second deploy should activate version 2")
	require.False(t, deployedV2.ApplicationCreated, "second deploy should not create the application")

	// status でアクティブバージョンが 2 になったことを確認する。
	data = h.Call(t, "status でアクティブバージョンが 2 になったことを確認する",
		"apprun-dedicated", "status", "--app", pathV2)
	statusV2 := parseStatusOutput(t, data)
	require.NotNil(t, statusV2.ApplicationStatus.ActiveVersion)
	require.EqualValues(t, 2, *statusV2.ApplicationStatus.ActiveVersion)
	require.NotNil(t, statusV2.ActiveVersionDetail)
	require.EqualValues(t, 2, statusV2.ActiveVersionDetail.Version)

	// versions で 2 つのバージョンと活性状態を確認する。
	data = h.Call(t, "versions で 2 つのバージョンを確認する",
		"apprun-dedicated", "versions", "--app", pathV2)
	versions = parseVersionEntries(t, data)
	require.Len(t, versions, 2, "versions should contain two entries after the second deploy")
	byVersion := map[int32]versionEntry{}
	for _, entry := range versions {
		byVersion[int32(entry.Version)] = entry
	}
	require.True(t, byVersion[2].Active, "version 2 should be active after the second deploy")
	require.False(t, byVersion[1].Active, "version 1 should be inactive after the second deploy")

	// rollback でバージョン 1 に切り替える。既定の対象はアクティブバージョンより
	// 前のバージョンのうち最大のもの (バージョン 1) です。
	data = h.CallWithTimeout(t, deployCommandLimit, "rollback でバージョン 1 に切り替える",
		"apprun-dedicated", "rollback", "--app", pathV2, "--force", "--wait-timeout", deployWaitTimeout.String())
	rolledBack := parseDeployResult(t, data)
	require.Equal(t, deployedV1.ApplicationID, rolledBack.ApplicationID)
	require.EqualValues(t, 1, rolledBack.Version, "rollback should activate version 1")
	require.False(t, rolledBack.ApplicationCreated, "rollback should not create the application")

	// status でアクティブバージョンが 1 に戻ったことを確認する。
	data = h.Call(t, "status でアクティブバージョンが 1 に戻ったことを確認する",
		"apprun-dedicated", "status", "--app", pathV2)
	statusRolledBack := parseStatusOutput(t, data)
	require.NotNil(t, statusRolledBack.ApplicationStatus.ActiveVersion)
	require.EqualValues(t, 1, *statusRolledBack.ApplicationStatus.ActiveVersion)
	require.NotNil(t, statusRolledBack.ActiveVersionDetail)
	require.EqualValues(t, 1, statusRolledBack.ActiveVersionDetail.Version)
	require.Equal(t, tutorialImage, statusRolledBack.ActiveVersionDetail.Image)

	// versions でバージョン 1 が活性に戻ったことを確認する。
	data = h.Call(t, "versions でバージョン 1 が活性に戻ったことを確認する",
		"apprun-dedicated", "versions", "--app", pathV2)
	versions = parseVersionEntries(t, data)
	byVersion = map[int32]versionEntry{}
	for _, entry := range versions {
		byVersion[int32(entry.Version)] = entry
	}
	require.True(t, byVersion[1].Active, "version 1 should be active after rollback")
	require.False(t, byVersion[2].Active, "version 2 should be inactive after rollback")

	// deactivate でアクティブバージョンを解除して停止する。
	h.Call(t, "deactivate でアクティブバージョンを解除して停止する",
		"apprun-dedicated", "deactivate", "--app", pathV2, "--force")

	// status で非活性になったことを確認する。
	data = h.Call(t, "status で非活性になったことを確認する",
		"apprun-dedicated", "status", "--app", pathV2)
	statusDeactivated := parseStatusOutput(t, data)
	require.Nil(t, statusDeactivated.ApplicationStatus.ActiveVersion,
		"active version should be unset after deactivate")
	require.Nil(t, statusDeactivated.ActiveVersionDetail,
		"active version detail should be unset after deactivate")
	if statusDeactivated.ApplicationStatus.DesiredCount != nil {
		require.EqualValues(t, 0, *statusDeactivated.ApplicationStatus.DesiredCount,
			"desired count should be 0 after deactivate")
	}

	// delete でアプリケーションを削除する。
	h.Call(t, "delete でアプリケーションを削除する",
		"apprun-dedicated", "delete", "--app", pathV2, "--force")

	// 削除後の不在確認をする (SDK)。
	requireApplicationAbsent(t, applications, deployedV1.ApplicationID)
}

// newAppRunDedicatedClient は安全確認用の SDK クライアントを作成します。
func newAppRunDedicatedClient(t *testing.T) *v1.Client {
	t.Helper()

	sc, err := skrsaclient.New(false)
	require.NoError(t, err, "create API client for safety checks")
	client, err := apprun_dedicated.NewClient(sc)
	require.NoError(t, err, "create AppRun dedicated API client for safety checks")
	return client
}

// requireNoClusterNamed は作成前に、同じ名前のクラスタが存在しないことを SDK で確認します。
// 確認の実行は証跡に残らず、既存リソースを変更しません。
func requireNoClusterNamed(t *testing.T, clusters *cluster.ClusterOp, name string) {
	t.Helper()

	var cursor *v1.ClusterID
	for {
		list, next, err := clusters.List(t.Context(), listPageSize, cursor)
		require.NoError(t, err, "list clusters before create")
		for _, c := range list {
			require.NotEqual(t, name, c.Name, "cluster %q already exists; refusing to modify it", name)
		}
		if next == nil {
			return
		}
		cursor = next
	}
}

// requireNoApplicationNamed は作成前に、同じ名前のアプリケーションが存在しないことを
// SDK で確認します。確認の実行は証跡に残らず、既存リソースを変更しません。
func requireNoApplicationNamed(t *testing.T, applications *application.ApplicationOp, name string) {
	t.Helper()

	var cursor *v1.ApplicationID
	for {
		list, next, err := applications.List(t.Context(), listPageSize, cursor)
		require.NoError(t, err, "list applications before create")
		for _, a := range list {
			require.NotEqual(t, name, a.Name, "application %q already exists; refusing to modify it", name)
		}
		if next == nil {
			return
		}
		cursor = next
	}
}

// verifyCreatedCluster は作成直後にクラスタを読み戻し、保持した ID と名前が
// 一致することを確認します。確認の実行は証跡に残らず、既存リソースを変更しません。
func verifyCreatedCluster(t *testing.T, clusters *cluster.ClusterOp, clusterID v1.ClusterID, clusterName string) {
	t.Helper()

	detail, err := clusters.Read(t.Context(), clusterID)
	require.NoError(t, err, "read the created cluster")
	require.Equal(t, clusterID, detail.ClusterID, "created cluster should keep the returned ID")
	require.Equal(t, clusterName, detail.Name, "created cluster should have the requested name")
	require.False(t, detail.HasLetsEncryptEmail,
		"created cluster should not have Let's Encrypt email (not requested)")
}

// verifyCreatedAutoScalingGroup は作成直後にオートスケーリンググループを読み戻し、
// 保持した ID と指定内容が一致することを確認します。確認の実行は証跡に残らず、
// 既存リソースを変更しません。
func verifyCreatedAutoScalingGroup(t *testing.T, asgs *autoscalinggroup.AutoScalingGroupOp, asgID v1.AutoScalingGroupID, asgName string) {
	t.Helper()

	detail, err := asgs.Read(t.Context(), asgID)
	require.NoError(t, err, "read the created auto-scaling group")
	require.Equal(t, asgID, detail.AutoScalingGroupID, "created group should keep the returned ID")
	require.Equal(t, asgName, detail.Name, "created group should have the requested name")
	require.Equal(t, apprunDedicatedZone, detail.Zone, "created group should be in the requested zone")
	require.Equal(t, workerServiceClassPath, detail.WorkerServiceClassPath,
		"created group should use the requested worker service class")
	require.EqualValues(t, 1, detail.MinNodes, "created group should have the requested min nodes")
	require.EqualValues(t, 1, detail.MaxNodes, "created group should have the requested max nodes")
	require.False(t, detail.Deleting, "created group should not be deleting")
	require.Len(t, detail.Interfaces, 1, "created group should have the requested interfaces")
	iface := detail.Interfaces[0]
	require.EqualValues(t, 0, iface.InterfaceIndex, "interface should be eth0")
	require.Equal(t, "shared", iface.Upstream, "interface should connect to the shared segment")
	require.False(t, iface.ConnectsToLB, "interface should not connect to the load balancer")
}

// waitForHealthyWorkerNode はオートスケーリンググループのワーカノードが
// healthy になるまで待ちます。ノードの作成に失敗したメッセージが返ってきたら
// そこで失敗します。
func waitForHealthyWorkerNode(t *testing.T, nodes *workernode.WorkerNodeOp) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), workerNodeWaitLimit)
	defer cancel()

	ticker := time.NewTicker(statePolling)
	defer ticker.Stop()

	for {
		list, _, err := nodes.List(ctx, listPageSize, nil)
		require.NoError(t, err, "list worker nodes while waiting for a healthy node")
		for _, n := range list {
			if n.Status == v1.WorkerNodeStatusHealthy {
				return
			}
			if n.CreateErrorMessage != nil && *n.CreateErrorMessage != "" {
				t.Fatalf("worker node creation failed: %s", *n.CreateErrorMessage)
			}
		}

		select {
		case <-ctx.Done():
			t.Fatalf("worker node did not become healthy within %s", workerNodeWaitLimit)
		case <-ticker.C:
		}
	}
}

// parseCreateClusterOutput は create-cluster の応答からクラスタ ID を取り出します。
// 低レベルコマンドは識別子をラップせずに出力します。
func parseCreateClusterOutput(t *testing.T, data []byte) v1.ClusterID {
	t.Helper()

	var out struct {
		ClusterID v1.ClusterID `json:"clusterID"`
	}
	require.NoError(t, json.Unmarshal(data, &out), "decode create-cluster output: %s", data)
	return out.ClusterID
}

// parseCreateAutoScalingGroupOutput は create-auto-scaling-group の応答から
// グループ ID を取り出します。低レベルコマンドは識別子をラップせずに出力します。
func parseCreateAutoScalingGroupOutput(t *testing.T, data []byte) v1.AutoScalingGroupID {
	t.Helper()

	var out struct {
		AutoScalingGroupID v1.AutoScalingGroupID `json:"autoScalingGroupID"`
	}
	require.NoError(t, json.Unmarshal(data, &out), "decode create-auto-scaling-group output: %s", data)
	return out.AutoScalingGroupID
}

// parseDeployResult は deploy と rollback の応答を解析します。
func parseDeployResult(t *testing.T, data []byte) deployResult {
	t.Helper()

	var out deployResult
	require.NoError(t, json.Unmarshal(data, &out), "decode deploy output: %s", data)
	return out
}

// parseStatusOutput は status の応答を解析します。
func parseStatusOutput(t *testing.T, data []byte) statusOutput {
	t.Helper()

	var out statusOutput
	require.NoError(t, json.Unmarshal(data, &out), "decode status output: %s", data)
	return out
}

// parseVersionEntries は versions の応答を解析します。
func parseVersionEntries(t *testing.T, data []byte) []versionEntry {
	t.Helper()

	var out []versionEntry
	require.NoError(t, json.Unmarshal(data, &out), "decode versions output: %s", data)
	return out
}

// parsePlacements は containers の応答を解析します。
func parsePlacements(t *testing.T, data []byte) []placementOutput {
	t.Helper()

	var out []placementOutput
	require.NoError(t, json.Unmarshal(data, &out), "decode containers output: %s", data)
	return out
}

// requireContainerPlacements は配置ごとに、実行中のコンテナが期待するバージョンと
// イメージで running になっていることを確認します。
func requireContainerPlacements(t *testing.T, placements []placementOutput, version int32, image string) {
	t.Helper()

	for i, placement := range placements {
		require.NotEqual(t, uuid.Nil, uuid.UUID(placement.NodeID),
			"placement[%d] should have a worker node ID", i)
		require.NotEmpty(t, placement.ContainersStats.Containers,
			"placement[%d] should have running container stats", i)
		for j, container := range placement.ContainersStats.Containers {
			require.Equal(t, "running", container.State,
				"placement[%d].containers[%d] should be running", i, j)
			require.EqualValues(t, version, container.ApplicationVersion,
				"placement[%d].containers[%d] should belong to version %d", i, j, version)
			require.Equal(t, image, container.Image,
				"placement[%d].containers[%d] should use the deployed image", i, j)
		}
	}
}

// requireInitDefinition は init が書き出した定義が、アクティブバージョンの内容と
// 一致することを確認します。registryPassword は API から取得できないため
// 含まれていてはいけません。
func requireInitDefinition(t *testing.T, path, clusterName, appName string) {
	t.Helper()

	data, err := os.ReadFile(path) // #nosec G304 -- パスは t.TempDir 配下でテスト自身が作成したものです。
	require.NoError(t, err, "read the definition restored by init")
	require.NotContains(t, string(data), "registryPassword",
		"init で書き出した定義に registryPassword を含めてはいけません")

	var restored map[string]any
	require.NoError(t, json.Unmarshal(data, &restored), "decode the definition restored by init: %s", data)
	require.Equal(t, clusterName, restored["cluster"])
	require.Equal(t, appName, restored["name"])
	require.EqualValues(t, 100, restored["cpu"])
	require.EqualValues(t, 128, restored["memory"])
	require.Equal(t, "manual", restored["scalingMode"])
	require.EqualValues(t, 1, restored["fixedScale"])
	require.Equal(t, tutorialImage, restored["image"])

	ports, ok := restored["exposedPorts"].([]any)
	require.True(t, ok, "exposedPorts should be restored: %v", restored["exposedPorts"])
	require.Len(t, ports, 1)
	port, ok := ports[0].(map[string]any)
	require.True(t, ok, "exposedPorts[0] should be an object: %v", ports[0])
	require.EqualValues(t, appTargetPort, port["targetPort"])
}

// cleanupApplication は保持した ID と識別情報を再照合したうえでアプリケーションを
// 削除し、不在を確認します。すでに削除済みのときは何もしません。削除は実行記録に
// 残る CLI 経路で行います (ADR 0064)。
func cleanupApplication(t *testing.T, h *helper.Helper, applications *application.ApplicationOp, id v1.ApplicationID, clusterName, appName string) {
	t.Helper()

	detail, err := applications.Read(t.Context(), id)
	if err != nil {
		if saclient.IsNotFoundError(err) {
			return
		}
		t.Errorf("read application %s for cleanup: %v", id, err)
		return
	}
	if detail.ApplicationID != id || detail.Name != appName || detail.ClusterName != clusterName {
		t.Errorf("application %s does not match the deployed application (ID %s, name %q, cluster %q); refusing to delete",
			id, detail.ApplicationID, detail.Name, detail.ClusterName)
		return
	}
	definition := h.WriteRequestJSONFile(t, "cleanup-app-definition", map[string]any{
		"cluster": clusterName,
		"name":    appName,
	})
	h.Call(t, "後片付け: アプリケーションを削除する",
		"apprun-dedicated", "delete", "--app", definition, "--force")
	requireApplicationAbsent(t, applications, id)
}

// applicationLister はアプリケーション一覧の取得に必要な操作です。
// テストで応答を差し替えられるように、ApplicationOp の List と同じ
// シグネチャで定義します。
type applicationLister interface {
	List(ctx context.Context, maxItems int64, cursor *v1.ApplicationID) ([]v1.ReadApplication, *v1.ApplicationID, error)
}

// findCreatedApplication は deploy 失敗後に、このテストが作成した可能性のある
// アプリケーションを再検索します。作成前の同名不在確認 (requireNoApplicationNamed)
// を前提に、名前と所属クラスタが一致する 1 件だけ見つかった場合はその ID を返します。
// 見つからなければ作成されなかった (または削除済み) として nil を返します。
// 照会に失敗した場合や同じクラスタに同名が複数ある場合は ID を確定できないため
// エラーを返します。呼び出し側は ID を確定できないときに削除してはなりません。
func findCreatedApplication(t *testing.T, applications applicationLister, clusterName, appName string) (*v1.ApplicationID, error) {
	t.Helper()

	var found *v1.ApplicationID
	var cursor *v1.ApplicationID
	for {
		list, next, err := applications.List(t.Context(), listPageSize, cursor)
		if err != nil {
			return nil, fmt.Errorf("list applications for cleanup: %w", err)
		}
		for _, a := range list {
			// 名前が一致しても所属クラスタが異なるアプリケーションは対象外です。
			// 名前はクラスタ内でのみ一意なため、別クラスタの同名は削除できません。
			if a.Name != appName || a.ClusterName != clusterName {
				continue
			}
			if found != nil {
				return nil, fmt.Errorf("multiple applications named %q found in cluster %q; refusing to delete", appName, clusterName)
			}
			id := a.ApplicationID
			found = &id
		}
		if next == nil {
			break
		}
		cursor = next
	}
	return found, nil
}

// cleanupApplicationAfterDeploy は deploy の成否にかかわらず、このテストが
// 作成した可能性のあるアプリケーションを後片付けします。deploy はアプリ作成後の
// 反映待ちで失敗することがあるため、失敗時も作成済みのアプリケーションを残しません。
// 再検索で ID・名前・所属クラスタを確定できた場合にだけ cleanupApplication で
// 削除し、不在を確認します。確定できないときは削除せず、その旨を報告します。
func cleanupApplicationAfterDeploy(t *testing.T, h *helper.Helper, applications *application.ApplicationOp, clusterName, appName string) {
	t.Helper()

	id, err := findCreatedApplication(t, applications, clusterName, appName)
	if err != nil {
		t.Errorf("cleanup application after deploy: %v", err)
		return
	}
	if id == nil {
		return
	}
	cleanupApplication(t, h, applications, *id, clusterName, appName)
}

// requireApplicationAbsent はアプリケーションが削除後に存在しないことを確認します。
func requireApplicationAbsent(t *testing.T, applications *application.ApplicationOp, id v1.ApplicationID) {
	t.Helper()

	_, err := applications.Read(t.Context(), id)
	require.Error(t, err, "application %s should not remain after deletion", id)
	require.True(t, saclient.IsNotFoundError(err),
		"application %s should be reported as not found, got: %v", id, err)
}

// cleanupAutoScalingGroup は保持した ID と名前を再照合したうえでオートスケーリング
// グループを削除し、不在を確認します。すでに削除済みのときは何もしません。
// グループの削除は非同期に完了するため、不在になるまで再確認します。
func cleanupAutoScalingGroup(t *testing.T, h *helper.Helper, asgs *autoscalinggroup.AutoScalingGroupOp, clusterID v1.ClusterID, asgID v1.AutoScalingGroupID, asgName string) {
	t.Helper()

	detail, err := asgs.Read(t.Context(), asgID)
	if err != nil {
		if saclient.IsNotFoundError(err) {
			return
		}
		t.Errorf("read auto-scaling group %s for cleanup: %v", asgID, err)
		return
	}
	if detail.AutoScalingGroupID != asgID || detail.Name != asgName {
		t.Errorf("auto-scaling group %s does not match the created group (ID %s, name %q); refusing to delete",
			asgID, detail.AutoScalingGroupID, detail.Name)
		return
	}
	h.Call(t, "後片付け: オートスケーリンググループを削除する",
		"apprun-dedicated-api", "delete-auto-scaling-group", "--cluster-id", uuid.UUID(clusterID).String(), "--auto-scaling-group-id", uuid.UUID(asgID).String())
	requireAutoScalingGroupAbsent(t, asgs, asgID)
}

// requireAutoScalingGroupAbsent はオートスケーリンググループが削除後に存在しない
// ことを確認します。削除は非同期に完了するため、`Deleting` の値にかかわらず
// 不在になるまで期限付きで再確認します。
func requireAutoScalingGroupAbsent(t *testing.T, asgs *autoscalinggroup.AutoScalingGroupOp, asgID v1.AutoScalingGroupID) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), deleteAbsentLimit)
	defer cancel()

	ticker := time.NewTicker(statePolling)
	defer ticker.Stop()

	for {
		detail, err := asgs.Read(ctx, asgID)
		if err != nil {
			require.True(t, saclient.IsNotFoundError(err),
				"auto-scaling group %s should be reported as not found after deletion, got: %v", asgID, err)
			return
		}
		t.Logf("auto-scaling group %s is still present (deleting=%t); waiting for deletion", asgID, detail.Deleting)

		select {
		case <-ctx.Done():
			t.Errorf("auto-scaling group %s did not disappear within %s", asgID, deleteAbsentLimit)
			return
		case <-ticker.C:
		}
	}
}

// cleanupCluster は保持した ID と名前を再照合したうえでクラスタを削除し、不在を
// 確認します。すでに削除済みのときは何もしません。クラスタの削除は非同期に
// 完了するため、不在になるまで再確認します。
func cleanupCluster(t *testing.T, h *helper.Helper, clusters *cluster.ClusterOp, clusterID v1.ClusterID, clusterName string) {
	t.Helper()

	detail, err := clusters.Read(t.Context(), clusterID)
	if err != nil {
		if saclient.IsNotFoundError(err) {
			return
		}
		t.Errorf("read cluster %s for cleanup: %v", clusterID, err)
		return
	}
	if detail.ClusterID != clusterID || detail.Name != clusterName {
		t.Errorf("cluster %s does not match the created cluster (ID %s, name %q); refusing to delete",
			clusterID, detail.ClusterID, detail.Name)
		return
	}
	h.Call(t, "後片付け: クラスタを削除する",
		"apprun-dedicated-api", "delete-cluster", uuid.UUID(clusterID).String())
	requireClusterAbsent(t, clusters, clusterID)
}

// requireClusterAbsent はクラスタが削除後に存在しないことを確認します。
func requireClusterAbsent(t *testing.T, clusters *cluster.ClusterOp, clusterID v1.ClusterID) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), deleteAbsentLimit)
	defer cancel()

	ticker := time.NewTicker(statePolling)
	defer ticker.Stop()

	for {
		_, err := clusters.Read(ctx, clusterID)
		if err != nil {
			require.True(t, saclient.IsNotFoundError(err),
				"cluster %s should be reported as not found after deletion, got: %v", clusterID, err)
			return
		}

		select {
		case <-ctx.Done():
			t.Errorf("cluster %s did not disappear within %s", clusterID, deleteAbsentLimit)
			return
		case <-ticker.C:
		}
	}
}

// fakeApplicationLister は findCreatedApplication のテストで一覧の応答を
// 差し替えるためのスタブです。
type fakeApplicationLister struct {
	// list は最初のページの応答です。
	list []v1.ReadApplication
	// next は最初のページの続きがあるときに返すカーソルです。
	next *v1.ApplicationID
	// page2 は next を指定した 2 ページ目の応答です。
	page2 []v1.ReadApplication
	// err が設定されているときは List がエラーを返します。
	err error
	// calls は List の呼び出し回数です。
	calls int
	// maxItems は最後の List 呼び出しで渡されたページサイズです。
	maxItems int64
}

func (f *fakeApplicationLister) List(_ context.Context, maxItems int64, cursor *v1.ApplicationID) ([]v1.ReadApplication, *v1.ApplicationID, error) {
	f.calls++
	f.maxItems = maxItems
	if f.err != nil {
		return nil, nil, f.err
	}
	if cursor == nil {
		return f.list, f.next, nil
	}
	return f.page2, nil, nil
}

// testApplication は検索結果のアプリケーションを組み立てます。
func testApplication(name, clusterName string) v1.ReadApplication {
	return v1.ReadApplication{
		ApplicationID: v1.ApplicationID(uuid.New()),
		Name:          name,
		ClusterName:   clusterName,
	}
}

// deploy 失敗後の再検索が、作成済みのアプリケーションを安全に特定できることを
// 確認します。ID を確定できない場合に削除しないための分岐も検証します。
func TestFindCreatedApplication(t *testing.T) {
	const (
		clusterName = "skr-e2e-cluster"
		appName     = "skr-e2e-app"
	)

	t.Run("作成済みのアプリケーションを ID と名前、所属クラスタで特定する", func(t *testing.T) {
		created := testApplication(appName, clusterName)
		lister := &fakeApplicationLister{list: []v1.ReadApplication{created}}
		id, err := findCreatedApplication(t, lister, clusterName, appName)
		require.NoError(t, err)
		require.NotNil(t, id)
		require.Equal(t, created.ApplicationID, *id, "should find the created application")
		require.Equal(t, 1, lister.calls, "single page should need a single call")
		require.EqualValues(t, listPageSize, lister.maxItems, "should request the scenario page size")
	})

	t.Run("アプリケーションが作成されなかった場合は nil を返す", func(t *testing.T) {
		lister := &fakeApplicationLister{list: []v1.ReadApplication{}}
		id, err := findCreatedApplication(t, lister, clusterName, appName)
		require.NoError(t, err)
		require.Nil(t, id)
	})

	t.Run("所属クラスタが異なる同名のアプリケーションは対象にしない", func(t *testing.T) {
		lister := &fakeApplicationLister{list: []v1.ReadApplication{
			testApplication(appName, "other-cluster"),
		}}
		id, err := findCreatedApplication(t, lister, clusterName, appName)
		require.NoError(t, err)
		require.Nil(t, id, "should not delete an application in another cluster")
	})

	t.Run("同じクラスタに同名が複数ある場合は ID を確定できない", func(t *testing.T) {
		lister := &fakeApplicationLister{list: []v1.ReadApplication{
			testApplication(appName, clusterName),
			testApplication(appName, clusterName),
		}}
		id, err := findCreatedApplication(t, lister, clusterName, appName)
		require.Error(t, err, "refusing to delete when the ID cannot be determined")
		require.Nil(t, id)
	})

	t.Run("照会に失敗した場合はエラーを返し ID を返さない", func(t *testing.T) {
		lister := &fakeApplicationLister{err: errors.New("list failed")}
		id, err := findCreatedApplication(t, lister, clusterName, appName)
		require.Error(t, err)
		require.Nil(t, id, "should not return an ID when the lookup fails")
	})

	t.Run("ページングをまたいで検索する", func(t *testing.T) {
		created := testApplication(appName, clusterName)
		lister := &fakeApplicationLister{
			list:  []v1.ReadApplication{testApplication("skr-e2e-other", clusterName)},
			next:  &[]v1.ApplicationID{v1.ApplicationID(uuid.New())}[0],
			page2: []v1.ReadApplication{created},
		}
		id, err := findCreatedApplication(t, lister, clusterName, appName)
		require.NoError(t, err)
		require.NotNil(t, id)
		require.Equal(t, created.ApplicationID, *id, "should find the application on the second page")
		require.Equal(t, 2, lister.calls, "should follow the cursor to the next page")
	})
}
