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

package networkingsuiteapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	networkingsuite "github.com/sacloud/sacloud-sdk-go/api/networking-suite"
	v1 "github.com/sacloud/sacloud-sdk-go/api/networking-suite/apis/v1"
	"github.com/sacloud/sacloud-sdk-go/srn"
	clientconfig "github.com/sacloud/skr/internal/saclient"
)

const (
	subnetGroupResource         = "sakura.networking-suite.subnet-group"
	subnetResource              = "sakura.networking-suite.subnet"
	interfaceConnectionResource = "sakura.networking-suite.interface-connection"
	addressResource             = "sakura.networking-suite.address"
	interfaceResource           = "sakura.iaas.interface"
)

type Commands struct {
	SubnetGroup         SubnetGroupCommands         `cmd:"" name:"subnet-group" help:"リージョン内のネットワーク範囲を管理します。サブネットを作成する前に作成してください。"`
	Subnet              SubnetCommands              `cmd:"" help:"サブネットグループ内のゾーン別ネットワークを管理します。"`
	InterfaceConnection InterfaceConnectionCommands `cmd:"" name:"interface-connection" help:"サーバの NIC とサブネットの接続を管理します。"`
	Address             AddressCommands             `cmd:"" help:"サブネットで使用中の IPv4 アドレスを参照します。"`
}

type Runtime struct {
	NewClient      func() (*v1.Client, error)
	ValidateOutput func(*kong.Context) error
	WriteOutput    func(*kong.Context, any) error
}

func NewCommands(runtime Runtime) Commands {
	var commands Commands
	commands.SubnetGroup.setRuntime(runtime)
	commands.Subnet.setRuntime(runtime)
	commands.InterfaceConnection.setRuntime(runtime)
	commands.Address.setRuntime(runtime)
	return commands
}

func NewClient(trace bool) (*v1.Client, error) {
	client, err := clientconfig.New(trace)
	if err != nil {
		return nil, err
	}
	return networkingsuite.NewClient(client)
}

func runValue[T any](ctx *kong.Context, runtime Runtime, call func(context.Context, *v1.Client) (T, error)) error {
	if runtime.ValidateOutput == nil || runtime.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	if err := runtime.ValidateOutput(ctx); err != nil {
		return err
	}
	if runtime.NewClient == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	client, err := runtime.NewClient()
	if err != nil {
		return err
	}
	result, err := call(context.Background(), client)
	if err != nil {
		return err
	}
	return runtime.WriteOutput(ctx, result)
}

func runNoOutput(ctx context.Context, runtime Runtime, call func(context.Context, *v1.Client) error) error {
	if runtime.NewClient == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	client, err := runtime.NewClient()
	if err != nil {
		return err
	}
	return call(ctx, client)
}

func decodeRequest(input string, destination any) error {
	var data []byte
	var err error
	switch {
	case input == "-":
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("標準入力からリクエストを読み込めません: %w", err)
		}
	case strings.HasPrefix(input, "@"):
		data, err = os.ReadFile(strings.TrimPrefix(input, "@"))
		if err != nil {
			return fmt.Errorf("リクエストファイルを読み込めません: %w", err)
		}
	default:
		data = []byte(input)
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("リクエスト JSON を読み込めません: %w", err)
	}
	return nil
}

func requestFromInput[T any](input *string, flagsSet bool, build func() (T, error)) (T, error) {
	var result T
	if input != nil {
		if flagsSet {
			return result, fmt.Errorf("--request と個別フラグは併用できません")
		}
		if err := decodeRequest(*input, &result); err != nil {
			return result, err
		}
		return result, nil
	}
	return build()
}

type requiredFlag struct {
	name  string
	value *string
}

