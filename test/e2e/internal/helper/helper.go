package helper

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/common/saclient"
	"github.com/stretchr/testify/require"
)

type Helper struct {
	name           string
	description    string
	binary         string
	defaultTimeout time.Duration
	results        []Result
	mu             sync.Mutex
}

// e2eProfileName はライブ API 認証に使うプロファイル名です。
const e2eProfileName = "skr-e2e"

func New(t *testing.T, name, description string) *Helper {
	t.Helper()

	// ライブ実行は SKR_E2E の設定を明示的な同意とする。通常の go test では実環境を変更しない。
	if os.Getenv("SKR_E2E") == "" {
		t.Skip("SKR_E2E not set, skipping E2E test")
	}

	t.Setenv("SAKURA_PROFILE", e2eProfileName)

	// ライブ API の認証に使う skr-e2e プロファイルが存在しない場合はテストをスキップする。
	op, err := saclient.NewProfileOp(os.Environ())
	require.NoError(t, err, "could not create profile operator: %v", err)
	if _, err := op.Read(e2eProfileName); err != nil {
		t.Skipf("profile %q is not available in %s, skipping E2E test: %v", e2eProfileName, op.Dir(), err)
	}

	return newHelper(t, name, description)
}

// NewWithTemporaryProfileDir はテスト専用の一時プロファイルディレクトリを使う helper を返します。
// テスト自身が config コマンドなどでプロファイルを作成・管理するシナリオで使用し、
// ライブ API 認証用の skr-e2e プロファイルの有無は確認しません。
func NewWithTemporaryProfileDir(t *testing.T, name, description string) *Helper {
	t.Helper()

	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())

	return newHelper(t, name, description)
}

func newHelper(t *testing.T, name, description string) *Helper {
	t.Helper()

	h := &Helper{
		name:           name,
		description:    description,
		binary:         "../../../skr",
		defaultTimeout: 10 * time.Minute,
	}

	t.Cleanup(func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if len(h.results) == 0 {
			return
		}

		// $PROJECTROOT/e2e-output/${h.name}-YYYYMMDDHHMMSS.md を出力する。
		// 実行されたコマンドの出力結果を記録する。
		dir := "e2e-output"
		if err := os.MkdirAll(dir, 0750); err != nil {
			t.Errorf("failed to create e2e-output directory: %v", err)
			return
		}

		filename := fmt.Sprintf("%s-%s.md", h.name, time.Now().Format("20060102150405"))
		path := filepath.Join(dir, filename)

		var buf bytes.Buffer
		buf.WriteString(fmt.Sprintf("# %s\n%s\n", h.name, h.description))
		for _, res := range h.results {
			buf.WriteString(fmt.Sprintf("## %s\n\n", res.Description()))
			buf.WriteString(res.Render())
		}

		if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
			t.Errorf("failed to write report file: %v", err)
		}
		t.Logf("report file: %s", path)
	})

	return h
}

func (h *Helper) WritePrivateKey(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "could not generate rsa key: %v", err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err, "could not marshal private key: %v", err)
	path := filepath.Join(dir, "key.pem")
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	err = os.WriteFile(path, pem.EncodeToMemory(block), 0o600)
	require.NoError(t, err, "could not write private key: %v", err)
	return path
}

func (h *Helper) Call(t *testing.T, description string, args ...string) []byte {
	t.Helper()

	return h.CallWithTimeout(t, h.defaultTimeout, description, args...)
}

// CallWithTimeout は指定されたコマンドを実行する。タイムアウト秒数経過してもコマンドが終了しない場合はエラーとなる。
// 返り値は stdout のバイト列
func (h *Helper) CallWithTimeout(t *testing.T, timeout time.Duration, description string, args ...string) []byte {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()

	t.Logf("%s: %s", description, strings.Join(args, " "))

	command := exec.CommandContext(ctx, h.binary, args...) //nolint:gosec // 選択されたローカルの skr 実行ファイルと構造化された引数を、シェルなしで直接実行する。
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	start := time.Now()
	runErr := command.Run()

	h.mu.Lock()
	h.results = append(h.results, &commandResult{
		description: description,
		args:        args,
		stdout:      stdout.String(),
		stderr:      stderr.String(),
		duration:    time.Since(start),
	})
	h.mu.Unlock()

	require.NoError(t, runErr, stderr.String())
	return stdout.Bytes()
}
