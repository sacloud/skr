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
	"fmt"
	"io"
	"os"
	"strings"

	proxylbSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/proxylb"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	"github.com/sacloud/skr/internal/proxylbapi"
)

func (c *cli) initProxyLBAPI() {
	c.ProxyLBAPI = proxylbapi.NewCommands(proxylbapi.Runtime{
		DecodeRequest:          decodeRequest,
		DecodeProtectedRequest: decodeProtectedRequest,
		ValidateOutput:         validateOutput,
		WriteOutput:            writeOutput,
	}, func() (proxylbapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return proxylbSDK.New(client), nil
	})
}

// decodeProtectedRequest は秘密情報を含む可能性があるリクエストを保護したファイル
// または標準入力から読み込みます。秘密情報を引数に直接指定させないため、
// 直接指定は受け付けません。
func decodeProtectedRequest(input string, destination any) error {
	if !strings.HasPrefix(input, "@") && input != "-" {
		return fmt.Errorf("秘密情報を含む可能性があるため、--request は @path.json または - (標準入力) で指定してください")
	}
	data, err := readProtectedRequestInput(input)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("decode request JSON: %w", err)
	}
	return nil
}

func readProtectedRequestInput(input string) ([]byte, error) {
	if input == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read request from standard input: %w", err)
		}
		return data, nil
	}
	path := strings.TrimPrefix(input, "@")
	//nolint:gosec // パスはリクエストデータとしてユーザーが明示的に指定したものです。
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read request file: %w", err)
	}
	return data, nil
}
