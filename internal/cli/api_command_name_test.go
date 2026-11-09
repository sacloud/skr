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
	"unicode"
	"unicode/utf8"

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

// splitCamelWords は Kong がフィールド名からコマンド名を生成する際に使う
// camelcase 分割と同じアルゴリズムで語列に分割します。
// Kong の既定命名は分割した語をハイフンで結合して小文字化したものです。
func splitCamelWords(src string) []string {
	// 無効な UTF-8 は分割しない
	if !utf8.ValidString(src) {
		return []string{src}
	}
	var runes [][]rune
	lastClass := 0
	// 文字の種類 (小文字 / 大文字 / 数字 / それ以外) が変わる位置で分割する
	for _, r := range src {
		var class int
		switch {
		case unicode.IsLower(r):
			class = 1
		case unicode.IsUpper(r):
			class = 2
		case unicode.IsDigit(r):
			class = 3
		default:
			class = 4
		}
		if class == lastClass {
			runes[len(runes)-1] = append(runes[len(runes)-1], r)
		} else {
			runes = append(runes, []rune{r})
		}
		lastClass = class
	}
	// 大文字から小文字へ続く語の最後の大文字を次の語に移動する
	// (例: "PDFL", "oader" -> "PDF", "Loader")
	for i := 0; i < len(runes)-1; i++ {
		if unicode.IsUpper(runes[i][0]) && unicode.IsLower(runes[i+1][0]) {
			runes[i+1] = append([]rune{runes[i][len(runes[i])-1]}, runes[i+1]...)
			runes[i] = runes[i][:len(runes[i])-1]
		}
	}
	var entries []string
	for _, s := range runes {
		if len(s) > 0 {
			entries = append(entries, string(s))
		}
	}
	return entries
}

// deriveOperationCommandName は SDK の operation 定数の値 (operationId を
// 正規化した operation 名) から末端サブコマンド名を派生します。
// 語列のハイフン結合と小文字化は Kong の既定命名と同じです。
func deriveOperationCommandName(operationName string) string {
	return strings.ToLower(strings.Join(splitCamelWords(operationName), "-"))
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
// operationId から派生した名前と一致するか、移行前の現行パスとして baseline
// (apiCommandNameBaseline) に登録されていることを確認します。
//
// baseline に登録できない新規コマンドは、最初から派生名に従う必要があります。
// 移行の進行に合わせて該当ドメインの baseline エントリを削除し、全ドメイン完了後に
// baseline の機構自体を削除します。
func TestAPICommandNamesFollowOperationIDs(t *testing.T) {
	paths, err := LeafCommandPaths()
	require.NoError(t, err)
	pathSet := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		pathSet[p] = struct{}{}
	}

	// baseline に登録されたパスは CLI に実在することを確認します。
	// 実在しないパスの登録は baseline の腐敗 (移行済みエントリの残置) を示します。
	for p := range apiCommandNameBaseline {
		_, ok := pathSet[p]
		require.True(t, ok, "baseline に登録されたパスが CLI に存在しません。エントリを整理してください: skr %s", p)
	}

	// baseline に登録した operation 定数から、ドメインごとの派生名を計算します。
	expected := make(map[string]map[string]struct{})
	for p, op := range apiCommandNameBaseline {
		dom := strings.Fields(p)[0]
		if expected[dom] == nil {
			expected[dom] = make(map[string]struct{})
		}
		expected[dom][deriveOperationCommandName(op)] = struct{}{}
	}

	// 対象ドメインの各リーフパスは、operationId から派生した名前か、
	// baseline に登録された現行パスであること。
	for _, p := range paths {
		if !isAPICommandNameTargetDomain(p) {
			continue
		}
		dom, leaf, found := strings.Cut(p, " ")
		if !found {
			continue
		}
		_, derived := expected[dom][leaf]
		_, baseline := apiCommandNameBaseline[p]
		require.True(t, derived || baseline,
			"パス skr %s は operationId から派生した名前でも、移行前の現行パスとして baseline に登録された名前でもありません", p)
	}
}

// isAPICommandNameTargetDomain はパスが operationId 機械派生の対象ドメイン配下かどうかを返します。
func isAPICommandNameTargetDomain(path string) bool {
	for _, dom := range apiCommandNameTargetDomains {
		if path == dom || strings.HasPrefix(path, dom+" ") {
			return true
		}
	}
	return false
}
