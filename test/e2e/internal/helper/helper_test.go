package helper

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestNewSkipsWithoutSKRE2E は SKR_E2E が未設定のときに New がテストをスキップすることを確認する。
func TestNewSkipsWithoutSKRE2E(t *testing.T) {
	// skr-e2e プロファイルが存在する状態にして、SKR_E2E ゲートだけがスキップの原因になるようにする
	dir := t.TempDir()
	createProfile(t, dir, e2eProfileName)
	t.Setenv("SAKURA_PROFILE_DIR", dir)

	completed := false
	t.Run("new", func(t *testing.T) {
		New(t, "test", "test")
		completed = true
	})
	if completed {
		t.Error("New should skip when SKR_E2E is not set")
	}
}

// TestNewSkipsWithoutProfile は skr-e2e プロファイルが存在しないときに New がテストをスキップすることを確認する。
func TestNewSkipsWithoutProfile(t *testing.T) {
	t.Setenv("SKR_E2E", "1")
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())

	completed := false
	t.Run("new", func(t *testing.T) {
		New(t, "test", "test")
		completed = true
	})
	if completed {
		t.Error("New should skip when the skr-e2e profile is missing")
	}
}

// TestNewWithProfile は SKR_E2E と skr-e2e プロファイルが揃ったときに New がスキップしないことを確認する。
func TestNewWithProfile(t *testing.T) {
	// newHelper が参照する ../../../skr が解決できるよう、ダミーの実行ファイルを配置する
	setupDummyBinary(t)

	dir := t.TempDir()
	createProfile(t, dir, e2eProfileName)
	t.Setenv("SKR_E2E", "1")
	t.Setenv("SAKURA_PROFILE_DIR", dir)

	completed := false
	t.Run("new", func(t *testing.T) {
		h := New(t, "test", "test")
		completed = true
		require.NotNil(t, h)
		// CLI の認証に使うプロファイル名が設定されていることを確認する
		require.Equal(t, e2eProfileName, os.Getenv("SAKURA_PROFILE"))
	})
	require.True(t, completed, "New should not skip when SKR_E2E and the skr-e2e profile are present")
}

// TestNewWithTemporaryProfileDir は SKR_E2E が未設定でも NewWithTemporaryProfileDir がスキップせず、
// 一時プロファイルディレクトリを設定することを確認する。
func TestNewWithTemporaryProfileDir(t *testing.T) {
	// newHelper が参照する ../../../skr が解決できるよう、ダミーの実行ファイルを配置する
	setupDummyBinary(t)

	// skr-e2e プロファイルが存在しないプロファイルディレクトリを設定しても影響しない
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	before := os.Getenv("SAKURA_PROFILE_DIR")

	completed := false
	t.Run("new", func(t *testing.T) {
		h := NewWithTemporaryProfileDir(t, "test", "test")
		completed = true
		require.NotNil(t, h)
		// ヘルパーが新しい一時プロファイルディレクトリを設定することを確認する
		got := os.Getenv("SAKURA_PROFILE_DIR")
		require.NotEqual(t, before, got)
		require.DirExists(t, got)
	})
	require.True(t, completed, "NewWithTemporaryProfileDir should not skip without SKR_E2E")
}

// TestNewSkipsWithoutBinary は skr バイナリが存在しないときに New がテストをスキップすることを確認する。
func TestNewSkipsWithoutBinary(t *testing.T) {
	// プロファイルが存在する状態にして、バイナリ不足だけがスキップの原因になるようにする
	dir := t.TempDir()
	createProfile(t, dir, e2eProfileName)
	t.Setenv("SKR_E2E", "1")
	t.Setenv("SAKURA_PROFILE_DIR", dir)

	// skr バイナリが存在しないリポジトリにするため、テスト専用ディレクトリに移動する
	moveToEmptyRepository(t)

	completed := false
	t.Run("new", func(t *testing.T) {
		New(t, "test", "test")
		completed = true
	})
	if completed {
		t.Error("New should skip when the skr binary is missing")
	}
}

