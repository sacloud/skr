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
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	iconSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/icon"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// TestScenarioIcon は icon コマンドのライブ E2E です。ユーザー作成アイコンの作成、
// 参照、名前とタグの更新、更新後の名前での検索、削除と不在確認まで行います。
// テスト対象の操作は skr CLI で実行し、事前確認と後片付けの確認は SDK で静かに実行します。
// 途中の失敗でも後片付けは実行され、その失敗はテストの結果に含まれます。
func TestScenarioIcon(t *testing.T) {
	h := helper.New(t, "icon-api", "IaaS アイコンを作成・検索・参照・更新・削除し、リソースのライフサイクルが正しく動作することを確認する")

	// 事前確認と後片付けの確認は SDK で静かに実行します。skr CLI と同じ認証設定 (プロファイル) を使います
	client, err := iaasclient.New(false)
	require.NoError(t, err, "create IaaS API client for verification")
	icons := iconSDK.New(client)

	// テスト用のアイコン画像を作成します。画像データは引数に渡さず、--image-file のパスとして渡します
	imagePath := createIconImage(t)

	name := iconPrefix + helper.RandomSuffix(t)
	updatedName := name + "-updated"

	// 作成前に同名の Icon が存在しないことを SDK で確認する。
	// 確認の実行は証跡に残らず、後片付けはまだ登録せず、既存リソースを変更しないまま終了する。
	requireNoIconNameCollision(t, icons, name)
	requireNoIconNameCollision(t, icons, updatedName)

	// アイコンを作成する
	data := h.Call(t, "テスト用のアイコンを作成する",
		"iaas-api", "icon", "create",
		"--name", name, "--image-file", imagePath,
		"--tags", "skr-e2e,temporary",
	)
	created, err := parseIcon(data)
	require.NoError(t, err)
	require.NoError(t, checkIconIdentity(created, created.ID, name, []string{"skr-e2e", "temporary"}),
		"the create response should contain the created Icon")

	// 後片付け: アイコンを識別情報の再照合つきで削除する。
	// 更新フェーズで名前が復元されていない場合は更新後の名前も許容する。
	var iconID types.ID
	defer func() {
		if err := cleanupIcon(t, h, icons, name, iconID); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()
	iconID = created.ID

	// 作成したアイコンを参照する
	read := readIcon(t, h, "作成したアイコンを参照する", created.ID)
	require.NoError(t, checkIconIdentity(read, created.ID, name, []string{"skr-e2e", "temporary"}),
		"icon read should return the created Icon")

	// 名前とタグを更新する
	data = h.Call(t, "アイコンの名前とタグを更新する",
		"iaas-api", "icon", "update",
		"--id", created.ID.String(),
		"--name", updatedName, "--tags", "skr-e2e,updated",
	)
	updated, err := parseIcon(data)
	require.NoError(t, err)
	require.NoError(t, checkIconIdentity(updated, created.ID, updatedName, []string{"skr-e2e", "updated"}),
		"the update should be reflected in the response")

	// 更新後のアイコンを参照する
	read = readIcon(t, h, "更新後のアイコンを参照する", created.ID)
	require.NoError(t, checkIconIdentity(read, created.ID, updatedName, []string{"skr-e2e", "updated"}),
		"icon read should return the updated Icon")

	// 更新後の名前で検索する。Names はフラグで指定できないため --request で JSON を渡す
	found := findIcons(t, h, "更新後の名前でアイコンを検索する", updatedName)
	require.Len(t, found, 1, "icon find should return exactly the updated Icon")
	require.NoError(t, checkIconIdentity(found[0], created.ID, updatedName, []string{"skr-e2e", "updated"}),
		"icon find should return the updated Icon")
}

// iconFindRequestJSON は名前検索のリクエスト JSON を組み立てます。
// Names はフラグで指定できないため、JSON 経路で渡します。
func iconFindRequestJSON(t *testing.T, name string) string {
	t.Helper()

	data, err := json.Marshal(map[string]any{"Names": []string{name}})
	require.NoError(t, err, "encode icon find request")
	return string(data)
}

// findIcons は skr CLI で Icon を名前で検索し、応答を解析して返します。
func findIcons(t *testing.T, h *helper.Helper, description, name string) []iconItem {
	t.Helper()

	data := h.Call(t, description,
		"iaas-api", "icon", "find", "--request", iconFindRequestJSON(t, name))
	items, err := decodeIconList(data)
	require.NoError(t, err)
	return items
}

// readIcon は skr CLI で Icon を参照し、応答を解析して返します。
func readIcon(t *testing.T, h *helper.Helper, description string, id types.ID) iconItem {
	t.Helper()

	data := h.Call(t, description,
		"iaas-api", "icon", "read", "--id", id.String())
	item, err := parseIcon(data)
	require.NoError(t, err)
	return item
}

// cleanupIcon は後片付けです。作成に成功した Icon の ID を優先して SDK で再照合し、
// ID がない場合は名前検索で回収を試みます。識別できない場合は削除しません。
// 削除は skr CLI で行い、削除後は SDK で不在を確認します。
// 更新確認は名前を元に戻さないため、更新後の名前も許容します。
func cleanupIcon(t *testing.T, h *helper.Helper, icons *iconSDK.Service, name string, knownID types.ID) error {
	t.Helper()

	updatedName := name + "-updated"

	id := knownID
	if id.IsEmpty() {
		matches, err := lookupIcons(t, icons, name)
		if err != nil {
			return fmt.Errorf("cannot locate possibly created Icon: %w", err)
		}
		if len(matches) == 0 {
			return nil
		}
		if len(matches) != 1 || matches[0].ID.IsEmpty() ||
			(matches[0].Name != name && matches[0].Name != updatedName) {
			return errors.New("cannot safely identify created Icon; manual cleanup required")
		}
		id = matches[0].ID
	}

	// 削除前に SDK で再照合し、保持した ID と許容する名前が一致する場合だけ削除する
	current, err := icons.ReadWithContext(t.Context(), &iconSDK.ReadRequest{ID: id})
	if err != nil {
		return fmt.Errorf("cannot verify Icon before cleanup: %w", err)
	}
	if current.ID != id || (current.Name != name && current.Name != updatedName) {
		return errors.New("cleanup refused: Icon identity changed; manual cleanup required")
	}
	if _, _, err := h.CallAllowingFailure(t, "テスト用のアイコンを削除する",
		"iaas-api", "icon", "delete", "--id", id.String(), "--fail-if-not-found",
	); err != nil {
		return fmt.Errorf("delete test Icon %s: %w", id, err)
	}
	for _, candidate := range []string{name, updatedName} {
		matches, err := lookupIcons(t, icons, candidate)
		if err != nil {
			return fmt.Errorf("verify Icon absence after cleanup: %w", err)
		}
		if len(matches) != 0 {
			return fmt.Errorf("Icon %q remains after cleanup; manual cleanup required", candidate)
		}
	}
	return nil
}
