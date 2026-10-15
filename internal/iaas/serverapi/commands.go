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

package serverapi

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/server"
)

func TransformCreateRequest(c *CreateCommand, request *server.CreateRequest) error {
	for i, input := range c.Disks {
		value, err := parseDisk(input, request.Name)
		if err != nil {
			return fmt.Errorf("--disk の %d 個目: %w", i+1, err)
		}
		if value.SourceArchiveID != 0 && request.BootAfterCreate {
			return fmt.Errorf("アーカイブから作成するディスクでは --boot-after-create=true を指定できません。user-data は server boot で渡してください")
		}
		request.Disks = append(request.Disks, value)
	}
	for i, input := range c.NICs {
		value, err := parseNIC(input)
		if err != nil {
			return fmt.Errorf("--nic の %d 個目: %w", i+1, err)
		}
		if i > 0 && value.Upstream == "shared" {
			return fmt.Errorf("--nic の %d 個目: shared は先頭の NIC にだけ指定できます", i+1)
		}
		request.NetworkInterfaces = append(request.NetworkInterfaces, value)
	}
	return nil
}

type BootCommand struct {
	Zone     string  `name:"zone" required:"" help:"必須: 対象ゾーン名。"`
	ID       int64   `name:"id" required:"" help:"必須: 起動するサーバ ID。"`
	UserData *string `name:"user-data" help:"任意: cloud-init user-data。省略すると user-data なしで起動します。@path.yaml でファイルから、- で標準入力から読み込みます。インライン値は指定できません。"`
	factory  APIFactory
	runtime  Runtime
}

func (c *BootCommand) Run(_ *kong.Context) error {
	var userData string
	if c.UserData != nil {
		var err error
		userData, err = readUserData(*c.UserData, os.Stdin)
		if err != nil {
			return err
		}
	}
	request := &server.BootRequest{
		Zone:     c.Zone,
		ID:       types.ID(c.ID),
		UserData: userData,
	}
	if err := ValidateBootRequest(request); err != nil {
		return err
	}
	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	return op.BootWithContext(context.Background(), request)
}

func readUserData(input string, stdin io.Reader) (string, error) {
	var data []byte
	switch {
	case input == "-":
		var err error
		data, err = io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("read user-data from standard input: %w", err)
		}
	case strings.HasPrefix(input, "@") && len(input) > 1:
		var err error

		data, err = os.ReadFile(strings.TrimPrefix(input, "@"))
		if err != nil {
			return "", fmt.Errorf("read user-data file: %w", err)
		}
	default:
		return "", fmt.Errorf("--user-data は @path.yaml または - で指定してください")
	}
	if strings.TrimSpace(string(data)) == "" {
		return "", fmt.Errorf("user-data is empty")
	}
	return string(data), nil
}
