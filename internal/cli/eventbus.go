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

package cli

import (
	eventbussdk "github.com/sacloud/sacloud-sdk-go/api/eventbus"
	"github.com/sacloud/skr/internal/eventbus"
	"github.com/sacloud/skr/internal/saclient"
)

// initEventbus は手書きの高レベル eventbus コマンド群を配線します (ADR 0075)。
// 出力処理は CLI 共通の実装を、API クライアントは factory 関数で注入します。
func (c *cli) initEventbus() {
	c.Eventbus = eventbus.NewCommands(
		eventbus.Runtime{
			ValidateOutput: validateOutput,
			WriteOutput:    writeOutput,
		},
		func() (*eventbus.Service, error) {
			return newEventbusService(c.Trace)
		},
		func() (*eventbus.Service, error) {
			return newEventbusSecretService(c.Trace)
		},
	)
}

// newEventbusService は高レベルの eventbus コマンド用の Service を構築します。
func newEventbusService(trace bool) (*eventbus.Service, error) {
	client, err := saclient.New(trace)
	if err != nil {
		return nil, err
	}
	v1Client, err := eventbussdk.NewClient(client)
	if err != nil {
		return nil, err
	}
	return eventbus.New(v1Client), nil
}

// newEventbusSecretService は set-secret 用の Service を構築します。
// secret を扱う操作では、実行前に実効トレース設定を検査し、
// トレースが有効な場合は秘密情報が記録されるおそれがあるため実行しません。
func newEventbusSecretService(trace bool) (*eventbus.Service, error) {
	client, err := saclient.New(trace)
	if err != nil {
		return nil, err
	}
	if err := eventbus.CheckTraceSettings(client); err != nil {
		return nil, err
	}
	v1Client, err := eventbussdk.NewClient(client)
	if err != nil {
		return nil, err
	}
	return eventbus.New(v1Client), nil
}
