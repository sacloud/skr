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
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	iconSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/icon"
	"github.com/stretchr/testify/require"
)

const (
	// iconPrefix はテスト用アイコンの名前の接頭辞です。ランダムな接尾辞とともに使い、
	// 過去の実行のリソースと衝突しないようにします。
	iconPrefix = "skr-e2e-icon-"
	// createdTags はテスト用アイコンに付ける作成時のタグです。後片付けの識別にも使います。
	createdTags = "skr-e2e,temporary"
	// updatedTags は更新確認中に使用する一時的なタグです。後片付けはこのタグも許容します。
	updatedTags = "skr-e2e,updated"
)

// createdTagList は作成時に指定するタグの期待値です。
func createdTagList() []string {
	return []string{"skr-e2e", "temporary"}
}

// updatedTagList は更新時に指定するタグの期待値です。
func updatedTagList() []string {
	return []string{"skr-e2e", "updated"}
}

// updatedIconName は更新確認中に使用する一時的な名前です。
func updatedIconName(name string) string {
	return name + "-updated"
}

// iconItem は Icon の参照結果のうち、確認に使う項目です。
type iconItem struct {
	ID   types.ID
	Name string
	Tags []string
}

// parseIcon は Icon の参照応答を解析します。ID が空の場合はエラーを返します。
func parseIcon(data []byte) (iconItem, error) {
	var item iconItem
	if err := json.Unmarshal(data, &item); err != nil {
		return iconItem{}, fmt.Errorf("decode Icon: %w", err)
	}
	if item.ID.IsEmpty() {
		return iconItem{}, errors.New("Icon ID is empty")
	}
	return item, nil
}

// decodeIconList は Icon の一括検索の応答を解析します。
func decodeIconList(data []byte) ([]iconItem, error) {
	var items []iconItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("decode Icon list: %w", err)
	}
	if items == nil {
		return nil, errors.New("expected a JSON array of Icons")
	}
	return items, nil
}

// sameTags はタグの並び順を無視して一致することを返します。
func sameTags(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	remaining := make(map[string]int, len(expected))
	for _, tag := range expected {
		remaining[tag]++
	}
	for _, tag := range actual {
		if remaining[tag] == 0 {
			return false
		}
		remaining[tag]--
	}
	return true
}

// checkIconIdentity は参照結果が期待する Icon (ID・名前・タグ) と一致することを確認します。
// 作成時と更新後のどちらの識別情報も、期待値を渡して同じ関数で確認します。
func checkIconIdentity(item iconItem, id types.ID, name string, tags []string) error {
	if item.ID != id || item.Name != name || !sameTags(item.Tags, tags) {
		return fmt.Errorf("Icon identity mismatch for ID %s (expected name %q)", id, name)
	}
	return nil
}

// createIconImage はテスト用のアイコン画像 (PNG) を一時ディレクトリに作成します。
// 画像データは引数に渡さず、--image-file のパスとして CLI に渡します。
func createIconImage(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "icon.png")
	imageData := image.NewRGBA(image.Rect(0, 0, 60, 56))
	imageData.Set(10, 10, color.RGBA{R: 255, A: 255})
	file, err := os.Create(path) //nolint:gosec // パスは t.TempDir() から組み立てます
	require.NoError(t, err, "create temporary icon image")
	require.NoError(t, png.Encode(file, imageData), "encode temporary icon image")
	require.NoError(t, file.Close(), "close temporary icon image")
	return path
}

// requireNoIconNameCollision は作成前に同名の Icon が存在しないことを SDK で確認します。
// 確認の実行は証跡に残しません。既存リソースは変更しません。
func requireNoIconNameCollision(t *testing.T, icons *iconSDK.Service, name string) {
	t.Helper()

	matches, err := lookupIcons(t, icons, name)
	require.NoError(t, err, "find existing Icons")
	require.Empty(t, matches, "test Icon name %q already exists; refusing to modify it", name)
}

// lookupIcons は SDK で名前の完全一致検索を行い、結果を返します。
// 事前確認と後片付けの確認など、証跡に残さない安全確認で使います。
func lookupIcons(t *testing.T, icons *iconSDK.Service, name string) ([]iconItem, error) {
	t.Helper()

	found, err := icons.FindWithContext(t.Context(), &iconSDK.FindRequest{Names: []string{name}})
	if err != nil {
		return nil, fmt.Errorf("find Icons by name: %w", err)
	}
	items := make([]iconItem, 0, len(found))
	for _, icon := range found {
		if icon == nil {
			continue
		}
		items = append(items, iconItem{ID: icon.ID, Name: icon.Name, Tags: []string(icon.Tags)})
	}
	return items, nil
}

