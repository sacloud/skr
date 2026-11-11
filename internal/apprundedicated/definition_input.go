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

package apprundedicated

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	apprundedicatedservice "github.com/sacloud/skr/service/apprun-dedicated-service-go"
)

// loadDefinition は --app の指定から定義を読み込みます。
//
// "-" は標準入力から EOF まで読み込み、それ以外はパスとして扱います。空文字列は
// エラーにします。値をインラインで指定する経路は設けません。定義には秘密情報
// (registryPassword、secret=true の環境変数の値) を含められるためです。
// 解釈は service 層の ParseDefinition を使い、未知のフィールドと末尾の余分な
// データを拒否したうえで、識別子として必要なフィールドを検証します。
func loadDefinition(stdin io.Reader, input string) (*apprundedicatedservice.ApplicationDefinition, error) {
	if input == "" {
		return nil, fmt.Errorf("--app に定義ファイルのパス、または - (標準入力) を指定してください")
	}
	var data []byte
	if input == "-" {
		var err error
		data, err = io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("--app - の標準入力を読み込めません: %w", err)
		}
	} else {
		var err error
		data, err = os.ReadFile(input) // #nosec G304 -- パスは --app で利用者が明示的に指定したものです。
		if err != nil {
			// --app の値と os.PathError (指定された値を含む) をエラーに含めません。
			// 利用者が誤って秘密情報を含む値をインラインで指定した場合に、
			// それがエラーメッセージへ漏れないようにするためです。
			// 原因の区別は errors.Is で行います。
			switch {
			case errors.Is(err, fs.ErrNotExist):
				return nil, fmt.Errorf("--app で指定された定義ファイルが存在しません")
			case errors.Is(err, fs.ErrPermission):
				return nil, fmt.Errorf("--app で指定された定義ファイルを読み取る権限がありません")
			default:
				return nil, fmt.Errorf("--app で指定された定義ファイルを読み込めません")
			}
		}
	}
	def, err := apprundedicatedservice.ParseDefinition(data)
	if err != nil {
		return nil, err
	}
	if err := def.Validate(); err != nil {
		return nil, err
	}
	return def, nil
}
