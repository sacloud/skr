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

package zoneapi

import (
	"context"
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/zone"
)

type FindCommand struct {
	Request *string `help:"任意: FindRequest JSON を直接または @path.json で指定します。Names、Sort、Count、From で検索条件を指定できます。省略時はゾーン一覧を取得します。"`
	factory APIFactory
	runtime Runtime
}

func (c *FindCommand) Run(ctx *kong.Context) error {
	if c.runtime.OutputType == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	format, err := c.runtime.OutputType(ctx)
	if err != nil {
		return err
	}

	request := &zone.FindRequest{}
	if c.Request != nil {
		if c.runtime.DecodeRequest == nil {
			return fmt.Errorf("API リクエスト処理が設定されていません")
		}
		if err := c.runtime.DecodeRequest(*c.Request, request); err != nil {
			return err
		}
	}

	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	api, err := c.factory()
	if err != nil {
		return err
	}
	result, err := api.FindWithContext(context.Background(), request)
	if err != nil {
		return err
	}
	if c.runtime.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return c.runtime.WriteOutput(ctx, format, result)
}
