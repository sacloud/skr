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
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	apprun_dedicated "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated"
	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	// deployFailureWaitTimeout は反映待ちを失敗させるために deploy に渡す短いタイムアウトです。
	deployFailureWaitTimeout = 1 * time.Minute
)

// ライブ E2E です。deploy が反映待ちで失敗した場合でも、作成済みのアプリケーションが
// 後片付けで安全に削除されることを確認します。アプリケーションの作成とバージョンの
// 有効化はオートスケーリンググループに依存しないため、クラスタだけを作成して
// ワーカノードの料金を発生させずに反映待ちをタイムアウトさせます。
// 後片付けは deploy より前に登録し、テストが途中で失敗しても実行されます。
func TestScenarioAppRunDedicatedDeployFailureCleanup(t *testing.T) {
	h := helper.New(t, "apprun-dedicated-deploy-failure",
		"AppRun 専有型の deploy が反映待ちで失敗した場合に、作成済みのアプリケーションが後片付けで削除されることを確認する。クラスタのみを作成し、ワーカノードは作らないためワーカノードの料金は発生しない")

	// サービスプリンシパルはプロジェクト固有のため、環境変数で渡します。
	// 未設定のときは依存不足としてスキップします。
	servicePrincipalID := os.Getenv(envServicePrincipalID)
	if servicePrincipalID == "" {
		t.Skipf("%s not set, skipping E2E test", envServicePrincipalID)
	}

	// 事前確認と後片付けの確認は SDK で静かに実行します。
	sdkClient := newAppRunDedicatedClient(t)
	clusters := apprun_dedicated.NewClusterOp(sdkClient)
	applications := apprun_dedicated.NewApplicationOp(sdkClient)

	suffix := helper.RandomSuffix(t)
	clusterName := "skr-" + suffix
	appName := "skr-" + suffix

	// 作成前に同名リソースが存在しないことを確認する。
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

	// 後片付け: クラスタを削除する。defer は LIFO のため、アプリケーションの
	// 後片付けのあとに実行されます。
	defer func() {
		cleanupCluster(t, h, clusters, clusterID, clusterName)
	}()

	// デプロイする定義を書き出す。公開イメージのためレジストリ認証は指定しません。
	definition := map[string]any{
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
	path := h.WriteRequestJSONFile(t, "deploy-definition", definition)

	// 後片付け: deploy の成否にかかわらず、再検索で作成済みを確認できた
	// アプリケーションを削除します。deploy より前に登録するのは、
	// deploy の失敗後も後片付けを実行するためです。
	defer func() {
		cleanupApplicationAfterDeploy(t, h, applications, clusterName, appName)
	}()

	// 待機の期限を短くして、アプリケーション作成後の反映待ちで失敗させます。
	// オートスケーリンググループがないためコンテナは起動せず、待機は必ずタイムアウトします。
	_, stderr, err := h.CallAllowingFailure(t, "deploy を実行し、反映待ちのタイムアウトで失敗させる",
		"apprun-dedicated", "deploy", "--app", path, "--wait-timeout", deployFailureWaitTimeout.String())
	require.Error(t, err, "deploy should fail when the wait times out without worker nodes")
	require.Contains(t, stderr, "待機がタイムアウトしました",
		"deploy failure should come from the wait timeout, stderr: %s", stderr)

	// deploy はアプリケーションを作成してから反映待ちで失敗したはずです。
	// 後片付けが対象を削除できる前提を、SDK で確認します。
	list, _, err := applications.List(t.Context(), listPageSize, nil)
	require.NoError(t, err, "list applications after the failed deploy")
	var created *v1.ReadApplication
	for i := range list {
		if list[i].Name == appName && list[i].ClusterName == clusterName {
			require.Nil(t, created, "application %q should be unique in cluster %q", appName, clusterName)
			created = &list[i]
		}
	}
	require.NotNil(t, created, "deploy should have created the application before the wait timed out")
}
