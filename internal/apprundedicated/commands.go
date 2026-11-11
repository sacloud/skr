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

// Package apprundedicated は AppRun 専有型の高レベルコマンド層です。
// 定義ファイル (--app) を入力として、デプロイ、ロールバック、状態の確認など
// 10 個の操作を提供します。
//
// service 層は service/apprun-dedicated-service-go モジュールに切り出してあり
// (ADR 0077、ADR 0079)、このパッケージは定義の読み込み、確認プロンプト、
// 出力処理だけを持ちます。低レベルの apprun-dedicated-api と JSON 入力の経路には
// 変更を加えません (ADR 0027、ADR 0075)。
package apprundedicated

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/google/uuid"
	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"

	apprundedicatedservice "github.com/sacloud/skr/service/apprun-dedicated-service-go"
)

// Runtime は CLI 側から注入する出力処理と入力の集合です。
// Stdin・IsTerminal はテストでの差し替え用で、nil の場合は標準入力と
// 既定の端末判定を使います。
type Runtime struct {
	ValidateOutput func(*kong.Context) error
	WriteOutput    func(*kong.Context, any) error
	// RejectQuery は --query に対応しないコマンドで、--query の指定をエラーにします。
	// nil の場合は検査しません (テスト向け)。
	RejectQuery func(*kong.Context) error
	// Stdin は確認プロンプトと --app - の読み込み元です。nil の場合は os.Stdin を使います。
	Stdin io.Reader
	// IsTerminal は標準入力が端末かどうかを返します。nil の場合は既定の判定を使います。
	IsTerminal func() bool
}

// ServiceFactory は service/apprun-dedicated-service-go の Service を返します。
// deploy 以外の操作で使います。
type ServiceFactory func() (*apprundedicatedservice.Service, error)

// DeployServiceFactory は deploy 専用の Service を返します。
// deploy は registryPassword や secret=true の環境変数の値を送信するため、
// 実行前にトレース設定を検査する factory を受け取ります。
type DeployServiceFactory func() (*apprundedicatedservice.Service, error)

func (r Runtime) validateOutput(kongCtx *kong.Context) error {
	if r.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.ValidateOutput(kongCtx)
}

func (r Runtime) writeOutput(kongCtx *kong.Context, value any) error {
	if r.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.WriteOutput(kongCtx, value)
}

// rejectQuery は --query に対応しないコマンドで、--query の指定を拒否します。
// 指定を暗黙に無視すると利用者の意図と異なる出力になるため、エラーにします。
func (r Runtime) rejectQuery(kongCtx *kong.Context) error {
	if r.RejectQuery == nil {
		return nil
	}
	return r.RejectQuery(kongCtx)
}

func (r Runtime) stdin() io.Reader {
	if r.Stdin != nil {
		return r.Stdin
	}
	return os.Stdin
}

func (r Runtime) isTerminal() bool {
	if r.IsTerminal != nil {
		return r.IsTerminal()
	}
	return stdinIsTerminal()
}

func (r Runtime) loadDefinition(input string) (*apprundedicatedservice.ApplicationDefinition, error) {
	return loadDefinition(r.stdin(), input)
}

// newService は deploy 以外の操作用の factory から Service を構築します。
func newService(factory ServiceFactory) (*apprundedicatedservice.Service, error) {
	if factory == nil {
		return nil, fmt.Errorf("API クライアントが設定されていません")
	}
	return factory()
}

// newDeployService は deploy 用の factory から Service を構築します。
func newDeployService(factory DeployServiceFactory) (*apprundedicatedservice.Service, error) {
	if factory == nil {
		return nil, fmt.Errorf("API クライアントが設定されていません")
	}
	return factory()
}

// commandContext は AppRun 専有型コマンド専用の実行境界を作ります。
// 割り込みシグナルで context がキャンセルされるため、読み取りと変更操作の間に
// 割り込まれた場合も service 層の確認が変更を続行させません。
func commandContext() (context.Context, func()) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// errAborted は確認プロンプトで操作を中止したことを表します。
var errAborted = errors.New("操作を中止しました")

