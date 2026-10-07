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

package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"
	"github.com/sacloud/skr/internal/apprundedicatedapi"
	"github.com/sacloud/skr/internal/eventbusapi"
	"github.com/sacloud/skr/internal/simplemqapi"
	"github.com/sacloud/skr/version"
)

type cli struct {
	Trace              bool                        `name:"trace" help:"SDK の HTTP リクエストとレスポンスをトレースします。認証情報などが出力される場合があります。"`
	Version            kong.VersionFlag            `name:"version" help:"Print version information and quit."`
	Query              *string                     `name:"query" help:"jq 式で API の JSON 出力を加工します。例: --query 'map({ID,Name})'"`
	VersionCmd         versionCommand              `cmd:"" name:"version" help:"Print version information."`
	Config             configCommand               `cmd:"" help:"Manage configuration profiles."`
	IaaSAPI            iaasAPICommand              `cmd:"" name:"iaas-api" help:"さくらのクラウド IaaS API を操作します。SDK のプロファイル、または SAKURA_ACCESS_TOKEN / SAKURA_ACCESS_TOKEN_SECRET 環境変数で認証します。結果は JSON で出力します。--query で必要な項目を抽出できます。"`
	EventbusAPI        eventbusapi.Commands        `cmd:"" name:"eventbus-api" help:"スケジュールまたはイベント検知をきっかけにジョブを実行する EventBus を操作します。実行先を process-configuration で定義し、schedule または trigger から参照します。認証には SDK のプロファイル、または SAKURA_ACCESS_TOKEN / SAKURA_ACCESS_TOKEN_SECRET 環境変数を使用します。結果は JSON で出力します。--query で必要な項目を抽出できます。ジョブ実行はベストエフォート型で、厳密なリアルタイム性は保証されません。詳細: https://manual.sakura.ad.jp/cloud/appliance/eventbus/about.html"`
	SimpleMQAPI        simplemqapi.Commands        `cmd:"" name:"simplemq-api" help:"SimpleMQ のキュー管理 API とメッセージ API を操作します。キュー管理 API は SDK プロファイル、または SAKURA_ACCESS_TOKEN / SAKURA_ACCESS_TOKEN_SECRET 環境変数で認証します。メッセージ API はキューの API キーを --api-key-file から読み込みます。結果は JSON で出力します。--query で必要な項目を抽出できます。"`
	AppRunDedicatedAPI apprundedicatedapi.Commands `cmd:"" name:"apprun-dedicated-api" help:"専用ワーカノード上でコンテナを実行する AppRun 専有型のクラスタ、アプリケーションなどを操作します。SDK プロファイル、または SAKURA_ACCESS_TOKEN / SAKURA_ACCESS_TOKEN_SECRET 環境変数で認証します。結果は JSON で出力します。--query で必要な項目を抽出できます。詳細: https://manual.sakura.ad.jp/cloud/apprun-dedicated/about.html"`
	HTTP               httpCommand                 `cmd:"" name:"http" help:"SDK の認証情報を使って HTTPS URL に直接リクエストを送ります。指定したホストへアカウントの認証情報が送信されるため、信頼できる API エンドポイントだけを指定してください。レスポンス本文は形式を変えずに出力し、--query による加工は行いません。例: skr http 'https://api.example.test/path' --method GET"`
}

type configCommand struct {
	Current currentCommand `cmd:"" help:"Print the current profile name."`
	List    listCommand    `cmd:"" help:"List configuration profiles."`
	Show    showCommand    `cmd:"" help:"Show a configuration profile."`
	Create  createCommand  `cmd:"" help:"コントロールパネルで作成したサービスプリンシパルキーを含む profile を新規作成します。"`
	Edit    editCommand    `cmd:"" help:"既存 profile のサービスプリンシパルキー認証情報を編集します。"`
	Use     useCommand     `cmd:"" help:"Set the current configuration profile."`
}

type versionCommand struct{}

func (versionCommand) Run(ctx *kong.Context) error {
	_, err := fmt.Fprintln(ctx.Stdout, version.FullVersion())
	return err
}

type currentCommand struct{}

func (currentCommand) Run(ctx *kong.Context) error {
	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		return err
	}

	name, err := profileOp.GetCurrentName()
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(ctx.Stdout, name)
	return err
}

func Run(args []string, stdout, stderr io.Writer) int {
	return runCLI(args, stdout, stderr, newCLI())
}

func run(args []string, stdout, stderr io.Writer) int {
	return Run(args, stdout, stderr)
}

func newCLI() *cli {
	result := &cli{}
	result.initIaaSAPI()
	result.initEventbusAPI()
	result.initSimpleMQAPI()
	result.initAppRunDedicatedAPI()
	result.initHTTP()
	return result
}

func runCLI(args []string, stdout, stderr io.Writer, commandLine *cli) int {
	exitCode := -1
	parser, err := kong.New(
		commandLine,
		kong.Name("skr"),
		kong.Description("CLI for Sakura Cloud."),
		kong.Writers(stdout, stderr),
		kong.Exit(func(code int) { exitCode = code }),
		kong.Vars{"version": version.FullVersion()},
	)
	if err != nil {
		if _, writeErr := fmt.Fprintln(stderr, err); writeErr != nil {
			return 1
		}
		return 1
	}

	ctx, err := parser.Parse(args)
	if exitCode >= 0 {
		return exitCode
	}
	if err == nil {
		err = ctx.Run()
	}
	if err != nil {
		if _, writeErr := fmt.Fprintln(stderr, err); writeErr != nil {
			return 1
		}
		return 1
	}
	return 0
}
