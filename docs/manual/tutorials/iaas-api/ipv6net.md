# IaaS IPv6Net API: ゾーン内の IPv6 ネットワークを確認する

[IPv6 ネットワーク](https://manual.sakura.ad.jp/cloud/network/ipv6/ipv6-basic.html)は、ルータ＋スイッチに IPv6 アドレスの割り当てを有効化したときに作られる、IPv6 プレフィクスの割り当て情報です。このチュートリアルでは、`skr iaas-api ipv6net` を使い、ゾーン内の IPv6 ネットワークを検索して、プレフィクスの内容を確認します。リソースの作成・変更・削除は行いません。

IPv6 アドレスの割り当ては、ルータ＋スイッチの詳細画面から IPv6 を有効化すると作成されます（[IPv6 の利用手順](https://manual.sakura.ad.jp/cloud/network/ipv6/ipv6-howto.html)）。`skr iaas-api internet` の `enable-ipv6` でも有効化できます。有効化していないプロジェクトでは、検索結果は空の一覧になります。

## 前提条件

- `skr iaas-api ipv6net` を含むバイナリが利用できること。リポジトリで `make build` を実行すると `./skr` が生成されます。以下の `skr` は、そのバイナリを実行するコマンドとして記載します。
- 対象プロジェクトに対する認証情報を、プロファイルまたは `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` 環境変数で設定済みであること。読み取り操作のため、参照できる権限があれば十分です。認証情報はコマンド例や JSON ファイルに書かないでください。

## Step 1: 対象環境とヘルプを確認する

```console
$ skr config current
$ skr iaas-api --help
$ skr iaas-api ipv6net --help
$ skr iaas-api ipv6net find --help
$ skr iaas-api ipv6net read --help
```

`config current` で意図したプロファイルが選択されていることを確認してください。`find` と `read` にはゾーン `--zone` が必要です。`read` では、参照する IPv6 ネットワークの数値の `ID` も必要です。`--request` で検索条件の JSON を渡すこともできますが、`--zone` などの個別フラグと併用はできません。

## Step 2: ゾーン内の IPv6 ネットワークを検索する

`--zone` に確認したいゾーン名を指定します。次の例は `is1b` の場合です。

```console
$ skr iaas-api ipv6net find --zone is1b --query 'map({ID,IPv6Prefix,IPv6PrefixLen})'
```

出力例は次のとおりです。`ID` とプレフィクスは実際のアカウントの値に置き換わります。

```json
[
  {
    "ID": 123456789012,
    "IPv6Prefix": "2001:db8:a001::",
    "IPv6PrefixLen": 64
  }
]
```

IPv6 アドレスの割り当てを有効化していないプロジェクトでは、空の一覧が出力されます。

```json
[]
```

検索結果の絞り込みや並び替えは、`--request` で検索条件の JSON を渡して行います。接続先のスイッチやルータ＋スイッチで絞り込むキー (`SwitchIDs`、`SwitchNames`、`InternetIDs`、`InternetNames`) と `Sort` は JSON で、取得件数の `Count` と開始位置の `From` は `--count` と `--from` で指定できます。

## Step 3: IPv6 ネットワークを参照する

Step 2 で得た `ID` を指定して、1件の IPv6 ネットワークを参照します。`123456789012` は仮の ID です。必ず検索結果の `ID` に置き換えてください。

```console
$ skr iaas-api ipv6net read --zone is1b --id 123456789012
```

出力の `IPv6Prefix` と `IPv6PrefixLen` が検索結果と一致することを確認します。`ID` は IPv6 ネットワーク (IPv6 アドレスの割り当て) のもので、ルータ＋スイッチの ID とは異なります。ルータ＋スイッチに割り当てられた IPv6 プレフィクスを確認するには、`skr iaas-api internet read-ipv6` にルータ＋スイッチの ID を指定します。

## 参考情報

- [IPv6 アドレスの基本](https://manual.sakura.ad.jp/cloud/network/ipv6/ipv6-basic.html)
- [IPv6 アドレスの利用手順](https://manual.sakura.ad.jp/cloud/network/ipv6/ipv6-howto.html)
- [sacloud-sdk-go v0.3.1 IPv6Net API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.1/service/iaas/ipv6net)
