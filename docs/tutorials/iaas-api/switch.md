# IaaS Switch API: Sandbox でスイッチの管理手順を確認する

[スイッチ](https://manual.sakura.ad.jp/cloud/network/switch/about.html)は、サーバやアプライアンスを L2 接続する仮想的な機器です。通常のスイッチはプライベートネットワークに使います。このチュートリアルでは、`skr iaas-api switch` を使い、`tk1v` の Sandbox ゾーンにテスト専用スイッチを1個作成して、検索、参照、名称変更、削除までの管理手順を確認します。ルータ＋スイッチやサーバの作成・接続は行いません。

> [!WARNING]
> この手順は、選択中のプロファイルが指すプロジェクトの `tk1v` ゾーンにリソースを作成・更新・削除します。操作前に対象プロファイルとプロジェクト、変更内容について承認を確認してください。この手順で作成したスイッチだけを削除してください。接続済みのリソースがあるスイッチは削除できません（[スイッチの作成・削除](https://manual.sakura.ad.jp/cloud/network/switch/router-switch.html)）。

## 前提条件

- `skr iaas-api switch` を含むバイナリが利用できること。リポジトリで `make build` を実行すると `./skr` が生成されます。以下の `skr` は、そのバイナリを実行するコマンドとして記載します。
- 対象プロジェクトに対する認証情報を、プロファイルまたは `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` 環境変数で設定済みであること。スイッチを作成・更新・削除できる権限も必要です。認証情報はコマンド例や JSON ファイルに書かないでください。
- 対象が `tk1v` の Sandbox ゾーンであることを確認できること。[Sandbox のマニュアル](https://manual.sakura.ad.jp/cloud/server/sandbox.html)によると、Sandbox で作成したリソースは課金されません。ただし、スイッチを含む機器はインターネットに接続できません。この手順で検証するのは API での管理操作であり、実際のネットワーク通信ではありません。
- `TEST-SWITCH-NAME` と `UPDATED-TEST-SWITCH-NAME` を、他のリソースと重複しない、自分のテスト専用の名前に置き換えること。後述の数値 `123456789012` は **仮の ID** です。必ず作成結果の `ID` に置き換えてください。

## 作成するリソースと順序

作成するのは、`tk1v` の通常のスイッチ1個だけです。作成結果の `ID` を使って参照と名称を変更し、同じ `ID` のスイッチを削除します。サーバ、インターフェース、ルータ＋スイッチなどの追加リソースは作成しません。通常のスイッチがローカルネットワーク用であることは[スイッチの概要](https://manual.sakura.ad.jp/cloud/network/switch/about.html)で確認できます。

## Step 1: 対象環境とヘルプを確認する

```console
$ skr config current
$ skr iaas-api --help
$ skr iaas-api switch --help
$ skr iaas-api switch create --help
$ skr iaas-api switch find --help
$ skr iaas-api switch read --help
$ skr iaas-api switch update --help
$ skr iaas-api switch delete --help
```

`config current` で意図したプロファイルが選択されていることを確認してください。環境変数による認証の場合も、対象プロジェクトと権限を別途確認してください。各操作には `--zone` などのフラグか、`--request` で JSON オブジェクトを渡します。両方を指定するとエラーになります。JSON はコマンドに直接指定するか、`@ファイル名` で読み込めます。JSON のキー名と値の種類は各操作の `--help` に記載されています。作成・参照・更新・削除では対象ゾーン名を指定します。`find --zone all` は SDK から取得したゾーン一覧を順に検索して結果をまとめます。全ゾーン検索は個別フラグで指定するため、複雑な検索条件を含む `--request` の JSON とは併用できません。

## Step 2: テスト用スイッチを作成する

`TEST-SWITCH-NAME` は自分のテスト専用の一意な名前に置き換えてください。`create` には作成先ゾーン `--zone` とスイッチ名 `--name` が必要です。スイッチのみを作成するため、この例では他のリソースの ID は渡しません。

```console
$ skr iaas-api switch create --zone tk1v --name TEST-SWITCH-NAME --description "Temporary switch for skr API tutorial"
```

`Tags` のような配列を指定するときは、`create --help` に記載されたキー名を使って JSON ファイルを作り、`--request @switch-create.json` で渡します。例えば次の内容を `switch-create.json` として保存し、名前を置き換えてください。こちらを使用する場合は、上のフラグを使った作成コマンドは実行しません。

```json
{
  "Zone": "tk1v",
  "Name": "TEST-SWITCH-NAME",
  "Description": "Temporary switch for skr API tutorial",
  "Tags": ["test"]
}
```

```console
$ skr iaas-api switch create --request @switch-create.json
```

`--request` と個別フラグは併用できません。成功時は Switch の JSON オブジェクトが出力されます。後続の操作と削除に使用する `ID` を控えてください。実際のアカウントの出力や ID を共有・公開しないでください。

## Step 3: スイッチを検索し、変更を確認する

`TEST-SWITCH-NAME` は、作成時に指定した名前へ置き換えてください。検索条件の `Names` は名前の文字列配列で、フラグでは指定できません。`Zone` とともに JSON で単一ゾーンを検索します。検索結果の `ID` が作成結果のものと一致するか確認します。`find` は JSON 配列を返します。

```console
$ skr iaas-api switch find --request='{"Zone":"tk1v","Names":["TEST-SWITCH-NAME"]}'
```

全ゾーンを検索するときは、複雑な検索条件を含む JSON ではなく、次のフラグ経路を使用します。

```console
$ skr iaas-api switch find --zone all
```

`123456789012` は仮の数値です。Step 2 で得た `ID` に置き換えてください。

```console
$ skr iaas-api switch read --zone tk1v --id 123456789012
```

出力の `Name` と `ID` が作成したスイッチの値に一致することを確認します。次に、同じ `ID` で名前だけを更新します。`UPDATED-TEST-SWITCH-NAME` も一意なテスト専用名に置き換えてください。`update` で省略した項目は変更されないため、`Description` は指定しません。

```console
$ skr iaas-api switch update --zone tk1v --id 123456789012 --name UPDATED-TEST-SWITCH-NAME
$ skr iaas-api switch read --zone tk1v --id 123456789012
```

読み取った `Name` が更新後の名前、`Description` が作成時の値であることを確認してください。これはスイッチの **設定の管理** を確認する手順です。Sandbox ではインターネットに接続できず、スイッチのネットワーク通信やサーバ間の接続性までは検証できません（[Sandbox の制限](https://manual.sakura.ad.jp/cloud/server/sandbox.html)）。

## Step 4: 作成したスイッチを削除する

削除前に `read --zone tk1v --id 123456789012` で `ID` と `Name` を再確認します。接続したリソースがある場合は削除しないでください。スイッチに接続したリソースがあると削除できないことは[スイッチの削除手順](https://manual.sakura.ad.jp/cloud/network/switch/router-switch.html)に記載されています。この手順ではスイッチ以外のリソースを作成しないため、依存リソースの削除は不要です。

`--fail-if-not-found` を指定すると、削除対象が見つからない場合もエラーになります。仮の `ID` を作成したスイッチの ID に置き換えてください。

```console
$ skr iaas-api switch delete --zone tk1v --id 123456789012 --fail-if-not-found
$ skr iaas-api switch find --request='{"Zone":"tk1v","Names":["UPDATED-TEST-SWITCH-NAME"]}'
```

削除成功時、`delete` は標準出力に何も出しません。`find` の結果に削除した `ID` が含まれていないことを確認します。削除後に `read --zone tk1v --id 123456789012` を実行して、対象が見つからないエラーになることも確認できます。既存のスイッチが検索結果にあっても削除しないでください。

## 参考情報

- [さくらのクラウドマニュアル: スイッチ・ルータ＋スイッチの概要](https://manual.sakura.ad.jp/cloud/network/switch/about.html)
- [さくらのクラウドマニュアル: スイッチの作成・削除](https://manual.sakura.ad.jp/cloud/network/switch/router-switch.html)
- [さくらのクラウドマニュアル: Sandbox（テスト用ゾーン）](https://manual.sakura.ad.jp/cloud/server/sandbox.html)
- [sacloud-sdk-go v0.3.0: Switch サービス](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/swytch)
- `skr iaas-api switch --help`、`skr iaas-api switch <operation> --help`
