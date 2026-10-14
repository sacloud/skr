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

type CreateCommand struct {
	Request         *string   "help:\"個別フラグと併用不可。JSON オブジェクトを直接または @path.json で指定します。Disks、NetworkInterfaces などの複雑な構成や、その他の API フィールドを指定できます。\""
	Zone            *string   "name:\"zone\" help:\"必須: 作成先のゾーン名。\""
	Name            *string   "name:\"name\" help:\"必須: サーバ名。\""
	CPU             *int      "name:\"cpu\" help:\"必須: 仮想コア数。MemoryGB と対象ゾーンで利用可能なプランの組み合わせを指定してください。\""
	MemoryGB        *int      "name:\"memory-gb\" help:\"必須: メモリ容量 (GB)。CPU と対象ゾーンで利用可能なプランの組み合わせを指定してください。\""
	Tags            *[]string "name:\"tags\" help:\"任意: タグの配列。カンマ区切りで指定します。API の Tags フィールドに対応します。\" sep:\",\""
	Description     *string   "name:\"description\" help:\"任意: サーバの説明 (最大512文字)。\""
	IconID          *int64    "name:\"icon-id\" help:\"任意: アイコンの ID (数値)。\""
	GPU             *int      "name:\"gpu\" help:\"任意: GPU 数。\""
	GPUModel        *string   "name:\"gpu-model\" help:\"任意: GPU モデル。\""
	CPUModel        *string   "name:\"cpu-model\" help:\"任意: CPU モデル。\""
	Commitment      *string   "name:\"commitment\" help:\"任意: CPU プランのコミットメント。standard または dedicatedcpu を指定します。\""
	Generation      *int      "name:\"generation\" help:\"任意: サーバプラン世代 (0: 既定、100、200)。\""
	ConfidentialVM  *bool     "name:\"confidential-vm\" help:\"任意: Confidential VM プランを有効にします。利用条件と提供ゾーンを確認してください。\""
	InterfaceDriver *string   "name:\"interface-driver\" help:\"任意: 仮想 NIC ドライバ。virtio または e1000 を指定します。\""
	BootAfterCreate *bool     "name:\"boot-after-create\" help:\"任意: true の場合、作成後にサーバを起動します。cloud-init user-data を使う場合は省略してください。\""
	CDROMID         *int64    "name:\"cdrom-id\" help:\"任意: 作成時に挿入する CD-ROM イメージの ID (数値)。\""
	PrivateHostID   *int64    "name:\"private-host-id\" help:\"任意: 接続先の専有ホスト ID (数値)。\""
	Disks           []string  `name:"disk" sep:"none" help:"任意: ディスク1個を key=value のカンマ区切りで指定します。必須キー: plan-id、size-gb (GB)。任意キー: archive-id。すべて正の整数です。例: --disk 'archive-id=123,plan-id=4,size-gb=20'。繰り返し指定でき、指定順に API の Disks 配列を組み立てます。接続は virtio です。archive-id 省略時は空のディスクを作成します。"`
	NICs            []string  `name:"nic" sep:"none" help:"任意: NIC 1個を upstream=shared または upstream=disconnected で指定します。共有セグメントまたは未接続です。繰り返し指定でき、指定順に API の NetworkInterfaces 配列を組み立てます。shared は先頭の NIC にだけ指定できます。"`
	factory         APIFactory
	runtime         Runtime
}