// Commands は skr apprun-dedicated 配下のコマンド群です。
type Commands struct {
	Init       InitCommand       `cmd:"" help:"デプロイ済みのアプリケーションから定義ファイルを生成します。アクティブバージョンがあればその内容から、なければ最新のバージョンから定義を復元します。バージョンが 1 つもない場合は識別子だけの定義を生成します。registryPassword と secret=true の環境変数の値は API から取得できないため、生成される定義には含まれません。同名のファイルが既に存在する場合は上書きせずにエラーになります。"`
	Deploy     DeployCommand     `cmd:"" help:"定義ファイルの内容をデプロイします。クラスタとアプリケーションを検索し、見つからなければアプリケーションを作成して、定義から新しいバージョンを作成して有効化します。registryPassword は定義に値があれば新しく登録し (registryPasswordAction: new)、値がなくアクティブバージョンがあれば維持し (keep)、初回デプロイで値がなければ削除します (remove)。有効化後、要求されたコンテナ数と同じ数のコンテナが running になるまで待ちます (--no-wait で省略)。トレースが有効な場合は実行しません。"`
	Diff       DiffCommand       `cmd:"" help:"ローカルの定義とデプロイ済みの現状を比較し、unified diff 形式で標準出力へ出力します。現状はアクティブバージョンの内容です。アプリケーションが存在しないか非活性の場合は、識別子だけの定義を現状とします。registryPassword と secret=true の環境変数の値、registryPasswordAction は応答で取得できないため比較から除外し、その旨を標準エラー出力に示します。差分がなければ標準出力には何も出力しません。このコマンドは --query には対応せず、指定した場合はエラーになります。"`
	Render     RenderCommand     `cmd:"" help:"定義を読み込み、検証と正規化を行った結果を JSON で出力します。registryPassword と secret=true の環境変数の値は出力に含めません。API には接続しません。"`
	Status     StatusCommand     `cmd:"" help:"アプリケーションの状態を表示します。アプリケーションの ID、名前、アクティブバージョン、DesiredCount と、アクティブバージョンの内容 (イメージ、CPU、メモリ、スケーリング方式、アクティブノード数) を表示します。registryPassword は表示しません。"`
	Versions   VersionsCommand   `cmd:"" help:"アプリケーションのバージョンの一覧を表示します。バージョン番号、イメージ、アクティブノード数、アクティブかどうか、作成日時 (UTC) を表示します。"`
	Containers ContainersCommand `cmd:"" help:"コンテナの配置を表示します。ノードごとのコンテナの状態を表示します。"`
	Rollback   RollbackCommand   `cmd:"" help:"既存のバージョンを有効化して、有効なバージョンを切り替えます。新しいバージョンは作成しません。--target でバージョン番号を指定できます。省略した場合、活性ならアクティブバージョンより前のバージョンのうち最大のものを、非活性なら最新のバージョンを切り替え先にします。実行前に現在の状態を表示して確認します (--force で省略)。"`
	Deactivate DeactivateCommand `cmd:"" help:"アプリケーションのアクティブバージョンを解除して停止します。アプリケーションとバージョンは残ります。実行前に現在の状態を表示して確認します (--force で省略)。"`
	Delete     DeleteCommand     `cmd:"" help:"アプリケーションを削除します。アクティブバージョンがあれば解除して停止を待ってから削除します。実行前に現在の状態を表示して確認します (--force で省略)。"`
}

// NewCommands は runtime と factory を各コマンドへ一括注入します。
// deploy は秘密情報を扱うため、トレース設定を検査する専用の factory を受け取ります。
func NewCommands(runtime Runtime, factory ServiceFactory, deployFactory DeployServiceFactory) Commands {
	return Commands{
		Init:       InitCommand{factory: factory, runtime: runtime},
		Deploy:     DeployCommand{factory: deployFactory, runtime: runtime},
		Diff:       DiffCommand{factory: factory, runtime: runtime},
		Render:     RenderCommand{runtime: runtime},
		Status:     StatusCommand{factory: factory, runtime: runtime},
		Versions:   VersionsCommand{factory: factory, runtime: runtime},
		Containers: ContainersCommand{factory: factory, runtime: runtime},
		Rollback:   RollbackCommand{factory: factory, runtime: runtime},
		Deactivate: DeactivateCommand{factory: factory, runtime: runtime},
		Delete:     DeleteCommand{factory: factory, runtime: runtime},
	}
}

// InitCommand はデプロイ済みのアプリケーションから定義ファイルを生成します。
type InitCommand struct {
	Cluster     string `arg:"" name:"cluster" help:"クラスタの名前。"`
	Application string `arg:"" name:"application" help:"アプリケーションの名前。"`
	OutputFile  string `name:"output-file" default:"application.json" help:"出力先のファイル。既存のファイルは上書きしません。"`
	factory     ServiceFactory
	runtime     Runtime
}

