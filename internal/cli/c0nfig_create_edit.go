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
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"
	"golang.org/x/term"
)

const (
	attrServicePrincipalID    = "ServicePrincipalID"
	attrServicePrincipalKeyID = "ServicePrincipalKeyID"
	attrPrivateKeyPEMPath     = "PrivateKeyPEMPath"
)

type createCommand struct {
	Name                string `arg:"" optional:"" help:"Profile 名。省略時は default です。"`
	ServicePrincipalID  string `name:"service-principal-id" help:"サービスプリンシパル ID。"`
	ServicePrincipalKey string `name:"service-principal-key-id" help:"サービスプリンシパルキーの ID (KID)。"`
	PrivateKeyFile      string `name:"private-key-file" help:"RSA 秘密鍵の PEM ファイルパス。"`
	Use                 bool   `name:"use" help:"作成した profile を現在の profile に設定します。"`
}

func (c *createCommand) Run(ctx *kong.Context) error {
	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		return err
	}

	interactive := isTerminal()
	name := c.Name
	if name == "" && interactive {
		name, err = promptProfileName("Enter profile name", "default", ctx.Stdout)
		if err != nil {
			return err
		}
	}
	if name == "" {
		name = "default"
	}
	if err := validateProfileName(name); err != nil {
		return err
	}

	if _, err := profileOp.Read(name); err == nil {
		return fmt.Errorf("profile %q already exists", name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read profile %q: %w", name, err)
	}

	values, err := c.collectValues(interactive, nil)
	if err != nil {
		return err
	}

	required := []string{attrServicePrincipalID, attrServicePrincipalKeyID, attrPrivateKeyPEMPath}
	if missing := missingKeys(values, required); len(missing) > 0 {
		return fmt.Errorf("missing required values: %s", strings.Join(missing, ", "))
	}

	if err := validatePrivateKeyFile(values[attrPrivateKeyPEMPath].(string)); err != nil {
		return err
	}

	profile := &saclient.Profile{
		Name:       name,
		Attributes: values,
	}
	if err := profileOp.Create(profile); err != nil {
		return fmt.Errorf("create profile %q: %w", name, err)
	}

	if c.Use {
		if err := profileOp.SetCurrentName(name); err != nil {
			return fmt.Errorf("set current profile %q: %w", name, err)
		}
	}

	_, err = fmt.Fprintf(ctx.Stdout, "Profile %q created: %s\n", name, profile.Pathname())
	return err
}

type editCommand struct {
	Name                string `arg:"" optional:"" help:"Profile 名。省略時は現在の profile です。"`
	ServicePrincipalID  string `name:"service-principal-id" help:"サービスプリンシパル ID。"`
	ServicePrincipalKey string `name:"service-principal-key-id" help:"サービスプリンシパルキーの ID (KID)。"`
	PrivateKeyFile      string `name:"private-key-file" help:"RSA 秘密鍵の PEM ファイルパス。"`
	Use                 bool   `name:"use" help:"編集した profile を現在の profile に設定します。"`
}

func (c *editCommand) Run(ctx *kong.Context) error {
	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		return err
	}

	interactive := isTerminal()
	name := c.Name
	if name == "" && interactive {
		current, _ := profileOp.GetCurrentName()
		name, err = promptProfileName("Enter profile name", current, ctx.Stdout)
		if err != nil {
			return err
		}
	}
	if name == "" {
		name, err = profileOp.GetCurrentName()
		if err != nil {
			return err
		}
	}
	if err := validateProfileName(name); err != nil {
		return err
	}

	existing, err := profileOp.Read(name)
	if err != nil {
		return fmt.Errorf("read profile %q: %w", name, err)
	}
	values, err := c.collectValues(interactive, existing.Attributes)
	if err != nil {
		return err
	}

	if !interactive && len(values) == 0 && !c.Use {
		return fmt.Errorf("at least one of --service-principal-id, --service-principal-key-id, or --private-key-file is required")
	}

	if path, ok := values[attrPrivateKeyPEMPath].(string); ok {
		if err := validatePrivateKeyFile(path); err != nil {
			return err
		}
	}

	hasChanges := len(values) > 0
	var pathname string
	if hasChanges {
		updated, err := profileOp.Update(&saclient.Profile{
			Name:       name,
			Attributes: values,
		})
		if err != nil {
			return fmt.Errorf("update profile %q: %w", name, err)
		}
		pathname = updated.Pathname()
	}

	current, err := profileOp.GetCurrentName()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if current != name {
		switchToCurrent := c.Use
		if interactive && !c.Use {
			switchToCurrent, err = confirm(bufio.NewReader(os.Stdin), ctx.Stdout, fmt.Sprintf("Switch current profile to %q?", name))
			if err != nil {
				return err
			}
		}
		if switchToCurrent {
			if err := profileOp.SetCurrentName(name); err != nil {
				return fmt.Errorf("set current profile %q: %w", name, err)
			}
		}
	}

	if hasChanges {
		_, err = fmt.Fprintf(ctx.Stdout, "Profile %q updated: %s\n", name, pathname)
	} else {
		_, err = fmt.Fprintf(ctx.Stdout, "Profile %q unchanged.\n", name)
	}
	return err
}

