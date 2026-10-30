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

package kmsapi

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/kms"
	v1 "github.com/sacloud/sacloud-sdk-go/api/kms/apis/v1"
	clientconfig "github.com/sacloud/skr/internal/saclient"
)

type Commands struct {
	Key KeyCommands `cmd:"" help:"KMS キーを作成・管理します。キーはプロジェクト単位で最大 100 個まで作成でき、削除は猶予期間 7〜90 日の削除予約制です。"`
}

// KeyCreateCommand は KMS キーの作成コマンドです。PlainKey (BYOK) を引数や
// インライン JSON で受け取らないため、手書きで実装しています。
type KeyCreateCommand struct {
	Request      *string `help:"個別フラグと併用不可。作成リクエスト JSON を @path.json または -（標準入力）で指定します。PlainKey (BYOK の秘密鍵) を含む可能性があるため、インライン JSON は受け付けません。Name、Description、Tags、PlainKey を指定できます。"`
	Name         *string `help:"必須: キー名 (1〜255 文字)。例: my-key"`
	Description  *string `help:"任意: キーの説明。"`
	PlainKeyFile *string `name:"plain-key-file" help:"任意: BYOK モードでインポートする秘密鍵。256 bit の対称鍵を base64 エンコードした内容のファイルを指定し、- の場合は標準入力から読み込みます。"`
	factory      KeyAPIFactory
	runtime      KeyRuntime
}

func (c *KeyCreateCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	request, err := createRequestParams(c.Request, c.Name, c.Description, c.PlainKeyFile, c.runtime.DecodeRequest)
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
	key, err := op.Create(context.Background(), request)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, key)
}

// KeyChangeStatusCommand は KMS キーの状態変更コマンドです。状態の選択値を
// CLI 側で検証するため手書きで実装しています。
type KeyChangeStatusCommand struct {
	ID      string `arg:"" help:"状態を変更する KMS キー ID。"`
	Status  string `name:"status" required:"" help:"必須: 設定する状態。active (有効: 暗号化・復号可)、restricted (制限中: 復号のみ可)、suspended (一時停止中: 操作不可) のいずれか。"`
	factory KeyAPIFactory
	runtime KeyRuntime
}

func (c *KeyChangeStatusCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	status, err := changeKeyStateStatus(c.Status)
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
	changed, err := op.ChangeStatus(context.Background(), c.ID, status)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, changed)
}

// KeyEncryptCommand は KMS キーでの暗号化コマンドです。平文をコマンドライン引数に
// 含めないため、手書きで実装しています。
type KeyEncryptCommand struct {
	ID        string `arg:"" help:"暗号化に使う KMS キー ID。"`
	PlainFile string `name:"plain-file" required:"" help:"必須: 暗号化する平文のファイル。- の場合は標準入力から読み込みます。平文は API 向けに base64 エンコードされて送信されます。"`
	Algo      string `name:"algo" help:"任意: 暗号化アルゴリズム。aes-256-gcm (既定)、aes-256-cbc、aes-256-kw のいずれか。"`
	factory   KeyAPIFactory
	runtime   KeyRuntime
}

func (c *KeyEncryptCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	algo, err := encryptionRequestAlgo(c.Algo)
	if err != nil {
		return err
	}
	plain, err := readSecretFile(c.PlainFile)
	if err != nil {
		return err
	}
	if len(plain) == 0 {
		return fmt.Errorf("plain file is empty")
	}

	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	cipher, err := op.Encrypt(context.Background(), c.ID, plain, algo)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, cipher)
}

// KeyDecryptCommand は KMS キーでの復号コマンドです。暗号文を引数に含めず、
// 復号結果をバイト列のまま出力するため手書きで実装しています。
type KeyDecryptCommand struct {
	ID         string `arg:"" help:"復号に使う KMS キー ID。"`
	CipherFile string `name:"cipher-file" required:"" help:"必須: encrypt が出力した暗号文 (base64 テキスト) のファイル。- の場合は標準入力から読み込みます。encrypt の JSON 出力をリダイレクトしたファイルもそのまま使えます。復号した平文はバイト列のまま標準出力に出力され、--query は適用されません。"`
	factory    KeyAPIFactory
	runtime    KeyRuntime
}

func (c *KeyDecryptCommand) Run(ctx *kong.Context) error {
	data, err := readSecretFile(c.CipherFile)
	if err != nil {
		return err
	}
	cipher, err := decodeCipherText(data)
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
	plain, err := op.Decrypt(context.Background(), c.ID, cipher)
	if err != nil {
		return err
	}
	if _, err := ctx.Stdout.Write(plain); err != nil {
		return err
	}
	return nil
}

