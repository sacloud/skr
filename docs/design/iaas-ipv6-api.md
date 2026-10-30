# IaaS IPv6 と IP アドレス API コマンド設計

## 対象操作

`github.com/sacloud/sacloud-sdk-go v0.3.1` の `service/iaas/ipv6net`、`service/iaas/ipv6addr`、`service/iaas/ipaddress` を `iaas-api` に接続します。コマンドグループの配置は [ADR 0069](../adr/0069-api-endpoint-command-groups.md) に従い、API の endpoint 構造を根拠にします。3 リソースはいずれもゾーン単位の平坦な endpoint を持ち、SDK も親リソースの ID を引数に取らないため、独立したコマンドグループとして公開します。

| コマンド | SDK 操作 | API endpoint | 識別子 |
| --- | --- | --- | --- |
| `skr iaas-api ipv6net find` | Find | `/{zone}/api/cloud/1.1/ipv6net` | ゾーン |
| `skr iaas-api ipv6net read` | Read | `/{zone}/api/cloud/1.1/ipv6net/{id}` | 数値の ID |
| `skr iaas-api ipv6addr find` | Find | `/{zone}/api/cloud/1.1/ipv6addr` | ゾーン |
| `skr iaas-api ipv6addr create` | Create | `/{zone}/api/cloud/1.1/ipv6addr` | ゾーン |
| `skr iaas-api ipv6addr read` | Read | `/{zone}/api/cloud/1.1/ipv6addr/{ipv6addr}` | IPv6 アドレス |
| `skr iaas-api ipv6addr update` | Update | `/{zone}/api/cloud/1.1/ipv6addr/{ipv6addr}` | IPv6 アドレス |
| `skr iaas-api ipv6addr delete` | Delete | `/{zone}/api/cloud/1.1/ipv6addr/{ipv6addr}` | IPv6 アドレス |
| `skr iaas-api ipaddress list` | List | `/{zone}/api/cloud/1.1/ipaddress` | ゾーン |
| `skr iaas-api ipaddress read` | Read | `/{zone}/api/cloud/1.1/ipaddress/{ipaddress}` | IPv4 アドレス |
| `skr iaas-api ipaddress update-host-name` | UpdateHostName | `/{zone}/api/cloud/1.1/ipaddress/{ipaddress}` | IPv4 アドレス |

`ipv6net` は検索と参照のみを持ち、IPv6 アドレスの割り当ての有効化と無効化は `/{zone}/api/1.0/internet/{id}/ipv6net` のネストした endpoint です。この操作は既存の `skr iaas-api internet` の `enable-ipv6` と `disable-ipv6` のままとします。`internet read-ipv6` (ルータ＋スイッチの ID を指定) と `ipv6net read` (IPv6 ネットワークの ID を指定) は別コマンドとして両方残し、ヘルプで識別子の違いを説明します。

## 入力と出力

- 検索と一覧は `--zone` を必須とします。取得件数と開始位置は `--count` と `--from` で指定できます。IPv6Net、IPv6Addr、IPAddress の検索条件には名前やタグのフィールドがないため、名前検索の `--name` やタグの `--tags` は設けません。絞り込みと並び替えは `--request` で検索条件の JSON を渡して行い、接続先スイッチやルータ＋スイッチでの絞り込み (IPv6Net の `SwitchIDs` など)、ホスト名での絞り込み (IPv6Addr の `HostNames` など)、`Sort` は JSON のみです。`--request` と個別フラグは併用できません。
- `ipv6net read` は数値の ID を必須とし、`--id` または `ReadRequest` JSON を受け付けます。`--id` は API の `ID` へ `types.ID` 変換します。
- `ipv6addr create` と `update` は、ゾーン、登録する IPv6 アドレス、逆引きホスト名を `--zone`、`--ipv6-addr`、`--host-name` の個別フラグで指定できます。登録するアドレスは API 側で IPv6 形式とプレフィクス内であることを検証します。`--request` の JSON でも指定でき、両者は併用できません。
- `ipv6addr read` と `delete` は、ゾーンと IPv6 アドレスを `--zone` と `--ipv6-addr` で指定します。識別子は数値の ID ではなくアドレス文字列です。
- `ipaddress read` は、ゾーンと IPv4 アドレスを `--zone` と `--ip-address` で指定します。
- `ipaddress update-host-name` は、ゾーンと IPv4 アドレスを必須とし、逆引きホスト名を `--host-name` で指定します。省略時は空に設定し、逆引きホスト名の登録を解除します。リトライの回数と間隔 (`RetryMax`、`RetryInterval`) は JSON のみです。
- `ipv6addr delete` と `ipaddress update-host-name` は成功時に出力しません。
- 結果は他の IaaS コマンドと同じ JSON 出力処理に渡します。`--query` で必要な項目に絞れます。