func requireFlags(values ...requiredFlag) error {
	var missing []string
	for _, value := range values {
		if value.value == nil {
			missing = append(missing, "--"+value.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s が必要です (--request JSON も指定できます)", strings.Join(missing, " と "))
	}
	return nil
}

func parseSRN(value, expectedResource string) (srn.SRN, error) {
	parsed, err := srn.Parse(value)
	if err != nil {
		return srn.SRN{}, fmt.Errorf("SRN の形式が正しくありません: %w", err)
	}
	if parsed.Resource != expectedResource {
		return srn.SRN{}, fmt.Errorf("SRN のリソース種別が正しくありません: %s を指定してください", expectedResource)
	}
	return parsed, nil
}

type SubnetGroupCommands struct {
	List   SubnetGroupListCommand   `cmd:"" help:"サブネットグループを一覧表示します。例: skr networking-suite-api subnet-group list"`
	Read   SubnetGroupReadCommand   `cmd:"" help:"SRN を指定してサブネットグループを参照します。例: skr networking-suite-api subnet-group read SUBNET-GROUP-SRN"`
	Create SubnetGroupCreateCommand `cmd:"" help:"リージョン単位のサブネットグループを作成します。例: skr networking-suite-api subnet-group create --name NAME --description DESCRIPTION --ipv4-address-range-cidr 10.0.0.0/16 --region is1"`
	Update SubnetGroupUpdateCommand `cmd:"" help:"サブネットグループの名前と説明を更新します。両方の値を指定してください。例: skr networking-suite-api subnet-group update SUBNET-GROUP-SRN --name NAME --description DESCRIPTION"`
	Delete SubnetGroupDeleteCommand `cmd:"" help:"サブネットグループを削除します。配下のサブネットを先に削除してください。例: skr networking-suite-api subnet-group delete SUBNET-GROUP-SRN"`
}

func (c *SubnetGroupCommands) setRuntime(runtime Runtime) {
	c.List.runtime = runtime
	c.Read.runtime = runtime
	c.Create.runtime = runtime
	c.Update.runtime = runtime
	c.Delete.runtime = runtime
}

type SubnetGroupListCommand struct{ runtime Runtime }

func (c *SubnetGroupListCommand) Run(ctx *kong.Context) error {
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) ([]v1.ReadSubnetGroup, error) {
		return networkingsuite.NewSubnetGroupsOp(client).List(ctx)
	})
}

type SubnetGroupReadCommand struct {
	SRN     string `arg:"" help:"参照するサブネットグループの SRN。list の出力に含まれます。"`
	runtime Runtime
}

func (c *SubnetGroupReadCommand) Run(ctx *kong.Context) error {
	resource, err := parseSRN(c.SRN, subnetGroupResource)
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*v1.ReadSubnetGroup, error) {
		return networkingsuite.NewSubnetGroupsOp(client).Read(ctx, resource)
	})
}

type SubnetGroupCreateCommand struct {
	Request          *string `help:"個別フラグと併用できません。CreateSubnetGroup JSON を直接、@path.json、または - (標準入力) で指定します。Name、Description、IPv4AddressRangeCIDR、Region.Code が必要です。"`
	Name             *string `help:"必須: サブネットグループ名。"`
	Description      *string `help:"必須: サブネットグループの説明。"`
	IPv4AddressRange *string `name:"ipv4-address-range-cidr" help:"必須: サブネットグループの IPv4 CIDR。10.0.0.0/8、172.16.0.0/12、192.168.0.0/16、100.64.0.0/10 の範囲内で /16 から /28 を指定します。"`
	Region           *string `help:"必須: Region.Code に対応するリージョンコード (例: is1)。"`
	runtime          Runtime
}

func (c *SubnetGroupCreateCommand) Run(ctx *kong.Context) error {
	flagsSet := c.Name != nil || c.Description != nil || c.IPv4AddressRange != nil || c.Region != nil
	request, err := requestFromInput(c.Request, flagsSet, func() (v1.CreateSubnetGroup, error) {
		if err := requireFlags(
			requiredFlag{"name", c.Name},
			requiredFlag{"description", c.Description},
			requiredFlag{"ipv4-address-range-cidr", c.IPv4AddressRange},
			requiredFlag{"region", c.Region},
		); err != nil {
			return v1.CreateSubnetGroup{}, err
		}
		return v1.CreateSubnetGroup{
			Name:                 *c.Name,
			Description:          *c.Description,
			IPv4AddressRangeCIDR: *c.IPv4AddressRange,
			Region:               v1.Region{Code: *c.Region},
		}, nil
	})
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*v1.ReadSubnetGroup, error) {
		return networkingsuite.NewSubnetGroupsOp(client).Create(ctx, request)
	})
}

type SubnetGroupUpdateCommand struct {
	SRN         string  `arg:"" help:"更新するサブネットグループの SRN。"`
	Request     *string `help:"個別フラグと併用できません。UpdateSubnetGroup JSON を直接、@path.json、または - (標準入力) で指定します。Name と Description が必要です。"`
	Name        *string `help:"必須: 更新後の名前。"`
	Description *string `help:"必須: 更新後の説明。"`
	runtime     Runtime
}

