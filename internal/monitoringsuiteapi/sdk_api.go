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

// Package monitoringsuiteapi はモニタリングスイート API のコマンドを提供します。
package monitoringsuiteapi

import (
	"context"
	"fmt"
	"net/url"
	"time"

	monitoringsuite "github.com/sacloud/sacloud-sdk-go/api/monitoring-suite"
	v1 "github.com/sacloud/sacloud-sdk-go/api/monitoring-suite/apis/v1"
	clientconfig "github.com/sacloud/skr/internal/saclient"
)

// Commands はモニタリングスイートのコマンド集合です。リソース別のグループ階層を
// Kong の embed で展開して 1 階層のフラット構造にしています (ADR 0074)。
type Commands struct {
	AlertProject        AlertProjectCommands        `embed:""`
	AlertRule           AlertRuleCommands           `embed:""`
	LogMeasureRule      LogMeasureRuleCommands      `embed:""`
	NotificationTarget  NotificationTargetCommands  `embed:""`
	NotificationRouting NotificationRoutingCommands `embed:""`
	DashboardProject    DashboardProjectCommands    `embed:""`
	LogRouting          LogRoutingCommands          `embed:""`
	LogStorage          LogStorageCommands          `embed:""`
	MetricsRouting      MetricsRoutingCommands      `embed:""`
	MetricsStorage      MetricsStorageCommands      `embed:""`
	TracesStorage       TracesStorageCommands       `embed:""`
	Publisher           PublisherCommands           `embed:""`
	Management          ManagementCommands          `embed:""`
}

func newClient(trace bool) (*v1.Client, error) {
	client, err := clientconfig.New(trace)
	if err != nil {
		return nil, err
	}
	return monitoringsuite.NewClient(client)
}

// NewAlertProjectAPI はアラートプロジェクトの API を返します。
func NewAlertProjectAPI(trace bool) (AlertProjectAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	op := monitoringsuite.NewAlertProjectOp(client)
	return &alertProjectAPI{AlertProjectAPI: op}, nil
}

// alertProjectAPI は SDK の操作を生成 interface に合わせて変換します。
type alertProjectAPI struct {
	monitoringsuite.AlertProjectAPI
}

// ListHistories は SDK の params 型に変換してアラート履歴を取得します。
func (a *alertProjectAPI) ListHistories(ctx context.Context, projectID string, count, from *int, open *bool, severity *string, startsAt *time.Time) ([]v1.History, error) {
	params := monitoringsuite.AlertsProjectsHistoriesListParams{
		ProjectID: projectID,
		Count:     count,
		From:      from,
		Open:      open,
		StartsAt:  startsAt,
	}
	if severity != nil {
		s := v1.ListAlertProjectHistoriesSeverity(*severity)
		params.Severity = &s
	}
	return a.AlertProjectAPI.ListHistories(ctx, params)
}

// NewAlertRuleAPI はアラートルールの API を返します。
func NewAlertRuleAPI(trace bool) (AlertRuleAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return monitoringsuite.NewAlertRuleOp(client), nil
}

// NewLogMeasureRuleAPI はログ計測ルールの API を返します。
func NewLogMeasureRuleAPI(trace bool) (LogMeasureRuleAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return monitoringsuite.NewLogMeasureRuleOp(client), nil
}

// NewNotificationTargetAPI は通知先の API を返します。
func NewNotificationTargetAPI(trace bool) (NotificationTargetAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	op := monitoringsuite.NewNotificationTargetOp(client)
	return &notificationTargetAPI{NotificationTargetAPI: op}, nil
}

// notificationTargetAPI は SDK の操作を生成 interface に合わせて変換します。
type notificationTargetAPI struct {
	monitoringsuite.NotificationTargetAPI
}

// Create は通知先 URL を検証して通知先を作成します。
func (a *notificationTargetAPI) Create(ctx context.Context, projectID, serviceType, rawURL string, description *string) (*v1.NotificationTarget, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("--url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("--url: スキームとホストを含む URL を指定してください")
	}
	return a.NotificationTargetAPI.Create(ctx, projectID, monitoringsuite.NotificationTargetCreateParams{
		ServiceType: v1.NotificationTargetServiceType(serviceType),
		URL:         parsed,
		Description: description,
	})
}

// NewNotificationRoutingAPI は通知ルーティングの API を返します。
func NewNotificationRoutingAPI(trace bool) (NotificationRoutingAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return monitoringsuite.NewNotificationRoutingOp(client), nil
}

// NewDashboardProjectAPI はダッシュボードプロジェクトの API を返します。
func NewDashboardProjectAPI(trace bool) (DashboardProjectAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return monitoringsuite.NewDashboardOp(client), nil
}

// NewLogRoutingAPI はログルーティングの API を返します。
func NewLogRoutingAPI(trace bool) (LogRoutingAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return monitoringsuite.NewLogRoutingOp(client), nil
}

// NewLogStorageAPI はログストレージの API を返します。
func NewLogStorageAPI(trace bool) (LogStorageAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return monitoringsuite.NewLogsStorageOp(client), nil
}

// NewMetricsRoutingAPI はメトリクスルーティングの API を返します。
func NewMetricsRoutingAPI(trace bool) (MetricsRoutingAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return monitoringsuite.NewMetricsRoutingOp(client), nil
}

// NewMetricsStorageAPI はメトリクスストレージの API を返します。
func NewMetricsStorageAPI(trace bool) (MetricsStorageAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return monitoringsuite.NewMetricsStorageOp(client), nil
}

// NewTracesStorageAPI はトレースストレージの API を返します。
func NewTracesStorageAPI(trace bool) (TracesStorageAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return monitoringsuite.NewTracesStorageOp(client), nil
}

// NewPublisherAPI は Publisher の API を返します。
func NewPublisherAPI(trace bool) (PublisherAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return monitoringsuite.NewPublisherOp(client), nil
}

// NewManagementAPI は管理操作の API を返します。
func NewManagementAPI(trace bool) (ManagementAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return monitoringsuite.NewManagementOp(client), nil
}
