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

package cli

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/common/saclient"
	"github.com/stretchr/testify/require"
)

func TestRunConfigList(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	require.NoError(t, err)
	for _, name := range []string{"default", "production", "staging"} {
		require.NoError(t, profileOp.Create(&saclient.Profile{
			Name:       name,
			Attributes: map[string]any{},
		}))
	}
	err = profileOp.SetCurrentName("production")
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"config", "list"}, &stdout, &stderr), "stderr: %s", stderr.String())

	got := stdout.String()
	for _, name := range []string{"default", "production", "staging"} {
		require.Contains(t, got, name, "stdout = %q, want to contain %q", got, name)
	}
	// stdout は bytes.Buffer なので端末判定が false となり、マーカーは出力されない。
	require.NotContains(t, got, "* ", "stdout = %q, want no current-profile marker", got)
	got2 := stderr.String()
	require.Empty(t, got2, "stderr = %q, want empty", got2)
}

func TestRunConfigListWithoutSelection(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	require.NoError(t, err)
	require.NoError(t, profileOp.Create(&saclient.Profile{
		Name:       "default",
		Attributes: map[string]any{},
	}))

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"config", "list"}, &stdout, &stderr), "stderr: %s", stderr.String())
	if got, want := stdout.String(), "default\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	got := stderr.String()
	require.Empty(t, got, "stderr = %q, want empty", got)
}

func TestRunConfigListWithUnreadableCurrentProfile(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	err := os.Mkdir(filepath.Join(profileDir, "current"), 0o700)
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"config", "list"}, &stdout, &stderr), "run should fail")
	got := stdout.String()
	require.Empty(t, got, "stdout = %q, want empty", got)
	if got := stderr.String(); got == "" {
		t.Error("stderr is empty, want an error reading the current profile")
	}
}

func TestRunConfigListEmpty(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"config", "list"}, &stdout, &stderr), "stderr: %s", stderr.String())
	got := stdout.String()
	require.Empty(t, got, "stdout = %q, want empty", got)
	got2 := stderr.String()
	require.Empty(t, got2, "stderr = %q, want empty", got2)
}

func TestRunConfigShowCurrent(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	require.NoError(t, err)
	require.NoError(t, profileOp.Create(&saclient.Profile{
		Name:       "production",
		Attributes: map[string]any{"zone": "is1a"},
	}))
	err = profileOp.SetCurrentName("production")
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"config", "show"}, &stdout, &stderr), "stderr: %s", stderr.String())

	var attrs map[string]any
	err2 := json.Unmarshal(stdout.Bytes(), &attrs)
	require.NoError(t, err2, "decode stdout: %v")
	if got, want := attrs["zone"], "is1a"; got != want {
		t.Errorf("zone = %q, want %q", got, want)
	}
	got := stderr.String()
	require.Empty(t, got, "stderr = %q, want empty", got)
}

func TestRunConfigShowNamed(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	require.NoError(t, err)
	for _, name := range []string{"default", "production"} {
		require.NoError(t, profileOp.Create(&saclient.Profile{
			Name:       name,
			Attributes: map[string]any{"name": name},
		}))
	}
	err = profileOp.SetCurrentName("default")
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"config", "show", "production"}, &stdout, &stderr), "stderr: %s", stderr.String())

	var attrs map[string]any
	err2 := json.Unmarshal(stdout.Bytes(), &attrs)
	require.NoError(t, err2, "decode stdout: %v")
	if got, want := attrs["name"], "production"; got != want {
		t.Errorf("name = %q, want %q", got, want)
	}
	got := stderr.String()
	require.Empty(t, got, "stderr = %q, want empty", got)
}

func TestRunConfigShowWithoutCurrent(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())

	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"config", "show"}, &stdout, &stderr), "run should fail")
	got := stdout.String()
	require.Empty(t, got, "stdout = %q, want empty", got)
	got2 := stderr.String()
	require.Contains(t, got2, "current", "stderr = %q, want an error mentioning the current profile", got2)
}

