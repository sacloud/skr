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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"
	"golang.org/x/term"
)

type listCommand struct{}

func (listCommand) Run(ctx *kong.Context) error {
	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		return err
	}

	names, err := profileOp.List()
	if err != nil {
		return err
	}

	current, err := profileOp.GetCurrentName()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	out := ctx.Stdout
	for _, name := range names {
		var err error
		if name == current && isConfigListTerminal(out) {
			_, err = fmt.Fprintf(out, "* %s\n", name)
		} else {
			_, err = fmt.Fprintln(out, name)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func isConfigListTerminal(stdout io.Writer) bool {
	stdoutFile, ok := stdout.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(stdoutFile.Fd())) && term.IsTerminal(int(os.Stdin.Fd()))
}

type showCommand struct {
	Name string `arg:"" optional:"" help:"Profile name to show. Defaults to the current profile."`
}

func (c *showCommand) Run(ctx *kong.Context) error {
	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		return err
	}

	name := c.Name
	if name == "" {
		name, err = profileOp.GetCurrentName()
		if err != nil {
			return err
		}
	}

	profile, err := profileOp.Read(name)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(maskSensitiveAttributes(profile.Attributes), "", "  ")
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(ctx.Stdout, string(data))
	return err
}

var sensitiveProfileAttributes = []string{"AccessToken", "AccessTokenSecret", "PrivateKey", "PrivateKeyPEMPath"}

func maskSensitiveAttributes(attrs map[string]any) map[string]any {
	if attrs == nil {
		return nil
	}
	masked := make(map[string]any, len(attrs))
	for k, v := range attrs {
		if isSensitiveAttribute(k) {
			if v != nil && v != "" {
				masked[k] = "(masked)"
				continue
			}
		}
		masked[k] = v
	}
	return masked
}

func isSensitiveAttribute(key string) bool {
	for _, s := range sensitiveProfileAttributes {
		if key == s {
			return true
		}
	}
	return false
}
