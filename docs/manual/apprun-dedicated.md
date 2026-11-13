# AppRun 専有型の高レベルコマンド (`skr apprun-dedicated`)

`skr apprun-dedicated` は、AppRun 専有型のアプリケーションを JSON の定義ファイル 1 つからデプロイ・運用し、オートスケーリンググループとクラスタの削除など、インフラライフサイクルの一部を操作する高レベルコマンドです。デプロイやロールバックの反映待ち、作成後の healthy 待ち、状態の確認、定義の差分確認を 1 つのコマンド群で行います。

このページはコマンドの対象、構造、各サブコマンドの挙動と制約を説明するリファレンスです。最初から順に試す手順は [AppRun 専有型: 定義ファイルからアプリケーションをデプロイする](tutorials/apprun-dedicated.md) を参照してください。フラグと使用例の一覧は `--help` で確認できます。

## 対象と対象外

対象は次の 2 つです。

- アプリケーションのライフサイクル (定義からのデプロイ、バージョンの切替、停止、削除)。
- インフラライフサイクルの一部。既存のクラスタに対するオートスケーリンググループの作成、オートスケーリンググループの削除、クラスタの削除です (`create-auto-scaling-group`、`delete-auto-scaling-group`、`delete-cluster`)。

対象外はインフラの中でも、クラスタの作成、ロードバランサ、証明書、ワーカノードの draining、サービスクラスの一覧と選択です。これらは低レベルの `skr apprun-dedicated-api` が提供します。クラスタの作成を含むインフラ構築は、低レベルコマンドのほか Terraform などの別手段で行うこともできます。

## 概念

