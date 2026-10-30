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
	archiveapi "github.com/sacloud/skr/internal/iaas/archiveapi"
	diskapi "github.com/sacloud/skr/internal/iaas/diskapi"
	diskplanapi "github.com/sacloud/skr/internal/iaas/diskplanapi"
	iconapi "github.com/sacloud/skr/internal/iaas/iconapi"
	internetapi "github.com/sacloud/skr/internal/iaas/internetapi"
	internetplanapi "github.com/sacloud/skr/internal/iaas/internetplanapi"
	ipaddressapi "github.com/sacloud/skr/internal/iaas/ipaddressapi"
	ipv6addrapi "github.com/sacloud/skr/internal/iaas/ipv6addrapi"
	ipv6netapi "github.com/sacloud/skr/internal/iaas/ipv6netapi"
	licenseinfoapi "github.com/sacloud/skr/internal/iaas/licenseinfoapi"
	packetfilterapi "github.com/sacloud/skr/internal/iaas/packetfilterapi"
	privatehostplanapi "github.com/sacloud/skr/internal/iaas/privatehostplanapi"
	regionapi "github.com/sacloud/skr/internal/iaas/regionapi"
	serverapi "github.com/sacloud/skr/internal/iaas/serverapi"
	serverplanapi "github.com/sacloud/skr/internal/iaas/serverplanapi"
	serviceclassapi "github.com/sacloud/skr/internal/iaas/serviceclassapi"
	switchapi "github.com/sacloud/skr/internal/iaas/switchapi"
	zoneapi "github.com/sacloud/skr/internal/iaas/zoneapi"
)

type iaasAPICommand struct {
	Archive      archiveapi.Commands      `cmd:"" help:"さくらのクラウドのアーカイブを検索・参照します。検索では対象ゾーン、タグまたは OS 種別を指定できます。"`
	Disk         diskapi.Commands         `cmd:"" help:"さくらのクラウドのディスクを検索、参照、作成、更新、削除します。作成と更新は --request JSON を使用します。"`
	Icon         iconapi.Commands         `cmd:"" help:"ユーザー作成アイコンを検索、参照、作成、更新、削除します。"`
	Internet     internetapi.Commands     `cmd:"" help:"さくらのクラウドのルータ＋スイッチと、関連する IP アドレスブロックおよび IPv6 設定を操作します。"`
	IPAddress    ipaddressapi.Commands    `cmd:"" name:"ipaddress" help:"さくらのクラウドのグローバル IP アドレスを検索・参照し、逆引きホスト名を変更します。"`
	IPv6Addr     ipv6addrapi.Commands     `cmd:"" name:"ipv6addr" help:"さくらのクラウドの IPv6 アドレスを検索、参照、作成、更新、削除します。IPv6 プレフィクス内のアドレスに逆引きホスト名を登録します。"`
	IPv6Net      ipv6netapi.Commands      `cmd:"" name:"ipv6net" help:"さくらのクラウドの IPv6 ネットワーク (IPv6 アドレス割り当て) を検索・参照します。IPv6 の有効化と無効化は skr iaas-api internet の enable-ipv6 と disable-ipv6 を使います。"`
	PacketFilter packetfilterapi.Commands `cmd:"" name:"packetfilter" help:"さくらのクラウドのパケットフィルタを検索、参照、作成、更新、削除します。ルール配列は --expressions JSON または --request JSON で指定します。"`
	Product      iaasProductCommand       `cmd:"" help:"商品 API のプラン情報を検索・参照します。"`
	Region       regionapi.Commands       `cmd:"" help:"リージョン情報を検索・参照します。"`
	Switch       switchapi.Commands       `cmd:"" help:"さくらのクラウドのスイッチを操作します。各操作で --zone などのフラグ、または --request JSON に Zone を指定します。両経路は併用できません。"`
	Server       serverapi.Commands       `cmd:"" help:"さくらのクラウドのサーバを検索、参照、作成、更新、起動、削除します。作成時はcloud-init用ディスクやNICを個別フラグで指定できます。"`
	ServiceClass serviceclassapi.Commands `cmd:"" help:"指定したゾーンのサービスクラスを検索します。"`
	Zone         zoneapi.Commands         `cmd:"" help:"さくらのクラウドのゾーン一覧を取得します。"`
}

type iaasProductCommand struct {
	Disk        diskplanapi.Commands        `cmd:"" help:"ディスクプランを検索・参照します。プランの利用可否とサイズを確認できます。"`
	Internet    internetplanapi.Commands    `cmd:"" help:"インターネットプランを検索・参照します。帯域幅と利用可否を確認できます。"`
	License     licenseinfoapi.Commands     `cmd:"" name:"license" help:"ライセンス情報を検索し、名称や利用条件を確認します。"`
	PrivateHost privatehostplanapi.Commands `cmd:"" name:"privatehost" help:"専有ホストプランを検索・参照します。CPU、メモリ、利用可否を確認できます。"`
	Server      serverplanapi.Commands      `cmd:"" help:"サーバプランを検索・参照します。CPU、メモリ、GPU、利用可否を確認できます。"`
}
