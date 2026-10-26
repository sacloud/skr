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

// createProfile は指定されたプロファイルディレクトリに空のプロファイルを作成する。
func createProfile(t *testing.T, dir, name string) {
	t.Helper()

	profileDir := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(profileDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(profileDir, "config.json"), []byte("{}"), 0600))
}
