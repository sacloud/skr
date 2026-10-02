// Copyright 2022-2026 The sacloud/go-template Authors
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

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"
)

type cli struct {
	Config configCommand `cmd:"" help:"Manage configuration profiles."`
}

type configCommand struct {
	Current currentCommand `cmd:"" help:"Print the current profile name."`
}

type currentCommand struct{}

func (currentCommand) Run(ctx *kong.Context) error {
	profileOp, err := saclient.NewProfileOp(os.Environ())
	if err != nil {
		return err
	}

	name, err := profileOp.GetCurrentName()
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(ctx.Stdout, name)
	return err
}

func run(args []string, stdout, stderr io.Writer) int {
	var cli cli
	parser, err := kong.New(
		&cli,
		kong.Name("usacloud"),
		kong.Description("CLI for Sakura Cloud."),
		kong.Writers(stdout, stderr),
	)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	ctx, err := parser.Parse(args)
	if err == nil {
		err = ctx.Run()
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
