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
	return r
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
