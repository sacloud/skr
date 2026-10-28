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

// このファイルは iam-api E2E の SSO シナリオです。
// 生成した IdP 証明書を持つ SSO profile のライフサイクルを確認します。
package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const idpCertificateCommonName = "skr-e2e-iam-idp.example.com"

type ssoProfileResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	SPEntityID  string `json:"sp_entity_id"`
	SPACSURL    string `json:"sp_acs_url"`
	Assigned    *bool  `json:"assigned"`
	Description string `json:"description"`
}

// TestScenarioIAMSSO は生成された IdP 証明書を持つ SSO profile を作成、読み込み、一覧表示、更新、削除します。
// link と unlink は organization 内のすべてのクラウドユーザーのログイン方法を変更するため、
// このシナリオではなく専用の追跡 Issue で対象となります。
func TestScenarioIAMSSO(t *testing.T) {
	h := helper.New(t, "iam-api-sso",
		"テスト用の SSO profile のライフサイクルが正しく動作することを確認する")

	name := resourceNamePrefix + helper.RandomSuffix(t)
	run := &iamRun{
		ssoProfile: iamResource{kind: "sso", name: name + "-sso"},
	}

	// SSO profile のライフサイクルを確認する
	runSSOScenario(t, h, run)
}

// runSSOScenario は生成された IdP 証明書を持つ SSO profile を作成、読み込み、一覧表示、更新、削除します。
// profile は organization 全体のものであるため、
// シナリオ内で削除されます。
// link と unlink は organization 内のすべてのクラウドユーザーのログイン方法を変更するため、
// このシナリオではなく専用の追跡 Issue で対象となります。
func runSSOScenario(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	run.ssoProfile.attempted = true
	certificateFile := writeIdPCertificate(t)

	createFile := writeSSORequest(t, run, "sso-create", run.ssoProfile.name, certificateFile)
	data := h.Call(t, "テスト用のSSO profileを作成する", "iam-api", "sso", "create",
		"--request", "@"+createFile, "--query", "{id: .id, name: .name}")
	id, name, err := decodeNamedResource("create-sso", data)
	require.NoError(t, err, "create-sso: 応答のデコードに失敗しました")
	require.Equal(t, run.ssoProfile.name, name, "create-sso: 作成後の名前がリクエストと一致すること")
	run.ssoProfile.id = id

	data = h.Call(t, "SSO profileを読み込む", "iam-api", "sso", "read", run.ssoProfile.id,
		"--query", "{id: (.id | tostring), name: .name, description: .description, sp_entity_id: .sp_entity_id, sp_acs_url: .sp_acs_url, assigned: .assigned}")
	read, err := decodeSSOProfile("read-sso", data)
	require.NoError(t, err, "read-sso: 応答のデコードに失敗しました")
	require.Equal(t, run.ssoProfile.id, read.ID, "read-sso: 読み込んだprofileのIDが一致すること")
	require.Equal(t, run.ssoProfile.name, read.Name, "read-sso: 読み込んだprofileの名前が一致すること")
	require.NotEmpty(t, read.SPEntityID, "read-sso: sp_entity_idが存在すること")
	require.NotEmpty(t, read.SPACSURL, "read-sso: sp_acs_urlが存在すること")
	require.NotNil(t, read.Assigned, "read-sso: assignedが存在すること")
	require.False(t, *read.Assigned, "read-sso: 作成したprofileがorganizationに割り当てられていないこと")

	data = h.Call(t, "SSO profileの一覧を確認する", "iam-api", "sso", "list", "--query", managedListQuery)
	found, err := checkManagedListed("list-sso", data, run.ssoProfile.id, run.ssoProfile.name)
	require.NoError(t, err, "list-sso: 一覧のデコードに失敗しました")
	require.True(t, found, "list-sso: profile %sが一覧に存在すること", run.ssoProfile.id)

	renamed := run.ssoProfile.name + "-renamed"
	run.ssoProfile.renamedTo = renamed
	updateFile := writeSSORequest(t, run, "sso-update", renamed, certificateFile)
	data = h.Call(t, "SSO profileの名前を更新する", "iam-api", "sso", "update", run.ssoProfile.id,
		"--request", "@"+updateFile, "--query", "{id: .id, name: .name}")
	_, name, err = decodeNamedResource("update-sso", data)
	require.NoError(t, err, "update-sso: 応答のデコードに失敗しました")
	require.Equal(t, renamed, name, "update-sso: 更新後の名前がリクエストと一致すること")
	run.ssoProfile.name = renamed
	run.ssoProfile.renamedTo = ""

	h.Call(t, "SSO profileを削除する", "iam-api", "sso", "delete", run.ssoProfile.id)
	run.ssoProfile.deleted = true
}

func decodeSSOProfile(step string, data []byte) (ssoProfileResponse, error) {
	var profile ssoProfileResponse
	if err := json.Unmarshal(data, &profile); err != nil {
		return ssoProfileResponse{}, fmt.Errorf("%s: decode profile: %w", step, err)
	}
	if profile.ID == "" {
		return ssoProfileResponse{}, fmt.Errorf("%s: profile ID is missing", step)
	}
	return profile, nil
}

// writeIdPCertificate は identity provider の証明書の代わりとなる自己署名の X.509 証明書を生成し、
// テスト専用ディレクトリの保護したファイルに書き込みます。
func writeIdPCertificate(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "IdP証明書の鍵を生成できること")
	template := x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: idpCertificateCommonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err, "IdP証明書を生成できること")
	var encoded bytes.Buffer
	require.NoError(t, pem.Encode(&encoded, &pem.Block{Type: "CERTIFICATE", Bytes: der}), "IdP証明書をエンコードできること")
	path := filepath.Join(t.TempDir(), "skr-e2e-iam-idp-certificate.pem")
	require.NoError(t, os.WriteFile(path, encoded.Bytes(), 0o600), "IdP証明書ファイルの書き込みに失敗しました")
	return path
}

// writeSSORequest は SSO profile の作成または更新用リクエスト JSON を書き込みます。
// 証明書は生成された証明書ファイルから読み込まれます。
func writeSSORequest(t *testing.T, run *iamRun, purpose, name, certificateFile string) string {
	t.Helper()
	certificate, err := os.ReadFile(certificateFile) //nolint:gosec // テスト専用ディレクトリ内のファイルを読み込む。
	require.NoError(t, err, "IdP証明書ファイルを読み込めること")
	request := map[string]string{
		"name":            name,
		"description":     run.ssoProfile.name,
		"idp_entity_id":   "https://" + idpCertificateCommonName + "/saml/metadata",
		"idp_login_url":   "https://" + idpCertificateCommonName + "/saml/sso",
		"idp_logout_url":  "https://" + idpCertificateCommonName + "/saml/slo",
		"idp_certificate": string(certificate),
	}
	encoded, err := json.Marshal(request)
	require.NoError(t, err, "SSOの%sリクエストをエンコードできること", purpose)
	path := filepath.Join(t.TempDir(), "skr-e2e-iam-"+purpose+".json")
	require.NoError(t, os.WriteFile(path, encoded, 0o600), "SSOの%sリクエストファイルの書き込みに失敗しました", purpose)
	return path
}
