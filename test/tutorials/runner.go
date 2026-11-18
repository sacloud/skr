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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	saclient "github.com/sacloud/sacloud-sdk-go/common/saclient"
	"github.com/sacloud/sakumock/core"
	eventbusmock "github.com/sacloud/sakumock/eventbus"
	simplemqmock "github.com/sacloud/sakumock/simplemq"
	sakumockiaas "github.com/sacloud/skr/internal/sakumock/iaas"
)

const (
	// stepTimeout は 1 step の実行タイムアウトです。
	stepTimeout = 60 * time.Second
	// profileName は harness が作成する skr プロファイルの名前です。
	profileName = "tutorials"
	// zonesEnv は harness 用 IaaS モックにゾーンを伝える環境変数です。
	zonesEnv = "SKR_TUTORIAL_HARNESS_ZONES"
	// iaasURLEnv は runner が起動した IaaS モックの HTTP ブリッジの URL を
	// サブプロセスに伝える環境変数です。
	iaasURLEnv = "SKR_TUTORIAL_HARNESS_IAAS_URL"
)

// runner はチュートリアル 1 ファイルの実行を担います。
type runner struct {
	// binDir はビルド済み skr バイナリを PATH の先頭に加えるディレクトリです。
	binDir string
	// fixturesDir は fixture ディレクトリです。
	fixturesDir string
	// workDir はチュートリアル実行に使う一時ディレクトリで、初期 cwd でもあります。
	workDir string
	// fixture はこのチュートリアルの fixture です。
	fixture *Fixture

	// stateDir は step 間の状態の受け渡しに使うディレクトリです。
	stateDir string
	// cwd は直前 step 終了時のカレントディレクトリです。
	cwd string
	// umask は直前 step 終了時の umask です。
	umask string
	// env は直前 step 終了時の環境変数の写像です。nil の場合は
	// まだ 1 step も実行しておらず、テストプロセスの環境変数を基準にします。
	env map[string]string
	// mqServer は SimpleMQ モックです。fixture の mock.services に simplemq が
	// 指定された場合のみ起動します。
	mqServer *simplemqmock.Server
	// eventbusServer は EventBus モックです。サービスリンクで SimpleMQ モックに
	// 接続し、トリガー発火時のメッセージ送信を再現します。
	eventbusServer *eventbusmock.Server
	// iaasMock は IaaS のインメモリモックです。スイッチなどの状態を保持します。
	iaasMock *sakumockiaas.Server
	// iaasServer は IaaS モックをサブプロセスに公開する HTTP ブリッジです。
	iaasServer *httptest.Server
	// envBackup は setEnv で変更した環境変数の変更前の値です。close で
	// 復元し、後続のチュートリアルに停止済みモックの URL を継承させないためです。
	envBackup map[string]envValue
}

// envValue は runner が変更する前の環境変数の値です。
type envValue struct {
	value  string
	exists bool
}

// setEnv はテストプロセスの環境変数を設定し、変更前の値を記録します。
// close で元に戻すためです。
func (r *runner) setEnv(key, value string) error {
	if r.envBackup == nil {
		r.envBackup = map[string]envValue{}
	}
	if _, seen := r.envBackup[key]; !seen {
		current, exists := os.LookupEnv(key)
		r.envBackup[key] = envValue{value: current, exists: exists}
	}
	return os.Setenv(key, value)
}

// restoreEnv は setEnv で変更した環境変数を、変更前の状態に戻します。
func (r *runner) restoreEnv() {
	for key, saved := range r.envBackup {
		if saved.exists {
			_ = os.Setenv(key, saved.value) // この後テストは失敗済みのため、復元エラーを通知する手段がない。
		} else {
			_ = os.Unsetenv(key)
		}
	}
	r.envBackup = nil
}

