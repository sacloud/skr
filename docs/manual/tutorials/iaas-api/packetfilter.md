# IaaS PacketFilter API: ルールを設定・管理する

[パケットフィルタ](https://manual.sakura.ad.jp/cloud/network/packet-filter.html)は、サーバの NIC に適用するルールを持つネットワーク機能です。ルールではプロトコル、送信元ネットワーク、ポート、許可／拒否アクションなどを設定できます。このチュートリアルでは `skr iaas-api packetfilter` でパケットフィルタを作成し、ルールを確認・更新して削除します。ここではフィルタを NIC に適用しないため、サーバの通信設定は変更しません。

> [!WARNING]
> この手順は、選択中のプロファイルが指すプロジェクトの `is1b` ゾーンにパケットフィルタを作成・更新・削除します。実行前に対象プロファイル、プロジェクト、変更内容を確認してください。作成したテスト用フィルタだけを削除してください。NIC に適用する操作はこのチュートリアルに含めません。

## 前提条件

- `skr iaas-api packetfilter` を含むバイナリが利用できること。リポジトリで `make build` を実行すると `./skr` が生成されます。
- 対象プロジェクトの認証情報がプロファイル、または `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` 環境変数で設定済みであること。パケットフィルタの作成・更新・削除に必要な権限も必要です。
- `TEST-PACKETFILTER-NAME` と `UPDATED-PACKETFILTER-NAME` を、他のリソースと重複しないテスト専用名に置き換えること。
- JSON の `ID` と `ExpressionHash` は実際の読み取り結果に置き換えること。認証情報をコマンドや JSON に含めないでください。

## 作成するリソースと順序

この手順では、`is1b` にパケットフィルタを1個作成し、ルールを設定して内容を確認した後、同じフィルタを削除します。サーバや NIC は作成せず、フィルタを NIC に適用しません。

## Step 1: 対象環境とヘルプを確認する

```console
$ skr config current
$ skr iaas-api --help
$ skr iaas-api packetfilter --help
$ skr iaas-api packetfilter create --help
$ skr iaas-api packetfilter find --help
$ skr iaas-api packetfilter read --help
$ skr iaas-api packetfilter update --help
$ skr iaas-api packetfilter delete --help
```

`config current` で対象プロファイルを確認してください。操作では `--zone` などの個別フラグ、または完全な `--request` JSON を使います。両方は併用できません。ルール配列は `--expressions` に JSON 配列を指定でき、直接入力と `@path.json` に対応しています。API の結果は JSON で出力します。

## Step 2: ルールを指定してパケットフィルタを作成する

`TEST-PACKETFILTER-NAME` を一意なテスト名に置き換えます。次の例では、TCP と送信元ネットワーク、宛先ポート、許可アクションを持つルールを1件指定しています。ルール配列は JSON のまま `--expressions` に渡します。

```console
$ skr iaas-api packetfilter create --zone is1b --name TEST-PACKETFILTER-NAME --description "Temporary PacketFilter for tutorial" --expressions '[{"Protocol":"tcp","SourceNetwork":"192.0.2.0/24","SourcePort":"","DestinationPort":"443","Action":"allow","Description":"Allow HTTPS"}]'
```

複数のルールなど長い配列は、同じ JSON 配列を `rules.json` に保存して `--expressions @rules.json` と指定できます。`--expressions` と `--request` は併用できません。完全な JSON リクエストを使う場合は、`Zone`、`Name`、`Expression` などを含む JSON オブジェクトを `--request @packetfilter-create.json` に渡し、上のフラグを使った作成コマンドは実行しません。

作成結果の `ID` を控えてください。後続の参照、更新、削除にはこの値を使います。`Expression` にはルールが配列の順序で含まれます。プロトコル、アクション、ネットワーク、ポートの形式は[パケットフィルタの設定](https://manual.sakura.ad.jp/cloud/network/packet-filter.html)と [PacketFilter の SDK リクエスト型](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/packetfilter)を参照してください。

## Step 3: 作成したルールを検索・参照する

`TEST-PACKETFILTER-NAME` を作成時の名前に置き換えます。`Names` は文字列配列のため、検索リクエストを JSON で渡します。`--query` は `ID` と `Name` のみを配列として投影します。

```console
$ skr iaas-api packetfilter find --request='{"Zone":"is1b","Names":["TEST-PACKETFILTER-NAME"]}' --query 'map({ID,Name})'
```

出力例です。ID と名前は説明用の値へ置換しています。

```json
[
  {
    "ID": 123456789012,
    "Name": "TEST-PACKETFILTER-NAME"
  }
]
```

作成結果の `ID` を使って詳細を読み取り、`Expression` の内容を確認します。

```console
$ skr iaas-api packetfilter read --zone is1b --id 123456789012
```

読み取り結果から、確認に使うフィールドを抜粋した例です。ID、名前、説明、ハッシュは説明用の値へ置換しています。

```json
{
  "ID": 123456789012,
  "Name": "TEST-PACKETFILTER-NAME",
  "Description": "Temporary PacketFilter for tutorial",
  "Expression": [
    {
      "Protocol": "tcp",
      "SourceNetwork": "192.0.2.0/24",
      "SourcePort": "",
      "DestinationPort": "443",
      "Action": "allow",
      "Description": "Allow HTTPS"
    }
  ],
  "ExpressionHash": "EXPRESSION-HASH-FROM-READ"
}
```

## Step 4: ルールを更新する

読み取り結果の `ExpressionHash` を更新コマンドの `--original-expression-hash` に指定します。`UPDATED-PACKETFILTER-NAME` と ID は実際の値に置き換えてください。次の例ではルールの宛先ポートと説明を変更します。

```console
$ skr iaas-api packetfilter update --zone is1b --id 123456789012 --name UPDATED-PACKETFILTER-NAME --original-expression-hash EXPRESSION-HASH-FROM-READ --expressions '[{"Protocol":"tcp","SourceNetwork":"192.0.2.0/24","SourcePort":"","DestinationPort":"8443","Action":"allow","Description":"Updated rule"}]'
```

`--expressions` を指定するとルール配列全体を更新するため、残すルールも含めて指定してください。省略した更新では現在のルールを保ちます。`OriginalExpressionHash` を含む完全なリクエスト JSON を渡す方法もありますが、その場合は個別フラグを同時に指定できません。

再度読み取り、`Name` が `UPDATED-PACKETFILTER-NAME`、ルールの `DestinationPort` が `"8443"`、ルールの `Description` が `"Updated rule"` に変わっていることを確認してください。フィルタ自体の `Description` は更新コマンドで指定していないため、作成時の値のままです。

```console
$ skr iaas-api packetfilter read --zone is1b --id 123456789012
```

この手順で確認するのは API に保存されたルールです。サーバの NIC へパケットフィルタを適用していないため、実際のパケットの許可・拒否や通信の変化は確認しません。NIC への適用手順は[パケットフィルタの NIC 設定](https://manual.sakura.ad.jp/cloud/network/packet-filter.html)を参照してください。適用すると対象 NIC の通信設定が変わります。

## Step 5: 作成したパケットフィルタを削除する

削除前に読み取り結果の `ID`、`Name`、`Description` を確認し、このチュートリアルで作成したフィルタであることを確かめます。`123456789012` は実際の ID に置き換えてください。

```console
$ skr iaas-api packetfilter delete --zone is1b --id 123456789012 --fail-if-not-found
$ skr iaas-api packetfilter find --request='{"Zone":"is1b","Names":["UPDATED-PACKETFILTER-NAME"]}' --query 'map({ID,Name})'
```

削除後の検索結果に該当するパケットフィルタがない場合、出力は次のとおりです。

```json
[]
```

削除成功時、`delete` は標準出力に何も出しません。削除対象が自分で作成したフィルタであることを確認できない場合は、削除しないでください。

## 参考情報

- [さくらのクラウドマニュアル: パケットフィルタの作成、ルール設定、NIC への適用](https://manual.sakura.ad.jp/cloud/network/packet-filter.html)
- [sacloud-sdk-go v0.3.0: PacketFilter サービス](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/packetfilter)