func (c *SubnetGroupUpdateCommand) Run(ctx *kong.Context) error {
	resource, err := parseSRN(c.SRN, subnetGroupResource)
	if err != nil {
		return err
	}
	request, err := requestFromInput(c.Request, c.Name != nil || c.Description != nil, func() (v1.UpdateSubnetGroup, error) {
		if err := requireFlags(requiredFlag{"name", c.Name}, requiredFlag{"description", c.Description}); err != nil {
			return v1.UpdateSubnetGroup{}, err
		}
		return v1.UpdateSubnetGroup{Name: *c.Name, Description: *c.Description}, nil
	})
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*v1.ReadSubnetGroup, error) {
		return networkingsuite.NewSubnetGroupsOp(client).Update(ctx, resource, request)
	})
}

type SubnetGroupDeleteCommand struct {
	SRN     string `arg:"" help:"削除するサブネットグループの SRN。配下のサブネットがないことを確認してください。"`
	runtime Runtime
}

func (c *SubnetGroupDeleteCommand) Run(_ *kong.Context) error {
	resource, err := parseSRN(c.SRN, subnetGroupResource)
	if err != nil {
		return err
	}
	return runNoOutput(context.Background(), c.runtime, func(ctx context.Context, client *v1.Client) error {
		return networkingsuite.NewSubnetGroupsOp(client).Delete(ctx, resource)
	})
}

type SubnetCommands struct {
	List   SubnetListCommand   `cmd:"" help:"サブネットグループの SRN を指定してサブネットを一覧表示します。例: skr networking-suite-api subnet list SUBNET-GROUP-SRN"`
	Read   SubnetReadCommand   `cmd:"" help:"SRN を指定してサブネットを参照します。例: skr networking-suite-api subnet read SUBNET-SRN"`
	Create SubnetCreateCommand `cmd:"" help:"サブネットグループにゾーン別サブネットを作成します。例: skr networking-suite-api subnet create --name NAME --description DESCRIPTION --ipv4-address-range-cidr 10.0.1.0/24 --zone is1b --subnet-group-srn SUBNET-GROUP-SRN"`
	Update SubnetUpdateCommand `cmd:"" help:"サブネットの名前と説明を更新します。両方の値を指定してください。例: skr networking-suite-api subnet update SUBNET-SRN --name NAME --description DESCRIPTION"`
	Delete SubnetDeleteCommand `cmd:"" help:"サブネットを削除します。接続中の NIC がないことを確認してください。例: skr networking-suite-api subnet delete SUBNET-SRN"`
}

func (c *SubnetCommands) setRuntime(runtime Runtime) {
	c.List.runtime = runtime
	c.Read.runtime = runtime
	c.Create.runtime = runtime
	c.Update.runtime = runtime
	c.Delete.runtime = runtime
}

type SubnetListCommand struct {
	SubnetGroupSRN string `arg:"" help:"一覧を取得する親サブネットグループの SRN。"`
	runtime        Runtime
}

func (c *SubnetListCommand) Run(ctx *kong.Context) error {
	group, err := parseSRN(c.SubnetGroupSRN, subnetGroupResource)
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) ([]v1.ReadSubnet, error) {
		return networkingsuite.NewSubnetsOp(client).List(ctx, group)
	})
}

type SubnetReadCommand struct {
	SRN     string `arg:"" help:"参照するサブネットの SRN。list の出力に含まれます。"`
	runtime Runtime
}

func (c *SubnetReadCommand) Run(ctx *kong.Context) error {
	resource, err := parseSRN(c.SRN, subnetResource)
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*v1.ReadSubnet, error) {
		return networkingsuite.NewSubnetsOp(client).Read(ctx, resource)
	})
}

type SubnetCreateCommand struct {
	Request          *string `help:"個別フラグと併用できません。CreateSubnet JSON を直接、@path.json、または - (標準入力) で指定します。Name、Description、IPv4AddressRangeCIDR、Zone.Code、SubnetGroup.SRN が必要です。"`
	Name             *string `help:"必須: サブネット名。"`
	Description      *string `help:"必須: サブネットの説明。"`
	IPv4AddressRange *string `name:"ipv4-address-range-cidr" help:"必須: 親範囲内で他のサブネットと重複しない IPv4 CIDR (/16 から /28)。"`
	Zone             *string `help:"必須: Zone.Code に対応する配置先ゾーンコード (例: is1c)。"`
	SubnetGroupSRN   *string `name:"subnet-group-srn" help:"必須: API の SubnetGroup.SRN に対応する親サブネットグループの SRN。"`
	runtime          Runtime
}

