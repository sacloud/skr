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

// service 層の単体テストです。Note の検索条件、ゾーンタグの照合、Content の
// 解析と並べ替え、available フィルタ、異常系を検証します。コマンド層の検証は
// commands_test.go、CLI 全体の接続と出力は internal/cli のテストで行います。
package nfs

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/stretchr/testify/require"
)

// fakeNoteFinder は Note の検索を記録し、用意した Note を返します。
type fakeNoteFinder struct {
	notes        []*iaas.Note
	err          error
	gotCondition *iaas.FindCondition
}

func (f *fakeNoteFinder) Find(ctx context.Context, conditions *iaas.FindCondition) (*iaas.NoteFindResult, error) {
	f.gotCondition = conditions
	if f.err != nil {
		return nil, f.err
	}
	return &iaas.NoteFindResult{
		Total: len(f.notes),
		Count: len(f.notes),
		Notes: f.notes,
	}, nil
}

// planNote はテスト用の NFS プラン情報 Note を組み立てます。
// content は Note (sys-nfs) と同じ形式の JSON 文字列を受け取ります。
func planNote(id types.ID, zone, content string, availability types.EAvailability) *iaas.Note {
	return &iaas.Note{
		ID:           id,
		Name:         planNoteName,
		Class:        "json",
		Tags:         types.Tags{planZoneTagPrefix + zone},
		Availability: availability,
		Content:      content,
	}
}

// planContent は Note Content の JSON を組み立てます。planId は Note の実データと
// 同じ桁の値を使い、数値の復元を確認します。
func planContent(ssd, hdd string) string {
	return fmt.Sprintf(`{"plans":{"SSD":%s,"HDD":%s}}`, ssd, hdd)
}

func TestListPlans(t *testing.T) {
	content := planContent(
		`[{"size":100,"availability":"available","planId":1001014001,"cloneable":true},{"size":20,"availability":"available","planId":20,"cloneable":true}]`,
		`[{"size":2048,"availability":"available","planId":1001012020,"cloneable":true},{"size":100,"availability":"available","planId":1001012001,"cloneable":true},{"size":100,"availability":"disabled","planId":100,"cloneable":true}]`,
	)
	finder := &fakeNoteFinder{notes: []*iaas.Note{
		planNote(113100243383, "tk1a", content, types.Availabilities.Available),
		planNote(113201250475, "is1a", `{"plans":{"SSD":[{"size":20,"availability":"available","planId":20,"cloneable":true}]}}`, types.Availabilities.Available),
	}}
	svc := NewService(finder)

	t.Run("available のみ", func(t *testing.T) {
		plans, err := svc.ListPlans(t.Context(), "tk1a", false)
		require.NoError(t, err)
		require.Equal(t, []PlanOutput{
			{Storage: "SSD", Size: 20, PlanID: 20, Availability: "available", Cloneable: true},
			{Storage: "SSD", Size: 100, PlanID: 1001014001, Availability: "available", Cloneable: true},
			{Storage: "HDD", Size: 100, PlanID: 1001012001, Availability: "available", Cloneable: true},
			{Storage: "HDD", Size: 2048, PlanID: 1001012020, Availability: "available", Cloneable: true},
		}, plans)
	})

	t.Run("--all は disabled も含む", func(t *testing.T) {
		plans, err := svc.ListPlans(t.Context(), "tk1a", true)
		require.NoError(t, err)
		require.Equal(t, []PlanOutput{
			{Storage: "SSD", Size: 20, PlanID: 20, Availability: "available", Cloneable: true},
			{Storage: "SSD", Size: 100, PlanID: 1001014001, Availability: "available", Cloneable: true},
			{Storage: "HDD", Size: 100, PlanID: 1001012001, Availability: "available", Cloneable: true},
			{Storage: "HDD", Size: 100, PlanID: 100, Availability: "disabled", Cloneable: true},
			{Storage: "HDD", Size: 2048, PlanID: 1001012020, Availability: "available", Cloneable: true},
		}, plans)
	})

	t.Run("検索条件", func(t *testing.T) {
		_, err := svc.ListPlans(t.Context(), "tk1a", false)
		require.NoError(t, err)
		require.NotNil(t, finder.gotCondition)
		require.Equal(t, noteFindCount, finder.gotCondition.Count)
		require.Equal(t, search.Filter{
			search.Key("Name"):  planNoteName,
			search.Key("Class"): "json",
		}, finder.gotCondition.Filter)
	})
}

