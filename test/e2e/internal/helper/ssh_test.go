package helper

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWriteSSHMaterial は一時的な SSH 鍵ペアと cloud-init の user-data の作成を確認する。
func TestWriteSSHMaterial(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen is not available, skipping test")
	}

	m := WriteSSHMaterial(t, "skr-e2e", false)

	// 秘密鍵は所有者のみ読めるパーミッションで作成される
	require.FileExists(t, m.PrivateKeyPath, "the private key should be created")
	info, err := os.Stat(m.PrivateKeyPath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())

	// user-data は所有者のみ読めるパーミッションで、公開鍵だけを記載する
	require.FileExists(t, m.UserDataPath, "the cloud-init user-data should be created")
	data, err := os.ReadFile(m.UserDataPath)
	require.NoError(t, err, "read cloud-init user-data")
	userData := string(data)
	require.Equal(t, os.FileMode(0600), func() os.FileMode {
		info, err := os.Stat(m.UserDataPath)
		require.NoError(t, err)
		return info.Mode().Perm()
	}())
	require.Contains(t, userData, "#cloud-config", "the user-data should start with the cloud-config directive")
	require.Contains(t, userData, "name: skr-e2e", "the user-data should register the requested user")
	require.NotContains(t, userData, "sudo:", "the user-data should not grant sudo unless requested")

	publicKey, err := os.ReadFile(m.PrivateKeyPath + ".pub")
	require.NoError(t, err, "read temporary public SSH key")
	require.Contains(t, userData, strings.TrimSpace(string(publicKey)), "the user-data should contain the generated public key")
	privateKey, err := os.ReadFile(m.PrivateKeyPath)
	require.NoError(t, err, "read temporary private SSH key")
	require.NotContains(t, userData, strings.TrimSpace(string(privateKey)), "the user-data should not contain the private key")

	// sudo を設定すると NOPASSWD の sudo 権限を user-data に含める
	sudoMaterial := WriteSSHMaterial(t, "skr-e2e-sudo", true)
	sudoData, err := os.ReadFile(sudoMaterial.UserDataPath)
	require.NoError(t, err, "read cloud-init user-data with sudo")
	require.Contains(t, string(sudoData), "sudo: ALL=(ALL) NOPASSWD:ALL", "the user-data should grant NOPASSWD sudo")

	// ホスト鍵の記録先は鍵と同じテスト専用ディレクトリに置く
	require.Equal(t, filepath.Dir(m.PrivateKeyPath), filepath.Dir(m.KnownHostsPath))
}

// TestWriteSSHMaterialWithRuncmd は追加の runcmd を含む cloud-init user-data の作成を確認する。
func TestWriteSSHMaterialWithRuncmd(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen is not available, skipping test")
	}

	m := WriteSSHMaterialWithRuncmd(t, "skr-e2e", false, "ip route replace default via 192.0.2.1", "echo done")

	data, err := os.ReadFile(m.UserDataPath)
	require.NoError(t, err, "read cloud-init user-data with runcmd")
	userData := string(data)
	require.Contains(t, userData, "#cloud-config")
	require.Contains(t, userData, "name: skr-e2e")
	require.Contains(t, userData, "runcmd:", "the user-data should contain the runcmd section")
	require.Contains(t, userData, `[sh, -c, "ip route replace default via 192.0.2.1"]`)
	require.Contains(t, userData, `[sh, -c, "echo done"]`)

	// runcmd を渡さない場合は従来と同じ user-data を作成する
	plain := WriteSSHMaterial(t, "skr-e2e", false)
	plainData, err := os.ReadFile(plain.UserDataPath)
	require.NoError(t, err, "read cloud-init user-data without runcmd")
	require.NotContains(t, string(plainData), "runcmd:")
}

// TestSSHConnectArgs は SSH ログイン確認のコマンド引数を確認する。
func TestSSHConnectArgs(t *testing.T) {
	m := SSHMaterial{
		PrivateKeyPath: "/tmp/ssh-key",
		KnownHostsPath: "/tmp/known_hosts",
		UserDataPath:   "/tmp/user-data.yaml",
	}

	args := sshConnectArgs(m, "skr-e2e", "192.0.2.10", "hostname")

	joined := strings.Join(args, " ")
	require.Contains(t, joined, "-i /tmp/ssh-key", "the args should use the temporary private key")
	require.Contains(t, joined, "BatchMode=yes", "the args should disable interactive prompts")
	require.Contains(t, joined, "IdentitiesOnly=yes", "the args should only use the temporary key")
	require.Contains(t, joined, "UserKnownHostsFile=/tmp/known_hosts", "the args should record host keys in the temporary file")
	require.Contains(t, joined, "skr-e2e@192.0.2.10", "the args should target the requested user and address")
	require.Equal(t, "hostname", args[len(args)-1], "the remote command should be the last argument")
	require.NotContains(t, joined, m.UserDataPath, "the SSH args should not reference the cloud-init user-data")
}
