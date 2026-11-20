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

package monitoringsuiteapi

import (
	"context"
	"fmt"
	"time"

	"github.com/alecthomas/kong"
)

// AlertProjectListHistoriesCommand はアラート履歴の一覧表示コマンドです。
// 絞り込み条件が多く SDK の params 型を直接扱うため手書きにしています。
type AlertProjectListHistoriesCommand struct {
	ProjectID string     `arg:"" name:"project-id" help:"アラートプロジェクトのリソース ID。"`
	Count     *int       `name:"count" help:"任意: 1 回の取得件数。"`
	From      *int       `name:"from" help:"任意: 取得開始位置 (0 開始)。"`
	Open      *bool      `name:"open" help:"任意: 未解決アラートのみを取得します。"`
	Severity  *string    `name:"severity" help:"任意: 重要度で絞り込みます。critical、warning のいずれか。"`
	StartsAt  *time.Time `name:"starts-at" help:"任意: 発生日時で絞り込みます。RFC3339 形式で指定します。"`

	factory AlertProjectAPIFactory
	runtime AlertProjectRuntime
}

func (c *AlertProjectListHistoriesCommand) Run(ctx *kong.Context) error {
	if c.runtime.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	if err := c.runtime.ValidateOutput(ctx); err != nil {
		return err
	}
	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	result, err := op.ListHistories(context.Background(), c.ProjectID, c.Count, c.From, c.Open, c.Severity, c.StartsAt)
	if err != nil {
		return err
	}
	if c.runtime.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return c.runtime.WriteOutput(ctx, result)
}

// NotificationTargetCreateCommand は通知先の作成コマンドです。
// SDK の params 型が通知先 URL をパース済みの値で保持するため手書きにしています。
type NotificationTargetCreateCommand struct {
	ProjectID   string  `arg:"" name:"project-id" help:"アラートプロジェクトのリソース ID。"`
	Request     *string `name:"request" help:"API リクエスト JSON を直接または @path.json で指定します。個別フラグと併用できません。"`
	ServiceType *string `name:"service-type" help:"必須: サービス種別。SAKURA_SIMPLE_NOTICE。"`
	URL         *string `name:"url" help:"必須: 通知先 URL。"`
	Description *string `name:"description" help:"任意: 説明。"`

	factory NotificationTargetAPIFactory
	runtime NotificationTargetRuntime
}

// notificationTargetCreateRequest は --request の入力を受ける中間型です。
// 通知先 URL は SDK の params 型ではパース済みの値を保持するため、そのままデコードできません。
type notificationTargetCreateRequest struct {
	ServiceType *string `json:"ServiceType"`
	URL         *string `json:"URL"`
	Description *string `json:"Description"`
}

func (c *NotificationTargetCreateCommand) Run(ctx *kong.Context) error {
	flagsSet := c.ServiceType != nil || c.URL != nil || c.Description != nil
	if c.Request != nil && flagsSet {
		return fmt.Errorf("--request と個別フラグは併用できません")
	}
	var serviceType, rawURL string
	var description *string
	if c.Request != nil {
		if c.runtime.DecodeRequest == nil {
			return fmt.Errorf("API リクエスト処理が設定されていません")
		}
		req := &notificationTargetCreateRequest{}
		if err := c.runtime.DecodeRequest(*c.Request, req); err != nil {
			return err
		}
		if req.ServiceType == nil {
			return fmt.Errorf("ServiceType は必須です")
		}
		if req.URL == nil {
			return fmt.Errorf("URL は必須です")
		}
		serviceType = *req.ServiceType
		rawURL = *req.URL
		description = req.Description
	} else {
		if c.ServiceType == nil {
			return fmt.Errorf("--service-type が必要です")
		}
		if c.URL == nil {
			return fmt.Errorf("--url が必要です")
		}
		serviceType = *c.ServiceType
		rawURL = *c.URL
		description = c.Description
	}

	if c.runtime.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	if err := c.runtime.ValidateOutput(ctx); err != nil {
		return err
	}
	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	result, err := op.Create(context.Background(), c.ProjectID, serviceType, rawURL, description)
	if err != nil {
		return err
	}
	if c.runtime.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return c.runtime.WriteOutput(ctx, result)
}
