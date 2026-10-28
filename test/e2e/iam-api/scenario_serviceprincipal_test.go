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

// このファイルは iam-api E2E のサービスプリンシパルシナリオです。
// 名前の更新、キーのライフサイクル、JWT assertion からの access token 発行を確認します。
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// servicePrincipalOAuth2Endpoint は IAM API の OAuth2 トークンエンドポイントです。
// API は aud クレームをこのエンドポイントと照合します。
// エンドポイントは SAKURA_ENDPOINTS_IAM で上書きできます。
const servicePrincipalOAuth2Endpoint = "https://secure.sakura.ad.jp/cloud/api/iam/1.0/service-principals/oauth2/token"

type servicePrincipalKey struct {
	ID     string `json:"id"`
	Kid    string `json:"kid"`
	Status string `json:"status"`
}

type servicePrincipalKeyList struct {
	Items []servicePrincipalKey `json:"items"`
}

type accessTokenSummary struct {
	TokenType string `json:"token_type"`
	ExpiresIn int64  `json:"expires_in"`
}

// TestScenarioIAMServicePrincipal はテスト用の service principal を作成し、
// 名前の更新、キーのライフサイクル、JWT assertion からの access token 発行を確認します。
// 秘密鍵はこのプロセスから出ません。
// 公開鍵と署名済み assertion のみがプライベートなファイルに書き込まれます。
func TestScenarioIAMServicePrincipal(t *testing.T) {
	h := helper.New(t, "iam-api-service-principal",
		"テスト用のサービスプリンシパルのキーと access token 発行が正しく動作することを確認する")

	suffix := helper.RandomSuffix(t)
	name := resourceNamePrefix + suffix
	desc := resourceDescription + suffix
	run := &iamRun{
		folder:           iamResource{kind: "folder", name: name, description: desc},
		project:          iamResource{kind: "project", name: name, code: "e2e" + suffix, description: desc},
		servicePrincipal: iamResource{kind: "service-principal", name: name, description: desc},
	}

	// 作成前に同名のリソースが存在しないことを確認する
	requireNoNameCollision(t, h, run)

	// 後片付け: service principal、project、folder を再照合つき削除して残っていないことを確認する
	defer func() {
		if err := cleanupIAMResources(t, h, run); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()

	// テスト用の folder と project を作成する
	createFolderAndProject(t, h, run)
	// テスト用の service principal を作成する
	createServicePrincipal(t, h, run)
	// service principal の名前を更新して元に戻す
	updateServicePrincipal(t, h, run)
	// service principal のキーと access token を確認する
	runServicePrincipalKeysScenario(t, h, run)
}

// updateServicePrincipal は service principal の名前を変更し、元の名前に戻します。
// description は実行の description のままなので、
// cleanup の identity チェックが機能し続けます。
func updateServicePrincipal(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	renamed := run.servicePrincipal.name + "-renamed"
	updateServicePrincipalName(t, h, run, "service principalの名前を更新する", renamed)
	updateServicePrincipalName(t, h, run, "service principalの名前を元に戻す", run.servicePrincipal.name)
}

func updateServicePrincipalName(t *testing.T, h *helper.Helper, run *iamRun, description, name string) {
	t.Helper()
	request := mustSettingsJSON(map[string]string{
		"name":        name,
		"description": run.servicePrincipal.description,
	})
	data := h.Call(t, description, "iam-api", "service-principal", "update", run.servicePrincipal.id,
		"--request", request, "--query", "{id: .id, name: .name}")
	_, updatedName, err := decodeNamedResource("update-service-principal", data)
	require.NoError(t, err, "%s: 応答のデコードに失敗しました", description)
	require.Equal(t, name, updatedName, "%s: 更新後の名前がリクエストと一致すること", description)
}

// runServicePrincipalKeysScenario は生成された公開鍵をアップロードし、
// キーのライフサイクルを確認して、
// 対応する秘密鍵で署名した JWT assertion から access token を発行します。
func runServicePrincipalKeysScenario(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	keyPair, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "サービスプリンシパルのキーペアを生成できること")
	publicKeyFile := writePublicKeyFile(t, keyPair)

	data := h.Call(t, "生成した公開鍵をservice principalに登録する", "iam-api", "service-principal", "upload-key",
		"--public-key-file", publicKeyFile, run.servicePrincipal.id,
		"--query", "{id: .id, kid: .kid, status: .status}")
	key, err := decodeServicePrincipalKey("upload-key", data)
	require.NoError(t, err, "upload-key: 応答のデコードに失敗しました")
	require.Equal(t, "enabled", key.Status, "upload-key: 登録直後のキーが有効であること")
	require.NotEmpty(t, key.Kid, "upload-key: kidが存在すること")
	require.NotEmpty(t, key.ID, "upload-key: IDが存在すること")

	data = h.Call(t, "service principalのキー一覧を確認する", "iam-api", "service-principal", "list-keys", run.servicePrincipal.id,
		"--query", "{items: [.items[] | {id: .id, kid: .kid, status: .status}]}")
	found, err := checkServicePrincipalKey("list-keys", data, key)
	require.NoError(t, err, "list-keys: キー一覧のデコードに失敗しました")
	require.True(t, found, "list-keys: キー %sが一覧に存在すること", key.ID)

	for _, transition := range []struct {
		description string
		method      string
		status      string
	}{
		{description: "キーを無効化する", method: "disable-key", status: "disabled"},
		{description: "キーを有効化する", method: "enable-key", status: "enabled"},
	} {
		data = h.Call(t, transition.description, "iam-api", "service-principal", transition.method,
			run.servicePrincipal.id, key.ID, "--query", "{id: .id, status: .status}")
		updated, err := decodeServicePrincipalKey(transition.method, data)
		require.NoError(t, err, "%s: 応答のデコードに失敗しました", transition.method)
		require.Equal(t, key.ID, updated.ID, "%s: キーIDが一致すること", transition.method)
		require.Equal(t, transition.status, updated.Status, "%s: キーの状態が遷移すること", transition.method)
	}

	issueServicePrincipalToken(t, h, run, keyPair, key.Kid)

	h.Call(t, "キーを削除する", "iam-api", "service-principal", "delete-key",
		run.servicePrincipal.id, key.ID)
	data = h.Call(t, "削除後のキー一覧を確認する", "iam-api", "service-principal", "list-keys", run.servicePrincipal.id,
		"--query", "{items: [.items[] | {id: .id, kid: .kid, status: .status}]}")
	found, err = checkServicePrincipalKey("list-keys-after-delete", data, key)
	require.NoError(t, err, "list-keys-after-delete: キー一覧のデコードに失敗しました")
	require.False(t, found, "list-keys-after-delete: キー %sが削除後に存在しないこと", key.ID)
}

