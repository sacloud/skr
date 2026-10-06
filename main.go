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

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"
	"github.com/sacloud/skr/internal/eventbusapi"
	"github.com/sacloud/skr/internal/simplemqapi"
)

type cli struct {
	Output      *string              `name:"output" enum:"json,yaml,table" help:"出力形式 (json、yaml、table)。未指定時はプロファイルの cli.default_output_type (v0: DefaultOutputType) を使います。"`
	Config      configCommand        `cmd:"" help:"Manage configuration profiles."`
	IaaSAPI     iaasAPICommand       `cmd:"" name:"iaas-api" help:"さくらのクラウド IaaS API を操作します。SDK のプロファイル、または SAKURA_ACCESS_TOKEN / SAKURA_ACCESS_TOKEN_SECRET 環境変数で認証します。結果は --output で JSON、YAML、table の形式を選択できます。"`
	EventbusAPI eventbusapi.Commands `cmd:"" name:"eventbus-api" help:"スケジュールまたはイベント検知をきっかけにジョブを実行する EventBus を操作します。実行先を process-configuration で定義し、schedule または trigger から参照します。認証には SDK のプロファイル、または SAKURA_ACCESS_TOKEN / SAKURA_ACCESS_TOKEN_SECRET 環境変数を使用します。結果は --output で JSON、YAML、table の形式を選択できます。ジョブ実行はベストエフォート型で、厳密なリアルタイム性は保証されません。詳細: https://manual.sakura.ad.jp/cloud/appliance/eventbus/about.html"`
	SimpleMQAPI simplemqapi.Commands `cmd:"" name:"simplemq-api" help:"SimpleMQ のキュー管理 API とメッセージ API を操作します。キュー管理 API は SDK プロファイル、または SAKURA_ACCESS_TOKEN / SAKURA_ACCESS_TOKEN_SECRET 環境変数で認証します。メッセージ API はキューの API キーを --api-key-file から読み込みます。結果は --output で JSON、YAML、table を選択できます。"`
	HTTP        httpCommand          `cmd:"" name:"http" help:"SDK の認証情報を使って HTTPS URL に直接リクエストを送ります。指定したホストへアカウントの認証情報が送信されるため、信頼できる API エンドポイントだけを指定してください。レスポンス本文は形式を変えずに出力し、--output による変換は行いません。例: skr http 'https://api.example.test/path' --method GET"`
}

type configCommand struct {
	Current currentCommand `cmd:"" help:"Print the current profile name."`
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

func run(args []string, stdout, stderr io.Writer) int {
	return runCLI(args, stdout, stderr, newCLI())
}

func newCLI() cli {
	result := cli{}
	result.initIaaSAPI()
	result.initEventbusAPI()
	result.initSimpleMQAPI()
	result.initHTTP()
	return result
}

func runCLI(args []string, stdout, stderr io.Writer, commandLine cli) int {
	exitCode := -1
	parser, err := kong.New(
		&commandLine,
		kong.Name("skr"),
		kong.Description("CLI for Sakura Cloud."),
		kong.Writers(stdout, stderr),
		kong.Exit(func(code int) { exitCode = code }),
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

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