func (c *createCommand) collectValues(interactive bool, existing map[string]any) (map[string]any, error) {
	values := make(map[string]any)
	if err := collectServicePrincipalValue(values, attrServicePrincipalID, "Service Principal ID", c.ServicePrincipalID, interactive, existing); err != nil {
		return nil, err
	}
	if err := collectServicePrincipalValue(values, attrServicePrincipalKeyID, "Service Principal Key ID", c.ServicePrincipalKey, interactive, existing); err != nil {
		return nil, err
	}
	if err := collectPrivateKeyPath(values, c.PrivateKeyFile, interactive, existing); err != nil {
		return nil, err
	}
	return values, nil
}

func (c *editCommand) collectValues(interactive bool, existing map[string]any) (map[string]any, error) {
	values := make(map[string]any)
	if err := collectServicePrincipalValue(values, attrServicePrincipalID, "Service Principal ID", c.ServicePrincipalID, interactive, existing); err != nil {
		return nil, err
	}
	if err := collectServicePrincipalValue(values, attrServicePrincipalKeyID, "Service Principal Key ID", c.ServicePrincipalKey, interactive, existing); err != nil {
		return nil, err
	}
	if err := collectPrivateKeyPath(values, c.PrivateKeyFile, interactive, existing); err != nil {
		return nil, err
	}
	return values, nil
}

func collectServicePrincipalValue(values map[string]any, key, label, flagValue string, interactive bool, existing map[string]any) error {
	if flagValue != "" {
		values[key] = flagValue
		return nil
	}
	if !interactive {
		return nil
	}

	current := ""
	if existing != nil {
		if v, ok := existing[key].(string); ok {
			current = v
		}
	}

	reader := bufio.NewReader(os.Stdin)
	change := true
	if current != "" {
		var err error
		change, err = confirm(reader, os.Stdout, fmt.Sprintf("Change %s (current: %s)?", label, current))
		if err != nil {
			return err
		}
	}

	if !change {
		return nil
	}

	input, err := prompt(reader, os.Stdout, fmt.Sprintf("Enter %s", label))
	if err != nil {
		return err
	}
	if input != "" {
		values[key] = input
	}
	return nil
}

func collectPrivateKeyPath(values map[string]any, flagValue string, interactive bool, existing map[string]any) error {
	if flagValue != "" {
		values[attrPrivateKeyPEMPath] = flagValue
		return nil
	}
	if !interactive {
		return nil
	}

	hasCurrent := false
	if existing != nil {
		if v, ok := existing[attrPrivateKeyPEMPath].(string); ok && v != "" {
			hasCurrent = true
		}
	}

	reader := bufio.NewReader(os.Stdin)
	change := true
	if hasCurrent {
		var err error
		change, err = confirm(reader, os.Stdout, "Change private key file?")
		if err != nil {
			return err
		}
	}

	if !change {
		return nil
	}

	input, err := prompt(reader, os.Stdout, "Enter private key file path")
	if err != nil {
		return err
	}
	if input != "" {
		values[attrPrivateKeyPEMPath] = input
	}
	return nil
}

func validateProfileName(name string) error {
	if name == "" {
		return fmt.Errorf("profile name is required")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid profile name: %q", name)
	}
	if strings.ContainsRune(name, filepath.Separator) || strings.ContainsRune(name, filepath.ListSeparator) {
		return fmt.Errorf("invalid profile name: %q", name)
	}
	return nil
}

func validatePrivateKeyFile(path string) error {
	if path == "" {
		return fmt.Errorf("private key file path is required")
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("private key file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("private key file %q is not a regular file", path)
	}

	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("private key file %q permissions are too lax (%o): group and other permissions must be disabled", path, info.Mode().Perm())
	}

	//nolint:gosec // The private key path is provided by the user and intentionally read.
	buf, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read private key file %q: %w", path, err)
	}
	if _, err := jwt.ParseRSAPrivateKeyFromPEM(buf); err != nil {
		return fmt.Errorf("private key file %q is not a valid RSA private key PEM: %w", path, err)
	}
	return nil
}

func isTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func prompt(r *bufio.Reader, out io.Writer, message string) (string, error) {
	if _, err := fmt.Fprintf(out, "%s: ", message); err != nil {
		return "", err
	}
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func promptProfileName(message, defaultValue string, out io.Writer) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	input, err := prompt(reader, out, fmt.Sprintf("%s [%s]", message, defaultValue))
	if err != nil {
		return "", err
	}
	if input == "" {
		return defaultValue, nil
	}
	return input, nil
}

func confirm(r *bufio.Reader, out io.Writer, message string) (bool, error) {
	for {
		if _, err := fmt.Fprintf(out, "%s [y/N]: ", message); err != nil {
			return false, err
		}
		line, err := r.ReadString('\n')
		if err != nil {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, nil
		case "n", "no", "":
			return false, nil
		}
	}
}

func missingKeys(values map[string]any, keys []string) []string {
	var missing []string
	for _, key := range keys {
		if values[key] == nil || values[key] == "" {
			missing = append(missing, key)
		}
	}
	return missing
}
