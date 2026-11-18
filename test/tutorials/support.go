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

package tutorials

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"

	"github.com/itchyny/gojq"
	"github.com/sacloud/skr/internal/cli"
)

// skrParseOnly は skr のコマンドライン引数を kong でパースし、
// 実行せずに妥当性を検証します。
func skrParseOnly(args []string) error {
	return cli.ParseOnly(args)
}

// compileGojq は gojq の式がコンパイルできることを確認します。
func compileGojq(expression string) error {
	query, err := gojq.Parse(expression)
	if err != nil {
		return fmt.Errorf("parse expression: %w", err)
	}
	if _, err := gojq.Compile(query); err != nil {
		return fmt.Errorf("compile expression: %w", err)
	}
	return nil
}

// writeHarnessPrivateKey はプロファイル作成に必要なダミーの RSA 秘密鍵を
// path に書き込みます。実環境の認証情報ではありません。
func writeHarnessPrivateKey(path string) error {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate rsa key: %w", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal private key: %w", err)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil { //nolint:gosec // runner の workDir 配下にダミー鍵を書き込む。
		return fmt.Errorf("write private key: %w", err)
	}
	return nil
}
