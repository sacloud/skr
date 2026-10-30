# IaaS IPv6Addr API: ゾーン内の IPv6 アドレスを確認する

[IPv6 アドレス](https://manual.sakura.ad.jp/cloud/network/ipv6/ipv6-basic.html)は、IPv6 ネットワークのプレフィクス内の個別アドレスです。逆引きホスト名を登録できます。このチュートリアルでは、`skr iaas-api ipv6addr` を使い、ゾーン内の IPv6 アドレスを検索して、内容を確認します。リソースの作成・変更・削除は行いません。

IPv6 アドレスの一覧は、ゾーンに割り当てられた IPv6 ネットワークのプレフィクス内のアドレスを、逆引きホスト名の登録の有無にかかわらず表示します。IPv6 アドレスの割り当てを有効化していないプロジェクトでは、検索結果は空の一覧になります。

> [!NOTE]
> `create`、`update`、`delete` は、有効化済みの IPv6 ネットワークが必要です。このチュートリアルでは扱いません。

## 前提条件

- `skr iaas-api ipv6addr` を含むバイナリが利用できること。リポジトリで `make build` を実行すると `./skr` が生成されます。以下の `skr` は、そのバイナリを実行するコマンドとして記載します。
- 対象プロジェクトに対する認証情報を、プロファイルまたは `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` 環境変数で設定済みであること。読み取り操作のため、参照できる権限があれば十分です。認証情報はコマンド例や JSON ファイルに書かないでください。

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

## 参考情報

- [IPv6 アドレスの基本](https://manual.sakura.ad.jp/cloud/network/ipv6/ipv6-basic.html)
- [IPv6 アドレスの利用手順](https://manual.sakura.ad.jp/cloud/network/ipv6/ipv6-howto.html)
- [sacloud-sdk-go v0.3.1 IPv6Addr API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.1/service/iaas/ipv6addr)
