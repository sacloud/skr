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

// Package secretmanagerapi はさくらのクラウド シークレットマネージャーの
// コマンド実装を提供します。
package secretmanagerapi

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	secretmanager "github.com/sacloud/sacloud-sdk-go/api/secretmanager"
	v1 "github.com/sacloud/sacloud-sdk-go/api/secretmanager/apis/v1"
	clientconfig "github.com/sacloud/skr/internal/saclient"
)

// Commands は secretmanager-api のコマンド群です。
type Commands struct {
	Vault  VaultCommands  `cmd:"" help:"シークレット保管庫 (Vault) を作成・管理します。Vault 単位で課金が行われ、シークレットは Vault に最新を含む 50 世代まで保管されます。"`
	Secret SecretCommands `cmd:"" help:"Vault 内のシークレットを操作します。値の入出力はファイルまたは標準入出力経由で行い、コマンド引数に含めません。"`
}

// SecretCreateCommand はシークレットの作成・更新コマンドです。値を引数や
// インライン JSON で受け取らないため、手書きで実装しています。
type SecretCreateCommand struct {
	VaultID   string  `arg:"" help:"シークレットを作成する Vault のリソース ID。"`
	Name      *string `name:"name" required:"" help:"必須: シークレット名 (1〜255 文字)。例: db-password"`
	ValueFile *string `name:"value-file" required:"" help:"必須: シークレットの値のファイル。- の場合は標準入力から読み込みます。前後の空白は除去されます。値を直接引数に指定することはできません。"`
	factory   SecretAPIFactory
	runtime   SecretRuntime
}

func (c *SecretCreateCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	if c.Name == nil {
		return fmt.Errorf("--name が必要です")
	}
	if c.ValueFile == nil {
		return fmt.Errorf("--value-file が必要です")
	}
	data, err := readSecretFile(*c.ValueFile)
	if err != nil {
		return err
	}
	value, err := secretValue(data)
	if err != nil {
		return err
	}

	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	result, err := op.Create(context.Background(), c.VaultID, v1.CreateSecretRequest{Name: *c.Name, Value: value})
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

// SecretUnveilCommand はシークレットの値を取得するコマンドです。値をそのまま
// 標準出力へ出力するため --query は適用されず、手書きで実装しています。
type SecretUnveilCommand struct {
	VaultID string `arg:"" help:"シークレットが属する Vault のリソース ID。"`
	Name    string `arg:"" help:"取得するシークレット名。"`
	Version *int   `name:"secret-version" help:"任意: 取得するバージョン番号。省略すると最新バージョンを取得します。"`
	factory SecretAPIFactory
	runtime SecretRuntime
}

func (c *SecretUnveilCommand) Run(ctx *kong.Context) error {
	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	result, err := op.Unveil(context.Background(), c.VaultID, c.Name, c.Version)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(ctx.Stdout, result.Value)
	return err
}

// secretOp は Vault のリソース ID をメソッド引数で受け取り、SDK の SecretOp を
// 生成して委譲する SecretAPI 実装です。SDK の Secret 操作は Vault スコープの
// クライアント (NewSecretOp) で提供されるため、Vault ID をコマンドの位置引数で
// 受け付けるためにこのアダプタを介します。
type secretOp struct {
	client *v1.Client
}

func (op *secretOp) List(ctx context.Context, vaultID string, count, from *int) ([]v1.SecretResponse, error) {
	return secretmanager.NewSecretOp(op.client, vaultID).List(ctx, count, from)
}

func (op *secretOp) Create(ctx context.Context, vaultID string, request secretmanager.CreateSecretParams) (*v1.CreateSecretResponse, error) {
	return secretmanager.NewSecretOp(op.client, vaultID).Create(ctx, request)
}

func (op *secretOp) Delete(ctx context.Context, vaultID string, name string) error {
	return secretmanager.NewSecretOp(op.client, vaultID).Delete(ctx, name)
}

func (op *secretOp) Unveil(ctx context.Context, vaultID string, name string, version *int) (*v1.UnveilResponse, error) {
	return secretmanager.NewSecretOp(op.client, vaultID).Unveil(ctx, secretmanager.UnveilParams{Name: name, Version: version})
}

// NewVaultAPI は Vault コマンド用の API を構築します。
func NewVaultAPI(trace bool) (VaultAPI, error) {
	client, err := clientconfig.New(trace)
	if err != nil {
		return nil, err
	}
	smClient, err := secretmanager.NewClient(client)
	if err != nil {
		return nil, err
	}
	return secretmanager.NewVaultOp(smClient), nil
}

// NewSecretAPI は Secret コマンド用の API を構築します。
func NewSecretAPI(trace bool) (SecretAPI, error) {
	client, err := clientconfig.New(trace)
	if err != nil {
		return nil, err
	}
	smClient, err := secretmanager.NewClient(client)
	if err != nil {
		return nil, err
	}
	return &secretOp{client: smClient}, nil
}

func (r SecretRuntime) validateOutput(ctx *kong.Context) error {
	if r.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.ValidateOutput(ctx)
}

func (r SecretRuntime) writeOutput(ctx *kong.Context, value any) error {
	if r.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.WriteOutput(ctx, value)
}

// secretValue はシークレットの値を正規化します。前後の空白は除去します。
func secretValue(data []byte) (string, error) {
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("secret value file is empty")
	}
	return value, nil
}

func readSecretFile(path string) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read secret from standard input: %w", err)
		}
		return data, nil
	}
	//nolint:gosec // パスは --value-file 経由でユーザーが明示的に指定したものです。
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read secret file: %w", err)
	}
	return data, nil
}
