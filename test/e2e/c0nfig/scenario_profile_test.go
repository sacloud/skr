package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	profileName       = "skr-e2e-config"
	secondProfileName = "skr-e2e-config-second"
)

func TestScenarioProfile(t *testing.T) {
	h := helper.NewWithTemporaryProfileDir(t, "c0nfig", "設定ファイルの作成/更新/確認が可能であることを確認する")
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
