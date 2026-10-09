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

package containerregistryapi

import (
	"context"
	"fmt"
	"regexp"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	sdk "github.com/sacloud/sacloud-sdk-go/service/iaas/containerregistry"
)

type API interface {
	Find(context.Context, *sdk.FindRequest) ([]*iaas.ContainerRegistry, error)
	Read(context.Context, *sdk.ReadRequest) (*iaas.ContainerRegistry, error)
	Create(context.Context, *sdk.CreateRequest) (*iaas.ContainerRegistry, error)
	Update(context.Context, *sdk.UpdateRequest) (*iaas.ContainerRegistry, error)
	Delete(context.Context, *sdk.DeleteRequest) error
	ListUsers(context.Context, types.ID) ([]User, error)
	AddUser(context.Context, types.ID, *iaas.ContainerRegistryUserCreateRequest) error
	UpdateUser(context.Context, types.ID, string, *iaas.ContainerRegistryUserUpdateRequest) error
	DeleteUser(context.Context, types.ID, string) error
}

type APIFactory func() (API, error)

type Runtime struct {
	ValidateOutput func(*kong.Context) error
	WriteOutput    func(*kong.Context, any) error
	ReadPassword   func(string) (string, error)
}

type Commands struct {
	Registry RegistryCommands `cmd:"" help:"コンテナレジストリと、その認証に使うユーザーを管理します。"`
}

type RegistryCommands struct {
	List   RegistryListCommand   `cmd:"" help:"コンテナレジストリを検索します。"`
	Create RegistryCreateCommand `cmd:"" help:"コンテナレジストリを作成します。"`
	Read   RegistryReadCommand   `cmd:"" help:"ID を指定してコンテナレジストリを読み取ります。"`
	Update RegistryUpdateCommand `cmd:"" help:"コンテナレジストリの説明、タグ、アイコン、独自ドメインを更新します。"`
	Delete RegistryDeleteCommand `cmd:"" help:"ID を指定してコンテナレジストリを削除します。"`
	User   UserCommands          `cmd:"" help:"コンテナレジストリの認証ユーザーを管理します。"`

	factory APIFactory
	runtime Runtime
}

func (c *Commands) SetRuntime(runtime Runtime) {
	c.Registry.setRuntime(runtime)
}

func (c *Commands) SetFactory(factory APIFactory) {
	c.Registry.setFactory(factory)
}

func (c *RegistryCommands) setRuntime(runtime Runtime) {
	c.runtime = runtime
	c.List.runtime = runtime
	c.Create.runtime = runtime
	c.Read.runtime = runtime
	c.Update.runtime = runtime
	c.Delete.runtime = runtime
	c.User.setRuntime(runtime)
}

func (c *RegistryCommands) setFactory(factory APIFactory) {
	c.factory = factory
	c.List.factory = factory
	c.Create.factory = factory
	c.Read.factory = factory
	c.Update.factory = factory
	c.Delete.factory = factory
	c.User.setFactory(factory)
}

type RegistryListCommand struct {
	Request *string `name:"request" help:"任意: 検索条件の JSON を直接または @path.json で指定します。Names、Tags、Sort、Count、From で検索条件を指定できます。例: --request='{\"Names\":[\"example-registry\"]}'"`
	factory APIFactory
	runtime Runtime
}

