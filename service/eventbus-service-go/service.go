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

// Package eventbus は、さくらのクラウド EventBus の実行設定 (process-configuration)、
// スケジュール (schedule)、トリガー (trigger) を操作する高レベルライブラリです。
//
// 低レベルの CommonServiceItem 操作を隠蔽し、宛先別の個別入力、未指定値を保持する
// 更新のマージ、応答の検証、出力射影を提供します。
package eventbus

import (
	"context"
	"fmt"

	sdk "github.com/sacloud/sacloud-sdk-go/api/eventbus"
	v1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
)

// Service は EventBus の実行設定 (process-configuration)、スケジュール (schedule)、
// トリガー (trigger) を操作します。
//
// 各メソッドは SDK の低レベル API (CommonServiceItem 操作) を隠蔽し、入力の検証と
// 変換、更新時の現状読み取りとマージを行います。操作対象の読み取りを伴うメソッドは、
// 変更 API を呼び出す前に context のキャンセルを再確認します。読み取りと変更の間に
// 他の利用者による変更がないことは検証しません。
//
// 零値の Service は利用できません。New で構築してください。
type Service struct {
	process  sdk.ProcessConfigurationAPI
	schedule sdk.ScheduleAPI
	trigger  sdk.TriggerAPI
}

// New は EventBus API クライアント (sacloud-sdk-go の api/eventbus パッケージ) から
// Service を構築します。
//
//	var client saclient.Client
//	if err := client.SetEnviron(os.Environ()); err != nil {
//		return err
//	}
//	v1Client, err := eventbussdk.NewClient(&client)
//	if err != nil {
//		return err
//	}
//	service := eventbus.New(v1Client)
//
// 応答は SDK の生のモデル (*v1.CommonServiceItem) で返ります。利用者向けの形状には
// ProjectProcessConfiguration、ProjectSchedule、ProjectTrigger で変換してください。
func New(caller *v1.Client) *Service {
	return &Service{
		process:  sdk.NewProcessConfigurationOp(caller),
		schedule: sdk.NewScheduleOp(caller),
		trigger:  sdk.NewTriggerOp(caller),
	}
}

// ReadRequest は ID を指定して単一のリソースを参照します。
// 実行設定・スケジュール・トリガーで共通のリクエスト型です。
type ReadRequest struct {
	// ID は対象リソースの ID です。空の場合はエラーを返します。
	ID string
}

// DeleteRequest は ID を指定して単一のリソースを削除します。
// 実行設定・スケジュール・トリガーで共通のリクエスト型です。
type DeleteRequest struct {
	// ID は対象リソースの ID です。空の場合はエラーを返します。
	ID string
}

// resourceAPI は 3 種類のリソース操作 API に共通する低レベル操作です。
// 単体テストではこの interface に fake を注入します。
type resourceAPI interface {
	List(context.Context) ([]v1.CommonServiceItem, error)
	Read(context.Context, string) (*v1.CommonServiceItem, error)
	Create(context.Context, v1.CreateCommonServiceItemRequest) (*v1.CommonServiceItem, error)
	Update(context.Context, string, v1.UpdateCommonServiceItemRequest) (*v1.CommonServiceItem, error)
	Delete(context.Context, string) error
}

func checkItem(item *v1.CommonServiceItem, id string, class v1.ProviderClass) error {
	if item == nil || item.ID == "" || (id != "" && item.ID != id) || item.Provider.Class != class {
		return fmt.Errorf("応答の ID またはリソース種別が一致しません")
	}
	valid := class == v1.ProviderClassEventbusprocessconfiguration && item.Settings.IsProcessConfigurationSettings() ||
		class == v1.ProviderClassEventbusschedule && item.Settings.IsScheduleSettings() ||
		class == v1.ProviderClassEventbustrigger && item.Settings.IsTriggerSettings()
	if !valid {
		return fmt.Errorf("応答の Settings の種別が一致しません")
	}
	return nil
}

func read(ctx context.Context, api resourceAPI, id string, class v1.ProviderClass) (*v1.CommonServiceItem, error) {
	if id == "" {
		return nil, fmt.Errorf("ID が必要です")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	item, err := api.Read(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := checkItem(item, id, class); err != nil {
		return nil, err
	}
	return item, nil
}

func remove(ctx context.Context, api resourceAPI, id string, class v1.ProviderClass) error {
	if _, err := read(ctx, api, id, class); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return api.Delete(ctx, id)
}

func createRequest(name, description string, settings v1.Settings) (v1.CreateCommonServiceItemRequest, error) {
	if name == "" {
		return v1.CreateCommonServiceItemRequest{}, fmt.Errorf("Name が必要です")
	}
	desc := v1.NewOptNilString(description)
	if description == "" {
		desc.SetToNull()
	}
	return v1.CreateCommonServiceItemRequest{CommonServiceItem: v1.CreateCommonServiceItemRequestCommonServiceItem{
		Name: name, Description: desc, Settings: settings,
	}}, nil
}

func updateRequest(name, description *string) v1.UpdateCommonServiceItemRequest {
	req := v1.UpdateCommonServiceItemRequest{}
	if name != nil {
		req.CommonServiceItem.Name.SetTo(*name)
	}
	if description != nil {
		req.CommonServiceItem.Description.SetTo(*description)
	}
	return req
}

func update(ctx context.Context, api resourceAPI, id string, req v1.UpdateCommonServiceItemRequest) (*v1.CommonServiceItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return api.Update(ctx, id, req)
}
