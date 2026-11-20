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
	"slices"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/stretchr/testify/require"
)

// nfsPlanEnvelope は Note (sys-nfs) の Contents の JSON 構造のうち、
// プラン解決に使う項目です。
type nfsPlanEnvelope struct {
	Plans struct {
		SSD []nfsPlanValue `json:"SSD"`
	} `json:"plans"`
}

// nfsPlanValue は SSD プランの1要素です。
type nfsPlanValue struct {
	Size         int                 `json:"size"`
	Availability types.EAvailability `json:"availability"`
	PlanID       types.ID            `json:"planId"`
}

// smallestAvailablePlan は利用可能な SSD プランのうち最小のサイズを選び、
// プラン ID とサイズを返します。利用可能なプランがない場合はゼロ値を返します。
func smallestAvailablePlan(values []nfsPlanValue) (types.ID, int) {
	var planID types.ID
	var size int
	for _, value := range values {
		if !value.Availability.IsAvailable() {
			continue
		}
		if size == 0 || value.Size < size {
			planID = value.PlanID
			size = value.Size
		}
	}
	return planID, size
}

// resolveNFSPlan は Note (sys-nfs) のプラン一覧から、テストで使う SSD プランの
// 最小サイズのプラン ID を解決します。利用可能なサイズをハードコードしないため、
// ゾーンの Note の内容に合わせて最小のプランを選びます。
// リソースを変更しない安全確認のため SDK 直呼びとします (背景は ADR 0064)。
func resolveNFSPlan(t *testing.T, client *iaas.Client, zone string) (types.ID, int) {
	t.Helper()

	noteOp := iaas.NewNoteOp(client)
	result, err := noteOp.Find(t.Context(), &iaas.FindCondition{
		Filter: map[search.FilterKey]any{
			search.Key("Name"):  "sys-nfs",
			search.Key("Class"): "json",
		},
	})
	require.NoError(t, err, "find note[sys-nfs]")

	// Note はゾーンごとに用意されるため、Tags に @zone=<ゾーン> を含むものを選ぶ
	var note *iaas.Note
	for _, candidate := range result.Notes {
		if candidate != nil && slices.Contains(candidate.Tags, "@zone="+zone) {
			note = candidate
			break
		}
	}
	require.NotNil(t, note, "note[sys-nfs] for zone %s not found", zone)

	var envelope nfsPlanEnvelope
	require.NoError(t, json.Unmarshal([]byte(note.Content), &envelope), "decode note[sys-nfs] content")
	planID, size := smallestAvailablePlan(envelope.Plans.SSD)
	require.NotZero(t, planID, "no available SSD plan for zone %s", zone)
	return planID, size
}

func TestSmallestAvailablePlan(t *testing.T) {
	t.Run("picks the smallest available plan", func(t *testing.T) {
		values := []nfsPlanValue{
			{Size: 100, Availability: types.Availabilities.Available, PlanID: 100001},
			{Size: 20, Availability: types.Availabilities.Available, PlanID: 100000},
			{Size: 500, Availability: types.Availabilities.Available, PlanID: 100003},
		}
		planID, size := smallestAvailablePlan(values)
		require.EqualValues(t, 100000, planID)
		require.Equal(t, 20, size)
	})

	t.Run("ignores unavailable plans", func(t *testing.T) {
		values := []nfsPlanValue{
			{Size: 20, Availability: types.Availabilities.Failed, PlanID: 100000},
			{Size: 100, Availability: types.Availabilities.Available, PlanID: 100001},
		}
		planID, size := smallestAvailablePlan(values)
		require.EqualValues(t, 100001, planID)
		require.Equal(t, 100, size)
	})

	t.Run("returns zero when no plan is available", func(t *testing.T) {
		values := []nfsPlanValue{
			{Size: 20, Availability: types.Availabilities.Failed, PlanID: 100000},
		}
		planID, size := smallestAvailablePlan(values)
		require.Zero(t, planID)
		require.Zero(t, size)
	})

	t.Run("returns zero for empty plans", func(t *testing.T) {
		planID, size := smallestAvailablePlan(nil)
		require.Zero(t, planID)
		require.Zero(t, size)
	})
}
