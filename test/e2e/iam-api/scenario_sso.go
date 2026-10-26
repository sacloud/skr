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
	"bytes"
	"context"
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
	"time"
)

const idpCertificateCommonName = "skr-e2e-iam-idp.example.com"

type ssoProfile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	SPEntityID  string `json:"sp_entity_id"`
	SPACSURL    string `json:"sp_acs_url"`
	Assigned    *bool  `json:"assigned"`
	Description string `json:"description"`
}

// ssoScenario は生成された IdP 証明書を持つ SSO profile を作成、読み込み、一覧表示、更新、削除します。
// profile は organization 全体のものであるため、
// シナリオ内で削除されます。
// link と unlink は organization 内のすべてのクラウドユーザーのログイン方法を変更するため、
// この runner ではなく専用の追跡 Issue で対象となります。
// この runner の対象とする。
func (s *scenario) ssoScenario(ctx context.Context) error {
	s.ssoProfile.attempted = true
	s.ssoProfile.currentName = s.ssoProfile.name
	certificateFile, removeCertificate, err := s.writeIdPCertificate()
	if err != nil {
		return err
	}
	defer removeCertificate()

	createFile, err := s.writeSSORequest("sso-create", s.ssoProfile.name, certificateFile)
	if err != nil {
		return err
	}
	defer func() {
		if err := os.Remove(createFile); err != nil && !os.IsNotExist(err) {
			fmt.Printf("remove private %s request file: %v\n", "sso", err)
		}
	}()
	data, err := s.call(ctx, "create-sso", "iam-api", "sso", "create",
		"--request", "@"+createFile, "--query", "{id: .id, name: .name}")
	if err != nil {
		return err
	}
	id, name, err := decodeNamedResource("create-sso", data)
	if err != nil {
		return err
	}
	if name != s.ssoProfile.name {
		return fmt.Errorf("create-sso: name = %q, want %q", name, s.ssoProfile.name)
	}
	s.ssoProfile.id = id

	data, err = s.call(ctx, "read-sso", "iam-api", "sso", "read", s.ssoProfile.id,
		"--query", "{id: (.id | tostring), name: .name, description: .description, sp_entity_id: .sp_entity_id, sp_acs_url: .sp_acs_url, assigned: .assigned}")
	if err != nil {
		return err
	}
	read, err := decodeSSOProfile("read-sso", data)
	if err != nil {
		return err
	}
	if read.ID != s.ssoProfile.id || read.Name != s.ssoProfile.currentName {
		return fmt.Errorf("read-sso: got %q/%q, want %q/%q", read.ID, read.Name, s.ssoProfile.id, s.ssoProfile.currentName)
	}
	if read.SPEntityID == "" || read.SPACSURL == "" {
		return fmt.Errorf("read-sso: sp_entity_id and sp_acs_url must be present")
	}
	if read.Assigned == nil || *read.Assigned {
		return fmt.Errorf("read-sso: the created profile must not be assigned to the organization")
	}

	data, err = s.call(ctx, "list-sso", "iam-api", "sso", "list", "--query", managedListQuery)
	if err != nil {
		return err
	}
	found, err := checkManagedListed("list-sso", data, s.ssoProfile.id, s.ssoProfile.currentName)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("list-sso: profile %s is missing from the list", s.ssoProfile.id)
	}

	renamed := s.ssoProfile.name + "-renamed"
	updateFile, err := s.writeSSORequest("sso-update", renamed, certificateFile)
	if err != nil {
		return err
	}
	defer func() {
		if err := os.Remove(updateFile); err != nil && !os.IsNotExist(err) {
			fmt.Printf("remove private %s request file: %v\n", "sso", err)
		}
	}()
	data, err = s.call(ctx, "update-sso", "iam-api", "sso", "update", s.ssoProfile.id,
		"--request", "@"+updateFile, "--query", "{id: .id, name: .name}")
	if err != nil {
		return err
	}
	if _, name, err = decodeNamedResource("update-sso", data); err != nil {
		return err
	}
	if name != renamed {
		return fmt.Errorf("update-sso: name = %q, want %q", name, renamed)
	}
	s.ssoProfile.currentName = renamed

	if _, err := s.call(ctx, "delete-sso", "iam-api", "sso", "delete", s.ssoProfile.id); err != nil {
		return err
	}
	s.ssoProfile.id = ""
	return nil
}

func decodeSSOProfile(step string, data []byte) (ssoProfile, error) {
	var profile ssoProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return ssoProfile{}, fmt.Errorf("%s: decode profile: %w", step, err)
	}
	if profile.ID == "" {
		return ssoProfile{}, fmt.Errorf("%s: profile ID is missing", step)
	}
	return profile, nil
}

// writeIdPCertificate は identity provider の証明書の代わりとなる自己署名の X.509 証明書を生成し、
// プライベートなファイルに書き込み、
// クリーンアップ関数付きでパスを返します。
func (s *scenario) writeIdPCertificate() (string, func(), error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", nil, fmt.Errorf("generate IdP certificate key: %w", err)
	}
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
	if err != nil {
		return "", nil, fmt.Errorf("generate IdP certificate: %w", err)
	}
	var encoded bytes.Buffer
	if err := pem.Encode(&encoded, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		return "", nil, fmt.Errorf("encode IdP certificate: %w", err)
	}
	path := filepath.Join(os.TempDir(), "skr-e2e-iam-idp-certificate-"+s.suffix+".pem")
	if err := os.WriteFile(path, encoded.Bytes(), 0o600); err != nil {
		return "", nil, fmt.Errorf("write IdP certificate file: %w", err)
	}
	return path, func() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			fmt.Printf("remove private IdP certificate file: %v\n", err)
		}
	}, nil
}

// writeSSORequest は SSO profile の作成または更新用リクエスト JSON を書き込みます。
// 証明書は生成された証明書ファイルから読み込まれます。
func (s *scenario) writeSSORequest(purpose, name, certificateFile string) (string, error) {
	certificate, err := os.ReadFile(certificateFile) //nolint:gosec // パスはこの runner によって os.TempDir() 内に作成される。
	if err != nil {
		return "", fmt.Errorf("read IdP certificate file: %w", err)
	}
	request := map[string]string{
		"name":            name,
		"description":     s.ssoProfile.name,
		"idp_entity_id":   "https://" + idpCertificateCommonName + "/saml/metadata",
		"idp_login_url":   "https://" + idpCertificateCommonName + "/saml/sso",
		"idp_logout_url":  "https://" + idpCertificateCommonName + "/saml/slo",
		"idp_certificate": string(certificate),
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encode SSO %s request: %w", purpose, err)
	}
	path := filepath.Join(os.TempDir(), "skr-e2e-iam-"+purpose+"-"+s.suffix+".json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return "", fmt.Errorf("write SSO %s request file: %w", purpose, err)
	}
	return path, nil
}