func TestListPlansAvailableOnly(t *testing.T) {
	// available のプランが 1 件もないゾーンでは、フィルタ結果の空配列を返します
	// (Note 自体にプランがない場合はエラーになります)。
	content := planContent(
		`[{"size":20,"availability":"disabled","planId":20,"cloneable":true}]`,
		`[{"size":100,"availability":"disabled","planId":100,"cloneable":true}]`,
	)
	svc := NewService(&fakeNoteFinder{notes: []*iaas.Note{
		planNote(113100243383, "tk1a", content, types.Availabilities.Available),
	}})
	plans, err := svc.ListPlans(t.Context(), "tk1a", false)
	require.NoError(t, err)
	require.Empty(t, plans)
}

func TestListPlanErrors(t *testing.T) {
	availableNote := planNote(113100243383, "tk1a",
		planContent(`[{"size":20,"availability":"available","planId":20,"cloneable":true}]`, `[]`),
		types.Availabilities.Available)

	t.Run("ゾーン未指定", func(t *testing.T) {
		svc := NewService(&fakeNoteFinder{notes: []*iaas.Note{availableNote}})
		_, err := svc.ListPlans(t.Context(), "", false)
		require.Error(t, err)
		require.Contains(t, err.Error(), "ゾーンを指定してください")
	})

	t.Run("Note が見つからない", func(t *testing.T) {
		svc := NewService(&fakeNoteFinder{notes: nil})
		_, err := svc.ListPlans(t.Context(), "tk1a", false)
		require.Error(t, err)
		require.Contains(t, err.Error(), `ゾーン tk1a の NFS プラン情報が見つかりません`)
		require.Contains(t, err.Error(), "@zone=tk1a")
	})

	t.Run("ゾーンタグが一致する Note が複数", func(t *testing.T) {
		svc := NewService(&fakeNoteFinder{notes: []*iaas.Note{availableNote, availableNote}})
		_, err := svc.ListPlans(t.Context(), "tk1a", false)
		require.Error(t, err)
		require.Contains(t, err.Error(), "複数見つかりました")
	})

	t.Run("Note 検索の失敗", func(t *testing.T) {
		svc := NewService(&fakeNoteFinder{err: errors.New("検索エラー")})
		_, err := svc.ListPlans(t.Context(), "tk1a", false)
		require.Error(t, err)
		require.Contains(t, err.Error(), "検索エラー")
	})

	t.Run("Note が利用可能な状態でない", func(t *testing.T) {
		svc := NewService(&fakeNoteFinder{notes: []*iaas.Note{
			planNote(113100243383, "tk1a", planContent(`[]`, `[]`), types.Availabilities.Discontinued),
		}})
		_, err := svc.ListPlans(t.Context(), "tk1a", false)
		require.Error(t, err)
		require.Contains(t, err.Error(), "利用可能な状態ではありません")
	})

	t.Run("Content が不正な JSON", func(t *testing.T) {
		svc := NewService(&fakeNoteFinder{notes: []*iaas.Note{
			planNote(113100243383, "tk1a", `{invalid`, types.Availabilities.Available),
		}})
		_, err := svc.ListPlans(t.Context(), "tk1a", false)
		require.Error(t, err)
		require.Contains(t, err.Error(), "解釈できませんでした")
	})

	t.Run("Content にプランがない", func(t *testing.T) {
		svc := NewService(&fakeNoteFinder{notes: []*iaas.Note{
			planNote(113100243383, "tk1a", `{"plans":{"SSD":[],"HDD":[]}}`, types.Availabilities.Available),
		}})
		_, err := svc.ListPlans(t.Context(), "tk1a", true)
		require.Error(t, err)
		require.Contains(t, err.Error(), "プランが含まれていません")
	})

	t.Run("factory なし", func(t *testing.T) {
		var svc *Service
		_, err := svc.ListPlans(t.Context(), "tk1a", false)
		require.Error(t, err)
		require.Contains(t, err.Error(), "API クライアントが設定されていません")
	})
}
