# IaaS Server API: ディスクレスサーバを作成して管理する

[サーバ](https://manual.sakura.ad.jp/cloud/server/create-delete.html)は、CPU とメモリのプランを選び、必要に応じてディスクやネットワークインターフェースを接続して利用します。このチュートリアルでは、ディスクとネットワークを接続しないサーバを停止状態で作成し、参照、名前の更新、削除をします。ゲスト OS を起動したり、ネットワークを使ったりしません。

> [!WARNING]
> サーバの作成・更新・削除は選択中のプロジェクトに影響します。対象プロファイル、プロジェクト、ゾーン、利用可能なサーバプランを確認してから実行してください。構成に応じた料金が発生する場合があります。料金や対象環境を確認し、このチュートリアルのための操作について必要な承認を得てください。

## 前提条件

- `skr iaas-api server` を含むバイナリが利用できること。リポジトリで `make build` を実行すると `./skr` が生成されます。以下の `skr` は、そのバイナリを実行するコマンドとして記載します。
- 対象プロジェクトへの認証情報を SDK プロファイル、または `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` 環境変数で設定済みであること。対象サーバを作成・更新・削除できる権限も必要です。認証情報をコマンド例や JSON ファイルに書かないでください。
- 対象ゾーンで利用可能な CPU とメモリの組み合わせを確認できること。CPU 数に応じて選択できるメモリ容量が異なります。コントロールパネルではサーバプラン一覧から選択できます（[サーバの作成・削除](https://manual.sakura.ad.jp/cloud/server/create-delete.html)）。
- `SERVER-NAME` と `UPDATED-SERVER-NAME` を、対象プロジェクトで重複しないテスト用の名前に置き換えること。後述の `123456789012` は仮の ID です。必ず作成結果の `ID` に置き換えてください。

## 作成するリソースと順序

作成するのはサーバ1台です。ディスクとネットワークインターフェースは接続しません。作成後もサーバを起動しません。サーバにはディスクレス構成と NIC の切断を設定できます（[サーバの作成・削除](https://manual.sakura.ad.jp/cloud/server/create-delete.html)）。この手順ではディスクやスイッチなどの別リソースを作成しません。クリーンアップで削除するのはサーバだけです。

## Step 1: 対象環境とコマンドを確認する

```console
$ skr config current
$ skr iaas-api --help
$ skr iaas-api server --help
$ skr iaas-api server find --help
$ skr iaas-api server read --help
$ skr iaas-api server create --help
$ skr iaas-api server update --help
$ skr iaas-api server delete --help
```

意図したプロファイルが選択されていることを確認してください。環境変数で認証する場合も、対象プロジェクトと権限を別途確認します。検索・参照・更新・削除では `--zone` などの個別フラグか、`--request` の JSON オブジェクトを指定します。両方は併用できません。作成では複雑な構成に対応するため `--request` を使います。JSON はコマンドラインに直接指定するか、`@ファイル名` で読み込めます。API の結果は JSON で出力し、`--query` で必要な項目を抽出できます。

検索では対象のゾーンを明示してください。

## Step 2: 停止状態のサーバを作成する

`Zone` と `Name` を環境に合わせて置き換えます。`CPU` と `MemoryGB` は数値の例です。対象ゾーンで利用可能な組み合わせに置き換えてください。料金の内訳は構成によって異なります。コントロールパネルでは作成前に料金内訳を表示できます（[サーバの作成・削除](https://manual.sakura.ad.jp/cloud/server/create-delete.html)）。

基本的な構成は `--zone`、`--name`、`--cpu`、`--memory-gb` で指定できます。作成後に起動しないよう `--boot-after-create=false` を指定します。

```console
$ skr iaas-api server create --zone ZONE --name SERVER-NAME --cpu 1 --memory-gb 1 --boot-after-create=false
```

配列などの複雑な値を渡す場合は JSON を使います。次の例では `Tags` を追加します。JSON と個別フラグは併用できません。フラグで作成した場合はこの JSON コマンドを実行せず、JSON を使う場合は上のフラグコマンドを実行しないでください。

```json
{
  "Zone": "ZONE",
  "Name": "SERVER-NAME",
  "CPU": 1,
  "MemoryGB": 1,
  "BootAfterCreate": false,
  "Tags": ["tutorial"]
}
```

ディスクを作成する `Disks` とネットワーク接続を設定する `NetworkInterfaces` は省略しています。OS をインストールしたり起動したりする構成ではありません。ディスク、アーカイブ、ネットワークインターフェースを指定する場合は、対象 ID など必要な値を含む JSON ファイルを用意してください。

```console
$ skr iaas-api server create --request @server-create.json
```

成功するとサーバ情報が出力されます。以降の操作で使う `ID` を控えてください。任意のスカラー値は `server create --help` にある対応フラグでも指定できます。`Tags`、`Disks`、`NetworkInterfaces` などの配列や複雑な値は JSON に残します。

## Step 3: サーバを参照して名前を更新する

`123456789012` は作成結果の `ID` に置き換えてください。最初に対象サーバの `ID` と `Name` を確認します。

```console
$ skr iaas-api server read --zone ZONE --id 123456789012
```

名前だけを更新します。`update` で省略した項目は変更されません。

```console
$ skr iaas-api server update --zone ZONE --id 123456789012 --name UPDATED-SERVER-NAME
$ skr iaas-api server read --zone ZONE --id 123456789012
```

2回目の `read` で `Name` が更新後の値であることを確認してください。CPU またはメモリを更新する場合は、ゾーンで利用可能なプランの組み合わせを確認してください。プラン変更ではサーバのリソース ID が変わります。以降の操作では新しいサーバ ID を使ってください（[サーバのプラン変更](https://manual.sakura.ad.jp/cloud/server/plan-update.html)）。

## Step 4: 作成したサーバを削除する

削除前に `read` で対象の `ID` と `Name` を再確認してください。サーバ作成時にディスクは作成していないため、このチュートリアルではサーバだけを削除します。`--with-disks` は指定しません。これは接続ディスクを削除せずに残す動作です。起動中のサーバは停止が必要です。意図せず強制停止しないよう、この例では `--force` も指定しません。

```console
$ skr iaas-api server delete --zone ZONE --id 123456789012 --fail-if-not-found
```

削除後に同じ ID を `read` し、対象が見つからないエラーになることを確認できます。`delete` 成功時は標準出力に何も出しません。削除対象を取り違えないよう、既存サーバの ID を使わないでください。

## 参考情報

- [さくらのクラウドマニュアル: サーバの作成・削除](https://manual.sakura.ad.jp/cloud/server/create-delete.html)
- [さくらのクラウドマニュアル: サーバのプラン変更](https://manual.sakura.ad.jp/cloud/server/plan-update.html)
- [sacloud-sdk-go v0.3.0: Server サービスとリクエスト型](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/server)
