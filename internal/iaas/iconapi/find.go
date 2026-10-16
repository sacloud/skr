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
	"github.com/sacloud/sacloud-sdk-go/service/iaas/icon"
)

type FindCommand struct {
	Request *string `name:"request" help:"FindRequest JSON を直接または @path.json で指定します。Names、Tags、Scope、Sort は JSON で指定します。--count、--from とは併用できません。"`
	Count   *int    `name:"count" help:"任意: 取得件数。"`
	From    *int    `name:"from" help:"任意: 取得開始位置。"`
	factory APIFactory
	runtime Runtime
}

func (c *FindCommand) Run(ctx *kong.Context) error {
	if c.Request != nil && (c.Count != nil || c.From != nil) {
		return fmt.Errorf("--request と個別フラグは併用できません")
	}
	if c.runtime.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	if err := c.runtime.ValidateOutput(ctx); err != nil {
		return err
	}

	request := &icon.FindRequest{}
	if c.Request != nil {
		if c.runtime.DecodeRequest == nil {
			return fmt.Errorf("API リクエスト処理が設定されていません")
		}
		if err := c.runtime.DecodeRequest(*c.Request, request); err != nil {
			return err
		}
	} else {
		if c.Count != nil {
			request.Count = *c.Count
		}
		if c.From != nil {
			request.From = *c.From
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
	return c.runtime.WriteOutput(ctx, result)
}
