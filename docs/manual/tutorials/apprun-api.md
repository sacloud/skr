# AppRun API: コンテナイメージからアプリケーションをデプロイする

AppRun 共用型は、コンテナイメージからアプリケーションをデプロイし、負荷に応じて実行数を調整します。料金はリソースプラン単価、スケール数、時間に応じて発生します。公開 URL へのアクセスで実行インスタンスが起動する場合もあるため、この手順を実行する前に対象プロジェクト、料金、アプリケーション作成の承認を確認してください（[AppRun 共用型の基本情報](https://manual.sakura.ad.jp/cloud/apprun/about.html)、[クイックスタート](https://manual.sakura.ad.jp/cloud/apprun/getting_started.html)）。

> [!WARNING]
> この手順では AppRun アプリケーションを1件作成し、公開 URL にアクセスしてから、作成したアプリケーションだけを削除します。既存のアプリケーションやコンテナレジストリは変更しません。作成結果から ID を取得できない場合や、削除前の ID・名前・イメージが作成時の値と一致しない場合は削除を中止してください。アプリケーションの利用には料金が発生する可能性があります。

## 前提条件

- `skr apprun-api` が利用でき、AppRun 共用型を使うプロジェクトの認証プロファイルが選択されていること。
- 対象プロジェクトで AppRun 共用型のアプリケーションを一覧取得・作成・参照・削除する権限があること（[IAM ポリシー](https://manual.sakura.ad.jp/cloud/controlpanel/iam-policy.html)）。
- プロジェクトで一意となる `APP-NAME` を決めること。以降の例では `skr-apprun-tutorial-` に続けて、利用者が決めた識別子を指定します。
- サンプルアプリケーション `ghcr.io/sacloud/apprun-samples/tutorial:latest` を使用します。このイメージはポート `8080` で応答します（[AppRun クイックスタート](https://manual.sakura.ad.jp/cloud/apprun/getting_started.html)）。

## Step 1: プロファイルとコマンドを確認する

```console
$ skr config current
$ skr apprun-api --help
$ skr apprun-api application list --help
$ skr apprun-api application create --help
$ skr apprun-api application read --help
$ skr apprun-api application status --help
$ skr apprun-api application delete --help
```

認証には選択中の SDK プロファイルを使います。`APP-NAME` に置き換えた名前で既存のアプリケーションが見つかった場合は、そのアプリケーションを変更せず、別の名前を選んでください。

```console
$ skr apprun-api application list --page-num 1 --page-size 100 --query '.data | map({id,name,status,public_url})'
```

対象ページにアプリケーションがない場合は、次のように `[]` が表示されます。これはライブ E2E で確認した出力です。

```json
[]
```

既存のアプリケーションが表示された場合は、`APP-NAME` と一致する名前がないことを確認します。一覧はページ単位です。100件表示される場合は `--page-num` を増やし、100件未満のページまで確認してください。1ページだけの結果でプロジェクト全体の不在を判断しないでください。

一覧取得で API 403（権限エラー）が返る場合は、作成へ進まず、選択中のプロファイルと対象プロジェクトの AppRun 操作権限を確認してください。API エラーは空一覧とは異なります。

## Step 2: 作成リクエストを保護したファイルに保存する

Application の作成リクエストには認証情報や Secret 値を含められるため、`--request` は `@path.json` または標準入力 `-` のみ受け付けます。インライン JSON は利用できません。次の例は公開サンプルイメージを使い、認証情報を含めないリクエストです。ファイルのアクセス権を限定して保存してください。

`APP-NAME` は Step 1 で確認した名前に置き換えます。
`apprun-tutorial` という作業用ディレクトリが既にある場合は、内容を上書きせず、別のディレクトリ名を選んでください。

```console
$ umask 077
$ mkdir -m 700 apprun-tutorial
$ cat > apprun-tutorial/application-create.json <<'JSON'
{
  "name": "APP-NAME",
  "timeout_seconds": 60,
  "port": 8080,
  "min_scale": 0,
  "max_scale": 1,
  "scale_target_concurrency": 100,
  "components": [
    {
      "name": "web",
      "max_cpu": "0.5",
      "max_memory": "1Gi",
      "deploy_source": {
        "container_registry": {
          "image": "ghcr.io/sacloud/apprun-samples/tutorial:latest"
        }
      }
    }
  ]
}
JSON
$ chmod 600 apprun-tutorial/application-create.json
```

このリクエストはアプリケーション全体の最大スケール数を `1` にしています。作成後にスケール数やリソースの上限を変更する場合は、設定変更の影響と料金を確認してください。秘密情報を追加する場合は、保護したファイルまたは標準入力で入力し、コマンド引数、シェル履歴、共有ログに値を残さないでください。`--trace` は HTTP リクエスト内容を出力する場合があるため、秘密情報を含むリクエストでは使用しないでください。

公開サンプルイメージではレジストリのユーザー名とパスワードを指定する必要はありません。コンテナレジストリの作成や、イメージのビルド・プッシュも不要です（[AppRun クイックスタート](https://manual.sakura.ad.jp/cloud/apprun/getting_started.html)）。

## Step 3: アプリケーションを作成して設定を確認する

作成結果の `id`、`name`、`public_url` を控えます。以降の `APP-ID` と `PUBLIC-URL` には、同じ作成結果から取得した値を指定します。出力を共有する場合は、環境固有の値や認証情報を含めないでください。

```console
$ skr apprun-api application create --request @apprun-tutorial/application-create.json
```

作成時点で `status` が `Deploying` でも、作成の失敗ではありません。公開 URL の応答確認は Step 4 で行います。

作成コマンドが失敗したり、応答から ID を取得できなかったりする場合は、作成を繰り返さず、コントロールパネルなどで作成状況を確認してください。同名のアプリケーションが見つかっただけでは今回の作成物と判断せず、ID を安全に特定できるまで削除しません。

作成した ID を使って設定を参照します。

```console
$ skr apprun-api application read APP-ID
```

`id`、`name`、`public_url` が作成結果と一致することを確認します。さらに、`port` が `8080`、`min_scale` が `0`、`max_scale` が `1` であることを照合してください。`components` が1件で、名前が `web`、CPU が `0.5`、メモリが `1Gi`、イメージが指定した公開サンプルと一致することも確認します。異なる場合は操作を止め、原因を確認してください。

一覧でも作成した ID と名前を確認します。

```console
$ skr apprun-api application list --page-num 1 --page-size 100 --query '.data | map({id,name,status,public_url})'
```

次はライブ E2E で確認した一覧を基に、ID、名前、公開 URL をプレースホルダーへ置き換えた例です。この例の一覧には、作成したアプリケーション1件だけが含まれています。

```json
[
  {
    "id": "APP-ID",
    "name": "APP-NAME",
    "public_url": "PUBLIC-URL",
    "status": "Deploying"
  }
]
```

## Step 4: ステータスと公開 URL の応答を確認する

デプロイの反映を待ち、アプリケーションのステータスを確認します。

- `Healthy` になるまで10秒間隔、最大3分を目安に確認してください。
- `Deploying` は反映中の状態です。
- `UnHealthy` は異常な状態です。`message` とアプリケーションの設定を確認してください。

デプロイは通常1〜2分程度で完了します（[AppRun クイックスタート](https://manual.sakura.ad.jp/cloud/apprun/getting_started.html)、[AppRun API / sacloud-sdk-go v0.3.1](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.1/api/apprun)）。

```console
$ skr apprun-api application status APP-ID
```

次はライブ E2E で確認した反映中の応答です。

```json
{
  "status": "Deploying",
  "message": ""
}
```

反映が完了すると、同じコマンドで次の応答を確認できます。これもライブ E2E で確認した出力です。3分経っても `Healthy` にならない場合や API エラーが発生した場合は、成功と扱わず設定と状態を確認してください。この待機時間は本手順の確認期限であり、サービスの反映時間を保証するものではありません。

```json
{
  "status": "Healthy",
  "message": ""
}
```

続いて、作成結果の `public_url` を使って実際の HTTP 応答を確認します。`Healthy` だけで通信の成功を判断しないでください。公開 URL へのリクエストがインスタンスの起動につながる場合があります。

```console
$ curl --fail --silent --show-error --connect-timeout 10 --max-time 60 --write-out '\nHTTP status: %{http_code}\n' 'PUBLIC-URL'
```

成功条件は HTTP ステータス `200` と、本文に `Hello from AppRun!` が含まれることです。次はライブ E2E で確認した本文と HTTP ステータスを、上のコマンドの表示形式にまとめた例です。

```text
Hello from AppRun!
HTTP status: 200
```

異なる応答、接続エラー、タイムアウトがある場合は成功と扱いません。再確認する場合は10秒間隔、最大5分を目安にし、解消しない場合はイメージとポート設定、ステータスを確認してください。途中で失敗した場合も、Step 5 の条件で今回作成した ID と設定を照合し、アプリケーションを後片付けしてください。

## Step 5: 作成したアプリケーションだけを削除する

削除前に ID、名前、ポート、公開 URL、コンテナイメージが Step 3 で確認したアプリケーションと一致することを再確認します。ID を取得できない場合や設定が一致しない場合は削除しないでください。

```console
$ skr apprun-api application read APP-ID
$ skr apprun-api application delete APP-ID
$ skr apprun-api application list --page-num 1 --page-size 100 --query '.data | map({id,name,status,public_url})'
```

削除後の一覧に対象 ID がないことを確認します。次はライブ E2E で確認した出力です。他のアプリケーションがないため、一覧全体が空になっています。

```json
[]
```

既存のアプリケーションがある環境では、一覧全体が空になる必要はありません。今回作成した `APP-ID` がないことを、Step 1 と同じ方法で全ページにわたって確認してください。削除コマンドの空出力や終了コードだけで、不在を確認したものとしないでください。削除・確認でエラーが出た場合は ID を控えたまま状態を調査し、別のリソースを推測で削除しません。

クラウドリソースの後片付けを確認した後、作業用の JSON ファイルとディレクトリを削除します。AppRun の各操作と入力制約は `skr apprun-api` の `--help` で確認できます。

```console
$ rm -- apprun-tutorial/application-create.json
$ rmdir apprun-tutorial
```

## 参考資料

- [AppRun 共用型の基本情報](https://manual.sakura.ad.jp/cloud/apprun/about.html)
- [AppRun 共用型クイックスタート](https://manual.sakura.ad.jp/cloud/apprun/getting_started.html)
- [AppRun 共用型 API](https://manual.sakura.ad.jp/api/cloud/apprun/)
- [AppRun API / sacloud-sdk-go v0.3.1](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.1/api/apprun)
