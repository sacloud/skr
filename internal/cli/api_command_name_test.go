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

package cli

import (
	"strings"
	"testing"

	"github.com/fatih/camelcase"
	"github.com/stretchr/testify/require"
)

// apiCommandNameTargetDomains は operationId 機械派生の対象ドメインです。
// 対象と規則の詳細は docs/design/api-command-operation-id-naming.md を参照してください。
var apiCommandNameTargetDomains = []string{
	"apprun-api",
	"apprun-dedicated-api",
	"eventbus-api",
	"iam-api",
	"kms-api",
	"networking-suite-api",
	"object-storage-api",
	"secretmanager-api",
	"security-control-api",
	"service-endpoint-gateway-api",
	"simple-notification-api",
	"simplemq-api",
}

// deriveOperationCommandName は SDK の operation 定数の値 (operationId を
// 正規化した operation 名) から末端サブコマンド名を派生します。
// camelcase 分割は Kong に内蔵された実装 (fatih/camelcase のコピー、
// Kong の camelcase.go 冒頭で由来を明記) と同じものを利用し、
// 語列のハイフン結合と小文字化は Kong の既定命名と同じです。
func deriveOperationCommandName(operationName string) string {
	return strings.ToLower(strings.Join(camelcase.Split(operationName), "-"))
}

// TestDeriveOperationCommandName は operation 名から末端サブコマンド名を派生する処理を確認します。
func TestDeriveOperationCommandName(t *testing.T) {
	// 入力は SDK (ogen) が operationId を正規化した operation 名の値です。
	// 期待値は docs/design/api-command-operation-id-naming.md の対応表と一致させます。
	// RotateAPIKey は SDK の操作名として正当な語で認証情報ではないため、
	// gosec G101 の誤検出を除外します。
	cases := map[string]string{ //nolint:gosec
		"SendMessage":                        "send-message",
		"SegApply":                           "seg-apply",
		"SegStatusGetPowerStatus":            "seg-status-get-power-status",
		"RotateAPIKey":                       "rotate-api-key",
		"ListCommonServiceItems":             "list-common-service-items",
		"GetBucketMetering":                  "get-bucket-metering",
		"UpdateWorkerNodeDraining":           "update-worker-node-draining",
		"ListLoadBalancerNodes":              "list-load-balancer-nodes",
		"SetProcessConfigurationSecret":      "set-process-configuration-secret",
		"EncryptDataWithKey":                 "encrypt-data-with-key",
		"CheckNotificationDestinationStatus": "check-notification-destination-status",
	}
	for op, want := range cases {
		require.Equal(t, want, deriveOperationCommandName(op), "operation 名 %q から派生した名前が期待と不一致です", op)
	}
}

// TestAPICommandNamesFollowOperationIDs は対象ドメインの末端サブコマンド名が、
// operationId から派生した名前と一致することを確認します。
//
// 期待名の計算元は、対象ドメインが公開する操作の一覧 (apiPublishedOperations) です。
// 新規コマンドは対応する operation 定数を一覧に登録することで、例外なしで検証できます。
// 移行は全ドメインで完了したため、baseline による例外は廃止しました。
// 詳細は docs/design/api-command-operation-id-naming.md を参照してください。
func TestAPICommandNamesFollowOperationIDs(t *testing.T) {
	paths, err := LeafCommandPaths()
	require.NoError(t, err)

	// 対象ドメインの集合と、公開操作の一覧のキーが一致することを確認します。
	// 片方だけドメインを更新すると、移行漏れや検証漏れが CI をすり抜けるためです。
	publishedDomains := make([]string, 0, len(apiPublishedOperations))
	for dom := range apiPublishedOperations {
		publishedDomains = append(publishedDomains, dom)
	}
	require.ElementsMatch(t, apiCommandNameTargetDomains, publishedDomains,
		"対象ドメインの集合と公開操作の一覧のドメインが一致しません。両方を同じ内容で更新してください")

	// 対象ドメインごとに、公開する操作の operation 定数から派生名を計算します。
	expected := make(map[string]map[string]struct{}, len(apiPublishedOperations))
	for dom, ops := range apiPublishedOperations {
		expected[dom] = make(map[string]struct{}, len(ops))
		for _, op := range ops {
			expected[dom][deriveOperationCommandName(op)] = struct{}{}
		}
	}

	// 各ドメインの末端パスの集合が、期待パスの集合と一致することを確認します。
	// 余分なコマンドや未整備のコマンドもここで検出します。
	for _, dom := range apiCommandNameTargetDomains {
		var actual []string
		for _, p := range paths {
			if leaf, ok := strings.CutPrefix(p, dom+" "); ok {
				actual = append(actual, leaf)
			}
		}
		want := make([]string, 0, len(expected[dom]))
		for leaf := range expected[dom] {
			want = append(want, leaf)
		}
		require.ElementsMatch(t, want, actual,
			"ドメイン %s の末端サブコマンドが operationId から派生した期待パスと一致しません", dom)
	}
}