func (c *SubnetCreateCommand) Run(ctx *kong.Context) error {
	flagsSet := c.Name != nil || c.Description != nil || c.IPv4AddressRange != nil || c.Zone != nil || c.SubnetGroupSRN != nil
	request, err := requestFromInput(c.Request, flagsSet, func() (v1.CreateSubnet, error) {
		if err := requireFlags(
			requiredFlag{"name", c.Name},
			requiredFlag{"description", c.Description},
			requiredFlag{"ipv4-address-range-cidr", c.IPv4AddressRange},
			requiredFlag{"zone", c.Zone},
			requiredFlag{"subnet-group-srn", c.SubnetGroupSRN},
		); err != nil {
			return v1.CreateSubnet{}, err
		}
		if _, err := parseSRN(*c.SubnetGroupSRN, subnetGroupResource); err != nil {
			return v1.CreateSubnet{}, fmt.Errorf("--subnet-group-srn: %w", err)
		}
		return v1.CreateSubnet{
			Name:                 *c.Name,
			Description:          *c.Description,
			IPv4AddressRangeCIDR: *c.IPv4AddressRange,
			Zone:                 v1.Zone{Code: *c.Zone},
			SubnetGroup:          v1.SakuraResourceNameRef{SRN: *c.SubnetGroupSRN},
		}, nil
	})
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*v1.ReadSubnet, error) {
		return networkingsuite.NewSubnetsOp(client).Create(ctx, request)
	})
}

type SubnetUpdateCommand struct {
	SRN         string  `arg:"" help:"更新するサブネットの SRN。"`
	Request     *string `help:"個別フラグと併用できません。UpdateSubnet JSON を直接、@path.json、または - (標準入力) で指定します。Name と Description が必要です。"`
	Name        *string `help:"必須: 更新後の名前。"`
	Description *string `help:"必須: 更新後の説明。"`
	runtime     Runtime
}

func (c *SubnetUpdateCommand) Run(ctx *kong.Context) error {
	resource, err := parseSRN(c.SRN, subnetResource)
	if err != nil {
		return err
	}
	request, err := requestFromInput(c.Request, c.Name != nil || c.Description != nil, func() (v1.UpdateSubnet, error) {
		if err := requireFlags(requiredFlag{"name", c.Name}, requiredFlag{"description", c.Description}); err != nil {
			return v1.UpdateSubnet{}, err
		}
		return v1.UpdateSubnet{Name: *c.Name, Description: *c.Description}, nil
	})
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*v1.ReadSubnet, error) {
		return networkingsuite.NewSubnetsOp(client).Update(ctx, resource, request)
	})
}

type SubnetDeleteCommand struct {
	SRN     string `arg:"" help:"削除するサブネットの SRN。接続中の NIC がないことを確認してください。"`
	runtime Runtime
}

func (c *SubnetDeleteCommand) Run(_ *kong.Context) error {
	resource, err := parseSRN(c.SRN, subnetResource)
	if err != nil {
		return err
	}
	return runNoOutput(context.Background(), c.runtime, func(ctx context.Context, client *v1.Client) error {
		return networkingsuite.NewSubnetsOp(client).Delete(ctx, resource)
	})
}

type InterfaceConnectionCommands struct {
	Create InterfaceConnectionCreateCommand `cmd:"" help:"指定したサーバ NIC をサブネットへ接続します。例: skr networking-suite-api interface-connection create --interface-srn INTERFACE-SRN --subnet-srn SUBNET-SRN"`
	Delete InterfaceConnectionDeleteCommand `cmd:"" help:"NIC とサブネットの接続を解除します。サーバは停止状態にしてください。例: skr networking-suite-api interface-connection delete INTERFACE-CONNECTION-SRN"`
}

func (c *InterfaceConnectionCommands) setRuntime(runtime Runtime) {
	c.Create.runtime = runtime
	c.Delete.runtime = runtime
}

