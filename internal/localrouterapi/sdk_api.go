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

package localrouterapi

import (
	"context"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	localrouter "github.com/sacloud/sacloud-sdk-go/service/iaas/localrouter"
)

// NewSDKAPI は service 層と API 層の LocalRouterOp を組み合わせた API を構築します。
func NewSDKAPI(caller iaas.APICaller) API {
	return &sdkAPI{
		caller:      caller,
		service:     localrouter.New(caller),
		localRouter: iaas.NewLocalRouterOp(caller),
	}
}

type sdkAPI struct {
	caller      iaas.APICaller
	service     *localrouter.Service
	localRouter iaas.LocalRouterAPI
}

func (a *sdkAPI) FindWithContext(ctx context.Context, request *localrouter.FindRequest) ([]*iaas.LocalRouter, error) {
	return a.service.FindWithContext(ctx, request)
}

func (a *sdkAPI) ReadWithContext(ctx context.Context, request *localrouter.ReadRequest) (*iaas.LocalRouter, error) {
	return a.service.ReadWithContext(ctx, request)
}

func (a *sdkAPI) CreateWithContext(ctx context.Context, request *localrouter.CreateRequest) (*iaas.LocalRouter, error) {
	return a.service.CreateWithContext(ctx, request)
}

func (a *sdkAPI) UpdateWithContext(ctx context.Context, request *localrouter.UpdateRequest) (*iaas.LocalRouter, error) {
	// Service.Update は Builder.ID に ID を引き継げず新規作成になるため、
	// Builder を直接組み立てて ID を設定して更新する
	builder, err := request.Builder(ctx, a.caller)
	if err != nil {
		return nil, err
	}
	builder.ID = request.ID
	return builder.Build(ctx)
}

// UpdateSettingsWithContext は設定のみの部分更新を行う。
// API 層の UpdateSettings は設定全体を置き換えるため、現在値を読み込んで
// 指定された設定だけを上書きしたリクエストを送る。SettingsHash には
// Read で取得した現在値を渡す。
func (a *sdkAPI) UpdateSettingsWithContext(ctx context.Context, request *UpdateSettingsRequest) (*iaas.LocalRouter, error) {
	current, err := a.localRouter.Read(ctx, request.ID)
	if err != nil {
		return nil, err
	}
	settings := &iaas.LocalRouterUpdateSettingsRequest{
		Switch:       current.Switch,
		Interface:    current.Interface,
		Peers:        current.Peers,
		StaticRoutes: current.StaticRoutes,
		SettingsHash: current.SettingsHash,
	}
	if request.Switch != nil {
		settings.Switch = request.Switch
	}
	if request.Interface != nil {
		settings.Interface = request.Interface
	}
	if request.Peers != nil {
		settings.Peers = *request.Peers
	}
	if request.StaticRoutes != nil {
		settings.StaticRoutes = *request.StaticRoutes
	}
	// マージ結果がスイッチとインターフェースの片側だけになる入力は、
	// 既存の対になる設定を失わせるため拒否します
	if err := requirePairedConnection(settings.Switch, settings.Interface); err != nil {
		return nil, err
	}
	return a.localRouter.UpdateSettings(ctx, request.ID, settings)
}

func (a *sdkAPI) DeleteWithContext(ctx context.Context, request *localrouter.DeleteRequest) error {
	return a.service.DeleteWithContext(ctx, request)
}

func (a *sdkAPI) HealthWithContext(ctx context.Context, request *localrouter.HealthRequest) (*iaas.LocalRouterHealth, error) {
	return a.service.HealthWithContext(ctx, request)
}

func (a *sdkAPI) MonitorLocalRouterWithContext(ctx context.Context, request *localrouter.MonitorLocalRouterRequest) ([]*iaas.MonitorLocalRouterValue, error) {
	return a.service.MonitorLocalRouterWithContext(ctx, request)
}
