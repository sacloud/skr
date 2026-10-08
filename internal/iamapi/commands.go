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

package iamapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/iam"
	"github.com/sacloud/sacloud-sdk-go/api/iam/apis/group"
	"github.com/sacloud/sacloud-sdk-go/api/iam/apis/user"
	v1 "github.com/sacloud/sacloud-sdk-go/api/iam/apis/v1"
	clientconfig "github.com/sacloud/skr/internal/saclient"
)

type Commands struct {
	User   UserCommands   `cmd:"" help:"IAM ユーザーを作成・管理します。ユーザー ID は list で確認できます。"`
	Group  GroupCommands  `cmd:"" help:"IAM グループを作成・管理します。グループへの所属は update-memberships で行います。"`
	Policy PolicyCommands `cmd:"" help:"IAM ポリシーバインディング（ロールとプリンシパルの割り当て）を参照・更新します。更新は現在の割り当て全体を置き換えます。"`
}

type UserListCommand struct {
	Page     *int    `name:"page" help:"任意: 取得するページ番号（1 開始）。"`
	PerPage  *int    `name:"per-page" help:"任意: 1 ページあたりの取得件数。"`
	Ordering *string `name:"ordering" help:"任意: 並び順。code または -code を指定します。"`
	factory  UserAPIFactory
	runtime  UserRuntime
}

func (c *UserListCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	params := user.ListParams{Page: c.Page, PerPage: c.PerPage}
	if c.Ordering != nil {
		ordering := v1.ListUsersOrdering(*c.Ordering)
		params.Ordering = &ordering
	}
	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	items, err := op.List(context.Background(), params)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, items)
}

type UserCreateCommand struct {
	Name         *string `name:"name" help:"必須: ユーザー名。"`
	Code         *string `name:"code" help:"必須: ユーザーコード。"`
	Description  *string `name:"description" help:"必須: ユーザーの説明。空文字列も指定できます。"`
	Email        *string `name:"email" help:"任意: SSO プロファイル有効時に外部 IdP のログインで利用するメールアドレス。ユーザーの通知用メールアドレスは register-email で登録します。"`
	PasswordFile string  `name:"password-file" help:"必須: パスワードを記載したファイル。- の場合は標準入力から読み込みます。パスワードは英数字と ASCII 記号のみで、英字と数字を必ず含めます。組織のパスワードポリシーに従います。"`
	factory      UserAPIFactory
	runtime      UserRuntime
}

func (c *UserCreateCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	if c.Name == nil || *c.Name == "" {
		return fmt.Errorf("--name が必要です")
	}
	if c.Code == nil || *c.Code == "" {
		return fmt.Errorf("--code が必要です")
	}
	if c.Description == nil {
		return fmt.Errorf("--description が必要です（空文字列は --description '' と指定します）")
	}
	password, err := readPassword(c.PasswordFile)
	if err != nil {
		return err
	}
	params := user.CreateParams{
		Name:        *c.Name,
		Password:    password,
		Code:        *c.Code,
		Description: *c.Description,
		Email:       c.Email,
	}
	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	created, err := op.Create(context.Background(), params)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, created)
}

type UserUpdateCommand struct {
	ID           int     `arg:"" help:"更新するユーザー ID。"`
	Name         *string `name:"name" help:"必須: ユーザー名。"`
	Description  *string `name:"description" help:"必須: ユーザーの説明。空文字列も指定できます。"`
	PasswordFile *string `name:"password-file" help:"任意: 変更後のパスワードを記載したファイル。- の場合は標準入力から読み込みます。未指定のときはパスワードを変更しません。"`
	factory      UserAPIFactory
	runtime      UserRuntime
}

func (c *UserUpdateCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	if c.Name == nil || *c.Name == "" {
		return fmt.Errorf("--name が必要です")
	}
	if c.Description == nil {
		return fmt.Errorf("--description が必要です（空文字列は --description '' と指定します）")
	}
	params := user.UpdateParams{Name: *c.Name, Description: *c.Description}
	if c.PasswordFile != nil {
		password, err := readPassword(*c.PasswordFile)
		if err != nil {
			return err
		}
		params.Password = &password
	}
	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	updated, err := op.Update(context.Background(), c.ID, params)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, updated)
}

