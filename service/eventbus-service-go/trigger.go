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
	"regexp"

	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
)

// TriggerCondition はトリガーのイベント一致条件です。
//
// Op は等号の "eq" か包含の "in" です。eq は Values がちょうど 1 件、in は
// Values が 1 件以上必要です。Key は data 以外の小文字英数字 (20 文字以内) です。
// この検証は Conditions を API へ変換するときに行います。
type TriggerCondition struct {
	// Key はイベントペイロード内の一致対象のキーです。
	Key string
	// Op は一致の種別です。eq または in です。
	Op string
	// Values は一致させる値です。eq は 1 件、in は 1 件以上です。
	Values []string
}

// TriggerSettings はトリガーの送信元と条件です。
//
// Source は //eventbus.sakura.ad.jp/eventlog または
// //eventbus.sakura.ad.jp/monitoringsuite/alert のいずれかです。Types と Conditions
// は nil で「指定なし」、non-nil のスライスで「指定した値全体」を表します。空スライス
// (既存値の消去) はエラーになります。消去は低レベル API (CommonServiceItem 操作) で
// 行ってください。
type TriggerSettings struct {
	// Source は送信元イベントの URI です。
	Source string
	// Types は対象のイベントタイプです。nil は指定なし (null) です。nil 以外は
	// 指定した値全体で置き換えます。要素に空文字列は指定できません。
	Types []string
	// Conditions はイベントの一致条件です。nil は指定なし (null) です。nil 以外は
	// 指定した条件全体で置き換えます。
	Conditions []TriggerCondition
	// ProcessConfigurationID は実行する実行設定の ID です。必須で、空の場合は
	// エラーを返します。
	ProcessConfigurationID string
}

// CreateTriggerRequest はトリガーを作成するリクエストです。
type CreateTriggerRequest struct {
	// Name はトリガーの名前です。必須で、空の場合はエラーを返します。
	Name string
	// Description は説明です。空文字列は API には null として送信します。
	Description string
	// Settings は送信元と条件の指定です。
	Settings TriggerSettings
}

// UpdateTriggerRequest はトリガーを更新するリクエストです。
//
// ポインター型・スライス型のフィールドは、nil なら現状維持、non-nil なら指定値への
// 変更です。Types と Conditions は指定時に既存値の全体を置き換えます。Settings 関連の
// フィールド (Source、ProcessConfigurationID、Types、Conditions) を 1 つでも指定すると
// 現状を読み取って API の Settings 全体を送信します。Name と Description は Settings と
// 独立して更新でき、その場合は Settings を送信しません。
//
// Types や Conditions を空にする (null への変更) は、このリクエストでは表現できません。
// 低レベル API (CommonServiceItem 操作) で行ってください。
//
// 読み取りと更新の間に他の利用者による変更がないことは検証しません。
type UpdateTriggerRequest struct {
	// ID は更新対象のトリガー ID です。存在しない場合や種別が一致しない場合は
	// エラーを返します。
	ID string
	// Name は新しい名前です。nil のときは現状維持します。
	Name *string
	// Description は新しい説明です。nil のときは現状維持します。空文字列は
	// API には空文字列として送信します。作成時と異なり null にはなりません。
	Description *string
	// Source は送信元イベントの新しい URI です。nil のときは現状維持します。
	Source *string
	// ProcessConfigurationID は実行する実行設定の新しい ID です。nil のときは
	// 現状維持し、空文字列はエラーになります。
	ProcessConfigurationID *string
	// Types は対象の新しいイベントタイプです。nil のときは現状維持し、nil 以外は
	// 指定した値全体で置き換えます。空スライスと空文字列の要素はエラーになります。
	Types []string
	// Conditions はイベントの新しい一致条件です。nil のときは現状維持し、nil 以外は
	// 指定した条件全体で置き換えます。空スライスはエラーになります。
	Conditions []TriggerCondition
}

var conditionKey = regexp.MustCompile(`^[a-z0-9]{1,20}$`)

// validateSource は送信元 URI を検証します。
func validateSource(source string) error {
	if source != "//eventbus.sakura.ad.jp/eventlog" && source != "//eventbus.sakura.ad.jp/monitoringsuite/alert" {
		return fmt.Errorf("Source は //eventbus.sakura.ad.jp/eventlog または //eventbus.sakura.ad.jp/monitoringsuite/alert を指定してください")
	}
	return nil
}

// validateTypes はイベントタイプの指定を検証します。
// nil は指定なし、空スライスは消去の意図と解釈されるため、後者はエラーにします。
func validateTypes(types []string) error {
	if types != nil && len(types) == 0 {
		return fmt.Errorf("Types の消去は低レベル API を使ってください")
	}
	for _, value := range types {
		if value == "" {
			return fmt.Errorf("Types に空文字列は指定できません")
		}
	}
	return nil
}