func (c *InitCommand) Run(kongCtx *kong.Context) error {
	// 出力は JSON ではないため --query は受け付けません。
	if err := c.runtime.rejectQuery(kongCtx); err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	def, err := svc.Init(ctx, c.Cluster, c.Application)
	if err != nil {
		return err
	}
	data, err := marshalDefinition(def)
	if err != nil {
		return err
	}
	// 定義には秘密情報を書けるため、所有者だけが読める権限で書き出します。
	// O_EXCL で作成するため、既存ファイルの存在確認と作成の間に上書きが
	// 挿入されることはありません。
	f, err := os.OpenFile(c.OutputFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600) // #nosec G304 -- パスは --output-file で利用者が明示的に指定したものです。
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("出力先のファイル %q が既に存在します。上書きはしないため、--output-file で別のパスを指定してください", c.OutputFile)
		}
		return fmt.Errorf("出力先のファイル %q を作成できません: %w", c.OutputFile, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("出力先のファイル %q を書き込めません: %w", c.OutputFile, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("出力先のファイル %q を閉じられません: %w", c.OutputFile, err)
	}
	// 進行状況のログは補足情報のため、書き込みエラーを無視します (ベストエフォート)。
	_, _ = fmt.Fprintf(kongCtx.Stderr, "定義を %s に書き出しました\n", c.OutputFile)
	_, _ = fmt.Fprintln(kongCtx.Stderr, "registryPassword と secret=true の環境変数の値は API から取得できないため、生成される定義には含まれません。必要な場合は手動で追記してください。")
	return nil
}

// DeployCommand は定義の内容をデプロイします。
type DeployCommand struct {
	App         string        `name:"app" required:"" placeholder:"FILE|-" help:"定義ファイルのパス。- で標準入力から読み込みます。skr apprun-dedicated init で生成したファイルを指定できます。"`
	NoWait      bool          `name:"no-wait" help:"有効化後の反映待ちを省略します。"`
	WaitTimeout time.Duration `name:"wait-timeout" placeholder:"DURATION" help:"待機のタイムアウト。待機中の API 呼び出しもこの期限で打ち切ります。有効化や停止などの変更自体は完了している場合があります。未指定のときは 5 分です。"`
	factory     DeployServiceFactory
	runtime     Runtime
}

func (c *DeployCommand) Run(kongCtx *kong.Context) error {
	if err := c.runtime.validateOutput(kongCtx); err != nil {
		return err
	}
	// deploy は秘密情報を含むリクエストを送信するため、定義の読み込みの前に
	// factory の構築時点でトレース設定を検査します。
	svc, err := newDeployService(c.factory)
	if err != nil {
		return err
	}
	def, err := c.runtime.loadDefinition(c.App)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	result, err := svc.Deploy(ctx, def, &apprundedicatedservice.DeployOptions{
		NoWait:  c.NoWait,
		Timeout: c.WaitTimeout,
	})
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(kongCtx, result)
}

// DiffCommand はローカルの定義と現状の差分を表示します。
type DiffCommand struct {
	App     string `name:"app" required:"" placeholder:"FILE|-" help:"定義ファイルのパス。- で標準入力から読み込みます。skr apprun-dedicated init で生成したファイルを指定できます。"`
	factory ServiceFactory
	runtime Runtime
}

func (c *DiffCommand) Run(kongCtx *kong.Context) error {
	// 出力は unified diff のテキストで JSON ではないため --query は受け付けません。
	if err := c.runtime.rejectQuery(kongCtx); err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	def, err := c.runtime.loadDefinition(c.App)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	result, err := svc.Diff(ctx, def)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(kongCtx.Stderr, "registryPassword と secret=true の環境変数の値、registryPasswordAction は応答で取得できないため比較から除外しています。")
	diffText := unifiedDiff(definitionText(result.Local), definitionText(result.Remote), diffContextLines)
	if diffText == "" {
		_, _ = fmt.Fprintln(kongCtx.Stderr, "差分はありません")
		return nil
	}
	if colorEnabled(kongCtx.Stdout) {
		diffText = colorizeDiff(diffText)
	}
	if _, err := fmt.Fprint(kongCtx.Stdout, diffText); err != nil {
		return fmt.Errorf("差分を書き込めません: %w", err)
	}
	return nil
}

// RenderCommand は検証と正規化を行った定義を表示します。
type RenderCommand struct {
	App     string `name:"app" required:"" placeholder:"FILE|-" help:"定義ファイルのパス。- で標準入力から読み込みます。skr apprun-dedicated init で生成したファイルを指定できます。"`
	runtime Runtime
}

func (c *RenderCommand) Run(kongCtx *kong.Context) error {
	if err := c.runtime.validateOutput(kongCtx); err != nil {
		return err
	}
	def, err := c.runtime.loadDefinition(c.App)
	if err != nil {
		return err
	}
	rendered, err := apprundedicatedservice.RenderDefinition(def)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(kongCtx, rendered)
}

