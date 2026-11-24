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
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
)

// Runtime は CLI 側から注入する出力処理の集合です。
type Runtime struct {
	ValidateOutput func(*kong.Context) error
	WriteOutput    func(*kong.Context, any) error
}

// ServiceFactory は API クライアントを構築して Service を返します。
type ServiceFactory func() (*Service, error)

func (r Runtime) validateOutput(kongCtx *kong.Context) error {
	if r.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.ValidateOutput(kongCtx)
}

func (r Runtime) writeOutput(kongCtx *kong.Context, value any) error {
	if r.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.WriteOutput(kongCtx, value)
}

// newService は factory から Service を構築します。
func newService(factory ServiceFactory) (*Service, error) {
	if factory == nil {
		return nil, fmt.Errorf("API クライアントが設定されていません")
	}
	return factory()
}

// commandContext は NFS コマンド専用の実行境界を作ります。
// 割り込みシグナルで context がキャンセルされるため、検索の途中で
// 割り込まれても API 呼び出しが続行しません。
func commandContext() (context.Context, func()) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// Commands は skr nfs 配下のコマンド群です。
type Commands struct {
	Plans PlansCommand `cmd:"" name:"plans" help:"ゾーンで利用できる NFS プランの一覧を表示します。ストレージ種別 (SSD / HDD) とディスク容量ごとに、skr nfs-api create の --plan-id に指定するプラン ID を出力します。省略時は利用可能 (available) なプランのみを出力し、--all で利用不可 (disabled) のプランも含めます。"`
}

// NewCommands は runtime と factory を各コマンドへ一括注入します。
func NewCommands(runtime Runtime, factory ServiceFactory) Commands {
	return Commands{
		Plans: PlansCommand{factory: factory, runtime: runtime},
	}
}

// PlansCommand はゾーンの NFS プラン一覧を表示します。
type PlansCommand struct {
	Zone string `flag:"" name:"zone" required:"" placeholder:"ZONE" help:"必須: プラン一覧を取得するゾーン名 (is1a など)。"`
	All  bool   `flag:"" name:"all" help:"利用不可 (disabled) のプランも含めて出力します。省略時は利用可能 (available) なプランのみを出力します。"`

	factory ServiceFactory
	runtime Runtime
}

func (c *PlansCommand) Run(kongCtx *kong.Context) error {
	if err := c.runtime.validateOutput(kongCtx); err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	plans, err := svc.ListPlans(ctx, c.Zone, c.All)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(kongCtx, plans)
}
