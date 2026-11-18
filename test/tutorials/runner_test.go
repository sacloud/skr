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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// newTestRunner は単体テスト用の runner を作成します。
func newTestRunner(t *testing.T) *runner {
	t.Helper()
	fixture := &Fixture{Mock: MockConfig{Zones: []string{"is1a"}}}
	require.NoError(t, fixture.validate("test.json"))
	r, err := newRunner("/does-not-exist", t.TempDir(), t.TempDir(), fixture)
	require.NoError(t, err)
	// 起動したモックと環境変数の変更はテスト終了時に後片併きします。
	t.Cleanup(r.close)
	return r
}
func TestRunnerCloseRestoresEnv(t *testing.T) {
	keys := []string{zonesEnv, iaasURLEnv, "SAKURA_ENDPOINTS_SIMPLE_MQ_QUEUE", "SAKURA_ENDPOINTS_SIMPLE_MQ_MESSAGE", "SAKURA_ENDPOINTS_EVENTBUS"}
	before := map[string]envValue{}
	for _, key := range keys {
		value, exists := os.LookupEnv(key)
		before[key] = envValue{value: value, exists: exists}
	}

	r := newTestRunner(t)
	require.Equal(t, r.iaasServer.URL, os.Getenv(iaasURLEnv))
	require.Equal(t, "is1a", os.Getenv(zonesEnv))

	r.close()
	for _, key := range keys {
		value, exists := os.LookupEnv(key)
		require.Equal(t, before[key].exists, exists, "key %s", key)
		if exists {
			require.Equal(t, before[key].value, value, "key %s", key)
		}
	}
}

func TestRunnerInjectEventWithoutTrigger(t *testing.T) {
	// 宣言順に依存しないことを確認するため、eventbus を先に宣言します。
	fixture := &Fixture{Mock: MockConfig{
		Zones:    []string{"is1a"},
		Services: []string{"eventbus", "simplemq"},
		Events: []*MockEvent{{
			After:  "skr iaas-api switch create --zone is1a",
			Source: "//eventbus.sakura.ad.jp/eventlog",
			Type:   "jp.ad.sakura.eventbus.eventlog.IaaS.request.Switch.normal.created",
		}},
	}}
	require.NoError(t, fixture.validate("test.json"))
	r, err := newRunner("/does-not-exist", t.TempDir(), t.TempDir(), fixture)
	require.NoError(t, err)
	t.Cleanup(r.close)

	// トリガーが 1 つもない状態でのイベント注入は配信 0 件のためエラーになります。
	// トリガーや fixture の宣言が壊れていてもチュートリアルを成功扱いにしないためです。
	err = r.injectEvents("skr iaas-api switch create --zone is1a")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no trigger")
}

func TestRunnerCarriesStateAcrossSteps(t *testing.T) {
	r := newTestRunner(t)

	// step 1: 変数の設定、ディレクトリ移動、umask の変更を行います。
	// set -a により変数は環境変数として引き継がれます。
	err := r.execStep(&Step{Command: "V=1\nmkdir -p sub\ncd sub\numask 077"}, "step 1")
	require.NoError(t, err)

	// step 2: 状態が引き継がれていることを確認します。
	err = r.execStep(&Step{Command: "echo \"$V\" > value.txt\npwd > out.txt"}, "step 2")
	require.NoError(t, err)

	require.Equal(t, filepath.Join(r.workDir, "sub"), r.cwd, "cwd should be carried across steps")
	require.Equal(t, "0077", r.umask, "umask should be carried across steps")
	require.Equal(t, "1", r.env["V"], "variables should be carried across steps")

	// step 2 の pwd は step 1 の cwd (sub) で実行されている。
	out, err := os.ReadFile(filepath.Join(r.cwd, "out.txt"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(r.workDir, "sub"), strings.TrimSpace(string(out)))

	// step 2 で作成したファイルは step 1 の cwd に置かれる。
	value, err := os.ReadFile(filepath.Join(r.cwd, "value.txt"))
	require.NoError(t, err)
	require.Equal(t, "1\n", string(value))
}

func TestRunnerFailingStep(t *testing.T) {
	r := newTestRunner(t)

	err := r.execStep(&Step{Command: "false"}, "failing step")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failing step")

	err = r.execStep(&Step{Command: "echo message >&2; exit 3"}, "failing step with stderr")
	require.Error(t, err)
	require.Contains(t, err.Error(), "message")
}

func TestRunnerMultiLineVariablesCarried(t *testing.T) {
	r := newTestRunner(t)

	// 複数行の値を持つ変数も環境変数経由で引き継がれます。
	err := r.execStep(&Step{Command: "MULTI=$(printf 'line1\\nline2')"}, "set variable")
	require.NoError(t, err)
	err = r.execStep(&Step{Command: "printf '%s' \"$MULTI\" > multi.txt"}, "use variable")
	require.NoError(t, err)

	out, err := os.ReadFile(filepath.Join(r.cwd, "multi.txt"))
	require.NoError(t, err)
	require.Equal(t, "line1\nline2", string(out))
}

func TestRunnerUnsetCarried(t *testing.T) {
	r := newTestRunner(t)

	// step 内の unset も次の step に引き継がれます。テストプロセスの環境変数が
	// 復活しないことを確認します。
	err := r.execStep(&Step{Command: "HARNESS_VAR=1"}, "set variable")
	require.NoError(t, err)
	err = r.execStep(&Step{Command: "unset HARNESS_VAR"}, "unset variable")
	require.NoError(t, err)
	err = r.execStep(&Step{Command: "echo \"${HARNESS_VAR-missing}\" > unset.txt"}, "check variable")
	require.NoError(t, err)

	out, err := os.ReadFile(filepath.Join(r.cwd, "unset.txt"))
	require.NoError(t, err)
	require.Equal(t, "missing\n", string(out))
}

func TestCompareInlineExpected(t *testing.T) {
	r := newTestRunner(t)

	// インライン期待出力が JSON で、実際の出力と構造が一致する場合は成功します。
	err := r.execStep(&Step{Command: `echo '{"id":1,"name":"x"}'`}, "output JSON")
	require.NoError(t, err)
	err = r.compareInlineExpected("test.md", &Step{Line: 1, Expected: []string{"{", "  \"id\": 999,", "  \"name\": \"masked\"", "}"}})
	require.NoError(t, err)

	// 構造が異なる JSON 期待出力は失敗します。
	err = r.compareInlineExpected("test.md", &Step{Line: 2, Expected: []string{"{", "  \"id\": 999", "}"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "line 2")

	// 期待出力が JSON として解釈できない (プレーンテキストの説明行) 場合は照合しません。
	err = r.compareInlineExpected("test.md", &Step{Line: 3, Expected: []string{"表示されるゾーンは環境によって異なります。"}})
	require.NoError(t, err)

	// 期待出力がない step は照合しません。
	err = r.compareInlineExpected("test.md", &Step{Line: 4})
	require.NoError(t, err)
}

func TestRunnerHeredocCommand(t *testing.T) {
	r := newTestRunner(t)

	// heredoc を含むコマンドは終端行まで含めて実行されます。
	// 終端行が欠けると bash の source が構文エラーになります。
	command := "cat > data.txt <<EOF\nline1\nline2\nEOF\nwc -l < data.txt"
	err := r.execStep(&Step{Line: 1, Command: command}, "heredoc step")
	require.NoError(t, err)

	out, err := os.ReadFile(filepath.Join(r.stateDir, "last-output"))
	require.NoError(t, err)
	require.Equal(t, "2\n", string(out))
}
