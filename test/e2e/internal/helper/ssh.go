package helper

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// sshCommandTimeout は SSH コマンド1回のタイムアウトです。
const sshCommandTimeout = 20 * time.Second

// SSHMaterial は SSH 接続確認に使う一時的な鍵と cloud-init の設定です。
type SSHMaterial struct {
	// PrivateKeyPath は生成した秘密鍵のパスです。
	PrivateKeyPath string
	// KnownHostsPath は SSH のホスト鍵の記録先パスです。
	KnownHostsPath string
	// UserDataPath は cloud-init の user-data ファイルのパスです。
	UserDataPath string
}

// WriteSSHMaterial は ssh-keygen で一時的な SSH 鍵ペアを生成し、
// 指定されたユーザーでログインするための cloud-init user-data を作成します。
// sudo を設定すると、NOPASSWD の sudo 権限を user-data に含めます。
// 公開鍵だけを user-data に記載し、秘密鍵の内容は含めません。
// 秘密鍵と user-data は所有者のみ読める一時ファイルに保存され、テスト終了時に削除されます。
func WriteSSHMaterial(t *testing.T, user string, sudo bool) SSHMaterial {
	t.Helper()

	dir := t.TempDir()
	privateKeyPath := filepath.Join(dir, "ssh-key")
	keygen := exec.Command("ssh-keygen", "-t", "rsa", "-b", "3072", "-N", "", "-f", privateKeyPath) //nolint:gosec // シェルを使わずに一時的なローカルのテストキーを生成する。
	if err := keygen.Run(); err != nil {
		t.Fatalf("generate temporary SSH key: %v", err)
	}
	if err := os.Chmod(privateKeyPath, 0o600); err != nil {
		t.Fatalf("protect temporary private SSH key: %v", err)
	}
	publicKey, err := os.ReadFile(privateKeyPath + ".pub") //nolint:gosec // 秘密鍵はテスト専用の一時ディレクトリ内に作成される。
	if err != nil {
		t.Fatalf("read temporary public SSH key: %v", err)
	}
	userDataPath := filepath.Join(dir, "user-data.yaml")
	userData := sshUserData(user, string(publicKey), sudo)
	if err := os.WriteFile(userDataPath, []byte(userData), 0o600); err != nil {
		t.Fatalf("write temporary cloud-init user-data: %v", err)
	}
	return SSHMaterial{
		PrivateKeyPath: privateKeyPath,
		KnownHostsPath: filepath.Join(dir, "known_hosts"),
		UserDataPath:   userDataPath,
	}
}

// sshUserData は SSH ログイン用ユーザーを登録する cloud-init の user-data を返します。
// 公開鍵だけを user-data に記載し、秘密鍵の内容は含めません。
func sshUserData(user, publicKey string, sudo bool) string {
	userData := fmt.Sprintf("#cloud-config\nusers:\n  - default\n  - name: %s\n    shell: /bin/bash\n", user)
	if sudo {
		userData += "    sudo: ALL=(ALL) NOPASSWD:ALL\n"
	}
	userData += fmt.Sprintf("    ssh_authorized_keys:\n      - %s\n", strings.TrimSpace(publicKey))
	return userData
}

// RunSSHCommand は一時的な SSH 鍵でリモートコマンドを1回実行し、stdout を返します。
// リモートコマンドは固定された引数として渡され、ローカルシェルを経由しません。
// 実行結果は実行記録へ保存されます。成否の判断は呼び出し元が行います。
func (h *Helper) RunSSHCommand(t *testing.T, m SSHMaterial, user, ip, description, remoteCommand string) (string, error) {
	t.Helper()

	stdout, stderr, err := h.RunLocalCommand(t, sshCommandTimeout, "ssh", description, sshConnectArgs(m, user, ip, remoteCommand)...)
	if err != nil {
		return stdout, fmt.Errorf("SSH command failed: %w (stderr: %s)", err, stderr)
	}
	return stdout, nil
}

// ConnectSSH は一時的な SSH 鍵でリモートの hostname を取得し、ログインできることを確認します。
// hostname が取得できない場合や空の出力はエラーとして返します。実行結果は実行記録へ保存されます。
func (h *Helper) ConnectSSH(t *testing.T, m SSHMaterial, user, ip, description string) error {
	t.Helper()

	stdout, err := h.RunSSHCommand(t, m, user, ip, description, "hostname")
	if err != nil {
		return err
	}
	if strings.TrimSpace(stdout) == "" {
		return errors.New("SSH login returned an empty hostname")
	}
	return nil
}

// sshConnectArgs は SSH ログイン確認のコマンド引数を組み立てます。
// バッチモードでパスフレーズや対話を禁止し、未知のホスト鍵は KnownHostsPath へ記録します。
func sshConnectArgs(m SSHMaterial, user, ip, remoteCommand string) []string {
	return []string{
		"-i", m.PrivateKeyPath,
		"-o", "IdentitiesOnly=yes",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=" + m.KnownHostsPath,
		user + "@" + ip,
		remoteCommand,
	}
}