func NewKeyAPI(trace bool) (KeyAPI, error) {
	client, err := clientconfig.New(trace)
	if err != nil {
		return nil, err
	}
	kmsClient, err := kms.NewClient(client)
	if err != nil {
		return nil, err
	}
	return kms.NewKeyOp(kmsClient), nil
}

// createRequestParams は create の入力を kms.CreateParams に組み立てます。
// PlainKey を含む可能性があるため、--request はファイルまたは標準入力だけを受け付けます。
func createRequestParams(input, name, description, plainKeyFile *string, decode func(string, any) error) (kms.CreateParams, error) {
	flags := name != nil || description != nil || plainKeyFile != nil
	if input != nil && flags {
		return kms.CreateParams{}, fmt.Errorf("--request と個別フラグは併用できません")
	}
	var request kms.CreateParams
	if input != nil {
		if *input != "-" && (len(*input) == 0 || (*input)[0] != '@') {
			return kms.CreateParams{}, fmt.Errorf("--request には @path.json または -（標準入力）を指定してください")
		}
		if decode == nil {
			return kms.CreateParams{}, fmt.Errorf("API リクエスト処理が設定されていません")
		}
		if err := decode(*input, &request); err != nil {
			return kms.CreateParams{}, err
		}
		return request, nil
	}
	if name == nil || *name == "" {
		return kms.CreateParams{}, fmt.Errorf("--name が必要です (--request JSON も指定できます)")
	}
	request.Name = *name
	if description != nil {
		request.Description = description
	}
	if plainKeyFile != nil {
		data, err := readSecretFile(*plainKeyFile)
		if err != nil {
			return kms.CreateParams{}, err
		}
		plainKey := strings.TrimSpace(string(data))
		if plainKey == "" {
			return kms.CreateParams{}, fmt.Errorf("plain key file is empty")
		}
		request.PlainKey = &plainKey
	}
	return request, nil
}

// changeKeyStateStatus は状態の選択値を検証します。
func changeKeyStateStatus(value string) (v1.ChangeKeyStateRequestStatus, error) {
	status := v1.ChangeKeyStateRequestStatus(value)
	switch status {
	case v1.ChangeKeyStateRequestStatusActive, v1.ChangeKeyStateRequestStatusRestricted, v1.ChangeKeyStateRequestStatusSuspended:
		return status, nil
	default:
		return "", fmt.Errorf("--status には active、restricted、suspended のいずれかを指定してください")
	}
}

// encryptionRequestAlgo は暗号化アルゴリズムの選択値を検証します。未指定の場合は
// API の既定値 aes-256-gcm を返します。
func encryptionRequestAlgo(value string) (v1.EncryptionRequestAlgo, error) {
	if value == "" {
		return v1.EncryptionRequestAlgoAes256Gcm, nil
	}
	algo := v1.EncryptionRequestAlgo(value)
	switch algo {
	case v1.EncryptionRequestAlgoAes256Gcm, v1.EncryptionRequestAlgoAes256Cbc, v1.EncryptionRequestAlgoAes256Kw:
		return algo, nil
	default:
		return "", fmt.Errorf("--algo には aes-256-gcm、aes-256-cbc、aes-256-kw のいずれかを指定してください")
	}
}

func (r KeyRuntime) validateOutput(ctx *kong.Context) error {
	if r.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.ValidateOutput(ctx)
}

func (r KeyRuntime) writeOutput(ctx *kong.Context, value any) error {
	if r.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.WriteOutput(ctx, value)
}

// decodeCipherText は暗号文の入力を正規化します。encrypt の JSON 出力を
// リダイレクトしたファイル (引用符付き) と、引用符のない base64 テキストの
// 両方を受け付けます。
func decodeCipherText(data []byte) (string, error) {
	cipher := strings.TrimSpace(string(data))
	cipher = strings.Trim(cipher, "\"")
	if cipher == "" {
		return "", fmt.Errorf("cipher file is empty")
	}
	return cipher, nil
}

func readSecretFile(path string) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read secret from standard input: %w", err)
		}
		return data, nil
	}
	//nolint:gosec // パスは --plain-key-file、--plain-file、--cipher-file 経由でユーザーが明示的に指定したものです。
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read secret file: %w", err)
	}
	return data, nil
}