AppRun 専有型のリソースは次の階層で構成されます。用語の定義は [用語集](https://manual.sakura.ad.jp/cloud/apprun-dedicated/glossary.html) を参照してください。

- **クラスタ** — 最上位の管理単位です。オートスケーリンググループとアプリケーションを含みます。
- **オートスケーリンググループ** — ワーカノードをまとめて管理する単位です。ゾーンとワーカノードのスペックを持ちます。
- **ワーカノード** — アプリケーションのコンテナを動かす専有の仮想サーバです。
- **アプリケーション** — クラスタ内で一意の名前を持ちます。定義ファイルの `cluster` と `name` がこの識別子に対応します。
- **バージョン** — デプロイ内容の履歴単位です。新しいバージョンを作成して有効化することでデプロイが進みます。
- **アクティブバージョン** — アプリケーションで現在有効なバージョンです。存在しない状態 (非活性) ではアプリケーションは停止しています。

1 つのワーカノードには、同一アプリケーションのコンテナが 1 つだけ配置されます。このため、要求したコンテナ数を配置できるワーカノードが足りない場合、コンテナは要求した数まで現れません (「[待機と反映の確認](#待機と反映の確認)」参照)。

## コマンド一覧

`init` 以外のコマンドは、定義ファイルを `--app` で指定します (「[定義ファイル](#定義ファイル)」参照)。各コマンドが組み合わせる低レベル `apprun-dedicated-api` の操作は次のとおりです。

| コマンド | 役割 | 低レベル `apprun-dedicated-api` の操作 |
| --- | --- | --- |
| `init` | デプロイ済みのアプリケーションから定義ファイルを生成する | `list-applications` での検索、`list-application-versions` と `read-application-version` での定義の復元 |
| `deploy` | 定義ファイルの内容をデプロイする (検索または作成、バージョン作成、有効化、反映待ち) | `list-applications`、`create-application`、`create-application-version`、`update-application`、反映待ちの `read-application` と `list-application-containers` |
| `diff` | ローカルの定義とデプロイ済みの現状の差分を unified diff で表示する | 対応する単独の操作はありません (`read-application` と `read-application-version` で現状を取得して比較します) |
| `render` | 定義を検証・正規化した結果を JSON で表示する | 対応する操作はありません (API に接続しません) |
| `status` | アプリケーションの状態を表示する | `read-application` と `read-application-version` |
| `versions` | アプリケーションのバージョンの一覧を表示する | `list-application-versions` |
| `containers` | コンテナの配置を表示する | `list-application-containers` |
| `rollback` | 既存のバージョンを有効化して切り替える | `update-application`、反映待ちの `read-application` と `list-application-containers` |
| `deactivate` | アクティブバージョンを解除してアプリケーションを停止する | `update-application --deactivate`、停止待ちの `read-application` |
| `delete` | アプリケーションとすべてのバージョンを削除する | `update-application --deactivate`、`delete-application`、削除後の不在確認 (`read-application`) |
| `create-auto-scaling-group` | クラスタにオートスケーリンググループを作成する | `list-clusters`、`list-auto-scaling-groups`、`create-auto-scaling-group`、作成後の healthy 待ち |
| `delete-auto-scaling-group` | クラスタのオートスケーリンググループを削除する | `list-clusters`、`list-auto-scaling-groups`、`delete-auto-scaling-group`、削除後の不在確認 |
| `delete-cluster` | クラスタを削除する | `list-clusters`、`list-applications`、`list-auto-scaling-groups`、`delete-cluster`、削除後の不在確認 |

デプロイ作業で毎回判断が必要な操作の組み合わせ (アプリケーションの検索または作成、`registryPasswordAction` の選択、反映待ち) は、高レベルコマンドが内部で行います。

## 定義ファイル

定義は JSON 1 ファイルです。`--app FILE` でパスを指定し、`--app -` で標準入力から読み込みます。インラインの JSON 文字列は受け付けません。

未知のフィールドや末尾の余分なデータはエラーになります。フィールド名の打ち間違いを、API を呼び出す前に確実に検出するための挙動です。

### フィールド

フィールド名は API のリクエストと同じ camelCase です。`cluster` と `name` はデプロイ対象の識別子で、API のリクエストには含まれません。

| フィールド | 型 | 必須 | 説明 |
| --- | --- | --- | --- |
| `cluster` | string | 必須 | デプロイ先クラスタの名前 |
| `name` | string | 必須 | アプリケーション名 |
| `cpu` | integer | deploy / render / diff で必須 | コンテナ 1 つあたりの CPU (単位 mCPU、API の `cpu`) |
| `memory` | integer | deploy / render / diff で必須 | コンテナ 1 つあたりのメモリ (単位 MB、API の `memory`) |
| `scalingMode` | string | deploy / render / diff で必須 | スケーリング方式 (API の `scalingMode`)。`manual` または `cpu` |
| `fixedScale` | integer | `manual` で必須 | 手動スケーリングのコンテナ数 (API の `fixedScale`) |
| `minScale` | integer | `cpu` で必須 | CPU ベーススケーリングの下限 (API の `minScale`) |
| `maxScale` | integer | `cpu` で必須 | CPU ベーススケーリングの上限 (API の `maxScale`) |
| `scaleInThreshold` | integer | 任意 | `cpu` 方式のスケールイン閾値 (単位は %、API の `scaleInThreshold`) |
| `scaleOutThreshold` | integer | 任意 | `cpu` 方式のスケールアウト閾値 (単位は %、API の `scaleOutThreshold`) |
| `image` | string | deploy / render / diff で必須 | コンテナイメージ (API の `image`) |
| `cmd` | array of string | 任意 | 起動コマンド (API の `cmd`) |
| `registryUsername` | string | 任意 | プライベートレジストリのユーザー名 (API の `registryUsername`) |
| `registryPassword` | string | 任意 | プライベートレジストリのパスワード (API の `registryPassword`)。秘密情報を含むため、値は定義ファイルまたは標準入力からのみ入力します |
| `exposedPorts` | array | 任意 | 公開ポートの設定 (API の `exposedPorts`) |
| `env` | array | 任意 | 環境変数 (API の `env`) |

値の範囲 (ポート番号、閾値、CPU とメモリの組み合わせなど) は API が検証します。範囲の詳細は [AppRun 専有型 API ドキュメント](https://manual.sakura.ad.jp/api/cloud/apprun-dedicated/) を参照してください。

`exposedPorts` の要素は次のフィールドを持ちます。

| フィールド | 型 | 説明 |
| --- | --- | --- |
| `targetPort` | integer | コンテナがリッスンするポート (API の `targetPort`) |
| `loadBalancerPort` | integer | ルーティング先のロードバランサのポート (API の `loadBalancerPort`)。未指定のときはロードバランサ経由で公開しません |
| `useLetsEncrypt` | boolean | Let's Encrypt の証明書を利用するか (API の `useLetsEncrypt`) |
| `host` | array of string | ホスト名ルーティングのホスト名 (API の `host`)。`loadBalancerPort` を指定したときは 1〜5 件が必須、指定しないときは指定できません |
| `healthCheck` | object | ヘルスチェックの設定。`path`、`intervalSeconds`、`timeoutSeconds` (API の `healthCheck`) |

`env` の要素は次のフィールドを持ちます。

| フィールド | 型 | 説明 |
| --- | --- | --- |
| `key` | string | 変数名 (API の `key`) |
| `value` | string | 値 (API の `value`) |
| `secret` | boolean | 秘密情報かどうか (API の `secret`)。`true` のとき値は API から取得できず、このバージョンの作成で値を指定しなかった場合は前のバージョンの値が引き継がれます |

### 操作別の必須項目

定義の検証は操作の種類に応じて行われます。

- 参照・停止・削除系 (`status`、`versions`、`containers`、`rollback`、`deactivate`、`delete`) は `cluster` と `name` があれば読み込めます。デプロイ内容を含まない定義でも、これらの操作には使えます。
- デプロイ系 (`deploy`、`render`、`diff`) は、それに加えて `cpu`、`memory`、`scalingMode`、`image` が必須です。`scalingMode` が `manual` のときは `fixedScale` が、`cpu` のときは `minScale` と `maxScale` が必須です。
- `exposedPorts` の要素で `loadBalancerPort` を指定したときは `host` が必須で、指定しないときは `host` を指定できません。この整合は CLI が検証します。

### 定義の例

```json
{
  "cluster": "my-cluster",
  "name": "my-app",
  "cpu": 1000,
  "memory": 1000,
  "scalingMode": "manual",
  "fixedScale": 1,
  "image": "ghcr.io/example/app:v1.0.0",
  "exposedPorts": [
    {
      "targetPort": 8080,
      "loadBalancerPort": 80,
      "host": ["app.example.com"]
    }
  ],
  "env": [
    { "key": "LOG_LEVEL", "value": "info", "secret": false }
  ]
}
```

## 各コマンドの挙動

各節の先頭に構文を示します。構文の `--app=FILE|-` は定義ファイルの指定です (「[定義ファイル](#定義ファイル)」参照)。フラグの詳細は各コマンドの `--help` で確認できます。

### deploy

```
skr apprun-dedicated deploy --app=FILE|-
```

定義ファイルの内容をデプロイします。

1. 定義を検証し、`cluster` の名前でクラスタを検索します。
2. `name` のアプリケーションをクラスタ内で検索し、見つからなければ作成します。
3. 定義から新しいバージョンを作成し、有効化します。
4. 反映待ちを行います (`--no-wait` で省略)。

クラスタとアプリケーションの検索にはページングを含む一覧を使い、同名でも所属クラスタが一致するものだけを対象にします。アプリケーションが見つからない場合は新規作成に進みます。新規作成は deploy だけの挙動で、他のコマンドでは対象が見つからないとエラーになります。

`registryPassword` は、リクエストの `registryPasswordAction` を次のとおりに自動選択して送ります。

| 条件 | 選択される値 | 動作 |
| --- | --- | --- |
| 定義に `registryPassword` の非空値がある | `new` | 新しいパスワードを設定します |
| パスワードがなく、アクティブバージョンがある | `keep` | 既存のパスワードを維持します |
| パスワードがなく、初回デプロイ | `remove` | パスワードを削除します |

定義にパスワードを書かずに済ませたい場合は、パスワードを設定した初回デプロイのあとは `keep` で維持されます。初回デプロイからパスワードなしでデプロイすると `remove` が選択されるため、プライベートレジストリを使うアプリケーションでは、初回にパスワードを設定してください。

### diff

```
skr apprun-dedicated diff --app=FILE|-
```

ローカルの定義とデプロイ済みの現状を比較し、unified diff を標準出力へ出力します。現状はアクティブバージョンの内容を定義と同じ形式に正規化したものです。アプリケーションが存在しない、または非活性の場合は、`cluster` と `name` だけの定義を現状として差分を表示します (初回デプロイで作成される内容の確認に使えます)。

`registryPassword`、`secret=true` の環境変数の値、`registryPasswordAction` は応答で取得できないため比較から除外し、その旨を標準エラー出力に示します。差分がなければ標準出力には何も書きません。

### render

```
skr apprun-dedicated render --app=FILE|-
```

定義を読み込み、検証と正規化を行った結果を JSON で出力します。API には接続しません。`registryPassword` と `secret=true` の環境変数の値は出力に含めません。デプロイ前に定義の構造を確認する用途に使います。

### status

```
skr apprun-dedicated status --app=FILE|-
```

アプリケーションの状態を出力します。アプリケーションの ID、名前、アクティブバージョン、要求されたコンテナ数 (API の `DesiredCount`) と、アクティブバージョンの内容 (イメージ、CPU、メモリ、スケーリング方式、アクティブノード数) を含みます。アクティブバージョンがなければその内容は null になります。`registryPassword` は表示しません。

### versions

```
skr apprun-dedicated versions --app=FILE|-
```

バージョンの一覧を出力します。各要素はバージョン番号、イメージ、アクティブノード数、アクティブかどうか、作成日時 (UTC) を含みます。

### containers

```
skr apprun-dedicated containers --app=FILE|-
```

コンテナの配置を出力します。ノードごとのコンテナの状態を確認できます。deploy の反映待ちがタイムアウトしたときの原因確認にも使います。

### rollback

```
skr apprun-dedicated rollback --app=FILE|-
```

新しいバージョンを作成せず、既存のバージョンを有効化して切り替えます。切り替え後は deploy と同じ反映待ちを行います。

切り替え先のバージョンは、`--target` でバージョン番号を指定できます。省略した場合は次のとおりに選択します。

- 活性 (アクティブバージョンあり) のとき: アクティブバージョンより前のバージョンのうち、番号が最大のもの。
- 非活性のとき: 最新のバージョン。停止したアプリケーションの再開に使えます。

指定したバージョンがすでにアクティブな場合はエラーになります。対象のバージョンが存在しない場合は変更しません。

### deactivate

```
skr apprun-dedicated deactivate --app=FILE|-
```

アクティブバージョンを解除し、コンテナの停止を待ちます。アプリケーションとバージョンは残るため、後から rollback で再開できます。すでに非活性のときは変更せずに成功します。

### delete

```
skr apprun-dedicated delete --app=FILE|-
```

アプリケーションとすべてのバージョンを削除します。アクティブバージョンがあれば先に解除し、停止を待ってから削除します。削除後は API の応答でアプリケーションが不在になったことを確認します。`--no-wait` を指定しても、削除自体と削除後の不在確認は行われます。

### create-auto-scaling-group

```
skr apprun-dedicated create-auto-scaling-group <cluster> <name> \
  --zone=ZONE \
  --worker-service-class-path=PATH \
  --name-servers=IPv4[,IPv4...] \
  --min-nodes=N \
  --max-nodes=N
```

クラスタにオートスケーリンググループを作成し、必要に応じてワーカノードが healthy になるまで待ちます。クラスタ名からクラスタを検索し、同名のグループが既に存在しないことを確認してから作成します。ネットワークインターフェースは共有セグメントに接続する既定の設定を使います。

`--name-servers` は 1 ～ 3 個の IPv4 アドレスをカンマ区切りで指定します。`--min-nodes` と `--max-nodes` は 1 以上で、`--max-nodes` は `--min-nodes` 以上である必要があります。

結果は JSON で出力され、`--query` で加工できます。

### delete-auto-scaling-group

```
skr apprun-dedicated delete-auto-scaling-group <cluster> <name>
```

クラスタのオートスケーリンググループを削除します。削除 API の呼び出しと、削除後の不在確認 (対象が見つからなくなるまでのポーリング) をします。削除は API 側で非同期に完了するため、不在確認が削除完了の確認を兼ね、`--no-wait` を指定しても行われます。実行前に確認を求め、`--force` で省略します。

### delete-cluster

```
skr apprun-dedicated delete-cluster <cluster>
```

クラスタを削除します。削除前に関連リソース (アプリケーション、オートスケーリンググループなど) が残っていないことを確認し、残っている場合は列挙して中断します。削除 API の呼び出しと、削除後の不在確認 (対象が見つからなくなるまでのポーリング) をします。削除は API 側で非同期に完了するため、不在確認が削除完了の確認を兼ね、`--no-wait` を指定しても行われます。実行前に確認を求め、`--force` で省略します。

### init

```
skr apprun-dedicated init <cluster> <application>
```

デプロイ済みのアプリケーションから定義ファイルを生成します。位置引数でクラスタ名とアプリケーション名を指定し、`--output-file` で出力先を指定します (既定は `application.json`)。

アクティブバージョンがあればその内容から、なければ最新のバージョンから定義を復元します。バージョンが 1 つもないアプリケーションでは、`cluster` と `name` だけの定義を生成します。`registryPassword` と `secret=true` の環境変数の値は API から取得できないため、生成される定義には含まれません。同名のファイルが既に存在する場合は上書きせず、エラーになります。

## 待機と反映の確認

`deploy` と `rollback` は、有効化した後、要求されたコンテナ数 (API の `DesiredCount`) と同じ数のコンテナが `running` 状態になるまで待ちます。`deactivate` と `delete` は停止の完了を待ちます。`create-auto-scaling-group` は、作成後にワーカノードが healthy 状態になるまで待ちます。

- `--wait` / `--no-wait` で待機を制御します。既定は待機し、`--no-wait` で省略します。どちらも同一オプションの別名で、後から指定した方が優先されます。
- `--wait-timeout DURATION` で待機の期限を指定します。未指定のときは 5 分です。期限には待機中の API 呼び出しも含まれます。

待機がタイムアウトした場合、有効化などの変更自体は完了していることがあります。タイムアウトの後は `status` や `containers` で状態を確認してください。

タイムアウトの典型的な原因は、要求されたコンテナ数に対してワーカノードが足りないことです。1 つのワーカノードには同一アプリケーションのコンテナが 1 つだけ配置されるため、ノード数が足りないとコンテナは要求した数まで現れず、待機は期限まで進んでタイムアウトします。オートスケーリンググループのワーカノード数と定義のコンテナ数 (`fixedScale`、`maxScale`) を確認し、必要であれば `skr apprun-dedicated-api` でオートスケーリンググループの希望ノード数を増やしてください。

## 確認プロンプトと --force

`rollback`、`deactivate`、`delete`、`delete-auto-scaling-group`、`delete-cluster` は、実行前にクラスタ名、アプリケーション名、ID、現在の状態を表示して確認を求めます。`--force` で省略します。

標準入力が端末でない場合 (パイプ、リダイレクト、CI) や、`--app -` で定義を標準入力から読む場合は、確認の入力が取れないため `--force` が必須です。指定がなければ操作を開始せず、エラーになります。

変更系の操作は、実行直前に API で対象を再照合し、定義の `cluster` と `name`、対象の ID が一致しない場合は変更しません。

## 出力

`deploy`、`render`、`status`、`versions`、`containers`、`rollback`、`deactivate`、`create-auto-scaling-group` は結果を JSON で出力し、グローバルフラグ `--query` で加工できます。進行状況のログは標準エラー出力に書き、標準出力は結果の JSON だけを含みます。

`init` は定義ファイルを書き出し、`diff` は unified diff、`delete`、`delete-auto-scaling-group`、`delete-cluster` は何も出力しないため、この 5 つは `--query` に対応しません。指定した場合はエラーになります。

## 秘密情報の扱い

- `registryPassword` と `secret=true` の環境変数の値は、定義ファイルまたは標準入力 (`--app -`) からのみ入力できます。フラグは設けません。CI で定義ファイルを残したくない場合は標準入力を使えます。
- トレース (`--trace`、または `SAKURA_TRACE_MODE` などの SDK 設定) が有効な場合、`deploy` は実行せずエラーになります。トレース出力にはリクエスト本文が含まれるためです。
- `status` と `versions` の出力に `registryPassword` は含まれません。`secret=true` の環境変数の値は API が返さないため、出力にも現れません。
- 定義ファイルに秘密情報を書く場合は、ファイルの権限に注意してください (例: `chmod 600`)。また、定義ファイルは Git などのバージョン管理へコミットしないでください。

## エラーと対処

| 状況 | 挙動 | 対処 |
| --- | --- | --- |
| 定義の検証エラー (未知のフィールド、必須項目の欠落、`host` と `loadBalancerPort` の整合) | API を呼び出す前にエラーになる | エラーメッセージに定義のフィールド名が示される。定義を修正する |
| クラスタまたはアプリケーションが見つからない | `deploy` は新規作成に進む。それ以外のコマンドはエラーになる | クラスタ名とアプリケーション名、定義の `cluster` と `name` を確認する |
| 対象の再照合で不一致 | 変更 API を呼び出さずにエラーになる | 定義の識別子と実環境の対応を確認する |
| 待機のタイムアウト | エラーになる。変更自体は完了していることがある | `status`、`containers` で状態を確認する。ワーカノード数とコンテナ数を確認する (「待機と反映の確認」参照) |
| トレースが有効な状態での `deploy` | 実行せずにエラーになる | `--trace` を外し、`SAKURA_TRACE_MODE` などの SDK 設定を確認する |
| 確認プロンプトで `--force` なし、非端末の標準入力 | 操作を開始せずにエラーになる | `--force` を指定する |

## 参照先

- [AppRun 専有型の基本情報](https://manual.sakura.ad.jp/cloud/apprun-dedicated/about.html)
- [AppRun 専有型の用語集](https://manual.sakura.ad.jp/cloud/apprun-dedicated/glossary.html)
- [AppRun 専有型のチュートリアル (手順)](tutorials/apprun-dedicated.md)
- 低レベル API コマンド: `skr apprun-dedicated-api --help`
