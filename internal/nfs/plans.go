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

package nfs

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

// planNoteName は NFS プラン情報が記録された Note の名前です。
// Note はさくらのクラウドがゾーンごとに公開する共有リソースで、Content に
// プランの一覧を JSON で持ちます。
const planNoteName = "sys-nfs"

// planZoneTagPrefix は Note が対応ゾーンを示すために付けるタグの接頭辞です。
const planZoneTagPrefix = "@zone="

// noteFindCount は NFS プラン情報の Note を検索するときの取得件数です。
// Name と Class で絞り込むため、ゾーン数分 (数十件程度) に収まります。
const noteFindCount = 100

// NoteFinder は NFS プラン情報が記録された Note の検索に必要な操作です。
// SDK の iaas.NoteAPI がこの interface を満たします。
type NoteFinder interface {
	Find(ctx context.Context, conditions *iaas.FindCondition) (*iaas.NoteFindResult, error)
}

// Service は NFS の高レベル操作を提供します。
type Service struct {
	notes NoteFinder
}

// NewService は Note 操作を受け取り Service を構築します。
func NewService(notes NoteFinder) *Service {
	return &Service{notes: notes}
}

// PlanOutput はゾーンで利用できる NFS プランの出力です。
type PlanOutput struct {
	// Storage はストレージ種別です。SSD または HDD です。
	Storage string
	// Size はディスク容量です (GB)。
	Size int
	// PlanID は skr nfs-api create の --plan-id に指定するプラン ID です。
	PlanID types.ID
	// Availability は Note に記録されたプランの利用可否です。
	// available または disabled です。
	Availability string
	// Cloneable は Note に記録されたプランの複製可否です。
	Cloneable bool
}

// notePlanContent は Note (sys-nfs) の Content に記録されたプラン情報です。
// Note の Content はさくらのクラウドが管理する JSON で、フィールドは size、
// availability、planId、cloneable です。
type notePlanContent struct {
	Plans notePlanGroups `json:"plans"`
}

type notePlanGroups struct {
	HDD []notePlan `json:"HDD"`
	SSD []notePlan `json:"SSD"`
}

type notePlan struct {
	Size         int    `json:"size"`
	Availability string `json:"availability"`
	PlanID       int64  `json:"planId"`
	Cloneable    bool   `json:"cloneable"`
}

// ListPlans はゾーンで利用できる NFS プランの一覧を返します。
// プラン情報は、対象ゾーンのタグ (@zone=<ゾーン>) を持つ Note (sys-nfs) から
// 取得します。出力は SSD、HDD の順で、同じ種別内はサイズ昇順です。
// all が false のときは利用可能 (available) なプランだけを返します。
func (s *Service) ListPlans(ctx context.Context, zone string, all bool) ([]PlanOutput, error) {
	if s == nil || s.notes == nil {
		return nil, fmt.Errorf("API クライアントが設定されていません")
	}
	if zone == "" {
		return nil, fmt.Errorf("ゾーンを指定してください")
	}
	note, err := s.findPlanNote(ctx, zone)
	if err != nil {
		return nil, err
	}
	var content notePlanContent
	if err := json.Unmarshal([]byte(note.Content), &content); err != nil {
		return nil, fmt.Errorf("NFS プラン情報 (Note %d) の内容を解釈できませんでした: %w", note.ID, err)
	}
	plans := buildPlanOutputs(content)
	if len(plans) == 0 {
		return nil, fmt.Errorf("NFS プラン情報 (Note %d) にプランが含まれていません", note.ID)
	}
	if !all {
		plans = slices.DeleteFunc(plans, func(plan PlanOutput) bool {
			return plan.Availability != "available"
		})
	}
	return plans, nil
}

// findPlanNote はゾーンのタグを持つ NFS プラン情報の Note を返します。
// 見つからない、複数見つかる、Note が利用可能な状態でない、のいずれかはエラーにします。
func (s *Service) findPlanNote(ctx context.Context, zone string) (*iaas.Note, error) {
	result, err := s.notes.Find(ctx, &iaas.FindCondition{
		Count: noteFindCount,
		Filter: map[search.FilterKey]any{
			search.Key("Name"):  planNoteName,
			search.Key("Class"): "json",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("NFS プラン情報の Note を検索できませんでした: %w", err)
	}
	var note *iaas.Note
	for _, candidate := range result.Notes {
		if candidate == nil || !slices.Contains(candidate.Tags, planZoneTagPrefix+zone) {
			continue
		}
		if note != nil {
			return nil, fmt.Errorf("ゾーン %s に対応する NFS プラン情報が複数見つかりました", zone)
		}
		note = candidate
	}
	if note == nil {
		return nil, fmt.Errorf("ゾーン %s の NFS プラン情報が見つかりません (Note %s のタグに %s%s がありません)", zone, planNoteName, planZoneTagPrefix, zone)
	}
	if note.Availability != types.Availabilities.Available {
		return nil, fmt.Errorf("ゾーン %s の NFS プラン情報 (Note %d) は利用可能な状態ではありません (availability: %s)", zone, note.ID, note.Availability)
	}
	return note, nil
}

// buildPlanOutputs は Note のプラン情報を出力に変換します。
// 既知のストレージ種別 (SSD、HDD) だけを対象にし、SSD、HDD の順、
// 同じ種別内はサイズ昇順に並べ替えます。
func buildPlanOutputs(content notePlanContent) []PlanOutput {
	groups := []struct {
		storage string
		plans   []notePlan
	}{
		{storage: "SSD", plans: content.Plans.SSD},
		{storage: "HDD", plans: content.Plans.HDD},
	}
	var outputs []PlanOutput
	for _, group := range groups {
		plans := slices.Clone(group.plans)
		slices.SortStableFunc(plans, func(a, b notePlan) int {
			return cmp.Compare(a.Size, b.Size)
		})
		for _, plan := range plans {
			outputs = append(outputs, PlanOutput{
				Storage:      group.storage,
				Size:         plan.Size,
				PlanID:       types.ID(plan.PlanID),
				Availability: plan.Availability,
				Cloneable:    plan.Cloneable,
			})
		}
	}
	return outputs
}
