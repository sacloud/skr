# IaaS IPAddress API: ゾーン内のグローバル IP アドレスを確認する

グローバル IP アドレスは、サーバやルータ＋スイッチに割り当てられたインターネット側の IP アドレスです。このチュートリアルでは、`skr iaas-api ipaddress` を使い、ゾーン内のグローバル IP アドレスを一覧表示して、内容を確認します。リソースの作成・変更・削除は行いません。

一覧は、ゾーンのプロジェクトに割り当てられたグローバル IP アドレスを表示します。割り当てられたグローバル IP アドレスがないプロジェクトでは、空の一覧になります。

> [!NOTE]
> `update-host-name` は、グローバル IP アドレスが割り当てられたサーバまたはスイッチが必要です。このチュートリアルでは扱いません。

## 前提条件

- `skr iaas-api ipaddress` を含むバイナリが利用できること。リポジトリで `make build` を実行すると `./skr` が生成されます。以下の `skr` は、そのバイナリを実行するコマンドとして記載します。
- 対象プロジェクトに対する認証情報を、プロファイルまたは `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` 環境変数で設定済みであること。読み取り操作のため、参照できる権限があれば十分です。認証情報はコマンド例や JSON ファイルに書かないでください。

## Step 1: 対象環境とヘルプを確認する

```console
$ skr config current
$ skr iaas-api --help
$ skr iaas-api ipaddress --help
$ skr iaas-api ipaddress list --help
$ skr iaas-api ipaddress read --help
```

`config current` で意図したプロファイルが選択されていることを確認してください。`list` と `read` にはゾーン `--zone` が必要です。`read` では、参照するグローバル IP アドレス `--ip-address` も必要です。`--request` でリクエストの JSON を渡すこともできますが、`--zone` などの個別フラグと併用はできません。

## Step 2: ゾーン内のグローバル IP アドレスを一覧表示する

`--zone` に確認したいゾーン名を指定します。次の例は `is1b` の場合です。

```console
$ skr iaas-api ipaddress list --zone is1b --query 'map({IPAddress,HostName})'
```

出力例は次のとおりです。アドレスとホスト名は実際のアカウントの値に置き換わります。

```json
[
  {
    "IPAddress": "192.0.2.10",
    "HostName": "www.example.com"
  }
]
```

グローバル IP アドレスを割り当てていないプロジェクトでは、空の一覧が出力されます。

```json
[]
```

## Step 3: グローバル IP アドレスを参照する

Step 2 で得たアドレスを指定して、1件のグローバル IP アドレスを参照します。`192.0.2.10` は仮のアドレスです。必ず一覧結果の `IPAddress` に置き換えてください。

```console
$ skr iaas-api ipaddress read --zone is1b --ip-address 192.0.2.10
```

出力の `HostName` が逆引きホスト名の登録内容と一致することを確認します。逆引きホスト名を登録していないアドレスでは、`HostName` は空になります。

## 参考情報

- [IP アドレスの逆引き設定 (スイッチ、ルータ＋スイッチ)](https://manual.sakura.ad.jp/cloud/network/switch/reverse-dns.html)
- [sacloud-sdk-go v0.3.1 IPAddress API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.1/service/iaas/ipaddress)
