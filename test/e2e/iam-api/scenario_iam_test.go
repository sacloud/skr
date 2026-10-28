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

// このファイルは iam-api E2E のエントリーシナリオです。
// ランダムな識別子を含む一時リソースを作成し、操作の正常動作と後片付けを確認します。
package main

import (
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// TestScenarioIAM は IAM API の E2E シナリオです。ランダムな識別子を含む一時リソースを
// 作成し、読み取り・更新・ライフサイクルの操作と後片付けを確認します。
func TestScenarioIAM(t *testing.T) {
	h := helper.New(t, "iam-api",
		"IAM API でユーザー、ポリシー、プロジェクト、サービスプリンシパル、API キー、SCIM、SSO などの操作が正しく動作することを確認する")

	suffix := helper.RandomSuffix(t)
	name := resourceNamePrefix + suffix
	desc := resourceDescription + suffix
	run := &iamRun{
		password:         randomPassword(t),
		email:            "skr-e2e-iam@example.com",
		user:             iamResource{kind: "user", name: name, code: "e2e_" + suffix, description: desc},
		group:            iamResource{kind: "group", name: name, description: desc},
		folder:           iamResource{kind: "folder", name: name, description: desc},
		childFolder:      iamResource{kind: "folder", name: name + "-child", description: desc},
		project:          iamResource{kind: "project", name: name, code: "e2e" + suffix, description: desc},
		servicePrincipal: iamResource{kind: "service-principal", name: name, description: desc},
		scim:             iamResource{kind: "scim", name: name + "-scim"},
		ssoProfile:       iamResource{kind: "sso", name: name + "-sso"},
		projectAPIKey:    iamResource{kind: "project-api-key", name: name + "-pak"},
	}

	// 作成前に同名のリソースが存在しないことを確認する
	requireNoNameCollision(t, h, run)

	// 後片付け: organization 全体の設定を先に削除し、依存関係の逆順で
	// 再照合つき削除して残っていないことを確認する
	defer func() {
		if err := cleanupIAMResources(t, h, run); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()

	// この実行に依存しない読み取りコマンドを確認する
	runReadOnlyScenario(t, h, run)
	// テスト用の folder と project を作成する
	createFolderAndProject(t, h, run)
	// folder と project のポリシー binding を読み込む
	readResourcePolicies(t, h, run)
	// folder と project のポリシー binding を更新して元に戻す
	runPolicyBindingScenario(t, h, run)
	// 子 folder を作成して階層を移動する
	runHierarchyScenario(t, h, run)
	// テスト用の user と service principal を作成する
	createUserAndServicePrincipal(t, h, run)
	// service principal の名前・キー・access token を確認する
	runServicePrincipalScenario(t, h, run)
	// project API key のライフサイクルを確認する
	runProjectAPIKeyScenario(t, h, run)
	// SCIM 設定のライフサイクルを確認する
	runScimScenario(t, h, run)
	// SSO profile のライフサイクルを確認する
	runSSOScenario(t, h, run)
	// 作成した user を確認し、名前とメールアドレスを更新する
	runUserLifecycleScenario(t, h, run)
	// folder と project の名前を更新する
	renameFolderAndProject(t, h, run)
	// group を作成し、所属を更新する
	runGroupScenario(t, h, run)
	// user 2FA コマンドを確認する
	runUser2FAScenario(t, h, run)
	// organization スコープの更新コマンドを確認する
	runAccountScenario(t, h, run)
}
