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

func TestScenarioIAM(t *testing.T) {
	h := helper.New(t, "iam-api",
		"IAM API でユーザー、ポリシー、プロジェクト、サービスプリンシパル、API キー、SCIM、SSO などの操作が正しく動作することを確認する")
	suffix := helper.RandomSuffix(t)
	name := resourceNamePrefix + suffix
	desc := resourceDescription + suffix
	s := &iamRun{
		h:        h,
		password: randomPassword(t),
		suffix:   suffix,
		user:     resource{kind: "user", name: name, code: "e2e_" + suffix, description: desc},
		group:    resource{kind: "group", name: name, description: desc},
		folder:   resource{kind: "folder", name: name, description: desc},
		childFolder: resource{
			kind: "folder", name: name + "-child", description: desc,
		},
		project: resource{kind: "project", name: name, code: "e2e" + suffix, description: desc},
		servicePrincipal: resource{
			kind: "service-principal", name: name, description: desc,
		},
		scim: managedResource{
			kind: "scim", name: name + "-scim",
		},
		ssoProfile: managedResource{
			kind: "sso", name: name + "-sso",
		},
		projectAPIKey: managedResource{
			kind: "project-api-key", name: name + "-pak",
		},
		email:   "skr-e2e-iam@example.com",
		renamed: name + "-renamed",
	}
	s.run(t)
}