// issueServicePrincipalToken は、アップロードされた公開鍵の秘密鍵で JWT bearer assertion に署名し、
// access token を発行します。応答にはトークンが含まれるため、
// query はトークンタイプと有効期間のみを投影します。
func issueServicePrincipalToken(t *testing.T, h *helper.Helper, run *iamRun, privateKey *rsa.PrivateKey, kid string) {
	t.Helper()
	now := time.Now()
	audience := servicePrincipalOAuth2Endpoint
	if override, ok := servicePrincipalOAuth2EndpointOverride(); ok {
		audience = override
	}
	claims := jwt.MapClaims{
		"iss": run.servicePrincipal.id,
		"sub": run.servicePrincipal.id,
		"aud": audience,
		"iat": now.Unix(),
		"exp": now.Add(3500 * time.Second).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	assertion, err := token.SignedString(privateKey)
	require.NoError(t, err, "サービスプリンシパルのassertionに署名できること")
	// 署名済み assertion は秘密情報を含むため、テスト専用ディレクトリの保護したファイルに書き込みます。
	assertionFile := filepath.Join(t.TempDir(), "assertion")
	require.NoError(t, os.WriteFile(assertionFile, []byte(assertion), 0o600), "assertionファイルの書き込みに失敗しました")

	data := h.Call(t, "JWT assertionからaccess tokenを発行する", "iam-api", "service-principal", "issue-token",
		"--assertion-file", assertionFile, "--query", "{token_type: .token_type, expires_in: .expires_in}")
	var summary accessTokenSummary
	require.NoError(t, json.Unmarshal(data, &summary), "issue-token: トークンサマリーのデコードに失敗しました")
	require.Equal(t, "Bearer", summary.TokenType, "issue-token: トークンタイプがBearerであること")
	require.Positive(t, summary.ExpiresIn, "issue-token: 有効期間が正の値であること")
}

func writePublicKeyFile(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err, "サービスプリンシパルの公開鍵をエンコードできること")
	encoded := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	// 公開鍵は秘密情報ではないため、テスト専用ディレクトリの保護したファイルに書き込みます。
	path := filepath.Join(t.TempDir(), "skr-e2e-iam-sp-public-key.pem")
	require.NoError(t, os.WriteFile(path, encoded, 0o600), "公開鍵ファイルの書き込みに失敗しました")
	return path
}

func decodeServicePrincipalKey(step string, data []byte) (servicePrincipalKey, error) {
	var key servicePrincipalKey
	if err := json.Unmarshal(data, &key); err != nil {
		return servicePrincipalKey{}, fmt.Errorf("%s: decode key: %w", step, err)
	}
	if key.ID == "" {
		return servicePrincipalKey{}, fmt.Errorf("%s: key ID is missing", step)
	}
	return key, nil
}

func checkServicePrincipalKey(step string, data []byte, expected servicePrincipalKey) (bool, error) {
	var list servicePrincipalKeyList
	if err := json.Unmarshal(data, &list); err != nil {
		return false, fmt.Errorf("%s: decode keys: %w", step, err)
	}
	for _, candidate := range list.Items {
		if candidate.ID == expected.ID {
			if candidate.Kid != expected.Kid || candidate.Status != expected.Status {
				return false, fmt.Errorf("%s: got %q/%q, want %q/%q", step, candidate.Kid, candidate.Status, expected.Kid, expected.Status)
			}
			return true, nil
		}
	}
	return false, nil
}

// servicePrincipalOAuth2EndpointOverride は、
// 設定されている場合に SAKURA_ENDPOINTS_IAM から導出されたトークンエンドポイントの audience を返します。
func servicePrincipalOAuth2EndpointOverride() (string, bool) {
	endpoint := os.Getenv("SAKURA_ENDPOINTS_IAM")
	if endpoint == "" {
		return "", false
	}
	return endpoint + "/service-principals/oauth2/token", true
}
