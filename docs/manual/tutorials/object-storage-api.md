# Object Storage API: サイトとバケットを管理する

このチュートリアルでは、`skr object-storage-api` でサイトを確認し、バケットとバケット単位のパーミッションを管理します。オブジェクトストレージは HTTPS と Amazon S3 互換プロトコルでオブジェクトを保管・取得するサービスです。この CLI が扱うのはサイトやバケットなどの管理 API であり、オブジェクト本体のアップロードや取得は別の S3 互換クライアントで行います。

> [!WARNING]
> バケットの作成が完了すると、そのバケットの基本料金が発生します。実行前に選択中のプロファイルと対象サイトを確認し、不要になったバケットは中身を削除してから削除してください。バケットにオブジェクトやフォルダが残っていると削除できません。

## 前提条件

- `skr` が利用でき、SDK プロファイルまたは `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` が設定されていること
- 対象サイトでオブジェクトストレージを利用でき、サイトアカウントが作成済みであること
- 対象サイト ID と、オブジェクトストレージ API を操作できる権限があること
- 他の利用者が使っていないバケット名を用意すること。バケット名はオブジェクトストレージ全体で一意です

サイトアカウント未作成の場合、対象のサイト ID を確認後、`skr object-storage-api account create SITE-ID` を実行します。作成応答の `resource_id` を控え、`skr object-storage-api account read SITE-ID` の ID と一致することを確認してください。アカウントの作成だけで利用料金は発生しません。既存のサイトアカウントを作成・削除せず、そのまま利用してください。

## 手順とリソース

1. サイト一覧から作成先サイトの ID を確認します。
2. サイト内にバケットを作成し、一覧で名前を確認します。
3. 作成したバケットだけにアクセスできるパーミッションを登録します。
4. パーミッションのアクセスキーを必要とする場合に発行し、秘密情報を保護します。
5. 権限を変更して確認し、アクセスキー、パーミッション、バケットを作成と逆の順で削除します。

## Step 1: 対象環境とコマンドを確認する

```console
$ skr config current
$ skr object-storage-api --help
$ skr object-storage-api site list --help
$ skr object-storage-api bucket create --help
$ skr object-storage-api permission create --help
```

サイト一覧で `site-id` を確認します。`SITE-ID` と `BUCKET-NAME` は対象環境に合わせて置き換えてください。

```console
$ skr object-storage-api site list --query 'map({id,display_name,region,plan_family})'
```

```json
[
  {"id":"isk01","display_name":"石狩第1サイト","region":"jp-north-1","plan_family":"standard"},
  {"id":"tky01","display_name":"東京第1サイト","region":"jp-east-1","plan_family":"standard"},
  {"id":"arc02","display_name":"東京第1サイト(アーカイブプラン)","region":"jp-east-1","plan_family":"archive"}
]
```

サイトはバケットなどのリソースを配置する場所です。サイト ID はさくらのクラウドの IaaS ゾーン名とは異なります。

## Step 2: バケットを作成して確認する

バケット名は半角3〜63文字で、小文字英字・数字・ハイフンを使用できます。名前の先頭と末尾にはハイフンを使えず、他の利用者のバケットと重複できません。
`bucket create` で指定できるサイトは `isk01`、`tky01`、`arc02` です。アーカイブプラン用サイト `arc02` 以外では `--plan` を指定しないでください。

```console
$ skr object-storage-api bucket create SITE-ID BUCKET-NAME
$ skr object-storage-api bucket list SITE-ID --query 'map({name,resource_id,plan})'
```

一覧で今回作成した名前と `resource_id` を控えます。作成応答の `cluster_id` と `name` が指定したサイトと名前に一致することも確認してください。以降の削除前に同じ `resource_id` を再照合します。一覧の `plan` が `null` の場合は、作成応答の `plan` でプランを確認します。

次は作成したバケットだけを抜粋し、名前と ID を置換した出力例です。

```json
[
  {"name":"BUCKET-NAME","resource_id":"RESOURCE-ID","plan":null}
]
```

アーカイブプラン用サイト `arc02` では、先にプラン一覧を確認して `--plan` を指定します。`--plan` には `service_class_path` の末尾にあるプラン値を渡します。

```console
$ skr object-storage-api site plans arc02
$ skr object-storage-api bucket create arc02 BUCKET-NAME --plan PLAN-VALUE
```

## Step 3: バケットにパーミッションを設定する

秘密情報やリクエストを保存する、他の利用者がアクセスできない一時ディレクトリを用意します。同じシェルで後片付けまで実行してください。

```console
$ umask 077
$ work_dir=$(mktemp -d)
```

`$work_dir/permission.json` を作成します。`BUCKET-NAME` は今回作成した名前に置き換えてください。`can_read` と `can_write` は、このパーミッションで許可する操作を表します。

```json
{
  "display_name": "skr tutorial",
  "bucket_controls": [
    {
      "bucket_name": "BUCKET-NAME",
      "can_read": true,
      "can_write": true
    }
  ]
}
```

```console
$ skr object-storage-api permission create SITE-ID --request "@$work_dir/permission.json"
$ skr object-storage-api permission list SITE-ID --query 'map({id,display_name,bucket_controls})'
```

作成応答の数値 `id` を控え、一覧の同じ ID の `display_name` と `bucket_controls` を照合します。次は対象の1件だけを抜粋し、ID・表示名・バケット名を置換した出力例です。他バケットの読み取り・書き込みを両方拒否する項目が含まれる場合もあります。

