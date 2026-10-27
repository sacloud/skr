package helper

import (
	"os"
	"path/filepath"
	"testing"

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

	// skr バイナリが存在しない状態にするため、テスト専用ディレクトリに移動する
	t.Chdir(t.TempDir())

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
	// skr バイナリが存在しない状態にするため、テスト専用ディレクトリに移動する
	t.Chdir(t.TempDir())

	completed := false
	t.Run("new", func(t *testing.T) {
		NewWithTemporaryProfileDir(t, "test", "test")
		completed = true
	})
	if completed {
		t.Error("NewWithTemporaryProfileDir should skip when the skr binary is missing")
	}
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

// createProfile は指定されたプロファイルディレクトリに空のプロファイルを作成する。
func createProfile(t *testing.T, dir, name string) {
	t.Helper()

	profileDir := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(profileDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(profileDir, "config.json"), []byte("{}"), 0600))
}

// setupDummyBinary は newHelper が参照する ../../../skr が解決できるよう、
// テスト専用ディレクトリにダミーの実行ファイルを配置して移動する。
// New と NewWithTemporaryProfileDir のテストは Call を実行しないため、ダミーで十分である。
func setupDummyBinary(t *testing.T) {
	t.Helper()

	// テストの cwd から 3 つ上に skr バイナリが解決されるため、
	// E2E シナリオと同じ 3 段階の深さのディレクトリを作成してダミーを配置する。
	base := t.TempDir()
	scenarioDir := filepath.Join(base, "test", "e2e", "service")
	require.NoError(t, os.MkdirAll(scenarioDir, 0700))
	//nolint:gosec // 実行ファイルとして Stat できることを確認するだけのダミーを、テスト専用ディレクトリ内に作成する。
	require.NoError(t, os.WriteFile(filepath.Join(base, "skr"), []byte("#!/bin/sh\n"), 0700))
	t.Chdir(scenarioDir)
}
