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
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// kmsKeyItem は kms-api key コマンドの参照結果です。
// KMS API の応答フィールドは PascalCase です。
type kmsKeyItem struct {
	ID            string `json:"ID"`
	Name          string `json:"Name"`
	Status        string `json:"Status"`
	KeyOrigin     string `json:"KeyOrigin"`
	ServiceClass  string `json:"ServiceClass"`
	LatestVersion int    `json:"LatestVersion"`
}

// createKmsKey は skr CLI で KMS キーを作成し、必要な項目に絞って返します。
func createKmsKey(t *testing.T, h *helper.Helper, name, description string) kmsKeyItem {
	t.Helper()

	data := h.Call(t, "KMS キーを作成する",
		"kms-api", "key", "create",
		"--name", name, "--description", description,
		"--query", "{ID,Name,Status,KeyOrigin,ServiceClass,LatestVersion}",
	)
	var item kmsKeyItem
	require.NoError(t, json.Unmarshal(data, &item), "decode kms-api key create response")
	return item
}

// createKmsKeyWithPlainKeyFile は skr CLI で BYOK モードの KMS キーを作成し、必要な項目に絞って返します。
func createKmsKeyWithPlainKeyFile(t *testing.T, h *helper.Helper, name, plainKeyFile string) kmsKeyItem {
	t.Helper()

	data := h.Call(t, "BYOK モードの KMS キーを作成する",
		"kms-api", "key", "create",
		"--name", name, "--plain-key-file", plainKeyFile,
		"--query", "{ID,Name,Status,KeyOrigin,ServiceClass,LatestVersion}",
	)
	var item kmsKeyItem
	require.NoError(t, json.Unmarshal(data, &item), "decode kms-api key create response")
	return item
}

// readKmsKey は skr CLI で指定した KMS キーを参照します。
func readKmsKey(t *testing.T, h *helper.Helper, id string) kmsKeyItem {
	t.Helper()

	data := h.Call(t, "KMS キーを参照する",
		"kms-api", "key", "read", id,
		"--query", "{ID,Name,Status,KeyOrigin,ServiceClass,LatestVersion}",
	)
	var item kmsKeyItem
	require.NoError(t, json.Unmarshal(data, &item), "decode kms-api key read response")
	return item
}

// listKmsKeys は skr CLI で KMS キーを一覧します。
func listKmsKeys(t *testing.T, h *helper.Helper, description string) []kmsKeyItem {
	t.Helper()

	data := h.Call(t, description,
		"kms-api", "key", "list",
		"--query", "map({ID,Name,Status,KeyOrigin,ServiceClass,LatestVersion})",
	)
	var items []kmsKeyItem
	require.NoError(t, json.Unmarshal(data, &items), "decode kms-api key list response")
	return items
}

// updateKmsKey は skr CLI で KMS キーの名前と説明を更新します。
func updateKmsKey(t *testing.T, h *helper.Helper, id, name, description string) kmsKeyItem {
	t.Helper()

	data := h.Call(t, "KMS キーの名前と説明を更新する",
		"kms-api", "key", "update", id,
		"--name", name, "--description", description,
		"--query", "{ID,Name,Status,KeyOrigin,ServiceClass,LatestVersion}",
	)
	var item kmsKeyItem
	require.NoError(t, json.Unmarshal(data, &item), "decode kms-api key update response")
	return item
}

// rotateKmsKey は skr CLI で KMS キーをローテーションします。
func rotateKmsKey(t *testing.T, h *helper.Helper, id string) kmsKeyItem {
	t.Helper()

	data := h.Call(t, "KMS キーをローテーションする",
		"kms-api", "key", "rotate", id,
		"--query", "{ID,Name,Status,KeyOrigin,ServiceClass,LatestVersion}",
	)
	var item kmsKeyItem
	require.NoError(t, json.Unmarshal(data, &item), "decode kms-api key rotate response")
	return item
}

// changeKmsKeyStatus は skr CLI で KMS キーの状態を変更し、API の応答文字列を返します。
func changeKmsKeyStatus(t *testing.T, h *helper.Helper, id, status string) string {
	t.Helper()

	data := h.Call(t, "KMS キーの状態を変更する",
		"kms-api", "key", "change-status", id, "--status", status,
	)
	var changed string
	require.NoError(t, json.Unmarshal(data, &changed), "decode kms-api key change-status response")
	return changed
}

// scheduleKmsKeyDestruction は skr CLI で KMS キーの削除を予約し、応答のステータスと削除予定日時を返します。
func scheduleKmsKeyDestruction(t *testing.T, h *helper.Helper, id string, pendingDays int) (string, string) {
	t.Helper()

	data := h.Call(t, "KMS キーの削除を予約する",
		"kms-api", "key", "schedule-destruction", id,
		"--pending-days", "7",
	)
	var scheduled struct {
		Status                 string `json:"Status"`
		DeletionScheduledAfter string `json:"DeletionScheduledAfter"`
	}
	require.NoError(t, json.Unmarshal(data, &scheduled), "decode kms-api key schedule-destruction response")
	return scheduled.Status, scheduled.DeletionScheduledAfter
}

// encryptKmsKey は skr CLI で KMS キーを使って平文ファイルを暗号化し、暗号文を返します。
func encryptKmsKey(t *testing.T, h *helper.Helper, id, plainFile string) string {
	t.Helper()

	data := h.Call(t, "KMS キーで平文を暗号化する",
		"kms-api", "key", "encrypt", id, "--plain-file", plainFile,
	)
	var cipher string
	require.NoError(t, json.Unmarshal(data, &cipher), "decode kms-api key encrypt response")
	return cipher
}

// decryptKmsKey は skr CLI で KMS キーを使って暗号文ファイルを復号し、平文を返します。
func decryptKmsKey(t *testing.T, h *helper.Helper, id, cipherFile string) string {
	t.Helper()

	data := h.Call(t, "KMS キーで暗号文を復号する",
		"kms-api", "key", "decrypt", id, "--cipher-file", cipherFile,
	)
	return string(data)
}

// deleteKmsKey は skr CLI で指定した KMS キーを削除します。
func deleteKmsKey(t *testing.T, h *helper.Helper, id string) {
	t.Helper()

	h.Call(t, "KMS キーを削除する", "kms-api", "key", "delete", id)
}

// writePlainFile は暗号化する平文を一時ファイルに書き込みます。
func writePlainFile(t *testing.T, plain string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "plain.txt")
	require.NoError(t, os.WriteFile(path, []byte(plain), 0o600), "write plain file")
	return path
}

// writeCipherFile は encrypt の JSON 出力を暗号文ファイルとして書き込みます。
func writeCipherFile(t *testing.T, cipherJSON []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "cipher.txt")
	require.NoError(t, os.WriteFile(path, cipherJSON, 0o600), "write cipher file")
	return path
}

// writeKmsPlainKeyFile は BYOK モードでインポートする秘密鍵を一時ファイルに書き込みます。
// プレーン鍵は 256 bit 長の対称鍵を base64 エンコードした形式が必要です。
func writeKmsPlainKeyFile(t *testing.T) string {
	t.Helper()

	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err, "generate 256 bit key")
	path := filepath.Join(t.TempDir(), "plainkey.txt")
	require.NoError(t, os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(key)+"\n"), 0o600), "write plain key file")
	return path
}