// TestParseIcon は Icon の参照応答の解析を確かめる。
func TestParseIcon(t *testing.T) {
	item, err := parseIcon([]byte(`{"ID":1234,"Name":"skr-e2e-icon-test","Tags":["skr-e2e","temporary"]}`))
	require.NoError(t, err)
	require.Equal(t, iconItem{ID: 1234, Name: "skr-e2e-icon-test", Tags: []string{"skr-e2e", "temporary"}}, item)

	for name, output := range map[string]string{
		"not a JSON object": `[`,
		"missing ID":        `{"Name":"skr-e2e-icon-test"}`,
		"empty ID":          `{"ID":0,"Name":"skr-e2e-icon-test"}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseIcon([]byte(output))
			require.Error(t, err, "an invalid Icon output should be refused")
		})
	}
}

// TestDecodeIconList は Icon の一括検索の応答の解析を確かめる。
func TestDecodeIconList(t *testing.T) {
	items, err := decodeIconList([]byte(`[{"ID":1234,"Name":"skr-e2e-icon-test","Tags":["skr-e2e"]}]`))
	require.NoError(t, err)
	require.Equal(t, []iconItem{{ID: 1234, Name: "skr-e2e-icon-test", Tags: []string{"skr-e2e"}}}, items)

	items, err = decodeIconList([]byte(`[]`))
	require.NoError(t, err)
	require.Empty(t, items, "an empty JSON array should be accepted")

	for name, output := range map[string]string{
		"not a JSON array": `{`,
		"JSON object":      `{"ID":1234}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := decodeIconList([]byte(output))
			require.Error(t, err, "a non-array output should be refused")
		})
	}
}

// TestSameTags はタグの並び順を無視した一致の確認を確かめる。
func TestSameTags(t *testing.T) {
	require.True(t, sameTags([]string{"skr-e2e", "temporary"}, []string{"skr-e2e", "temporary"}))
	require.True(t, sameTags([]string{"temporary", "skr-e2e"}, []string{"skr-e2e", "temporary"}),
		"tag order should not matter")
	require.True(t, sameTags(nil, nil))

	for name, tc := range map[string][2][]string{
		"length mismatch": {{}, {"skr-e2e"}},
		"unexpected tag":  {{"skr-e2e", "unexpected"}, {"skr-e2e", "temporary"}},
		"duplicate tag":   {{"skr-e2e", "skr-e2e"}, {"skr-e2e", "temporary"}},
		"missing tag":     {{"skr-e2e"}, {"skr-e2e", "temporary"}},
		"actual nil":      {nil, {"skr-e2e"}},
		"expected nil":    {{"skr-e2e"}, nil},
		"same tag twice":  {{"skr-e2e", "temporary"}, {"skr-e2e", "skr-e2e"}},
	} {
		t.Run(name, func(t *testing.T) {
			require.False(t, sameTags(tc[0], tc[1]), "mismatched tags should not be identified")
		})
	}
}

// TestCheckIconIdentity は Icon の識別情報の照合を確かめる。
func TestCheckIconIdentity(t *testing.T) {
	item := iconItem{ID: 1234, Name: "skr-e2e-icon-test", Tags: []string{"skr-e2e", "temporary"}}
	require.NoError(t, checkIconIdentity(item, 1234, "skr-e2e-icon-test", []string{"temporary", "skr-e2e"}))

	for name, tc := range map[string]iconItem{
		"other ID":    {ID: 9876, Name: "skr-e2e-icon-test", Tags: []string{"skr-e2e", "temporary"}},
		"other name":  {ID: 1234, Name: "changed", Tags: []string{"skr-e2e", "temporary"}},
		"other tags":  {ID: 1234, Name: "skr-e2e-icon-test", Tags: []string{"skr-e2e"}},
		"missing tag": {ID: 1234, Name: "skr-e2e-icon-test", Tags: []string{"skr-e2e", "unexpected"}},
	} {
		t.Run(name, func(t *testing.T) {
			require.Error(t, checkIconIdentity(tc, 1234, "skr-e2e-icon-test", []string{"skr-e2e", "temporary"}),
				"mismatched Icons should not be identified")
		})
	}
}

// TestCreateIconImageProducesPNG はテスト用のアイコン画像の生成を確かめる。
func TestCreateIconImageProducesPNG(t *testing.T) {
	path := createIconImage(t)
	require.FileExists(t, path)

	file, err := os.Open(path) //nolint:gosec // createIconImage は一時ディレクトリに作成されたパスを返す
	require.NoError(t, err)
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close generated image file: %v", err)
		}
	}()
	config, format, err := image.DecodeConfig(file)
	require.NoError(t, err)
	require.Equal(t, "png", format)
	require.Equal(t, 60, config.Width)
	require.Equal(t, 56, config.Height)
}
