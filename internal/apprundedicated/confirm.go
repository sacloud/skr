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
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	saclient "github.com/sacloud/sacloud-sdk-go/common/saclient"
)

// stdinIsTerminal は標準入力が端末かどうかを返します。
// パイプやリダイレクトの標準入力では確認プロンプトを出せないため false を返します。
func stdinIsTerminal() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return stat.Mode()&os.ModeCharDevice != 0
}

// readConfirm は確認プロンプトへの応答を標準入力から読みます。
//
// y を入力すると続行、n または空行を入力すると中止します。それ以外の入力は
// 再度応答を求めます。入力を読み取れない場合 (EOF) は中止します。
func readConfirm(stdin io.Reader, promptOut io.Writer) (bool, error) {
	reader := bufio.NewReader(stdin)
	for {
		// プロンプトと案内は補足情報のため、書き込みエラーを無視します (ベストエフォート)。
		_, _ = fmt.Fprint(promptOut, "続行しますか? [y/N]: ")
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			_, _ = fmt.Fprintln(promptOut)
			_, _ = fmt.Fprintln(promptOut, "確認への応答を読み取れないため中止しました。")
			return false, nil
		}
		switch strings.TrimSpace(line) {
		case "y", "Y":
			return true, nil
		case "n", "N", "":
			return false, nil
		default:
			_, _ = fmt.Fprintln(promptOut, "y または n を入力してください。")
		}
	}
}

// CheckTraceSettings は deploy の前に、SDK の実効トレース設定を検査します。
//
// deploy は registryPassword や secret=true の環境変数の値をリクエスト本文に含める
// ため、トレースが有効な場合は値がログへ出力されるおそれがあります。設定解決の
// エラーを見逃さないよう解決を先に行い、TraceMode が有効な場合は定義の読み込みと
// 送信の前に操作を実行させずにエラーを返します。認証情報を含み得る設定全体は
// 返さず、エラーメッセージにも含めません。
func CheckTraceSettings(client *saclient.Client) error {
	if _, err := client.Profile(); err != nil {
		return fmt.Errorf("SDK の設定を解決できません: %w", err)
	}
	if mode, ok := client.JSON()["TraceMode"]; ok {
		return fmt.Errorf("deploy は秘密情報を含むリクエストを送信するため、トレースを無効にしてください (有効な TraceMode: %v)", mode)
	}
	return nil
}