// convertConditions は条件を API の sum 型へ変換し、Key・Op・Values の妥当性を
// 検証します。nil は null へ、空スライスはエラーになります。
func convertConditions(conditions []TriggerCondition) (v1.OptNilTriggerSettingsConditionsItemArray, error) {
	result := v1.OptNilTriggerSettingsConditionsItemArray{}
	if conditions == nil {
		result.SetToNull()
		return result, nil
	}
	if len(conditions) == 0 {
		return result, fmt.Errorf("Conditions の消去は低レベル API を使ってください")
	}
	values := make([]v1.TriggerSettingsConditionsItem, 0, len(conditions))
	for _, condition := range conditions {
		if !conditionKey.MatchString(condition.Key) || condition.Key == "data" {
			return result, fmt.Errorf("Conditions.Key は data 以外の小文字英数字で指定してください")
		}
		switch condition.Op {
		case "eq":
			if len(condition.Values) != 1 {
				return result, fmt.Errorf("Conditions の eq は Values が1件必要です")
			}
			values = append(values, v1.NewTriggerConditionEqTriggerSettingsConditionsItem(v1.TriggerConditionEq{
				Key: condition.Key, Op: v1.TriggerConditionEqOpEq, Values: condition.Values,
			}))
		case "in":
			if len(condition.Values) == 0 {
				return result, fmt.Errorf("Conditions の in は Values が必要です")
			}
			values = append(values, v1.NewTriggerConditionInTriggerSettingsConditionsItem(v1.TriggerConditionIn{
				Key: condition.Key, Op: v1.TriggerConditionInOpIn, Values: condition.Values,
			}))
		default:
			return result, fmt.Errorf("Conditions.Op は eq または in を指定してください")
		}
	}
	result.SetTo(values)
	return result, nil
}

// ListTriggers はトリガーの一覧を返します。
//
// 応答は生の SDK モデルです。利用者向けの形状には ProjectTrigger を使ってください。
func (s *Service) ListTriggers(ctx context.Context) ([]v1.CommonServiceItem, error) {
	return s.trigger.List(ctx)
}

// ReadTrigger は ID を指定してトリガーを返します。
//
// 応答の ID とリソース種別、Settings の種別を検証し、一致しない場合はエラーを返します。
func (s *Service) ReadTrigger(ctx context.Context, req *ReadRequest) (*v1.CommonServiceItem, error) {
	return read(ctx, s.trigger, req.ID, v1.ProviderClassEventbustrigger)
}

// CreateTrigger はトリガーを作成します。
//
// 送信元 URI、ProcessConfigurationID、Types、Conditions を検証します。Description の
// 空文字列は API には null として送信します。Types と Conditions を nil にすると
// API には null を送信します。
func (s *Service) CreateTrigger(ctx context.Context, req *CreateTriggerRequest) (*v1.CommonServiceItem, error) {
	if err := validateSource(req.Settings.Source); err != nil {
		return nil, err
	}
	if req.Settings.ProcessConfigurationID == "" {
		return nil, fmt.Errorf("ProcessConfigurationID が必要です")
	}
	if err := validateTypes(req.Settings.Types); err != nil {
		return nil, err
	}
	conditions, err := convertConditions(req.Settings.Conditions)
	if err != nil {
		return nil, err
	}
	types := v1.NewOptNilStringArray(req.Settings.Types)
	if req.Settings.Types == nil {
		types.SetToNull()
	}
	settings := v1.NewTriggerSettingsSettings(v1.TriggerSettings{
		Source: req.Settings.Source, Types: types, Conditions: conditions, ProcessConfigurationID: req.Settings.ProcessConfigurationID,
	})
	request, err := createRequest(req.Name, req.Description, settings)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.trigger.Create(ctx, request)
}

// UpdateTrigger はトリガーを更新します。
//
// Settings 関連のフィールドを指定した場合は現状を API から読み取り、未指定の項目を
// 既存値で補って API の Settings を組み立てます (UpdateTriggerRequest を参照)。
// Settings 関連のフィールドを 1 つも指定しない場合は Settings を送信せず、Name と
// Description だけを更新します。指定した Types と Conditions の検証は、読み取りの前
// に行います。読み取りと更新の間に context がキャンセルされた場合は更新 API を
// 呼び出しません。
func (s *Service) UpdateTrigger(ctx context.Context, req *UpdateTriggerRequest) (*v1.CommonServiceItem, error) {
	if req.Source != nil {
		if err := validateSource(*req.Source); err != nil {
			return nil, err
		}
	}
	if req.ProcessConfigurationID != nil && *req.ProcessConfigurationID == "" {
		return nil, fmt.Errorf("ProcessConfigurationID が必要です")
	}
	if err := validateTypes(req.Types); err != nil {
		return nil, err
	}
	conditions, err := convertConditions(req.Conditions)
	if err != nil {
		return nil, err
	}
	current, err := s.ReadTrigger(ctx, &ReadRequest{ID: req.ID})
	if err != nil {
		return nil, err
	}
	request := updateRequest(req.Name, req.Description)
	if req.Source != nil || req.ProcessConfigurationID != nil || req.Types != nil || req.Conditions != nil {
		settings := current.Settings.TriggerSettings
		if req.Source != nil {
			settings.Source = *req.Source
		}
		if req.ProcessConfigurationID != nil {
			settings.ProcessConfigurationID = *req.ProcessConfigurationID
		}
		if req.Types != nil {
			settings.Types.SetTo(req.Types)
		}
		if req.Conditions != nil {
			settings.Conditions = conditions
		}
		request.CommonServiceItem.Settings.SetTo(v1.NewTriggerSettingsSettings(settings))
	}
	return update(ctx, s.trigger, req.ID, request)
}

// DeleteTrigger は ID を指定してトリガーを削除します。
//
// 削除前に API から対象を再取得し、ID とリソース種別、Settings の種別が一致しない
// 場合は削除せずにエラーを返します。context がキャンセルされた場合は削除 API を
// 呼び出しません。
func (s *Service) DeleteTrigger(ctx context.Context, req *DeleteRequest) error {
	return remove(ctx, s.trigger, req.ID, v1.ProviderClassEventbustrigger)
}
