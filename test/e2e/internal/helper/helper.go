package helper

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
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
	startedAt      time.Time
	results        []Result
	mu             sync.Mutex
}

// e2eProfileName はライブ API 認証に使うプロファイル名です。
const e2eProfileName = "skr-e2e"

// skrBinaryName は helper が実行するビルド済み CLI バイナリのファイル名です。
const skrBinaryName = "skr"

// findBinary はカレントディレクトリから上に向かって実行可能な skr バイナリを探します。
// 探索は go.mod があるディレクトリ（リポジトリルート）まで行い、見つからない場合は空文字列を返します。
func findBinary() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dir := wd
	for {
		path := filepath.Join(dir, skrBinaryName)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
			return path, nil
		}

		// リポジトリルート（go.mod）まで辿ったら探索を打ち切る
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return "", nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

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

	// カレントディレクトリから上に向かってビルド済みの skr バイナリを探す。
	// 見つからない場合はテストをスキップする。
	binary, err := findBinary()
	require.NoError(t, err, "could not get working directory: %v", err)
	if binary == "" {
		t.Skipf("skr binary is not available, run \"make build\" first, skipping E2E test")
	}

	h := &Helper{
		name:           name,
		description:    description,
		binary:         binary,
		defaultTimeout: 10 * time.Minute,
		startedAt:      time.Now(),
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
		// レポートの先頭にシナリオ全体の所要時間と生成日時を記録する。
		// 所要時間は1分以上「1m47s」のように分と秒で表示する。
		buf.WriteString(fmt.Sprintf("\n**total duration**:\n%v\n\n", time.Since(h.startedAt).Round(time.Second)))
		buf.WriteString(fmt.Sprintf("**generated at**:\n%s\n\n", time.Now().Format(time.RFC3339)))
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

	stdout, stderr, err := h.run(t, timeout, description, args...)
	require.NoError(t, err, stderr.String())
	return stdout.Bytes()
}

// CallAllowingFailure は指定されたコマンドを実行し、stdout・stderr・エラーを返す。
// コマンドの失敗はテストを停止しない。実行結果は Call と同様に実行記録へ保存する。
// 失敗が想定内かどうかの判断は呼び出し元が行う。
func (h *Helper) CallAllowingFailure(t *testing.T, description string, args ...string) ([]byte, string, error) {
	t.Helper()

	stdout, stderr, err := h.run(t, h.defaultTimeout, description, args...)
	return stdout.Bytes(), stderr.String(), err
}

// run は指定されたコマンドを実行し、stdout・stderr・エラーを返す。実行結果は実行記録へ保存する。
func (h *Helper) run(t *testing.T, timeout time.Duration, description string, args ...string) (bytes.Buffer, bytes.Buffer, error) {
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

	return stdout, stderr, runErr
}

// Sleep は指定された時間だけ待機し、待機した時間を実行記録へ保存する。
// 反映待ちなどで待機した時間もレポートに残す。実行記録は待機の開始前に追加する。
func (h *Helper) Sleep(t *testing.T, description string, d time.Duration) {
	t.Helper()

	t.Logf("%s: %s", description, d)

	h.mu.Lock()
	h.results = append(h.results, &sleepResult{
		description: description,
		duration:    d,
	})
	h.mu.Unlock()

	time.Sleep(d)
}

// WriteRequestJSONFile は --request @path で CLI に渡すリクエストファイルを作成し、パスを返します。
func (h *Helper) WriteRequestJSONFile(t *testing.T, name string, value any) string {
	t.Helper()

	data, err := json.Marshal(value)
	require.NoError(t, err, "marshal request %s: %v", name, err)

	t.Logf("Writing %s.json: %s", name, string(data))

	h.mu.Lock()
	h.results = append(h.results, &requestResult{
		name: name,
		data: string(data),
	})
	h.mu.Unlock()

	path := filepath.Join(t.TempDir(), name+".json")
	err = os.WriteFile(path, data, 0o600)
	require.NoError(t, err, "write request file %s: %v", name, err)
	return path
}
