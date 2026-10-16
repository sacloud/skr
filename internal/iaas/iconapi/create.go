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
	"encoding/base64"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/icon"
)

type CreateCommand struct {
	Request   *string   `name:"request" help:"CreateRequest JSON を直接または @path.json で指定します。必須: Name、Base64 形式の Image。任意: Tags 配列。個別フラグとは併用できません。例: --request @icon-create.json"`
	Name      *string   `name:"name" help:"必須: 作成するアイコンの名前。"`
	ImageFile *string   `name:"image-file" help:"必須: PNG または JPEG の画像ファイル。API 用に Base64 形式へ変換します。画像データを引数に直接指定しないでください。"`
	Tags      *[]string `name:"tags" sep:"," help:"任意: タグ。カンマ区切りで指定します。"`
	factory   APIFactory
	runtime   Runtime
}

func (c *CreateCommand) Run(ctx *kong.Context) error {
	flagsSet := c.Name != nil || c.ImageFile != nil || c.Tags != nil
	if c.Request != nil && flagsSet {
		return fmt.Errorf("--request と個別フラグは併用できません")
	}
	if c.runtime.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	if err := c.runtime.ValidateOutput(ctx); err != nil {
		return err
	}

	request := &icon.CreateRequest{}
	if c.Request != nil {
		if c.runtime.DecodeRequest == nil {
			return fmt.Errorf("API リクエスト処理が設定されていません")
		}
		if err := c.runtime.DecodeRequest(*c.Request, request); err != nil {
			return err
		}
	} else {
		if c.Name == nil {
			return fmt.Errorf("--name が必要です (--request JSON も指定できます)")
		}
		if c.ImageFile == nil {
			return fmt.Errorf("--image-file が必要です (--request JSON も指定できます)")
		}
		image, err := readImageFile(*c.ImageFile)
		if err != nil {
			return err
		}
		request.Name = *c.Name
		request.Image = image
		if c.Tags != nil {
			request.Tags = types.Tags(*c.Tags)
		}
	}

	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	api, err := c.factory()
	if err != nil {
		return err
	}
	result, err := api.CreateWithContext(context.Background(), request)
	if err != nil {
		return err
	}
	if c.runtime.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return c.runtime.WriteOutput(ctx, result)
}

func readImageFile(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // The image path is explicit CLI input from the user.
	if err != nil {
		return "", fmt.Errorf("read icon image file: %w", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}