// TestNewWithTemporaryProfileDirSkipsWithoutBinary は skr バイナリが存在しないときに
// NewWithTemporaryProfileDir もテストをスキップすることを確認する。
func TestNewWithTemporaryProfileDirSkipsWithoutBinary(t *testing.T) {
	// skr バイナリが存在しないリポジトリにするため、テスト専用ディレクトリに移動する
	moveToEmptyRepository(t)

	completed := false
	t.Run("new", func(t *testing.T) {
		NewWithTemporaryProfileDir(t, "test", "test")
		completed = true
	})
	if completed {
		t.Error("NewWithTemporaryProfileDir should skip when the skr binary is missing")
	}
}

// TestFindBinary はカレントディレクトリから上に向かって skr バイナリを探す処理を確認する。
func TestFindBinary(t *testing.T) {
	// 上のディレクトリに実行可能な skr がある場合、そのパスを返す
	base := t.TempDir()
	writeExecutable(t, filepath.Join(base, "skr"))
	enterScenarioDir(t, base)

	path, err := findBinary()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(base, "skr"), path)
}

// TestFindBinaryNotFound は skr バイナリがなく go.mod まで辿った場合に空文字列を返すことを確認する。
func TestFindBinaryNotFound(t *testing.T) {
	base := t.TempDir()
	// 探索の打ち切り位置（リポジトリルート）を決めるための go.mod を置く
	writeGoMod(t, base)
	enterScenarioDir(t, base)

	path, err := findBinary()
	require.NoError(t, err)
	require.Empty(t, path)
}

// TestFindBinarySkipsNonExecutable は実行可能パーミッションのない skr を見つけないことを確認する。
func TestFindBinarySkipsNonExecutable(t *testing.T) {
	base := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(base, "skr"), []byte("#!/bin/sh\n"), 0600))
	writeGoMod(t, base)
	enterScenarioDir(t, base)

	path, err := findBinary()
	require.NoError(t, err)
	require.Empty(t, path)
}

// TestFindBinarySkipsDirectory は skr という名前のディレクトリを見つけないことを確認する。
func TestFindBinarySkipsDirectory(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, "skr"), 0700))
	writeGoMod(t, base)
	enterScenarioDir(t, base)

	path, err := findBinary()
	require.NoError(t, err)
	require.Empty(t, path)
}