func (c *CreateCommand) Run(ctx *kong.Context) error {
	flagsSet := false
	flagsSet = flagsSet || c.Zone != nil
	flagsSet = flagsSet || c.Name != nil
	flagsSet = flagsSet || c.CPU != nil
	flagsSet = flagsSet || c.MemoryGB != nil
	flagsSet = flagsSet || c.Tags != nil
	flagsSet = flagsSet || c.Description != nil
	flagsSet = flagsSet || c.IconID != nil
	flagsSet = flagsSet || c.GPU != nil
	flagsSet = flagsSet || c.GPUModel != nil
	flagsSet = flagsSet || c.CPUModel != nil
	flagsSet = flagsSet || c.Commitment != nil
	flagsSet = flagsSet || c.Generation != nil
	flagsSet = flagsSet || c.ConfidentialVM != nil
	flagsSet = flagsSet || c.InterfaceDriver != nil
	flagsSet = flagsSet || c.BootAfterCreate != nil
	flagsSet = flagsSet || c.CDROMID != nil
	flagsSet = flagsSet || c.PrivateHostID != nil
	flagsSet = flagsSet || len(c.Disks) != 0
	flagsSet = flagsSet || len(c.NICs) != 0
	if c.Request != nil && flagsSet {
		return fmt.Errorf("--request と個別フラグは併用できません")
	}

	request := &server.CreateRequest{}
	if c.Request != nil {
		if c.runtime.DecodeRequest == nil {
			return fmt.Errorf("API リクエスト処理が設定されていません")
		}
		if err := c.runtime.DecodeRequest(*c.Request, request); err != nil {
			return err
		}
	} else {
		if !flagsSet {
			return fmt.Errorf("--request または個別フラグが必要です")
		}
		if c.Zone == nil || c.Name == nil || c.CPU == nil || c.MemoryGB == nil {
			return fmt.Errorf("--zone、--name、--cpu、--memory-gb が必要です")
		}
		request.Zone = *c.Zone
		request.Name = *c.Name
		request.CPU = *c.CPU
		request.MemoryGB = *c.MemoryGB
		if c.Tags != nil {
			request.Tags = types.Tags(*c.Tags)
		}
		if c.Description != nil {
			request.Description = *c.Description
		}
		if c.IconID != nil {
			request.IconID = types.ID(*c.IconID)
		}
		if c.GPU != nil {
			request.GPU = *c.GPU
		}
		if c.GPUModel != nil {
			request.GPUModel = *c.GPUModel
		}
		if c.CPUModel != nil {
			request.CPUModel = *c.CPUModel
		}
		if c.Commitment != nil {
			request.Commitment = types.ECommitment(*c.Commitment)
		}
		if c.Generation != nil {
			request.Generation = types.EPlanGeneration(*c.Generation)
		}
		if c.ConfidentialVM != nil {
			request.ConfidentialVM = *c.ConfidentialVM
		}
		if c.InterfaceDriver != nil {
			request.InterfaceDriver = types.EInterfaceDriver(*c.InterfaceDriver)
		}
		if c.BootAfterCreate != nil {
			request.BootAfterCreate = *c.BootAfterCreate
		}
		if c.CDROMID != nil {
			request.CDROMID = types.ID(*c.CDROMID)
		}
		if c.PrivateHostID != nil {
			request.PrivateHostID = types.ID(*c.PrivateHostID)
		}
		if err := c.setDevices(request); err != nil {
			return err
		}
	}
	if c.runtime.ValidateRequest == nil {
		return fmt.Errorf("API リクエスト検証が設定されていません")
	}
	if err := c.runtime.ValidateRequest("validateServerCreateRequest", request); err != nil {
		return err
	}
	if c.runtime.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	if err := c.runtime.ValidateOutput(ctx); err != nil {
		return err
	}
	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	result, err := op.CreateWithContext(context.Background(), request)
	if err != nil {
		return err
	}
	if c.runtime.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return c.runtime.WriteOutput(ctx, result)
}

func (c *CreateCommand) setDevices(request *server.CreateRequest) error {
	for i, input := range c.Disks {
		value, err := parseDisk(input, request.Name)
		if err != nil {
			return fmt.Errorf("--disk の %d 個目: %w", i+1, err)
		}
		if value.SourceArchiveID != 0 && c.BootAfterCreate != nil && *c.BootAfterCreate {
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
	if c.runtime.ValidateRequest == nil {
		return fmt.Errorf("API リクエスト検証が設定されていません")
	}
	if err := c.runtime.ValidateRequest("validateServerBootRequest", request); err != nil {
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
