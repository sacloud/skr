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

// このファイルは iam-api E2E のコアリソースシナリオです。
// ランダムな識別子を含む一時リソースを作成し、一覧・読み取り・更新と後片付けを確認します。
package main

import (
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// TestScenarioIAM はテスト用の user、group、folder、project、service principal を作成し、
// 一覧・読み取り・名前の更新、メールアドレスの登録と解除、group の所属更新と後片付けを確認します。
func TestScenarioIAM(t *testing.T) {
	h := helper.New(t, "iam-api",
		"テスト用のユーザー、グループ、フォルダ、プロジェクト、サービスプリンシパルを作成し、一覧・読み取り・更新と後片付けが正しく動作することを確認する")

	suffix := helper.RandomSuffix(t)
	name := resourceNamePrefix + suffix
	desc := resourceDescription + suffix
	run := &iamRun{
		password:         randomPassword(t),
		email:            "skr-e2e-iam@example.com",
		user:             iamResource{kind: "user", name: name, code: "e2e_" + suffix, description: desc},
		group:            iamResource{kind: "group", name: name, description: desc},
		folder:           iamResource{kind: "folder", name: name, description: desc},
		project:          iamResource{kind: "project", name: name, code: "e2e" + suffix, description: desc},
		servicePrincipal: iamResource{kind: "service-principal", name: name, description: desc},
	}

	// 作成前に同名のリソースが存在しないことを確認する
	requireNoNameCollision(t, h, run)

	// 後片付け: 依存関係の逆順で再照合つき削除して残っていないことを確認する
	defer func() {
		if err := cleanupIAMResources(t, h, run); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()

	// テスト用の folder と project を作成する
	createFolderAndProject(t, h, run)
	// テスト用の user と service principal を作成する
	createUserAndServicePrincipal(t, h, run)
	// 作成した user を確認し、名前とメールアドレスを更新する
	runUserLifecycleScenario(t, h, run)
	// folder と project の名前を更新する
	renameFolderAndProject(t, h, run)
	// group を作成し、所属を user だけに更新する
	runGroupScenario(t, h, run)
}