// TestWriteRequestJSONFile は --request 用リクエストファイルの作成を確認する。
func TestWriteRequestJSONFile(t *testing.T) {
	h := &Helper{}

	path := h.WriteRequestJSONFile(t, "find", map[string]any{"Names": []string{"test"}})

	// テスト専用ディレクトリの find.json に、マーシャルした JSON が 0600 で書かれる
	require.Equal(t, "find.json", filepath.Base(path))
	data, err := os.ReadFile(path) //nolint:gosec // helper が返したテスト専用ディレクトリ内のパスを読み込む。
	require.NoError(t, err, "read request file")
	require.JSONEq(t, `{"Names":["test"]}`, string(data))
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

// TestRunLocalCommand は skr 以外のローカルコマンドを実行し、実行結果を実行記録に保存することを確認する。
func TestRunLocalCommand(t *testing.T) {
	base := t.TempDir()
	// stdout・stderr を出力して失敗するダミーコマンドを配置する
	writeExecutableWithScript(t, filepath.Join(base, "dummy-cmd"), "#!/bin/sh\necho local-ok\necho local-err >&2\nexit 4\n")

	h := &Helper{name: "test", description: "test"}
	stdout, stderr, err := h.RunLocalCommand(t, 10*time.Second, filepath.Join(base, "dummy-cmd"), "ローカルコマンドを実行する", "-x", "value")
	require.Error(t, err, "the dummy command should fail")
	require.Equal(t, "local-ok\n", stdout)
	require.Equal(t, "local-err\n", stderr)

	// 実行したコマンド名と引数、stdout・stderr を実行記録に保存する
	require.Len(t, h.results, 1, "the local command should be recorded")
	rendered := h.results[0].Render()
	require.Contains(t, rendered, "dummy-cmd -x value", "the report should show the executed command")
	require.Contains(t, rendered, "local-ok", "the report should contain the stdout")
	require.Contains(t, rendered, "local-err", "the report should contain the stderr")
}

// TestCallAllowingFailure は失敗するコマンドでもテストを停止せず、stdout・stderr・エラーを返すことを確認する。
func TestCallAllowingFailure(t *testing.T) {
	base := t.TempDir()
	// stdout・stderr を出力して失敗するダミーコマンドを配置する
	writeExecutableWithScript(t, filepath.Join(base, "skr"), "#!/bin/sh\necho ok\necho err >&2\nexit 3\n")
	enterScenarioDir(t, base)

	h := &Helper{name: "test", description: "test", binary: filepath.Join(base, "skr"), defaultTimeout: 10 * time.Second}
	stdout, stderr, err := h.CallAllowingFailure(t, "失敗するコマンドを実行する")
	require.Error(t, err, "the dummy command should fail")
	require.Equal(t, "ok\n", string(stdout))
	require.Equal(t, "err\n", stderr)
	require.Len(t, h.results, 1, "the failed call should be recorded as well")
}

// TestCallAllowingFailureSucceeds は成功するコマンドで CallAllowingFailure がエラーを返さないことを確認する。
func TestCallAllowingFailureSucceeds(t *testing.T) {
	base := t.TempDir()
	writeExecutableWithScript(t, filepath.Join(base, "skr"), "#!/bin/sh\necho ok\n")
	enterScenarioDir(t, base)

	h := &Helper{name: "test", description: "test", binary: filepath.Join(base, "skr"), defaultTimeout: 10 * time.Second}
	stdout, stderr, err := h.CallAllowingFailure(t, "成功するコマンドを実行する")
	require.NoError(t, err, "the dummy command should succeed")
	require.Equal(t, "ok\n", string(stdout))
	require.Empty(t, stderr)
}

// TestCallHidingStdout は stdout に秘密情報が含まれるコマンドで、実行記録に stdout を残さないことを確認する。
func TestCallHidingStdout(t *testing.T) {
	base := t.TempDir()
	// stdout に秘密情報を出力するダミーコマンドを配置する
	writeExecutableWithScript(t, filepath.Join(base, "skr"), "#!/bin/sh\nprintf '%s' '{\"APIKey\":\"test-only-api-key\"}'\n")
	enterScenarioDir(t, base)

	h := &Helper{name: "test", description: "test", binary: filepath.Join(base, "skr"), defaultTimeout: 10 * time.Second}
	stdout := h.CallHidingStdout(t, "API キーを発行する")

	// 呼び出し元には生の stdout を返す
	require.Contains(t, string(stdout), "test-only-api-key", "the caller should receive the raw stdout")

	// 実行記録には秘密情報を残さず、省略したことを記録する
	require.Len(t, h.results, 1, "the call should be recorded")
	rendered := h.results[0].Render()
	require.NotContains(t, rendered, "test-only-api-key", "the report should not contain the secret")
	require.Contains(t, rendered, "省略します", "the report should note the hidden stdout")
}

// TestSleep は Sleep が待機した時間を実行記録に保存することを確認する。
func TestSleep(t *testing.T) {
	h := &Helper{name: "test", description: "test"}

	start := time.Now()
	h.Sleep(t, "反映待ちとして待機する", 20*time.Millisecond)
	elapsed := time.Since(start)

	// 待機した時間を実行記録に保存する
	require.Len(t, h.results, 1, "the sleep should be recorded")
	res := h.results[0]
	require.Equal(t, "反映待ちとして待機する", res.Description())
	require.Contains(t, res.Render(), "20ms", "the report should show the sleep duration")
	require.GreaterOrEqual(t, elapsed, 20*time.Millisecond, "Sleep should wait for the requested duration")
}

// TestReportIncludesTotalDuration はレポートの先頭にシナリオ全体の所要時間が記録されることを確認する。
func TestReportIncludesTotalDuration(t *testing.T) {
	base := t.TempDir()
	// 所要時間が秒単位で記録されるよう、1.2s かかるダミーコマンドを配置する
	writeExecutableWithScript(t, filepath.Join(base, "skr"), "#!/bin/sh\nsleep 1.2\necho ok\n")
	enterScenarioDir(t, base)

	// レポートはサブテストの t.Cleanup で出力される。完了後に内容を確認する
	t.Run("new", func(t *testing.T) {
		// newHelper は上のディレクトリからダミーの skr を見つける
		h := newHelper(t, "test", "test")
		h.Call(t, "ダミーコマンドを実行する")
	})

	files, err := filepath.Glob("e2e-output/test-*.md")
	require.NoError(t, err)
	require.Len(t, files, 1, "the report file should be written by t.Cleanup")
	data, err := os.ReadFile(files[0])
	require.NoError(t, err, "read report file")

	// 所要時間は説明の後、最初のステップより前に秒単位で、生成日時とともに記録される
	require.Regexp(t, `^# test\ntest\n\n\*\*total duration\*\*:\n[1-9]\d*s\n\n\*\*generated at\*\*:\n\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(Z|[+-]\d{2}:\d{2})\n\n## `, string(data))
}

// createProfile は指定されたプロファイルディレクトリに空のプロファイルを作成する。
func createProfile(t *testing.T, dir, name string) {
	t.Helper()

	profileDir := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(profileDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(profileDir, "config.json"), []byte("{}"), 0600))
}