func TestRunConfigUse(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	require.NoError(t, err)
	for _, name := range []string{"default", "production"} {
		require.NoError(t, profileOp.Create(&saclient.Profile{
			Name:       name,
			Attributes: map[string]any{},
		}))
	}
	err = profileOp.SetCurrentName("default")
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"config", "use", "production"}, &stdout, &stderr), "stderr: %s", stderr.String())
	got := stdout.String()
	require.Empty(t, got, "stdout = %q, want empty", got)
	got2 := stderr.String()
	require.Empty(t, got2, "stderr = %q, want empty", got2)

	current, err := profileOp.GetCurrentName()
	require.NoError(t, err)
	if got, want := current, "production"; got != want {
		t.Errorf("current profile = %q, want %q", got, want)
	}
}

func TestRunConfigUseDefaultsToCurrent(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	require.NoError(t, err)
	require.NoError(t, profileOp.Create(&saclient.Profile{
		Name:       "production",
		Attributes: map[string]any{},
	}))
	err = profileOp.SetCurrentName("production")
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"config", "use"}, &stdout, &stderr), "stderr: %s", stderr.String())

	current, err := profileOp.GetCurrentName()
	require.NoError(t, err)
	got, want := current, "production"
	require.Equal(t, want, got, "current profile = %q, want %q")
}

func TestRunConfigUseUnknownProfile(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	require.NoError(t, err)
	require.NoError(t, profileOp.Create(&saclient.Profile{
		Name:       "default",
		Attributes: map[string]any{},
	}))

	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"config", "use", "missing"}, &stdout, &stderr), "run should fail")
	got := stdout.String()
	require.Empty(t, got, "stdout = %q, want empty", got)
	got2 := stderr.String()
	require.Contains(t, got2, "missing", "stderr = %q, want an error mentioning the profile", got2)
}

func TestRunConfigUseInvalidName(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())

	for _, invalidName := range []string{".", "..", "a/b"} {
		t.Run(invalidName, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.NotEqual(t, 0, run([]string{"config", "use", invalidName}, &stdout, &stderr), "run should fail")
			if got := stderr.String(); got == "" {
				t.Error("stderr is empty, want an error")
			}
		})
	}
}

func TestRunConfigUseWithoutCurrent(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())

	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"config", "use"}, &stdout, &stderr), "run should fail")
	got := stdout.String()
	require.Empty(t, got, "stdout = %q, want empty", got)
	got2 := stderr.String()
	require.Contains(t, got2, "current", "stderr = %q, want an error mentioning the current profile", got2)
}

func TestRunConfigShowUnknownProfile(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)

	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"config", "show", "missing"}, &stdout, &stderr), "run should fail")
	got := stdout.String()
	require.Empty(t, got, "stdout = %q, want empty", got)
	got2 := stderr.String()
	require.Contains(t, got2, "missing", "stderr = %q, want an error mentioning the profile", got2)
}

func writeTestPrivateKey(t *testing.T, dir string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	path := filepath.Join(dir, "key.pem")
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	err = os.WriteFile(path, pem.EncodeToMemory(block), 0o600)
	require.NoError(t, err)
	return path
}

func TestRunConfigCreateNonInteractive(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := writeTestPrivateKey(t, profileDir)

	var stdout, stderr bytes.Buffer
	args := []string{
		"config", "create", "prod",
		"--service-principal-id", "spid-123",
		"--service-principal-key-id", "kid-456",
		"--private-key-file", keyPath,
	}
	require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())
	got := stderr.String()
	require.Empty(t, got, "stderr = %q, want empty", got)
	got2 := stdout.String()
	require.Contains(t, got2, "prod", "stdout = %q, want creation message", got2)
	require.Contains(t, got2, "created", "missing expected text %q", "created")

	profileOp, err := saclient.NewProfileOp(os.Environ())
	require.NoError(t, err)
	profile, err := profileOp.Read("prod")
	require.NoError(t, err)
	if got, want := profile.Attributes[attrServicePrincipalID], "spid-123"; got != want {
		t.Errorf("ServicePrincipalID = %q, want %q", got, want)
	}
	if got, want := profile.Attributes[attrServicePrincipalKeyID], "kid-456"; got != want {
		t.Errorf("ServicePrincipalKeyID = %q, want %q", got, want)
	}
	if got, want := profile.Attributes[attrPrivateKeyPEMPath], keyPath; got != want {
		t.Errorf("PrivateKeyPEMPath = %q, want %q", got, want)
	}

	current, err := profileOp.GetCurrentName()
	require.Error(t, err, "current profile = %q, want no current profile set", current)
}

