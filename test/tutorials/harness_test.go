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
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// tutorialRoot はチュートリアルのルートディレクトリ (docs/manual/tutorials) です。
var tutorialRoot = func() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("cannot determine source file location")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "docs", "manual", "tutorials")
}()

// repoRoot はリポジトリのルートディレクトリです。
var repoRoot = func() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("cannot determine source file location")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}()

// TestMain はテスト全体で使う skr バイナリをビルドします。
// バイナリには skr_tutorial_harness ビルドタグを付け、IaaS API を
// ローカルのモックに向けます。
func TestMain(m *testing.M) {
	os.Exit(runTestMain(m))
}

// runTestMain は TestMain の本体です。os.Exit と defer を共存させないために関数を分けます。
func runTestMain(m *testing.M) int {
	binDir, err := os.MkdirTemp("", "skr-tutorials-bin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temp dir for binary: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(binDir) }()

	binary := filepath.Join(binDir, "skr")
	build := exec.Command("go", "build", "-tags", "skr_tutorial_harness", "-o", binary, "./cmd/skr") //nolint:gosec // テストがビルドするローカルのバイナリを、シェルなしで実行する。
	build.Dir = repoRoot
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "build skr binary: %v\n", err)
		return 1
	}
	if err := os.Setenv("SKR_TUTORIALS_BIN_DIR", binDir); err != nil {
		fmt.Fprintf(os.Stderr, "set env: %v\n", err)
		return 1
	}
	return m.Run()
}

// listTutorialFiles はチュートリアルの Markdown ファイルを列挙します。
// 戻り値のパスは docs/manual/tutorials からの相対パスです。
func listTutorialFiles(t *testing.T) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(tutorialRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel, err := filepath.Rel(tutorialRoot, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, paths)
	return paths
}

// TestTutorialScripts はチュートリアルの console ブロックを抽出し、
// baseline と照合したうえで、実装済みのチュートリアルを実行検証します。
func TestTutorialScripts(t *testing.T) {
	// チュートリアルの実行はプロセスの環境変数を上書きするため、直列に行います。

	paths := listTutorialFiles(t)
	require.NoError(t, detectFixtureNameConflicts(paths), "fixture file names must not conflict")

	baseline, err := loadBaseline(filepath.Join(repoRoot, "test", "tutorials", "baseline.txt"))
	require.NoError(t, err)

	for _, path := range paths {
		path := path
		t.Run(path, func(t *testing.T) {
			entry, ok := baseline[path]
			require.True(t, ok, "tutorial %s is not listed in test/tutorials/baseline.txt", path)

			content, err := os.ReadFile(filepath.Join(tutorialRoot, filepath.FromSlash(path))) //nolint:gosec // チュートリアルのルート配下のファイルを読み込む。
			require.NoError(t, err)
			script, err := extractScript(path, string(content))
			require.NoError(t, err)
			require.GreaterOrEqual(t, len(script.Blocks), entry.Blocks,
				"tutorial %s has fewer console blocks than the baseline; update test/tutorials/baseline.txt", path)

			if entry.Status == baselineStatusImplemented {
				runImplementedTutorial(t, path, script)
			}
		})
	}
}

// runImplementedTutorial は実装済みチュートリアルの console ブロックを
// モックに対して実行します。
func runImplementedTutorial(t *testing.T, path string, script *tutorialScript) {
	t.Helper()

	fixture, err := loadFixture(filepath.Join(repoRoot, "test", "tutorials", "fixtures"), path)
	require.NoError(t, err, "implemented tutorial %s requires a fixture file", path)

	binDir := os.Getenv("SKR_TUTORIALS_BIN_DIR")
	require.NotEmpty(t, binDir, "SKR_TUTORIALS_BIN_DIR must be set by TestMain")

	workDir := t.TempDir()
	r, err := newRunner(binDir, filepath.Join(repoRoot, "test", "tutorials", "fixtures"), workDir, fixture)
	require.NoError(t, err)

	// プロファイルを作成するため、テストプロセスの環境変数を設定します。
	// 実環境の認証情報は使わない。
	t.Setenv("SAKURA_PROFILE_DIR", filepath.Join(workDir, "profiles"))
	t.Setenv("SAKURACLOUD_ACCESS_TOKEN", "")
	t.Setenv("SAKURACLOUD_ACCESS_TOKEN_SECRET", "")

	require.NoError(t, r.setup(), "tutorial %s: setup failed", path)
	require.NoError(t, r.run(script), "tutorial %s failed", path)
}