type GroupListCommand struct {
	Page     *int    `name:"page" help:"任意: 取得するページ番号（1 開始）。"`
	PerPage  *int    `name:"per-page" help:"任意: 1 ページあたりの取得件数。"`
	Ordering *string `name:"ordering" help:"任意: 並び順。name または -name を指定します。"`
	UserID   *int    `name:"user-id" help:"任意: 指定したユーザーが所属するグループに絞り込みます。"`
	factory  GroupAPIFactory
	runtime  GroupRuntime
}

func (c *GroupListCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	params := group.ListParams{Page: c.Page, PerPage: c.PerPage}
	if c.Ordering != nil {
		ordering := v1.ListGroupsOrdering(*c.Ordering)
		params.Ordering = &ordering
	}
	if c.UserID != nil {
		params.User = &v1.User{ID: *c.UserID}
	}
	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	items, err := op.List(context.Background(), params)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, items)
}

func (r UserRuntime) validateOutput(ctx *kong.Context) error {
	if r.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.ValidateOutput(ctx)
}

func (r UserRuntime) writeOutput(ctx *kong.Context, value any) error {
	if r.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.WriteOutput(ctx, value)
}

func (r GroupRuntime) validateOutput(ctx *kong.Context) error {
	if r.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.ValidateOutput(ctx)
}

func (r GroupRuntime) writeOutput(ctx *kong.Context, value any) error {
	if r.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.WriteOutput(ctx, value)
}

// userAPI wraps the SDK UserAPI so that list output is the items array.
// The SDK returns paging information (count, next, previous) with the items;
// skr keeps list output a plain JSON array like the other domains.
type userAPI struct{ user.UserAPI }

func (a userAPI) List(ctx context.Context, params user.ListParams) ([]v1.User, error) {
	result, err := a.UserAPI.List(ctx, params)
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

// groupAPI wraps the SDK GroupAPI so that list output is the items array.
type groupAPI struct{ group.GroupAPI }

func (a groupAPI) List(ctx context.Context, params group.ListParams) ([]v1.Group, error) {
	result, err := a.GroupAPI.List(ctx, params)
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

func NewUserAPI(trace bool) (UserAPI, error) {
	client, err := newIAMClient(trace)
	if err != nil {
		return nil, err
	}
	return userAPI{user.NewUserOp(client)}, nil
}

func NewGroupAPI(trace bool) (GroupAPI, error) {
	client, err := newIAMClient(trace)
	if err != nil {
		return nil, err
	}
	return groupAPI{group.NewGroupOp(client)}, nil
}

func NewPolicyAPI(trace bool) (PolicyAPI, error) {
	client, err := newIAMClient(trace)
	if err != nil {
		return nil, err
	}
	return iam.NewIAMPolicyOp(client), nil
}

func newIAMClient(trace bool) (*v1.Client, error) {
	client, err := clientconfig.New(trace)
	if err != nil {
		return nil, err
	}
	return iam.NewClient(client)
}

// DecodeRequest decodes --request JSON, supporting the @path.json form.
func DecodeRequest(input string, destination any) error {
	if strings.HasPrefix(input, "@") {
		data, err := os.ReadFile(strings.TrimPrefix(input, "@"))
		if err != nil {
			return fmt.Errorf("read request file: %w", err)
		}
		if err := json.Unmarshal(data, destination); err != nil {
			return fmt.Errorf("decode request JSON: %w", err)
		}
		return nil
	}
	if err := json.Unmarshal([]byte(input), destination); err != nil {
		return fmt.Errorf("decode request JSON: %w", err)
	}
	return nil
}

// readPassword reads a password from a file or standard input.
func readPassword(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("--password-file が必要です（標準入力を使う場合は - を指定します）")
	}
	var data []byte
	if path == "-" {
		var err error
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read password from standard input: %w", err)
		}
	} else {
		var err error
		//nolint:gosec // The path is explicitly supplied by the user through --password-file.
		data, err = os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read password file: %w", err)
		}
	}
	password := strings.TrimSpace(string(data))
	if password == "" {
		return "", fmt.Errorf("password is empty")
	}
	return password, nil
}