func TestRunConfigCreateWithUse(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := writeTestPrivateKey(t, profileDir)

	var stdout, stderr bytes.Buffer
	args := []string{
		"config", "create", "prod",
		"--service-principal-id", "spid-123",
		"--service-principal-key-id", "kid-456",
		"--private-key-file", keyPath,
		"--use",
	}
	require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())

	profileOp, err := saclient.NewProfileOp(os.Environ())
	require.NoError(t, err)
	current, err := profileOp.GetCurrentName()
	require.NoError(t, err)
	got, want := current, "prod"
	require.Equal(t, want, got, "current profile = %q, want %q")
}

func TestRunConfigCreateInvalidName(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := writeTestPrivateKey(t, profileDir)
	baseArgs := []string{"--service-principal-id", "spid", "--service-principal-key-id", "kid", "--private-key-file", keyPath}

	for _, invalidName := range []string{".", "..", "a/b"} {
		t.Run(invalidName, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := append([]string{"config", "create", invalidName}, baseArgs...)
			require.NotEqual(t, 0, run(args, &stdout, &stderr), "run should fail")
			if got := stderr.String(); got == "" {
				t.Error("stderr is empty, want an error")
			}
		})
	}
}

func TestRunConfigCreateDefaultName(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := writeTestPrivateKey(t, profileDir)

	var stdout, stderr bytes.Buffer
	args := []string{
		"config", "create",
		"--service-principal-id", "spid-123",
		"--service-principal-key-id", "kid-456",
		"--private-key-file", keyPath,
	}
	require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())

	profileOp, err := saclient.NewProfileOp(os.Environ())
	require.NoError(t, err)
	if _, err := profileOp.Read("default"); err != nil {
		t.Fatalf("read default profile: %v", err)
	}
}

func TestRunConfigCreateExistingProfileFails(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	require.NoError(t, err)
	err = profileOp.Create(&saclient.Profile{Name: "prod", Attributes: map[string]any{}})
	require.NoError(t, err)
	keyPath := writeTestPrivateKey(t, profileDir)

	var stdout, stderr bytes.Buffer
	args := []string{
		"config", "create", "prod",
		"--service-principal-id", "spid-123",
		"--service-principal-key-id", "kid-456",
		"--private-key-file", keyPath,
	}
	require.NotEqual(t, 0, run(args, &stdout, &stderr), "run should fail")
	got := stderr.String()
	require.Contains(t, got, "already exists", "stderr = %q, want already exists error", got)
}

func TestRunConfigCreateMissingFlagsFails(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := writeTestPrivateKey(t, profileDir)

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "missing service principal id",
			args: []string{"config", "create", "prod", "--service-principal-key-id", "kid", "--private-key-file", keyPath},
		},
		{
			name: "missing service principal key id",
			args: []string{"config", "create", "prod", "--service-principal-id", "spid", "--private-key-file", keyPath},
		},
		{
			name: "missing private key file",
			args: []string{"config", "create", "prod", "--service-principal-id", "spid", "--service-principal-key-id", "kid"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.NotEqual(t, 0, run(tt.args, &stdout, &stderr), "run should fail")
			if got := stderr.String(); got == "" {
				t.Error("stderr is empty, want an error")
			}
		})
	}
}

func TestRunConfigCreateInvalidPrivateKeyFails(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := filepath.Join(profileDir, "invalid.pem")
	err := os.WriteFile(keyPath, []byte("not a key"), 0o600)
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	args := []string{
		"config", "create", "prod",
		"--service-principal-id", "spid-123",
		"--service-principal-key-id", "kid-456",
		"--private-key-file", keyPath,
	}
	require.NotEqual(t, 0, run(args, &stdout, &stderr), "run should fail")
	got := stderr.String()
	require.Contains(t, got, "private key", "stderr = %q, want private key error", got)
}

