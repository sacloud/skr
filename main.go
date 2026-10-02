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
)

type cli struct {
	Config      configCommand      `cmd:"" help:"Manage configuration profiles."`
	EventbusAPI eventbusAPICommand `cmd:"" name:"eventbus-api" help:"スケジュールまたはイベント検知をきっかけにジョブを実行する EventBus を操作します。実行先を process-configuration で定義し、schedule または trigger から参照します。認証には SDK のプロファイル、または SAKURA_ACCESS_TOKEN / SAKURA_ACCESS_TOKEN_SECRET 環境変数を使用します。結果は JSON で出力します。ジョブ実行はベストエフォート型で、厳密なリアルタイム性は保証されません。詳細: https://manual.sakura.ad.jp/cloud/appliance/eventbus/about.html"`
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
	commandLine := newCLI()
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