// newRunner は runner を初期化し、ラッパースクリプトを書き込みます。
func newRunner(binDir, fixturesDir, workDir string, fixture *Fixture) (*runner, error) {
	r := &runner{
		binDir:      binDir,
		fixturesDir: fixturesDir,
		workDir:     workDir,
		fixture:     fixture,
		stateDir:    filepath.Join(workDir, ".harness-state"),
		cwd:         workDir,
		umask:       "022",
		// env は nil のままにし、最初の step はテストプロセスの環境変数を基準にする。
		env: nil,
	}
	if err := os.MkdirAll(r.stateDir, 0o750); err != nil {
		return nil, err
	}
	if len(fixture.Mock.Zones) == 0 {
		return nil, fmt.Errorf("fixture must seed at least one zone for the IaaS mock")
	}
	// チュートリアルは順次実行されるため、テストプロセスの環境変数を上書きしてよい。
	if err := r.setEnv(zonesEnv, strings.Join(fixture.Mock.Zones, ",")); err != nil {
		return nil, err
	}
	// IaaS モックは runner 側で 1 つだけ起動し、サブプロセスから HTTP で参照します。
	// スイッチなどの状態をプロセス間で共有するためです。
	r.iaasMock = sakumockiaas.NewTestServer(sakumockiaas.Config{Zones: fixture.Mock.Zones})
	r.iaasServer = sakumockiaas.NewTestHTTPServer(r.iaasMock)
	if err := r.setEnv(iaasURLEnv, r.iaasServer.URL); err != nil {
		r.close()
		return nil, err
	}
	if err := r.startMockServices(); err != nil {
		r.close()
		return nil, err
	}
	if err := r.writeWrapper(); err != nil {
		return nil, err
	}
	return r, nil
}

// startMockServices は fixture の mock.services で宣言された Ogen 系モック
// サービスを起動し、エンドポイント env 変数をテストプロセスに設定します。
// 宣言順に依存しないため、サービスリンクの宛先になる SimpleMQ を先に起動します。
func (r *runner) startMockServices() error {
	for _, service := range r.fixture.Mock.Services {
		if service == "simplemq" {
			if err := r.startSimpleMQMock(); err != nil {
				return err
			}
		}
	}
	for _, service := range r.fixture.Mock.Services {
		if service == "eventbus" {
			if err := r.startEventBusMock(); err != nil {
				return err
			}
		}
	}
	return nil
}

// startSimpleMQMock は SimpleMQ モックを起動します。キュー (コントロールプレーン) と
// メッセージ (データプレーン) で同じ URL のエンドポイント変数を2つ使います。
func (r *runner) startSimpleMQMock() error {
	server := simplemqmock.NewTestServer(simplemqmock.Config{})
	r.mqServer = server
	for _, key := range []string{"SAKURA_ENDPOINTS_SIMPLE_MQ_QUEUE", "SAKURA_ENDPOINTS_SIMPLE_MQ_MESSAGE"} {
		if err := r.setEnv(key, server.TestURL()); err != nil {
			return err
		}
	}
	return nil
}

// startEventBusMock は EventBus モックを、SimpleMQ モック宛てのサービスリンクと
// 一起動します。モック内のフォワーダーがトリガー発火時に SimpleMQ モックへ
// メッセージを送信できるよう、エンドポイント変数を渡します。
func (r *runner) startEventBusMock() error {
	env := []core.EnvVar{
		{Key: "SAKURA_ENDPOINTS_SIMPLE_MQ_MESSAGE", Value: r.mqServer.TestURL()},
	}
	env = append(env, core.DummyCredentialEnv()...)
	server := eventbusmock.NewTestServerWithServiceLink(eventbusmock.Config{}, env)
	r.eventbusServer = server
	return r.setEnv("SAKURA_ENDPOINTS_EVENTBUS", server.TestURL())
}

// close は起動したモックサーバーを停止し、プロセス環境への変更を元に戻します。
func (r *runner) close() {
	if r.mqServer != nil {
		r.mqServer.Close()
		r.mqServer = nil
	}
	if r.eventbusServer != nil {
		r.eventbusServer.Close()
		r.eventbusServer = nil
	}
	if r.iaasServer != nil {
		r.iaasServer.Close()
		r.iaasServer = nil
	}
	r.iaasMock = nil
	r.restoreEnv()
}

// setup は実行環境を作成します。ダミーのアクセストークン認証による skr プロファイルを
// 作成し、current プロファイルに設定します。
// サービスプリンシパル認証は実環境の認証エンドポイントで JWT を検証するため、
// モック対象外の通信が発生します。そこで Ogen 系モックが受理する Basic 認証の
// ダミートークンをプロファイルに設定します。プロファイル作成はリソースを変更しない
// harness 内部の準備のため、SDK で直接行います。
func (r *runner) setup() error {
	op, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		return fmt.Errorf("create profile operator: %w", err)
	}
	profile := &saclient.Profile{Version: 1}
	profile.Name = profileName
	if err := profile.Credentials.AccessToken.Set("dummy"); err != nil {
		return fmt.Errorf("set access token: %w", err)
	}
	if err := profile.Credentials.AccessTokenSecret.Set("dummy"); err != nil {
		return fmt.Errorf("set access token secret: %w", err)
	}
	if err := op.Create(profile); err != nil {
		return fmt.Errorf("create harness profile: %w", err)
	}
	if err := op.SetCurrentName(profileName); err != nil {
		return fmt.Errorf("set current profile: %w", err)
	}
	return nil
}