func TestRunConfigCreateInsecurePrivateKeyPermissionsFail(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("private key permission validation is not supported on Windows")
	}

	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := writeTestPrivateKey(t, profileDir)
	if err := os.Chmod(keyPath, 0o644); err != nil { // #nosec G302 -- 緩いパーミッションは意図的なものであり、このテストは CLI が安全でない秘密鍵ファイルを拒否することを検証する。
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	args := []string{
		"config", "create", "prod",
		"--service-principal-id", "spid-123",
		"--service-principal-key-id", "kid-456",
		"--private-key-file", keyPath,
	}
	require.NotEqual(t, 0, run(args, &stdout, &stderr), "run should fail")
	got := stderr.String()
	require.Contains(t, got, "permissions are too lax", "stderr = %q, want private key permission error", got)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	require.NoError(t, err)
	if _, err := profileOp.Read("prod"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("read profile error = %v, want os.ErrNotExist", err)
	}
}

func TestRunConfigEditNonInteractive(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	oldKey := writeTestPrivateKey(t, profileDir)
	newKey := writeTestPrivateKey(t, profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	require.NoError(t, err)
	require.NoError(t, profileOp.Create(&saclient.Profile{
		Name: "prod",
		Attributes: map[string]any{
			attrServicePrincipalID:    "old-spid",
			attrServicePrincipalKeyID: "old-kid",
			attrPrivateKeyPEMPath:     oldKey,
			"zone":                    "is1a",
		},
	}))
	err = profileOp.SetCurrentName("prod")
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	args := []string{
		"config", "edit", "prod",
		"--service-principal-key-id", "new-kid",
		"--private-key-file", newKey,
	}
	require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())

	profile, err := profileOp.Read("prod")
	require.NoError(t, err)
	if got, want := profile.Attributes[attrServicePrincipalID], "old-spid"; got != want {
		t.Errorf("ServicePrincipalID = %q, want %q", got, want)
	}
	if got, want := profile.Attributes[attrServicePrincipalKeyID], "new-kid"; got != want {
		t.Errorf("ServicePrincipalKeyID = %q, want %q", got, want)
	}
	if got, want := profile.Attributes[attrPrivateKeyPEMPath], newKey; got != want {
		t.Errorf("PrivateKeyPEMPath = %q, want %q", got, want)
	}
	if got, want := profile.Attributes["zone"], "is1a"; got != want {
		t.Errorf("zone = %q, want %q", got, want)
	}
}

func TestRunConfigEditInsecurePrivateKeyPermissionsFail(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("private key permission validation is not supported on Windows")
	}

	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	oldKey := writeTestPrivateKey(t, profileDir)
	newKey := filepath.Join(profileDir, "insecure.pem")
	if err := os.WriteFile(newKey, []byte("not used"), 0o644); err != nil { // #nosec G306 -- 緩いパーミッションは意図的なものであり、このテストは CLI が安全でない秘密鍵ファイルを拒否することを検証する。
		t.Fatal(err)
	}

	profileOp, err := saclient.NewProfileOp(os.Environ())
	require.NoError(t, err)
	attributes := map[string]any{
		attrServicePrincipalID:    "old-spid",
		attrServicePrincipalKeyID: "old-kid",
		attrPrivateKeyPEMPath:     oldKey,
	}
	err = profileOp.Create(&saclient.Profile{Name: "prod", Attributes: attributes})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	args := []string{"config", "edit", "prod", "--private-key-file", newKey}
	require.NotEqual(t, 0, run(args, &stdout, &stderr), "run should fail")
	got := stderr.String()
	require.Contains(t, got, "permissions are too lax", "stderr = %q, want private key permission error", got)

	profile, err := profileOp.Read("prod")
	require.NoError(t, err)
	if got, want := profile.Attributes[attrPrivateKeyPEMPath], oldKey; got != want {
		t.Errorf("PrivateKeyPEMPath = %q, want %q", got, want)
	}
}

