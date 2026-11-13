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

// このファイルは apprun-dedicated 高レベルコマンドのインフラライフサイクル
// (create-auto-scaling-group、delete-auto-scaling-group、delete-cluster) の
// ライブ E2E シナリオです。クラスタとオートスケーリンググループの作成を伴うため、
// 実行にはワーカノード (最小構成 1vCPU/2GB x 1 台) の時間割料金が発生します。
// 承認は [#59](https://github.com/sacloud/skr/issues/59) で管理します。
package main

import (
	"os"
	"testing"

	"github.com/google/uuid"
	apprun_dedicated "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// TestScenarioAppRunDedicatedInfrastructureLifecycle は apprun-dedicated 高レベルコマンドの
// インフラライフサイクル (create-auto-scaling-group、delete-auto-scaling-group、delete-cluster) の
// ライブ E2E です。クラスタは低レベルコマンドで作成し、オートスケーリンググループの作成と削除、
// クラスタの削除は高レベルコマンドで行います。作成直後・削除後の確認は SDK で静かに実行します。
func TestScenarioAppRunDedicatedInfrastructureLifecycle(t *testing.T) {
	h := helper.New(t, "apprun-dedicated-infrastructure",
		"AppRun 専有型の高レベルコマンドによるインフラライフサイクル (create-auto-scaling-group、delete-auto-scaling-group、delete-cluster) を確認する。クラスタとワーカノード 1 台のオートスケーリンググループを作成するため、ワーカノードの時間割料金が発生する")

	servicePrincipalID := os.Getenv(envServicePrincipalID)
	if servicePrincipalID == "" {
		t.Skipf("%s not set, skipping E2E test", envServicePrincipalID)
	}

	sdkClient := newAppRunDedicatedClient(t)
	clusters := apprun_dedicated.NewClusterOp(sdkClient)

	suffix := helper.RandomSuffix(t)
	clusterName := "skr-" + suffix
	asgName := "skr-" + suffix

	requireNoClusterNamed(t, clusters, clusterName)

	// クラスタを低レベルコマンドで作成する。
	clusterRequest := h.WriteRequestJSONFile(t, "create-cluster", map[string]any{
		"name":               clusterName,
		"ports":              []any{},
		"servicePrincipalID": servicePrincipalID,
	})
	data := h.CallWithTimeout(t, createCommandLimit, "クラスタを作成する",
		"apprun-dedicated-api", "create-cluster", "--request", "@"+clusterRequest)
	clusterID := parseCreateClusterOutput(t, data)
	require.NotEqual(t, uuid.Nil, uuid.UUID(clusterID), "created cluster should have an ID")

	verifyCreatedCluster(t, clusters, clusterID, clusterName)

	defer func() {
		cleanupCluster(t, h, clusters, clusterID, clusterName)
	}()

	// 高レベルコマンドでオートスケーリンググループを作成する。
	data = h.CallWithTimeout(t, createCommandLimit, "オートスケーリンググループを作成する (高レベルコマンド)",
		"apprun-dedicated", "create-auto-scaling-group", clusterName, asgName,
		"--zone", apprunDedicatedZone,
		"--worker-service-class-path", workerServiceClassPath,
		"--name-servers", "210.188.224.10",
		"--min-nodes", "1", "--max-nodes", "1",
		"--wait-timeout", workerNodeWaitLimit.String())
	asgID := parseCreateAutoScalingGroupOutput(t, data)
	require.NotEqual(t, uuid.Nil, uuid.UUID(asgID), "created auto-scaling group should have an ID")

	asgs := apprun_dedicated.NewAutoScalingGroupOp(sdkClient, clusterID)
	verifyCreatedAutoScalingGroup(t, asgs, asgID, asgName)
	waitForHealthyWorkerNode(t, apprun_dedicated.NewWorkerNodeOp(sdkClient, clusterID, asgID))

	// 高レベルコマンドでオートスケーリンググループを削除する。
	h.CallWithTimeout(t, createCommandLimit, "オートスケーリンググループを削除する (高レベルコマンド)",
		"apprun-dedicated", "delete-auto-scaling-group", clusterName, asgName, "--force", "--no-wait")
	requireAutoScalingGroupAbsent(t, asgs, asgID)

	// 後片付け: 失敗時などにオートスケーリンググループが残っていれば削除する。
	defer func() {
		cleanupAutoScalingGroup(t, h, asgs, clusterID, asgID, asgName)
	}()

	// 高レベルコマンドでクラスタを削除する。依存リソースはこの時点で残っていない。
	h.CallWithTimeout(t, createCommandLimit, "クラスタを削除する (高レベルコマンド)",
		"apprun-dedicated", "delete-cluster", clusterName, "--force", "--no-wait")

	// 削除後の不在確認を SDK で行う。
	requireClusterAbsent(t, clusters, clusterID)
}