```json
[
  {
    "id": 1001,
    "display_name": "skr tutorial",
    "bucket_controls": [
      {"bucket_name":"BUCKET-NAME","can_read":true,"can_write":true}
    ]
  }
]
```

パーミッションを S3 互換クライアントで利用するには、そのパーミッションに対するアクセスキーが必要です。アクセスキーは作成時に秘密情報を含む応答を返します。既存ファイルと衝突しない一時ディレクトリへ保存して発行結果を保護します。アクセスキー操作時は `--trace` を使わないでください。

```console
$ skr object-storage-api permission access-key create SITE-ID PERMISSION-ID > "$work_dir/permission-key.json"
```

`PERMISSION-ID` は作成結果の ID に置き換えます。保存した応答の `id` を `KEY-ID` として控えてください。`$work_dir/permission-key.json` にはシークレットが含まれるため、共有・コミット・通常のログ保存をしないでください。シークレットは後から読み直せません。失った場合は既存のアクセスキーを削除し、新しいものを発行してください。

## Step 4: 権限を変更して確認する

`$work_dir/permission.json` の `can_write` を `false` に変更し、同じパーミッションを更新します。ID、表示名、バケット名と読み取り許可は維持します。バケットごとの設定を含むリクエスト全体を渡してください。

```console
$ skr object-storage-api permission update SITE-ID PERMISSION-ID --request "@$work_dir/permission.json"
$ skr object-storage-api permission read SITE-ID PERMISSION-ID
$ skr object-storage-api permission access-key read SITE-ID PERMISSION-ID KEY-ID --query '{id}'
```

更新後の `bucket_controls` で、対象バケットの `can_read` が `true`、`can_write` が `false` であることと、余分なバケットへの許可がないことを確認します。キーの参照結果の `id` も控えた値と照合します。ここでは管理 API の設定を確認し、S3 互換 API でのオブジェクト操作は実行しません。

## Step 5: 設定を確認して削除する

削除前にパーミッション ID・表示名・対象バケットの権限、キー ID を再照合してください。キーを失効させ、不在を確認してからパーミッションを削除します。バケットにオブジェクトやフォルダがある場合は、S3 互換クライアントでそれらを先に削除してください。この CLI はオブジェクト本体を削除しません。

```console
$ skr object-storage-api permission read SITE-ID PERMISSION-ID
$ skr object-storage-api permission access-key list SITE-ID PERMISSION-ID --query 'map({id})'
```

対象キーの出力例です。ID は置換しています。

```json
[{"id":"KEY-ID"}]
```

```console
$ skr object-storage-api permission access-key delete SITE-ID PERMISSION-ID KEY-ID
$ skr object-storage-api permission access-key list SITE-ID PERMISSION-ID --query 'map({id})'
```

今回発行したキーしかない場合、失効後は次の空配列になります。

```json
[]
```

```console
$ skr object-storage-api permission delete SITE-ID PERMISSION-ID
$ skr object-storage-api permission list SITE-ID --query 'map({id,display_name,bucket_controls})'
```

削除したパーミッション ID が一覧にないことを確認します。他のパーミッションがなければ `[]` です。続いてバケット一覧を確認します。

```console
$ skr object-storage-api bucket list SITE-ID --query 'map({name,resource_id,plan})'
```

対象の1件だけを抜粋し、名前と ID を置換した出力例です。

```json
[{"name":"BUCKET-NAME","resource_id":"RESOURCE-ID","plan":null}]
```

作成時に控えた `resource_id` と同じ場合に限り、削除します。同名の別 ID や照合不能な応答なら中止してください。

```console
$ skr object-storage-api bucket delete SITE-ID BUCKET-NAME
$ skr object-storage-api bucket list SITE-ID --query 'map({name,resource_id,plan})'
```

削除後の一覧で今回の `BUCKET-NAME` が表示されないことを確認します。他のバケットがない場合は次の空配列になります。

```json
[]
```

サイトアカウントをこの手順で新規作成した場合に限り、`account read` の ID が作成時と一致し、バケット・パーミッション・アカウントアクセスキーがすべて空であることを確認してから、`skr object-storage-api account delete SITE-ID` を実行できます。`account read` が API の404を返すことで削除後の不在を確認します。403や通信失敗を不在とは判断しません。既存のサイトアカウントは削除しないでください。

キーの失効が完了したら、この手順で作成したローカルファイルと空の一時ディレクトリを削除します。

```console
$ rm "$work_dir/permission-key.json" "$work_dir/permission.json"
$ rmdir "$work_dir"
```

途中で失敗した場合も、取得済みの ID を使ってクラウドの残存状態を確認し、今回作成したキー・パーミッション・バケットだけを削除してください。所有を確認できないリソースを推測で削除しないでください。ローカルの秘密ファイルも放置しないでください。

## 参考資料

- [オブジェクトストレージのサービス基本情報](https://manual.sakura.ad.jp/cloud/objectstorage/about.html)
- [オブジェクトストレージの基本操作](https://manual.sakura.ad.jp/cloud/objectstorage/basic.html)
- [オブジェクトストレージ API の種類と対応操作](https://manual.sakura.ad.jp/cloud/objectstorage/api.html)
- [オブジェクトストレージのよくある質問](https://manual.sakura.ad.jp/cloud/objectstorage/faq.html)
- [sacloud-sdk-go v0.3.1 Object Storage API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.1/api/object-storage)