// run はチュートリアルの各 step を実行します。エラーはチュートリアル内の
// 行番号付きで返します。stub と skip はチュートリアルに書かれたコマンドテキスト
// (正規化後) をキーに判定し、値の置換は実行するコマンドと stub の置換後テキストに
// 適用します。
func (r *runner) run(script *tutorialScript) error {
	for blockIdx, block := range script.Blocks {
		for _, step := range block.Steps {
			if replacement, stubbed := r.fixture.stubReplacement(step.Command); stubbed {
				// stub 化したコマンドは実行せず、値の置換後に構文検証だけを行います。
				replacement, err := r.fixture.substituteValues(replacement)
				if err != nil {
					return fmt.Errorf("%s: line %d: stub: %w", script.Path, step.Line, err)
				}
				if err := validateStubCommand(replacement); err != nil {
					return fmt.Errorf("%s: line %d: stub: %w", script.Path, step.Line, err)
				}
				continue
			}

			if r.fixture.shouldSkip(step.Command) {
				continue
			}

			command, err := r.fixture.substituteValues(step.Command)
			if err != nil {
				return fmt.Errorf("%s: line %d: %w", script.Path, step.Line, err)
			}
			description := fmt.Sprintf("%s: line %d: %s", script.Path, step.Line, strings.Join(strings.Fields(step.Command), " "))
			if err := r.execStep(&Step{Line: step.Line, Command: command}, description); err != nil {
				return err
			}
			// 実世界ではリソース操作の副作用として発生するイベントを、fixture の
			// 宣言に従って EventBus モックへ注入します。
			if err := r.injectEvents(step.Command); err != nil {
				return fmt.Errorf("%s: line %d: inject mock event: %w", script.Path, step.Line, err)
			}
			if err := r.compareInlineExpected(script.Path, step); err != nil {
				return err
			}
		}
		// ブロックの直後に置かれた JSON 例は、ブロック内で最後に実行した
		// step の出力と照合します。
		if example, ok := script.JSONExamples[blockIdx]; ok {
			actual, err := os.ReadFile(filepath.Join(r.stateDir, "last-output")) //nolint:gosec // runner が作成した state ディレクトリ配下のファイルを読み込む。
			if err != nil {
				return fmt.Errorf("%s: line %d: read last output: %w", script.Path, example.Line, err)
			}
			if err := compareOutput(example.Text, string(actual)); err != nil {
				return fmt.Errorf("%s: line %d: %w", script.Path, example.Line, err)
			}
		}
	}
	// チュートリアルの編集で対応が外れた fixture の項目を検出します。
	return r.fixture.checkUnused()
}

// compareInlineExpected は step 直後にチュートリアルに書かれたインライン期待出力を
// 実行結果と照合します。期待出力が JSON として解釈できない場合 (プレーンテキストの
// 説明行など) は照合しません。
func (r *runner) compareInlineExpected(path string, step *Step) error {
	if len(step.Expected) == 0 {
		return nil
	}
	actual, err := os.ReadFile(filepath.Join(r.stateDir, "last-output")) //nolint:gosec // runner が作成した state ディレクトリ配下のファイルを読み込む。
	if err != nil {
		return fmt.Errorf("%s: line %d: read last output: %w", path, step.Line, err)
	}
	if err := compareOutput(strings.Join(step.Expected, "\n"), string(actual)); err != nil {
		return fmt.Errorf("%s: line %d: %w", path, step.Line, err)
	}
	return nil
}

// injectEvents はコマンド (正規化後のチュートリアルのテキスト) が fixture の
// mock.events の after と一致するたびに、EventBus モックへイベントを注入します。
// 注入によりトリガーが発火し、サービスリンク経由で SimpleMQ モックにメッセージが
// 送られます。実世界ではリソース操作の副作用としてイベントが発生するため、
// その経路を再現する harness 固有の処理です。
func (r *runner) injectEvents(command string) error {
	if r.eventbusServer == nil || len(r.fixture.Mock.Events) == 0 {
		return nil
	}
	normalized := normalizeCommand(command)
	for _, event := range r.fixture.Mock.Events {
		if normalizeCommand(event.After) != normalized {
			continue
		}
		r.fixture.usedEvents[event] = true
		if err := r.injectEvent(event); err != nil {
			return err
		}
	}
	return nil
}