// setupDummyBinary は findBinary が skr バイナリを見つけられるよう、
// テスト専用ディレクトリにダミーの実行ファイルを配置して移動する。
// New と NewWithTemporaryProfileDir のテストは Call を実行しないため、ダミーで十分である。
func setupDummyBinary(t *testing.T) {
	t.Helper()

	base := t.TempDir()
	writeExecutable(t, filepath.Join(base, "skr"))
	enterScenarioDir(t, base)
}

// moveToEmptyRepository は skr バイナリが存在しないリポジトリを模したテスト専用ディレクトリに移動する。
// go.mod が探索の打ち切り位置（リポジトリルート）を決めるため、ダミーの go.mod を配置する。
func moveToEmptyRepository(t *testing.T) {
	t.Helper()

	base := t.TempDir()
	writeGoMod(t, base)
	enterScenarioDir(t, base)
}

// enterScenarioDir はテスト専用ディレクトリに E2E シナリオと同じ test/e2e/service の
// ディレクトリ構造を作成して移動する。
func enterScenarioDir(t *testing.T, base string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Join(base, "test", "e2e", "service"), 0700))
	t.Chdir(filepath.Join(base, "test", "e2e", "service"))
}

// writeExecutable は指定されたパスに実行可能なダミーファイルを作成する。
func writeExecutable(t *testing.T, path string) {
	writeExecutableWithScript(t, path, "#!/bin/sh\n")
}

// writeExecutableWithScript は指定されたスクリプトの実行可能ファイルを作成する。
func writeExecutableWithScript(t *testing.T, path, script string) {
	t.Helper()

	//nolint:gosec // 探索対象の実行可能ファイルとして Stat できることを確認するだけのダミーを、テスト専用ディレクトリ内に作成する。
	require.NoError(t, os.WriteFile(path, []byte(script), 0700))
}

// writeGoMod は指定されたディレクトリに探索の打ち切り位置を決めるための go.mod を作成する。
func writeGoMod(t *testing.T, dir string) {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.27.3\n"), 0600))
}
