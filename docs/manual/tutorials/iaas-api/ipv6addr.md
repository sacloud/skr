# IaaS IPv6Addr API: ゾーン内の IPv6 アドレスを確認する

[IPv6 アドレス](https://manual.sakura.ad.jp/cloud/network/switch/ipv6.html)は、IPv6 ネットワークのプレフィクス内の個別アドレスです。逆引きホスト名を登録できます。このチュートリアルでは、`skr iaas-api ipv6addr` を使い、ゾーン内の IPv6 アドレスを検索して内容を確認し、逆引きホスト名を登録、変更、解除します。

IPv6 アドレスの一覧は、ゾーンに割り当てられた IPv6 ネットワークのプレフィクス内のアドレスを、逆引きホスト名の登録の有無にかかわらず表示します。IPv6 アドレスの割り当てを有効化していないプロジェクトでは、検索結果は空の一覧になります。

## 前提条件

- `skr iaas-api ipv6addr` を含むバイナリが利用できること。リポジトリで `make build` を実行すると `./skr` が生成されます。以下の `skr` は、そのバイナリを実行するコマンドとして記載します。
- 対象プロジェクトに対する認証情報を、プロファイルまたは `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` 環境変数で設定済みであること。Step 4 以降の操作はリソースを変更するため、ルータ＋スイッチを作成・削除できる権限が必要です。認証情報はコマンド例や JSON ファイルに書かないでください。
- 逆引き登録の対象には、ルータ＋スイッチの IPv6 アドレス割り当ての有効化が必要です。有効化と無効化は `skr iaas-api internet enable-ipv6` と `disable-ipv6` で行えます。手順は [IPv6Net API のチュートリアル](ipv6net.md)を参照してください。

## Step 1: 対象環境とヘルプを確認する

```console
$ skr config current
$ skr iaas-api --help
$ skr iaas-api ipv6addr --help
$ skr iaas-api ipv6addr find --help
$ skr iaas-api ipv6addr read --help
```

`config current` で意図したプロファイルが選択されていることを確認してください。`find` と `read` にはゾーン `--zone` が必要です。`read` では、参照する IPv6 アドレス `--ipv6-addr` も必要です。`--request` で検索条件やリクエストの JSON を渡すこともできますが、`--zone` などの個別フラグと併用はできません。

## Step 2: ゾーン内の IPv6 アドレスを検索する

`--zone` に確認したいゾーン名を指定します。次の例は `is1b` の場合です。

```console
$ skr iaas-api ipv6addr find --zone is1b --query 'map({IPv6Addr,HostName})'
```

出力例は次のとおりです。アドレスとホスト名は実際のアカウントの値に置き換わります。

```json
[
  {
    "IPv6Addr": "2001:db8:a001::1",
    "HostName": "www.example.com"
  }
]
```

IPv6 アドレスの割り当てを有効化していないプロジェクトでは、空の一覧が出力されます。

```json
[]
```

検索結果の絞り込みや並び替えは、`--request` で検索条件の JSON を渡して行います。接続先のスイッチやホスト名で絞り込むキー (`SwitchIDs`、`HostNames`、`IPv6Addresses`) と `Sort` は JSON で、取得件数の `Count` と開始位置の `From` は `--count` と `--from` で指定できます。

## Step 3: IPv6 アドレスを参照する

Step 2 で得たアドレスを指定して、1件の IPv6 アドレスを参照します。`2001:db8:a001::1` は仮のアドレスです。必ず検索結果の `IPv6Addr` に置き換えてください。

```console
$ skr iaas-api ipv6addr read --zone is1b --ipv6-addr 2001:db8:a001::1
```

出力の `HostName` が逆引きホスト名の登録内容と一致することを確認します。逆引きホスト名を登録していないアドレスでは、`HostName` は空になります。

## Step 4: 逆引きホスト名を登録する

登録するアドレスは、使用可能な範囲 (末尾64bitが `0:0:0:4` 以降) から選んでください。

```console
$ skr iaas-api ipv6addr create --help
$ skr iaas-api ipv6addr create --zone is1b --ipv6-addr 2001:db8:a001::10 --host-name www.example.com
```

登録するアドレスへの正引き (AAAA レコード) が設定されていない場合、逆引き登録はできません。あらかじめ DNS に AAAA レコードを登録してから操作してください。詳細は [IP アドレスの逆引き設定](https://manual.sakura.ad.jp/cloud/network/switch/reverse-dns.html)を参照してください。

## Step 5: 逆引きホスト名を変更する

```console
$ skr iaas-api ipv6addr update --help
$ skr iaas-api ipv6addr update --zone is1b --ipv6-addr 2001:db8:a001::10 --host-name www2.example.com
```

変更後のホスト名についても、同じアドレスへの AAAA レコードが必要です。変更後に `read` で `HostName` を確認してください。

## Step 6: 逆引きホスト名の登録を解除する

`delete` は成功時に出力しません。解除後に `read` を実行すると、参照する IPv6 アドレスが存在しないためエラーになります。

```console
$ skr iaas-api ipv6addr delete --help
$ skr iaas-api ipv6addr delete --zone is1b --ipv6-addr 2001:db8:a001::10
$ skr iaas-api ipv6addr read --zone is1b --ipv6-addr 2001:db8:a001::10
```

## 参考情報

- [IPv6 アドレス (スイッチ、ルータ＋スイッチ)](https://manual.sakura.ad.jp/cloud/network/switch/ipv6.html)
- [IP アドレスの逆引き設定 (スイッチ、ルータ＋スイッチ)](https://manual.sakura.ad.jp/cloud/network/switch/reverse-dns.html)
- [sacloud-sdk-go v0.3.1 IPv6Addr API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.1/service/iaas/ipv6addr)