## CLI とテストの配置

生成設定は `api/commands/iaas-ipv6net.json`、`api/commands/iaas-ipv6addr.json`、`api/commands/iaas-ipaddress.json` に置きます。生成コードは `internal/iaas/ipv6netapi/`、`internal/iaas/ipv6addrapi/`、`internal/iaas/ipaddressapi/` に置きます。SDK factory とルートコマンドは `internal/cli/iaas.go`、配線は生成された `internal/cli/iaas_wiring_generated.go` ([ADR 0068](../adr/0068-generate-cli-wiring.md)) で接続します。ライブ E2E は `test/e2e/ipv6net/`、`test/e2e/ipv6addr/`、`test/e2e/ipaddress/` に置きます。

## E2E の状況

- `ipv6net` は、ルータ＋スイッチを作成して IPv6 アドレス割り当てを有効化したうえで、`find` と `read` を検証する [ライブ E2E](../../test/e2e/ipv6net/scenario_ipv6net_test.go) があります。2026-10-30 に is1b でライブ実行し、有効化した IPv6 ネットワークの ID とプレフィクスが `find` と `read` の応答に一致することを確認しました。IPv6 ネットワークの ID はルータ＋スイッチの ID とは別の番号 (数値) です。後片て付けはルータ＋スイッチの削除で行い、削除前に SDK で識別情報を再照合し、削除後に名前検索で不在を確認しました。
- `ipv6addr` と `ipaddress` は、検索と一覧、および存在しないアドレスの参照を確認する [ライブ E2E](../../test/e2e/ipv6addr/scenario_ipv6addr_test.go) があります。2026-10-30 に is1b でライブ実行し、`ipv6addr` の find は空の一覧 (割り当てられた IPv6 アドレスがない正常な不在) を確認し、存在しないアドレスの参照が 404 Not Found でエラーになることを確認しました。`ipaddress` の list は割り当て済みのアドレス 1件を確認し、read の応答が一覧と一致すること、存在しないアドレスの参照が 404 Not Found でエラーになることを確認しました。
- `ipv6addr` の create、update、delete と `ipaddress update-host-name` は、有効化済みの IPv6 ネットワークや割り当て済みのサーバが必要なため、[Issue 180](https://github.com/sacloud/skr/issues/180) で追跡します。

## チュートリアル

読み取り系の操作について、[IPv6Net](../manual/tutorials/iaas-api/ipv6net.md)、[IPv6Addr](../manual/tutorials/iaas-api/ipv6addr.md)、[IPAddress](../manual/tutorials/iaas-api/ipaddress.md) のチュートリアルがあります。リソースを作成しない手順です。

## 参照

- [SDK IPv6Net API v0.3.1 の検索操作](https://raw.githubusercontent.com/sacloud/sacloud-sdk-go/v0.3.1/service/iaas/ipv6net/find_service.go)
- [SDK IPv6Addr API v0.3.1 の登録操作](https://raw.githubusercontent.com/sacloud/sacloud-sdk-go/v0.3.1/service/iaas/ipv6addr/create_service.go)
- [SDK IPAddress API v0.3.1 の逆引きホスト名変更操作](https://raw.githubusercontent.com/sacloud/sacloud-sdk-go/v0.3.1/service/iaas/ipaddress/update_host_name_service.go)
- [さくらのクラウド API v1.1 IPv6 関連 API](https://manual.sakura.ad.jp/cloud-api/1.1/ipv6/index.html)
- [IPv6 アドレスの基本](https://manual.sakura.ad.jp/cloud/network/ipv6/ipv6-basic.html)
