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
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/golang-jwt/jwt/v5"
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

// servicePrincipalScenario は、この実行の service principal を更新・復元し、
// 生成された RSA キーペアをアップロードしてキーのライフサイクルを確認し、
// 秘密鍵で署名した JWT assertion から access token を発行します。
// 秘密鍵はこのプロセスから出ません。
// 公開鍵と署名済み assertion のみがプライベートなファイルに書き込まれます。
func (s *scenario) servicePrincipalScenario(ctx context.Context) error {
	if err := s.updateServicePrincipal(ctx); err != nil {
		return err
	}
	return s.servicePrincipalKeysScenario(ctx)
}

// updateServicePrincipal は service principal の名前を変更し、元の名前に戻します。
// description は実行の description のままなので、
// cleanup の identity チェックが機能し続けます。
func (s *scenario) updateServicePrincipal(ctx context.Context) error {
	renamed := s.servicePrincipal.name + "-renamed"
	if err := s.updateServicePrincipalName(ctx, "update-service-principal", renamed); err != nil {
		return err
	}
	return s.updateServicePrincipalName(ctx, "restore-service-principal", s.servicePrincipal.name)
}

func (s *scenario) updateServicePrincipalName(ctx context.Context, step, name string) error {
	request, err := json.Marshal(map[string]string{
		"name":        name,
		"description": s.servicePrincipal.description,
	})
	if err != nil {
		return fmt.Errorf("%s: build request: %w", step, err)
	}
	data, err := s.call(ctx, step, "iam-api", "service-principal", "update", s.servicePrincipal.id,
		"--request", string(request), "--query", "{id: .id, name: .name}")
	if err != nil {
		return err
	}
	_, updatedName, err := decodeNamedResource(step, data)
	if err != nil {
		return err
	}
	if updatedName != name {
		return fmt.Errorf("%s: name = %q, want %q", step, updatedName, name)
	}
	return nil
}

// servicePrincipalKeysScenario は生成された公開鍵をアップロードし、
// キーのライフサイクルを確認して、
// 対応する秘密鍵で署名した JWT assertion から access token を発行します。
func (s *scenario) servicePrincipalKeysScenario(ctx context.Context) error {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate service principal key pair: %w", err)
	}
	publicKeyFile, err := writePublicKeyFile(privateKey, s.suffix)
	if err != nil {
		return err
	}
	defer func() {
		if err := os.Remove(publicKeyFile); err != nil && !os.IsNotExist(err) {
			fmt.Printf("remove private public key file: %v\n", err)
		}
	}()

	data, err := s.call(ctx, "upload-key", "iam-api", "service-principal", "upload-key",
		"--public-key-file", publicKeyFile, s.servicePrincipal.id,
		"--query", "{id: .id, kid: .kid, status: .status}")
	if err != nil {
		return err
	}
	key, err := decodeServicePrincipalKey("upload-key", data)
	if err != nil {
		return err
	}
	if key.Status != "enabled" {
		return fmt.Errorf("upload-key: status = %q, want %q", key.Status, "enabled")
	}
	if key.Kid == "" || key.ID == "" {
		return fmt.Errorf("upload-key: kid and id must be present")
	}

	data, err = s.call(ctx, "list-keys", "iam-api", "service-principal", "list-keys", s.servicePrincipal.id,
		"--query", "{items: [.items[] | {id: .id, kid: .kid, status: .status}]}")
	if err != nil {
		return err
	}
	if found, err := checkServicePrincipalKey("list-keys", data, key); err != nil {
		return err
	} else if !found {
		return fmt.Errorf("list-keys: key %s is missing from the list", key.ID)
	}

	for _, transition := range []struct {
		step   string
		method string
		status string
	}{
		{step: "disable-key", method: "disable-key", status: "disabled"},
		{step: "enable-key", method: "enable-key", status: "enabled"},
	} {
		data, err = s.call(ctx, transition.step, "iam-api", "service-principal", transition.method,
			s.servicePrincipal.id, key.ID, "--query", "{id: .id, status: .status}")
		if err != nil {
			return err
		}
		updated, err := decodeServicePrincipalKey(transition.step, data)
		if err != nil {
			return err
		}
		if updated.ID != key.ID || updated.Status != transition.status {
			return fmt.Errorf("%s: got %q/%q, want %q/%q", transition.step, updated.ID, updated.Status, key.ID, transition.status)
		}
	}

	if err := s.issueServicePrincipalToken(ctx, privateKey, key.Kid); err != nil {
		return err
	}

	if _, err := s.call(ctx, "delete-key", "iam-api", "service-principal", "delete-key",
		s.servicePrincipal.id, key.ID); err != nil {
		return err
	}
	data, err = s.call(ctx, "list-keys-after-delete", "iam-api", "service-principal", "list-keys", s.servicePrincipal.id,
		"--query", "{items: [.items[] | {id: .id, kid: .kid, status: .status}]}")
	if err != nil {
		return err
	}
	if found, err := checkServicePrincipalKey("list-keys-after-delete", data, key); err != nil {
		return err
	} else if found {
		return fmt.Errorf("list-keys-after-delete: key %s remains after delete", key.ID)
	}
	return nil
}

// issueServicePrincipalToken は、アップロードされた公開鍵の秘密鍵で JWT bearer assertion に署名し、
// access token を発行します。応答にはトークンが含まれるため、
// query はトークンタイプと有効期間のみを投影します。
func (s *scenario) issueServicePrincipalToken(ctx context.Context, privateKey *rsa.PrivateKey, kid string) error {
	now := time.Now()
	audience := servicePrincipalOAuth2Endpoint
	if override, ok := servicePrincipalOAuth2EndpointOverride(); ok {
		audience = override
	}
	claims := jwt.MapClaims{
		"iss": s.servicePrincipal.id,
		"sub": s.servicePrincipal.id,
		"aud": audience,
		"iat": now.Unix(),
		"exp": now.Add(3500 * time.Second).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	assertion, err := token.SignedString(privateKey)
	if err != nil {
		return fmt.Errorf("sign service principal assertion: %w", err)
	}
	assertionFile := filepath.Join(os.TempDir(), "skr-e2e-iam-assertion-"+s.suffix)
	if err := os.WriteFile(assertionFile, []byte(assertion), 0o600); err != nil {
		return fmt.Errorf("write private assertion file: %w", err)
	}
	defer func() {
		if err := os.Remove(assertionFile); err != nil && !os.IsNotExist(err) {
			fmt.Printf("remove private assertion file: %v\n", err)
		}
	}()

	data, err := s.call(ctx, "issue-token", "iam-api", "service-principal", "issue-token",
		"--assertion-file", assertionFile, "--query", "{token_type: .token_type, expires_in: .expires_in}")
	if err != nil {
		return err
	}
	var summary accessTokenSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return fmt.Errorf("issue-token: decode token summary: %w", err)
	}
	if summary.TokenType != "Bearer" {
		return fmt.Errorf("issue-token: token_type = %q, want %q", summary.TokenType, "Bearer")
	}
	if summary.ExpiresIn <= 0 {
		return fmt.Errorf("issue-token: expires_in = %d, want a positive value", summary.ExpiresIn)
	}
	return nil
}

func writePublicKeyFile(key *rsa.PrivateKey, suffix string) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", fmt.Errorf("encode service principal public key: %w", err)
	}
	encoded := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	path := filepath.Join(os.TempDir(), "skr-e2e-iam-sp-public-key-"+suffix+".pem")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return "", fmt.Errorf("write public key file: %w", err)
	}
	return path, nil
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