// StatusCommand はアプリケーションの状態を表示します。
type StatusCommand struct {
	App     string `name:"app" required:"" placeholder:"FILE|-" help:"定義ファイルのパス。- で標準入力から読み込みます。skr apprun-dedicated init で生成したファイルを指定できます。"`
	factory ServiceFactory
	runtime Runtime
}

func (c *StatusCommand) Run(kongCtx *kong.Context) error {
	if err := c.runtime.validateOutput(kongCtx); err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	def, err := c.runtime.loadDefinition(c.App)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	status, err := svc.Status(ctx, def)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(kongCtx, status)
}

// VersionsCommand はバージョンの一覧を表示します。
type VersionsCommand struct {
	App     string `name:"app" required:"" placeholder:"FILE|-" help:"定義ファイルのパス。- で標準入力から読み込みます。skr apprun-dedicated init で生成したファイルを指定できます。"`
	factory ServiceFactory
	runtime Runtime
}

func (c *VersionsCommand) Run(kongCtx *kong.Context) error {
	if err := c.runtime.validateOutput(kongCtx); err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	def, err := c.runtime.loadDefinition(c.App)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	entries, err := svc.Versions(ctx, def)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(kongCtx, entries)
}

// ContainersCommand はコンテナの配置を表示します。
type ContainersCommand struct {
	App     string `name:"app" required:"" placeholder:"FILE|-" help:"定義ファイルのパス。- で標準入力から読み込みます。skr apprun-dedicated init で生成したファイルを指定できます。"`
	factory ServiceFactory
	runtime Runtime
}

func (c *ContainersCommand) Run(kongCtx *kong.Context) error {
	if err := c.runtime.validateOutput(kongCtx); err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	def, err := c.runtime.loadDefinition(c.App)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	placements, err := svc.Containers(ctx, def)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(kongCtx, placements)
}

// RollbackCommand は既存のバージョンを有効化して切り替えます。
type RollbackCommand struct {
	App         string        `name:"app" required:"" placeholder:"FILE|-" help:"定義ファイルのパス。- で標準入力から読み込みます。skr apprun-dedicated init で生成したファイルを指定できます。"`
	Target      *int          `name:"target" placeholder:"VERSION" help:"有効化するバージョン番号。省略した場合、活性ならアクティブバージョンより前のバージョンのうち最大のものを、非活性なら最新のバージョンを切り替え先にします。"`
	Force       bool          `name:"force" help:"確認プロンプトを省略します。標準入力が端末でない場合や --app - の場合は必須です。"`
	NoWait      bool          `name:"no-wait" help:"有効化後の反映待ちを省略します。"`
	WaitTimeout time.Duration `name:"wait-timeout" placeholder:"DURATION" help:"待機のタイムアウト。待機中の API 呼び出しもこの期限で打ち切ります。有効化や停止などの変更自体は完了している場合があります。未指定のときは 5 分です。"`
	factory     ServiceFactory
	runtime     Runtime
}

func (c *RollbackCommand) Run(kongCtx *kong.Context) error {
	if err := c.runtime.validateOutput(kongCtx); err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	def, err := c.runtime.loadDefinition(c.App)
	if err != nil {
		return err
	}
	target, err := versionNumberFlag(c.Target)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	if err := confirmOperation(ctx, kongCtx, c.runtime, svc, def, c.Force, c.App == "-"); err != nil {
		return err
	}
	result, err := svc.Rollback(ctx, def, &apprundedicatedservice.RollbackOptions{
		Target:  target,
		NoWait:  c.NoWait,
		Timeout: c.WaitTimeout,
	})
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(kongCtx, result)
}

// DeactivateCommand はアクティブバージョンを解除して停止します。
type DeactivateCommand struct {
	App         string        `name:"app" required:"" placeholder:"FILE|-" help:"定義ファイルのパス。- で標準入力から読み込みます。skr apprun-dedicated init で生成したファイルを指定できます。"`
	Force       bool          `name:"force" help:"確認プロンプトを省略します。標準入力が端末でない場合や --app - の場合は必須です。"`
	NoWait      bool          `name:"no-wait" help:"停止待ちを省略します。"`
	WaitTimeout time.Duration `name:"wait-timeout" placeholder:"DURATION" help:"待機のタイムアウト。待機中の API 呼び出しもこの期限で打ち切ります。有効化や停止などの変更自体は完了している場合があります。未指定のときは 5 分です。"`
	factory     ServiceFactory
	runtime     Runtime
}