func TestRunConfigEditNoFlagsFails(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := writeTestPrivateKey(t, profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	require.NoError(t, err)
	require.NoError(t, profileOp.Create(&saclient.Profile{
		Name:       "prod",
		Attributes: map[string]any{attrServicePrincipalID: "spid", attrServicePrincipalKeyID: "kid", attrPrivateKeyPEMPath: keyPath},
	}))
	err = profileOp.SetCurrentName("prod")
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	require.NotEqual(t, 0, run([]string{"config", "edit"}, &stdout, &stderr), "run should fail")
	if got := stderr.String(); got == "" {
		t.Error("stderr is empty, want an error")
	}
}

func TestRunConfigEditUseSwitchesCurrent(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := writeTestPrivateKey(t, profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	require.NoError(t, err)
	for _, name := range []string{"default", "prod"} {
		attrs := map[string]any{attrServicePrincipalID: "spid", attrServicePrincipalKeyID: "kid", attrPrivateKeyPEMPath: keyPath}
		err := profileOp.Create(&saclient.Profile{Name: name, Attributes: attrs})
		require.NoError(t, err)
	}
	err = profileOp.SetCurrentName("default")
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	args := []string{"config", "edit", "prod", "--service-principal-id", "new-spid", "--use"}
	require.Equal(t, 0, run(args, &stdout, &stderr), "stderr: %s", stderr.String())

	current, err := profileOp.GetCurrentName()
	require.NoError(t, err)
	got, want := current, "prod"
	require.Equal(t, want, got, "current profile = %q, want %q")
}

func TestRunConfigEditUseSwitchesCurrentWithoutChanges(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := writeTestPrivateKey(t, profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	require.NoError(t, err)
	for _, name := range []string{"default", "prod"} {
		attrs := map[string]any{attrServicePrincipalID: "spid", attrServicePrincipalKeyID: "kid", attrPrivateKeyPEMPath: keyPath}
		err := profileOp.Create(&saclient.Profile{Name: name, Attributes: attrs})
		require.NoError(t, err)
	}
	err = profileOp.SetCurrentName("default")
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"config", "edit", "prod", "--use"}, &stdout, &stderr), "stderr: %s", stderr.String())

	current, err := profileOp.GetCurrentName()
	require.NoError(t, err)
	got, want := current, "prod"
	require.Equal(t, want, got, "current profile = %q, want %q")
}

func TestRunConfigShowMasksSensitiveAttributes(t *testing.T) {
	profileDir := t.TempDir()
	t.Setenv("SAKURA_PROFILE_DIR", profileDir)
	keyPath := writeTestPrivateKey(t, profileDir)

	profileOp, err := saclient.NewProfileOp(os.Environ())
	require.NoError(t, err)
	if err := profileOp.Create(&saclient.Profile{
		Name: "prod",
		Attributes: map[string]any{
			"zone":                    "is1a",
			attrServicePrincipalID:    "spid-123",
			attrServicePrincipalKeyID: "kid-456",
			attrPrivateKeyPEMPath:     keyPath,
			"AccessToken":             "token",
			"AccessTokenSecret":       "secret",
			"PrivateKey":              "pem-content",
		},
	}); err != nil {
		t.Fatal(err)
	}
	err = profileOp.SetCurrentName("prod")
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"config", "show"}, &stdout, &stderr), "stderr: %s", stderr.String())

	var attrs map[string]any
	err2 := json.Unmarshal(stdout.Bytes(), &attrs)
	require.NoError(t, err2, "decode stdout: %v")
	if got, want := attrs["zone"], "is1a"; got != want {
		t.Errorf("zone = %q, want %q", got, want)
	}
	if got, want := attrs[attrServicePrincipalID], "spid-123"; got != want {
		t.Errorf("ServicePrincipalID = %q, want %q", got, want)
	}
	if got, want := attrs[attrServicePrincipalKeyID], "kid-456"; got != want {
		t.Errorf("ServicePrincipalKeyID = %q, want %q", got, want)
	}
	for _, key := range []string{"AccessToken", "AccessTokenSecret", "PrivateKey", attrPrivateKeyPEMPath} {
		if got := attrs[key]; got != "(masked)" {
			t.Errorf("%s = %q, want masked", key, got)
		}
	}
	got := stderr.String()
	require.Empty(t, got, "stderr = %q, want empty", got)
}
