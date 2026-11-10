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

package eventbus

import (
	"context"
	"fmt"
	"strconv"
	"time"

	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
)

// ScheduleSettings はスケジュールの繰り返し設定です。
//
// 間隔指定 (RecurringStep と RecurringUnit) と Crontab はどちらか一方だけを指定
// します。両方を指定した場合や、間隔指定を片方だけ指定した場合はエラーになります。
// RecurringUnit は min、hour、day のいずれかです。
type ScheduleSettings struct {
	// ProcessConfigurationID は実行する実行設定の ID です。必須で、空の場合は
	// エラーを返します。
	ProcessConfigurationID string
	// StartsAt は実行を開始する日時です。必須で、零値の場合はエラーを返します。
	// API はエポックミリ秒で保持するため、ミリ秒より細かい小数秒は指定できません
	// (指定するとエラーを返します)。
	StartsAt time.Time
	// RecurringStep は繰り返しの間隔です。RecurringUnit と組で指定します。
	RecurringStep *int
	// RecurringUnit は繰り返しの単位です。min、hour、day のいずれか。
	// RecurringStep と組で指定します。
	RecurringUnit *string
	// Crontab は繰り返しを Crontab 形式で指定します。空文字列はエラーになります。
	Crontab *string
}

// CreateScheduleRequest はスケジュールを作成するリクエストです。
type CreateScheduleRequest struct {
	// Name はスケジュールの名前です。必須で、空の場合はエラーを返します。
	Name string
	// Description は説明です。空文字列は API には null として送信します。
	Description string
	// Settings は実行設定と繰り返しの指定です。
	Settings ScheduleSettings
}

// UpdateScheduleRequest はスケジュールを更新するリクエストです。
//
// ポインター型のフィールドは、nil なら現状維持、non-nil なら指定値への変更です。
// Settings 関連のフィールド (ProcessConfigurationID、StartsAt、RecurringStep、
// RecurringUnit、Crontab) を 1 つでも指定すると、現状を読み取って API の Settings
// 全体を送信します。Name と Description は Settings と独立して更新でき、その場合は
// Settings を送信しません。
//
// 繰り返しモードの切り替えは次のとおりです。
//
//   - Crontab を指定すると、既存の間隔指定 (RecurringStep と RecurringUnit) は解除
//     されます。Crontab と間隔指定の同時指定は事前にエラーになります。
//   - RecurringStep または RecurringUnit を指定すると、既存の Crontab は解除され
//     ます。既存が間隔指定のときは、片方だけの指定でもう片方の既存値を保持して
//     送信します。既存が Crontab のときは、間隔指定へ切り替えるのに両方の指定が
//     必要です (片方だけではエラーになります)。
//
// 読み取りと更新の間に他の利用者による変更がないことは検証しません。
type UpdateScheduleRequest struct {
	// ID は更新対象のスケジュール ID です。存在しない場合や種別が一致しない場合は
	// エラーを返します。
	ID string
	// Name は新しい名前です。nil のときは現状維持します。
	Name *string
	// Description は新しい説明です。nil のときは現状維持します。空文字列は
	// API には空文字列として送信します。作成時と異なり null にはなりません。
	Description *string
	// ProcessConfigurationID は実行する実行設定の新しい ID です。nil のときは
	// 現状維持します。
	ProcessConfigurationID *string
	// StartsAt は実行を開始する新しい日時です。nil のときは現状維持します。
	// ミリ秒より細かい小数秒は指定できません。API にはエポックミリ秒で送信します。
	StartsAt *time.Time
	// RecurringStep は繰り返しの新しい間隔です。Crontab と同時に指定できません。
	RecurringStep *int
	// RecurringUnit は繰り返しの新しい単位です。min、hour、day のいずれか。
	// Crontab と同時に指定できません。
	RecurringUnit *string
	// Crontab は繰り返しを Crontab 形式で指定します。指定すると間隔指定は解除
	// されます。空文字列はエラーになります。
	Crontab *string
}

// startsAtMillis は応答の StartsAt をエポックミリ秒へ正規化します。
// 整数と数値文字列のどちらも受け取り、それ以外はエラーになります。
func startsAtMillis(value v1.ScheduleSettingsStartsAt) (int64, error) {
	if ms, ok := value.GetInt64(); ok {
		return ms, nil
	}
	if text, ok := value.GetString(); ok {
		ms, err := strconv.ParseInt(text, 10, 64)
		if err == nil {
			return ms, nil
		}
	}
	return 0, fmt.Errorf("StartsAt をエポックミリ秒として解釈できません")
}

// scheduleSettings は入力を API の Settings へ変換し、必須項目と繰り返しモードの
// 排他を検証します。
func scheduleSettings(input ScheduleSettings) (v1.Settings, error) {
	if input.ProcessConfigurationID == "" || input.StartsAt.IsZero() {
		return v1.Settings{}, fmt.Errorf("ProcessConfigurationID と StartsAt が必要です")
	}
	if input.StartsAt.Nanosecond()%int(time.Millisecond) != 0 {
		return v1.Settings{}, fmt.Errorf("StartsAt の小数秒はミリ秒まで指定できます (API はエポックミリ秒で保持します)")
	}
	periodic := input.RecurringStep != nil || input.RecurringUnit != nil
	if (input.Crontab != nil) == periodic || periodic && (input.RecurringStep == nil || input.RecurringUnit == nil) {
		return v1.Settings{}, fmt.Errorf("RecurringStep・RecurringUnit の組と Crontab のどちらか一方が必要です")
	}
	settings := v1.ScheduleSettings{ProcessConfigurationID: input.ProcessConfigurationID,
		StartsAt: v1.NewInt64ScheduleSettingsStartsAt(input.StartsAt.UnixMilli())}
	if input.Crontab != nil {
		if *input.Crontab == "" {
			return v1.Settings{}, fmt.Errorf("Crontab は空にできません")
		}
		settings.Crontab.SetTo(*input.Crontab)
	} else {
		switch *input.RecurringUnit {
		case "min", "hour", "day":
		default:
			return v1.Settings{}, fmt.Errorf("RecurringUnit は min、hour、day のいずれかです")
		}
		settings.RecurringStep.SetTo(*input.RecurringStep)
		settings.RecurringUnit.SetTo(v1.ScheduleSettingsRecurringUnit(*input.RecurringUnit))
	}
	return v1.NewScheduleSettingsSettings(settings), nil
}