func (c *DeactivateCommand) Run(kongCtx *kong.Context) error {
	if err := c.runtime.validateOutput(kongCtx); err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	def, err := c.runtime.loadDefinition(c.App)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	if err := confirmOperation(ctx, kongCtx, c.runtime, svc, def, c.Force, c.App == "-"); err != nil {
		return err
	}
	result, err := svc.Deactivate(ctx, def, &apprundedicatedservice.WaitOptions{
		NoWait:  c.NoWait,
		Timeout: c.WaitTimeout,
	})
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(kongCtx, result)
}

// DeleteCommand はアプリケーションを削除します。
type DeleteCommand struct {
	App         string        `name:"app" required:"" placeholder:"FILE|-" help:"定義ファイルのパス。- で標準入力から読み込みます。skr apprun-dedicated init で生成したファイルを指定できます。"`
	Force       bool          `name:"force" help:"確認プロンプトを省略します。標準入力が端末でない場合や --app - の場合は必須です。"`
	NoWait      bool          `name:"no-wait" help:"停止待ちを省略します。削除自体と削除後の不在確認は行います。"`
	WaitTimeout time.Duration `name:"wait-timeout" placeholder:"DURATION" help:"待機のタイムアウト。待機中の API 呼び出しもこの期限で打ち切ります。有効化や停止などの変更自体は完了している場合があります。未指定のときは 5 分です。"`
	factory     ServiceFactory
	runtime     Runtime
}

func (c *DeleteCommand) Run(kongCtx *kong.Context) error {
	// 出力は JSON ではないため --query は受け付けません。
	if err := c.runtime.rejectQuery(kongCtx); err != nil {
		return err
	}
	svc, err := newService(c.factory)
	if err != nil {
		return err
	}
	def, err := c.runtime.loadDefinition(c.App)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext()
	defer cancel()
	if err := confirmOperation(ctx, kongCtx, c.runtime, svc, def, c.Force, c.App == "-"); err != nil {
		return err
	}
	// delete は結果を出力しません (低レベルの delete と同じです)。
	_, err = svc.Delete(ctx, def, &apprundedicatedservice.WaitOptions{
		NoWait:  c.NoWait,
		Timeout: c.WaitTimeout,
	})
	return err
}

// confirmOperation は変更系操作の実行前に、対象の現在の状態を表示して確認します。
// force が true の場合は表示と確認を行わずに続行します。
// 定義が標準入力から読まれた場合や標準入力が端末でない場合は、確認への応答を
// 読み込めないため --force を必須にします。
func confirmOperation(ctx context.Context, kongCtx *kong.Context, runtime Runtime, svc *apprundedicatedservice.Service, def *apprundedicatedservice.ApplicationDefinition, force bool, stdinDefinition bool) error {
	if force {
		return nil
	}
	status, err := svc.Status(ctx, def)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(kongCtx.Stderr, "対象: クラスタ %s のアプリケーション %s (ID: %s)\n", status.Cluster, status.Application.Name, formatApplicationID(status.Application.ID))
	_, _ = fmt.Fprintf(kongCtx.Stderr, "現在の状態: %s\n", statusSummary(status))
	if !runtime.isTerminal() || stdinDefinition {
		return errors.New("確認プロンプトを利用できないため --force を指定してください。標準入力が端末でない場合や --app - の場合は確認への応答を読み込めません")
	}
	confirmed, err := readConfirm(runtime.stdin(), kongCtx.Stderr)
	if err != nil {
		return err
	}
	if !confirmed {
		return errAborted
	}
	return nil
}

// formatApplicationID はアプリケーション ID を表示用に整形します。
func formatApplicationID(id v1.ApplicationID) string {
	return uuid.UUID(id).String()
}

// statusSummary は確認表示用に現在の状態を 1 行の説明へ整えます。
func statusSummary(status *apprundedicatedservice.Status) string {
	if status.Application.ActiveVersion == nil {
		return "非活性 (アクティブバージョンなし)"
	}
	summary := fmt.Sprintf("アクティブバージョン %d", *status.Application.ActiveVersion)
	if status.Application.DesiredCount != nil {
		summary += fmt.Sprintf("、コンテナ数 %d", *status.Application.DesiredCount)
	}
	return summary
}

// versionNumberFlag は --target のバージョン番号を検証して API の型へ変換します。
func versionNumberFlag(target *int) (*v1.ApplicationVersionNumber, error) {
	if target == nil {
		return nil, nil
	}
	if *target < 0 || *target > math.MaxInt32 {
		return nil, fmt.Errorf("--target には 0 以上 %d 以下のバージョン番号を指定してください", math.MaxInt32)
	}
	number := v1.ApplicationVersionNumber(*target)
	return &number, nil
}
