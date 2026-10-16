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

package iconapi

import (
	"context"
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/icon"
)

type UpdateCommand struct {
	Request *string   `name:"request" help:"UpdateRequest JSON を直接または @path.json で指定します。必須: ID。任意: Name、Tags。省略した項目は変更しません。個別フラグとは併用できません。例: --request='{\"ID\":123456789012,\"Name\":\"updated\"}'"`
	ID      *int64    `name:"id" help:"必須: 更新するアイコンの ID (数値)。"`
	Name    *string   `name:"name" help:"任意: 新しい名前。未指定なら変更しません。"`
	Tags    *[]string `name:"tags" sep:"," help:"任意: タグ。カンマ区切りで指定します。未指定なら変更しません。"`
	factory APIFactory
	runtime Runtime
}

func (c *UpdateCommand) Run(ctx *kong.Context) error {
	flagsSet := c.ID != nil || c.Name != nil || c.Tags != nil
	if c.Request != nil && flagsSet {
		return fmt.Errorf("--request と個別フラグは併用できません")
	}
	request := &icon.UpdateRequest{}
	if c.Request != nil {
		if c.runtime.DecodeRequest == nil {
			return fmt.Errorf("API リクエスト処理が設定されていません")
		}
		if err := c.runtime.DecodeRequest(*c.Request, request); err != nil {
			return err
		}
	} else {
		if c.ID == nil {
			return fmt.Errorf("--id が必要です (--request JSON も指定できます)")
		}
		request.ID = types.ID(*c.ID)
		if c.Name != nil {
			request.Name = c.Name
		}
		if c.Tags != nil {
			tags := types.Tags(*c.Tags)
			request.Tags = &tags
		}
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
	api, err := c.factory()
	if err != nil {
		return err
	}
	result, err := api.UpdateWithContext(context.Background(), request)
	if err != nil {
		return err
	}
	if result == nil {
		return fmt.Errorf("Icon update returned no result")
	}
	if result.ID.IsEmpty() {
		result.ID = request.ID
	}
	if c.runtime.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return c.runtime.WriteOutput(ctx, result)
}