// ListSchedules はスケジュールの一覧を返します。
//
// 応答は生の SDK モデルです。利用者向けの形状には ProjectSchedule を使ってください。
func (s *Service) ListSchedules(ctx context.Context) ([]v1.CommonServiceItem, error) {
	return s.schedule.List(ctx)
}

// ReadSchedule は ID を指定してスケジュールを返します。
//
// 応答の ID とリソース種別、Settings の種別を検証し、一致しない場合はエラーを返します。
func (s *Service) ReadSchedule(ctx context.Context, req *ReadRequest) (*v1.CommonServiceItem, error) {
	return read(ctx, s.schedule, req.ID, v1.ProviderClassEventbusschedule)
}

// CreateSchedule はスケジュールを作成します。
//
// 必須項目 (Name、ProcessConfigurationID、StartsAt) と繰り返しモードの排他を検証
// します。Description の空文字列は API には null として送信します。
func (s *Service) CreateSchedule(ctx context.Context, req *CreateScheduleRequest) (*v1.CommonServiceItem, error) {
	settings, err := scheduleSettings(req.Settings)
	if err != nil {
		return nil, err
	}
	request, err := createRequest(req.Name, req.Description, settings)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.schedule.Create(ctx, request)
}

// UpdateSchedule はスケジュールを更新します。
//
// 現状を API から読み取り、未指定の Settings 項目を既存値で補って API の Settings を
// 組み立てます (UpdateScheduleRequest を参照)。Settings 関連のフィールドを 1 つも
// 指定しない場合は Settings を送信せず、Name と Description だけを更新します。
// 応答の StartsAt がエポックミリ秒として解釈できない場合は更新 API を呼び出さずに
// エラーを返します。読み取りと更新の間に context がキャンセルされた場合は更新 API を
// 呼び出しません。
func (s *Service) UpdateSchedule(ctx context.Context, req *UpdateScheduleRequest) (*v1.CommonServiceItem, error) {
	if req.Crontab != nil && (req.RecurringStep != nil || req.RecurringUnit != nil) {
		return nil, fmt.Errorf("Crontab と周期指定は併用できません")
	}
	current, err := s.ReadSchedule(ctx, &ReadRequest{ID: req.ID})
	if err != nil {
		return nil, err
	}
	request := updateRequest(req.Name, req.Description)
	if req.ProcessConfigurationID != nil || req.StartsAt != nil || req.RecurringStep != nil || req.RecurringUnit != nil || req.Crontab != nil {
		old := current.Settings.ScheduleSettings
		ms, err := startsAtMillis(old.StartsAt)
		if err != nil {
			return nil, err
		}
		input := ScheduleSettings{ProcessConfigurationID: old.ProcessConfigurationID, StartsAt: time.UnixMilli(ms)}
		if old.Crontab.IsSet() {
			value := old.Crontab.Value
			input.Crontab = &value
		}
		if old.RecurringStep.IsSet() {
			value := old.RecurringStep.Value
			input.RecurringStep = &value
		}
		if old.RecurringUnit.IsSet() {
			value := string(old.RecurringUnit.Value)
			input.RecurringUnit = &value
		}
		if req.ProcessConfigurationID != nil {
			input.ProcessConfigurationID = *req.ProcessConfigurationID
		}
		if req.StartsAt != nil {
			input.StartsAt = *req.StartsAt
		}
		if req.Crontab != nil {
			input.Crontab, input.RecurringStep, input.RecurringUnit = req.Crontab, nil, nil
		} else if req.RecurringStep != nil || req.RecurringUnit != nil {
			if input.Crontab != nil {
				input.RecurringStep, input.RecurringUnit = nil, nil
			}
			input.Crontab = nil
			if req.RecurringStep != nil {
				input.RecurringStep = req.RecurringStep
			}
			if req.RecurringUnit != nil {
				input.RecurringUnit = req.RecurringUnit
			}
		}
		settings, err := scheduleSettings(input)
		if err != nil {
			return nil, err
		}
		request.CommonServiceItem.Settings.SetTo(settings)
	}
	return update(ctx, s.schedule, req.ID, request)
}

// DeleteSchedule は ID を指定してスケジュールを削除します。
//
// 削除前に API から対象を再取得し、ID とリソース種別、Settings の種別が一致しない
// 場合は削除せずにエラーを返します。context がキャンセルされた場合は削除 API を
// 呼び出しません。
func (s *Service) DeleteSchedule(ctx context.Context, req *DeleteRequest) error {
	return remove(ctx, s.schedule, req.ID, v1.ProviderClassEventbusschedule)
}