func (c *RegistryListCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	request := &sdk.FindRequest{}
	if c.Request != nil {
		if err := DecodeRequest(*c.Request, request); err != nil {
			return err
		}
	}
	api, err := c.newAPI()
	if err != nil {
		return err
	}
	result, err := api.Find(context.Background(), request)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type RegistryCreateInput struct {
	Name          string     `json:"Name"`
	Description   string     `json:"Description"`
	Tags          types.Tags `json:"Tags"`
	IconID        types.ID   `json:"IconID"`
	VirtualDomain string     `json:"VirtualDomain"`
}

type RegistryCreateCommand struct {
	Request       *string   `name:"request" help:"JSON を直接または @path.json で指定します。個別フラグとは相互排他です。Name は必須で、レジストリ接続名にも使われます。Description、Tags、IconID、VirtualDomain を指定できます。Tags は JSON 専用です。Users、パスワード、廃止された公開設定は指定できません。例: --request @registry.json"`
	Name          *string   `name:"name" help:"フラグ入力では必須: API の Name とレジストリ接続名です。小文字英字から始まり英数字で終わる小文字英数字・ハイフンの名前です。他ユーザーと重複できず、作成後は変更できません。例: --name example-registry"`
	Description   *string   `name:"description" help:"任意: API の Description。最大512文字です。--request とは相互排他です。"`
	IconID        *types.ID `name:"icon-id" help:"任意: API の IconID。--request とは相互排他です。"`
	VirtualDomain *string   `name:"virtual-domain" help:"任意: API の VirtualDomain。対応する CNAME を設定してください。--request とは相互排他です。"`
	factory       APIFactory
	runtime       Runtime
}

func (c *RegistryCreateCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	var input RegistryCreateInput
	hasFlags := c.Name != nil || c.Description != nil || c.IconID != nil || c.VirtualDomain != nil
	if c.Request != nil {
		if hasFlags {
			return fmt.Errorf("--request と個別フラグは同時に指定できません")
		}
		if err := DecodeRequest(*c.Request, &input); err != nil {
			return err
		}
	} else {
		if c.Name == nil {
			return fmt.Errorf("--name または --request が必要です")
		}
		input.Name = *c.Name
		if c.Description != nil {
			input.Description = *c.Description
		}
		if c.IconID != nil {
			input.IconID = *c.IconID
		}
		if c.VirtualDomain != nil {
			input.VirtualDomain = *c.VirtualDomain
		}
	}
	if err := validateRegistryName(input.Name); err != nil {
		return err
	}
	request := &sdk.CreateRequest{
		Name:           input.Name,
		Description:    input.Description,
		Tags:           input.Tags,
		IconID:         input.IconID,
		VirtualDomain:  input.VirtualDomain,
		SubDomainLabel: input.Name,
	}
	api, err := c.newAPI()
	if err != nil {
		return err
	}
	result, err := api.Create(context.Background(), request)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type RegistryUpdateInput struct {
	Description   *string     `json:"Description"`
	Tags          *types.Tags `json:"Tags"`
	IconID        *types.ID   `json:"IconID"`
	VirtualDomain *string     `json:"VirtualDomain"`
}

type RegistryReadCommand struct {
	ID      types.ID `arg:"" name:"id" help:"読み取るコンテナレジストリ ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *RegistryReadCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	api, err := c.newAPI()
	if err != nil {
		return err
	}
	result, err := api.Read(context.Background(), &sdk.ReadRequest{ID: c.ID})
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type RegistryUpdateCommand struct {
	ID      types.ID `arg:"" name:"id" help:"更新するコンテナレジストリ ID。"`
	Request string   `name:"request" required:"" help:"必須: JSON を直接または @path.json で指定します。Description、Tags、IconID、VirtualDomain を指定できます。指定したフィールドだけを更新し、省略したフィールドは変更しません。Name は作成後に変更できません。独自ドメインを使う場合は対応する CNAME も設定します。例: --request @registry-update.json"`
	factory APIFactory
	runtime Runtime
}

func (c *RegistryUpdateCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	var input RegistryUpdateInput
	if err := DecodeRequest(c.Request, &input); err != nil {
		return err
	}
	request := &sdk.UpdateRequest{
		ID:            c.ID,
		Description:   input.Description,
		Tags:          input.Tags,
		IconID:        input.IconID,
		VirtualDomain: input.VirtualDomain,
	}
	api, err := c.newAPI()
	if err != nil {
		return err
	}
	result, err := api.Update(context.Background(), request)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type RegistryDeleteCommand struct {
	ID      types.ID `arg:"" name:"id" help:"削除するコンテナレジストリ ID。"`
	factory APIFactory
	runtime Runtime
}

func (c *RegistryDeleteCommand) Run(ctx *kong.Context) error {
	api, err := c.newAPI()
	if err != nil {
		return err
	}
	return api.Delete(context.Background(), &sdk.DeleteRequest{ID: c.ID})
}

type UserCommands struct {
	List   UserListCommand   `cmd:"" help:"コンテナレジストリのユーザーを一覧表示します。"`
	Add    UserAddCommand    `cmd:"" help:"認証ユーザーを追加します。パスワードは保護されたファイルまたは標準入力から読み込みます。"`
	Update UserUpdateCommand `cmd:"" help:"認証ユーザーのパスワードまたは権限を更新します。"`
	Delete UserDeleteCommand `cmd:"" help:"認証ユーザーを削除します。"`

	factory APIFactory
	runtime Runtime
}

func (c *UserCommands) setRuntime(runtime Runtime) {
	c.runtime = runtime
	c.List.runtime = runtime
	c.Add.runtime = runtime
	c.Update.runtime = runtime
	c.Delete.runtime = runtime
}

func (c *UserCommands) setFactory(factory APIFactory) {
	c.factory = factory
	c.List.factory = factory
	c.Add.factory = factory
	c.Update.factory = factory
	c.Delete.factory = factory
}

type User struct {
	UserName   string                             `json:"UserName"`
	Permission types.EContainerRegistryPermission `json:"Permission"`
}

type UserListCommand struct {
	RegistryID types.ID `arg:"" name:"registry-id" help:"ユーザーを一覧表示するコンテナレジストリ ID。"`
	factory    APIFactory
	runtime    Runtime
}

func (c *UserListCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	api, err := c.newAPI()
	if err != nil {
		return err
	}
	result, err := api.ListUsers(context.Background(), c.RegistryID)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

type UserAddCommand struct {
	RegistryID   types.ID `arg:"" name:"registry-id" help:"ユーザーを追加するコンテナレジストリ ID。"`
	UserName     string   `name:"user-name" required:"" help:"必須: 1〜63文字。英数字と - + . @ _ を指定できます。"`
	PasswordFile string   `name:"password-file" required:"" help:"必須: 1〜63文字のパスワードが入った保護されたファイル。- を指定すると標準入力から読み込みます。秘密情報を引数に直接指定しないでください。"`
	Permission   string   `name:"permission" required:"" help:"必須: all、readwrite、readonly のいずれか。"`
	factory      APIFactory
	runtime      Runtime
}

func (c *UserAddCommand) Run(ctx *kong.Context) error {
	if err := validateUserName(c.UserName); err != nil {
		return err
	}
	permission, err := parsePermission(c.Permission)
	if err != nil {
		return err
	}
	password, err := c.runtime.readPassword(c.PasswordFile)
	if err != nil {
		return err
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	api, err := c.newAPI()
	if err != nil {
		return err
	}
	return api.AddUser(context.Background(), c.RegistryID, &iaas.ContainerRegistryUserCreateRequest{
		UserName:   c.UserName,
		Password:   password,
		Permission: permission,
	})
}

type UserUpdateCommand struct {
	RegistryID   types.ID `arg:"" name:"registry-id" help:"ユーザーを更新するコンテナレジストリ ID。"`
	UserName     string   `arg:"" name:"user-name" help:"更新するユーザー名。"`
	PasswordFile *string  `name:"password-file" help:"任意: 新しい1〜63文字のパスワードが入った保護されたファイル。- を指定すると標準入力から読み込みます。秘密情報を引数に直接指定しないでください。"`
	Permission   *string  `name:"permission" help:"任意: 変更後の権限 all、readwrite、readonly。"`
	factory      APIFactory
	runtime      Runtime
}

func (c *UserUpdateCommand) Run(ctx *kong.Context) error {
	if err := validateUserName(c.UserName); err != nil {
		return err
	}
	if c.PasswordFile == nil && c.Permission == nil {
		return fmt.Errorf("--password-file または --permission が必要です")
	}
	request := &iaas.ContainerRegistryUserUpdateRequest{}
	if c.PasswordFile != nil {
		password, err := c.runtime.readPassword(*c.PasswordFile)
		if err != nil {
			return err
		}
		if err := validatePassword(password); err != nil {
			return err
		}
		request.Password = password
	}
	if c.Permission != nil {
		permission, err := parsePermission(*c.Permission)
		if err != nil {
			return err
		}
		request.Permission = permission
	}
	api, err := c.newAPI()
	if err != nil {
		return err
	}
	if c.Permission == nil {
		users, err := api.ListUsers(context.Background(), c.RegistryID)
		if err != nil {
			return err
		}
		found := false
		for _, user := range users {
			if user.UserName != c.UserName {
				continue
			}
			if found {
				return fmt.Errorf("対象ユーザーが重複しています")
			}
			permission, err := parsePermission(string(user.Permission))
			if err != nil {
				return err
			}
			request.Permission = permission
			found = true
		}
		if !found {
			return fmt.Errorf("対象ユーザーが見つかりません")
		}
	}
	return api.UpdateUser(context.Background(), c.RegistryID, c.UserName, request)
}

type UserDeleteCommand struct {
	RegistryID types.ID `arg:"" name:"registry-id" help:"ユーザーを削除するコンテナレジストリ ID。"`
	UserName   string   `arg:"" name:"user-name" help:"削除するユーザー名。"`
	factory    APIFactory
	runtime    Runtime
}

func (c *UserDeleteCommand) Run(ctx *kong.Context) error {
	if err := validateUserName(c.UserName); err != nil {
		return err
	}
	api, err := c.newAPI()
	if err != nil {
		return err
	}
	return api.DeleteUser(context.Background(), c.RegistryID, c.UserName)
}

var (
	registryNamePattern = regexp.MustCompile(`^[a-z](?:[a-z0-9-]*[a-z0-9])?$`)
	userNamePattern     = regexp.MustCompile(`^[A-Za-z0-9.+@_-]{1,63}$`)
	passwordPattern     = regexp.MustCompile(`^[A-Za-z0-9.@_*-]{1,63}$`)
)

func validateUserName(name string) error {
	if !userNamePattern.MatchString(name) {
		return fmt.Errorf("ユーザー名は1〜63文字で、英数字と - + . @ _ のみ指定できます")
	}
	return nil
}

func validateRegistryName(name string) error {
	if !registryNamePattern.MatchString(name) {
		return fmt.Errorf("レジストリ名は小文字英字で始まり、小文字英数字またはハイフンで構成し、英数字で終わる形式にしてください")
	}
	return nil
}

func validatePassword(password string) error {
	if !passwordPattern.MatchString(password) {
		return fmt.Errorf("パスワードファイルには1〜63文字の英数字または - . @ _ * のみ指定できます")
	}
	return nil
}

func parsePermission(value string) (types.EContainerRegistryPermission, error) {
	switch permission := types.EContainerRegistryPermission(value); permission {
	case types.ContainerRegistryPermissions.All, types.ContainerRegistryPermissions.ReadWrite, types.ContainerRegistryPermissions.ReadOnly:
		return permission, nil
	default:
		return "", fmt.Errorf("権限は all、readwrite、readonly のいずれかを指定してください")
	}
}

func (c *RegistryListCommand) newAPI() (API, error)   { return openAPI(c.factory) }
func (c *RegistryCreateCommand) newAPI() (API, error) { return openAPI(c.factory) }
func (c *RegistryReadCommand) newAPI() (API, error)   { return openAPI(c.factory) }
func (c *RegistryUpdateCommand) newAPI() (API, error) { return openAPI(c.factory) }
func (c *RegistryDeleteCommand) newAPI() (API, error) { return openAPI(c.factory) }
func (c *UserListCommand) newAPI() (API, error)       { return openAPI(c.factory) }
func (c *UserAddCommand) newAPI() (API, error)        { return openAPI(c.factory) }
func (c *UserUpdateCommand) newAPI() (API, error)     { return openAPI(c.factory) }
func (c *UserDeleteCommand) newAPI() (API, error)     { return openAPI(c.factory) }

func openAPI(factory APIFactory) (API, error) {
	if factory == nil {
		return nil, fmt.Errorf("API クライアントが設定されていません")
	}
	return factory()
}

func (r Runtime) validateOutput(ctx *kong.Context) error {
	if r.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.ValidateOutput(ctx)
}

func (r Runtime) writeOutput(ctx *kong.Context, value any) error {
	if r.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.WriteOutput(ctx, value)
}

func (r Runtime) readPassword(path string) (string, error) {
	if r.ReadPassword == nil {
		return "", fmt.Errorf("パスワード入力処理が設定されていません")
	}
	return r.ReadPassword(path)
}