type InterfaceConnectionCreateCommand struct {
	InterfaceSRN         *string `name:"interface-srn" help:"必須: 接続するサーバ NIC の SRN。Server read の NIC ID とゾーンから srnv1:sakura-ZONE:sakura.iaas.interface:ID の形式で構成します。"`
	SubnetSRN            *string `name:"subnet-srn" help:"必須: 接続先サブネットの SRN。"`
	EphemeralIPv4Address *string `name:"ephemeral-ipv4-address" help:"任意: API の EphemeralIPv4Address に対応する、サブネット内で未使用の IPv4 アドレス。省略すると自動割り当てです。"`
	runtime              Runtime
}

func (c *InterfaceConnectionCreateCommand) Run(ctx *kong.Context) error {
	if err := requireFlags(requiredFlag{"interface-srn", c.InterfaceSRN}, requiredFlag{"subnet-srn", c.SubnetSRN}); err != nil {
		return err
	}
	iface, err := parseSRN(*c.InterfaceSRN, interfaceResource)
	if err != nil {
		return fmt.Errorf("--interface-srn: %w", err)
	}
	subnet, err := parseSRN(*c.SubnetSRN, subnetResource)
	if err != nil {
		return fmt.Errorf("--subnet-srn: %w", err)
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*v1.ReadInterfaceConnection, error) {
		return networkingsuite.NewInterfaceConnectionOp(client).Create(ctx, networkingsuite.CreateInterfaceConnectionParams{
			InterfaceSRN: iface,
			SubnetSRN:    subnet,
			IPAddress:    c.EphemeralIPv4Address,
		})
	})
}

type InterfaceConnectionDeleteCommand struct {
	SRN     string `arg:"" help:"削除するインターフェースコネクションの SRN。"`
	runtime Runtime
}

func (c *InterfaceConnectionDeleteCommand) Run(_ *kong.Context) error {
	resource, err := parseSRN(c.SRN, interfaceConnectionResource)
	if err != nil {
		return err
	}
	return runNoOutput(context.Background(), c.runtime, func(ctx context.Context, client *v1.Client) error {
		return networkingsuite.NewInterfaceConnectionOp(client).Delete(ctx, resource)
	})
}

type AddressCommands struct {
	List AddressListCommand `cmd:"" help:"サブネットに割り当てられた IPv4 アドレスを一覧表示します。例: skr networking-suite-api address list SUBNET-SRN"`
	Read AddressReadCommand `cmd:"" help:"SRN を指定して IPv4 アドレスを参照します。例: skr networking-suite-api address read ADDRESS-SRN"`
}

func (c *AddressCommands) setRuntime(runtime Runtime) {
	c.List.runtime = runtime
	c.Read.runtime = runtime
}

type AddressListCommand struct {
	SubnetSRN              string  `arg:"" help:"アドレス一覧を取得するサブネットの SRN。"`
	InterfaceConnectionSRN *string `name:"interface-connection-srn" help:"任意: 対象のインターフェースコネクション SRN で結果を絞り込みます。"`
	runtime                Runtime
}

func (c *AddressListCommand) Run(ctx *kong.Context) error {
	subnet, err := parseSRN(c.SubnetSRN, subnetResource)
	if err != nil {
		return err
	}
	var connection *srn.SRN
	if c.InterfaceConnectionSRN != nil {
		parsed, err := parseSRN(*c.InterfaceConnectionSRN, interfaceConnectionResource)
		if err != nil {
			return fmt.Errorf("--interface-connection-srn: %w", err)
		}
		connection = &parsed
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) ([]v1.ReadAddress, error) {
		return networkingsuite.NewAddressesOp(client).List(ctx, networkingsuite.ListAddressesParams{
			SubnetSRN:              subnet,
			InterfaceConnectionSRN: connection,
		})
	})
}

type AddressReadCommand struct {
	SRN     string `arg:"" help:"参照するアドレスの SRN。list の出力に含まれます。"`
	runtime Runtime
}

func (c *AddressReadCommand) Run(ctx *kong.Context) error {
	resource, err := parseSRN(c.SRN, addressResource)
	if err != nil {
		return err
	}
	return runValue(ctx, c.runtime, func(ctx context.Context, client *v1.Client) (*v1.ReadAddress, error) {
		return networkingsuite.NewAddressesOp(client).Read(ctx, resource)
	})
}