// injectEventTimeout はイベント注入 1 件のタイムアウトです。モック内の
// フォワーダーは宛先ごとに 5 秒のタイムアウトを持つため、十分な余裕を持たせます。
const injectEventTimeout = 30 * time.Second

// injectEvent は 1 件のイベントを EventBus モックの検証用エンドポイント
// (POST /_sakumock/events) へ POST します。mock.events は発火を期待する宣言のため、
// 配信が 0 件 (トリガーが 1 つも一致しない) 場合も、発火した配信にエラーがある場合も
// fail-closed でエラーにします。メッセージが届かない状態でチュートリアルを
// 成功扱いにしないためです。
func (r *runner) injectEvent(event *MockEvent) error {
	body, err := r.fixture.substituteEvent(event)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode mock event: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, r.eventbusServer.TestURL()+"/_sakumock/events", bytes.NewReader(payload)) //nolint:gosec // runner が起動したローカルのモックサーバーへリクエストする。
	if err != nil {
		return fmt.Errorf("build mock event request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: injectEventTimeout}
	resp, err := client.Do(req) //nolint:gosec // runner が起動したローカルのモックサーバーへリクエストする。
	if err != nil {
		return fmt.Errorf("send mock event: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // 読み込み済みのレスポンス本文のクローズエラーは無視する。
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read mock event response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mock event endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var result struct {
		Deliveries []struct {
			Error string `json:"Error"`
		} `json:"Deliveries"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("decode mock event response: %w", err)
	}
	// モックはトリガーに 1 つも一致しない場合も 2xx と空の Deliveries を返す。
	// 発火を期待する注入で配信が 0 件なら、チュートリアルのトリガーか fixture の
	// 宣言が壊れているため失敗にする。
	if len(result.Deliveries) == 0 {
		return fmt.Errorf("mock event (source=%s type=%s) was delivered to no trigger: check the trigger in the tutorial and the mock.events declaration", body.Source, body.Type)
	}
	for _, delivery := range result.Deliveries {
		if delivery.Error != "" {
			return fmt.Errorf("mock event delivery failed: %s", delivery.Error)
		}
	}
	return nil
}

// execStep は 1 step を bash で実行し、状態を回収します。
func (r *runner) execStep(step *Step, description string) error {
	stepPath := filepath.Join(r.stateDir, "step.sh")
	if err := os.WriteFile(stepPath, []byte(step.Command+"\n"), 0o600); err != nil { //nolint:gosec // runner が作成した state ディレクトリ配下に step スクリプトを書き込む。
		return err
	}

	// 出力を受け取るファイルを用意します。os.Create は既存ファイルを切り詰めます。
	outputFile, err := os.Create(filepath.Join(r.stateDir, "last-output")) //nolint:gosec // runner が作成した state ディレクトリ配下に出力ファイルを作成する。
	if err != nil {
		return err
	}
	defer func() { _ = outputFile.Close() }() // step の出力はファイルに書き終えているためクローズエラーは無視する。

	stderrFile, err := os.Create(filepath.Join(r.stateDir, "last-stderr")) //nolint:gosec // runner が作成した state ディレクトリ配下に stderr ファイルを作成する。
	if err != nil {
		return err
	}
	defer func() { _ = stderrFile.Close() }() // step の stderr はファイルに書き終えているためクローズエラーは無視する。

	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", filepath.Join(r.stateDir, "wrapper.sh")) //nolint:gosec // runner が書き込んだ固定のラッパースクリプトを、シェル経由で実行する。
	cmd.Stdout = outputFile
	cmd.Stderr = stderrFile
	cmd.Env = r.processEnv()
	if err := cmd.Run(); err != nil {
		stderr, _ := os.ReadFile(filepath.Join(r.stateDir, "last-stderr")) //nolint:gosec // runner が作成した state ディレクトリ配下のファイルを読み込む。
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("%s: timeout after %s", description, stepTimeout)
		}
		return fmt.Errorf("%s: %w\nstderr:\n%s", description, err, stderr)
	}

	return r.harvestState()
}

// processEnv は bash プロセスに渡す環境変数を組み立てます。
// 最初の step はテストプロセスの環境変数を基にし、2 step 目以降は直前 step
// 終了時の完全な環境 (unset の反映を含む) を基にします。
func (r *runner) processEnv() []string {
	merged := map[string]string{}
	if r.env == nil {
		for _, entry := range os.Environ() {
			key, value, _ := strings.Cut(entry, "=")
			merged[key] = value
		}
	} else {
		for key, value := range r.env {
			merged[key] = value
		}
	}
	// PATH はビルド済みバイナリを優先します。2 step 目以降の PATH には
	// すでに binDir が含まれるため、重複して先頭に加えない。
	prefix := r.binDir + string(os.PathListSeparator)
	if !strings.HasPrefix(merged["PATH"], prefix) {
		merged["PATH"] = prefix + merged["PATH"]
	}
	// harness 固有の変数は明示的に組み立て直すため、引き継ぎ分を除去します。
	for _, key := range []string{"SKR_HARNESS_CWD", "SKR_HARNESS_UMASK", "SKR_HARNESS_STATE", "SKR_HARNESS_STEP"} {
		delete(merged, key)
	}
	env := make([]string, 0, len(merged)+4)
	for key, value := range merged {
		env = append(env, key+"="+value)
	}
	return append(env,
		"SKR_HARNESS_CWD="+r.cwd,
		"SKR_HARNESS_UMASK="+r.umask,
		"SKR_HARNESS_STATE="+r.stateDir,
		"SKR_HARNESS_STEP="+filepath.Join(r.stateDir, "step.sh"),
	)
}

// harvestState は step 終了後の状態ファイルを読み込みます。
func (r *runner) harvestState() error {
	cwd, err := os.ReadFile(filepath.Join(r.stateDir, "cwd")) //nolint:gosec // runner が作成した state ディレクトリ配下のファイルを読み込む。
	if err != nil {
		return fmt.Errorf("read cwd state: %w", err)
	}
	r.cwd = strings.TrimSpace(string(cwd))

	umask, err := os.ReadFile(filepath.Join(r.stateDir, "umask")) //nolint:gosec // runner が作成した state ディレクトリ配下のファイルを読み込む。
	if err != nil {
		return fmt.Errorf("read umask state: %w", err)
	}
	r.umask = strings.TrimSpace(string(umask))

	raw, err := os.ReadFile(filepath.Join(r.stateDir, "env")) //nolint:gosec // runner が作成した state ディレクトリ配下のファイルを読み込む。
	if err != nil {
		return fmt.Errorf("read env state: %w", err)
	}
	env := map[string]string{}
	for _, entry := range bytes.Split(raw, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		parts := bytes.SplitN(entry, []byte("="), 2)
		key := string(parts[0])
		// SHLVL と _ はシェルが管理するため引き継がない。
		if key == "SHLVL" || key == "_" {
			continue
		}
		env[key] = string(parts[1])
	}
	r.env = env
	return nil
}

// writeWrapper は step 実行に使うラッパースクリプトを書き込みます。
// set -a により step 内の変数代入も環境変数として env ダンプに現れ、
// 次の step に引き継がれます。
func (r *runner) writeWrapper() error {
	wrapper := `set -o errexit -o nounset -o pipefail
set -a
cd "$SKR_HARNESS_CWD"
umask "$SKR_HARNESS_UMASK"
source "$SKR_HARNESS_STEP"
printf '%s' "$(pwd)" > "$SKR_HARNESS_STATE/cwd"
umask > "$SKR_HARNESS_STATE/umask"
env -0 > "$SKR_HARNESS_STATE/env"
`
	return os.WriteFile(filepath.Join(r.stateDir, "wrapper.sh"), []byte(wrapper), 0o600)
}

// validateStubCommand は stub 化したコマンドの構文を検証します。
// skr コマンドは kong のパースで検証し、--query / --query-raw の式は
// gojq でコンパイルできることを確認します。
func validateStubCommand(command string) error {
	fields := shellFields(command)
	if len(fields) == 0 {
		return fmt.Errorf("empty command")
	}
	if fields[0] == "skr" {
		if err := skrParseOnly(fields[1:]); err != nil {
			return err
		}
	}
	for i := 0; i < len(fields); i++ {
		if (fields[i] == "--query" || fields[i] == "--query-raw") && i+1 < len(fields) {
			if err := compileGojq(fields[i+1]); err != nil {
				return fmt.Errorf("%s: %w", fields[i], err)
			}
		}
	}
	return nil
}

// shellFields はコマンドテキストをシェル風にトークン化します。
// シングルクォートとダブルクォートをトークンの区切りとして扱い、
// クォート文字自体はトークンに含めません。エスケープは解釈しません。
func shellFields(command string) []string {
	var fields []string
	var current strings.Builder
	var inSingle, inDouble bool
	flush := func() {
		if current.Len() > 0 {
			fields = append(fields, current.String())
			current.Reset()
		}
	}
	for _, r := range command {
		switch {
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case (r == ' ' || r == '\t' || r == '\n') && !inSingle && !inDouble:
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return fields
}
