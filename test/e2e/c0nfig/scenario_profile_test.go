package main

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

	"github.com/stretchr/testify/require"
)

const (
	profileName       = "skr-e2e-config"
	secondProfileName = "skr-e2e-config-second"
)

func TestScenarioProfile(t *testing.T) {
	h := NewHelper(t, "c0nfig", "設定ファイルの作成/更新/確認が可能であることを確認する")
	keyPath := h.WritePrivateKey(t)

	_ = h.Call(t, "新しいプロファイルを作成し、同時に current に設定する",
		"config", "create", profileName,
		"--service-principal-id", "spid-e2e",
		"--service-principal-key-id", "kid-e2e",
		"--private-key-file", keyPath,
		"--use",
	)

	current := h.Call(t, "作成直後の current プロファイル名が期待値であることを確認する", "config", "current")
	require.Equal(t, profileName, strings.TrimSpace(string(current)))

	_ = h.Call(t, "2 つ目のプロファイルを作成する",
		"config", "create", secondProfileName,
		"--service-principal-id", "spid-e2e",
		"--service-principal-key-id", "kid-e2e",
		"--private-key-file", keyPath,
	)

	_ = h.Call(t, "2 つ目のプロファイルを current に切り替える",
		"config", "use", secondProfileName,
	)

	current = h.Call(t, "use 後の current プロファイル名が切り替わっていることを確認する", "config", "current")
	require.Equal(t, secondProfileName, strings.TrimSpace(string(current)))

	list := h.Call(t, "作成した 2 つのプロファイルが一覧に含まれることを確認する", "config", "list")
	for _, name := range []string{profileName, secondProfileName} {
		require.Contains(t, string(list), name, "profile list should contain %q", name)
	}

	show := h.Call(t, "作成時の属性が show 出力に正しく含まれることを確認する",
		"config", "show", profileName)
	attrs := parseProfileAttributes2(t, show)
	requireProfileAttribute(t, attrs, "ServicePrincipalID", "spid-e2e")
	requireProfileAttribute(t, attrs, "ServicePrincipalKeyID", "kid-e2e")
	requireProfileAttribute(t, attrs, "PrivateKeyPEMPath", "(masked)")

	_ = h.Call(t, "ServicePrincipalID を変更する",
		"config", "edit", profileName,
		"--service-principal-id", "spid-edited",
	)

	showAfterEdit := h.Call(t, "編集後の属性が show 出力に反映されていることを確認する", "config", "show", profileName)
	attrsAfterEdit := parseProfileAttributes2(t, showAfterEdit)
	requireProfileAttribute(t, attrsAfterEdit, "ServicePrincipalID", "spid-edited")
	requireProfileAttribute(t, attrsAfterEdit, "ServicePrincipalKeyID", "kid-e2e")
	requireProfileAttribute(t, attrsAfterEdit, "PrivateKeyPEMPath", "(masked)")
}

func parseProfileAttributes2(t *testing.T, data []byte) map[string]any {
	t.Helper()

	var attrs map[string]any
	err := json.Unmarshal(data, &attrs)
	require.NoError(t, err)
	return attrs
}

func requireProfileAttribute(t *testing.T, attrs map[string]any, key, want string) {
	t.Helper()

	got, ok := attrs[key].(string)
	require.True(t, ok, "key %q not found in %q", key, attrs)
	require.Equal(t, want, got, "attribute %q = %q, want %q", key, got, want)
}

type Helper struct {
	name           string
	description    string
	binary         string
	profileDir     string
	defaultTimeout time.Duration
	results        []commandResult
	mu             sync.Mutex
}

type commandResult struct {
	description string
	args        []string
	stdout      string
	stderr      string
}

func NewHelper(t *testing.T, name, description string) *Helper {
	t.Helper()

	if os.Getenv("SKR_E2E") == "" {
		t.Skip("SKR_E2E not set, skipping E2E test")
	}

	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)

	h := &Helper{
		name:           name,
		description:    description,
		binary:         "../../../skr",
		profileDir:     profileDir,
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
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Errorf("failed to create e2e-output directory: %v", err)
			return
		}

		filename := fmt.Sprintf("%s-%s.md", h.name, time.Now().Format("20060102150405"))
		path := filepath.Join(dir, filename)

		var buf bytes.Buffer
		buf.WriteString(fmt.Sprintf("# %s\n%s\n", h.name, h.description))
		for _, res := range h.results {
			buf.WriteString(fmt.Sprintf("## %s\n\n", res.description))
			buf.WriteString("**command**:\n```\n")
			buf.WriteString(strings.Join(append([]string{h.binary}, res.args...), " "))
			buf.WriteString("\n```\n\n")
			buf.WriteString("**stdout**:\n```\n")
			buf.WriteString(res.stdout)
			buf.WriteString("\n```\n\n")
			buf.WriteString("**stderr**:\n```\n")
			buf.WriteString(res.stderr)
			buf.WriteString("\n```\n\n")
		}

		if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
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

	command := exec.CommandContext(ctx, h.binary, args...) //nolint:gosec // 選択されたローカルの skr 実行ファイルと構造化された引数を、シェルなしで直接実行する。
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()

	h.mu.Lock()
	h.results = append(h.results, commandResult{
		description: description,
		args:        args,
		stdout:      stdout.String(),
		stderr:      stderr.String(),
	})
	h.mu.Unlock()

	require.NoError(t, runErr, stderr.String())
	return stdout.Bytes()
}
