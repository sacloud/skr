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
	"os/exec"
	"testing"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// certificateMaterial は証明書の設定確認に使う自己署名証明書と秘密鍵のパスです。
type certificateMaterial struct {
	// CertPath は自己署名証明書 (サーバー証明書) の PEM のパスです。
	CertPath string
	// KeyPath は証明書の秘密鍵の PEM のパスです。
	KeyPath string
}

// generateSelfSignedCertificate は openssl で自己署名証明書と秘密鍵を生成します。
// 秘密鍵の内容は引数に含めず、所有者のみ読める一時ファイルに保存します。
// 実行自体は実行記録に残りますが、鍵の内容は残りません。
func generateSelfSignedCertificate(t *testing.T, h *helper.Helper, commonName string) certificateMaterial {
	t.Helper()

	if _, err := exec.LookPath("openssl"); err != nil {
		require.NoError(t, err, "openssl is required for the certificate live E2E")
	}

	dir := t.TempDir()
	material := certificateMaterial{
		CertPath: dir + "/server.crt",
		KeyPath:  dir + "/server.key",
	}
	_, _, err := h.RunLocalCommand(t, time.Minute, "openssl", "自己署名証明書と秘密鍵を生成する",
		"req", "-x509", "-newkey", "rsa:2048", "-sha256", "-days", "1", "-nodes",
		"-keyout", material.KeyPath, "-out", material.CertPath,
		"-subj", "/CN="+commonName,
	)
	require.NoError(t, err, "generate self-signed certificate")
	return material
}
