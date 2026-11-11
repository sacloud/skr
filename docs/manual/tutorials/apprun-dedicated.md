# AppRun 専有型: 定義ファイルからアプリケーションをデプロイする

AppRun 専有型は、専用の仮想サーバ (ワーカノード) 上でコンテナをデプロイ・実行する、専有型のコンテナオーケストレーションサービスです。他の利用者とリソースを共有しないため、セキュリティや性能の分離が求められるユースケースに向いています（[AppRun 専有型の基本情報](https://manual.sakura.ad.jp/cloud/apprun-dedicated/about.html)）。

skr は 2 つの階層のコマンドを提供します。

- `skr apprun-dedicated` — JSON の定義ファイル 1 つからアプリケーションのデプロイと運用をする高レベルコマンドです。クラスタとアプリケーションの検索、バージョンの作成と有効化、反映待ちを 1 つのコマンドで行います。
- `skr apprun-dedicated-api` — AppRun 専有型の API 操作を 1 対 1 で実行する低レベルコマンドです。クラスタやオートスケーリンググループなどのインフラ構築と、個別の操作に使います。

高レベルコマンドが行う操作と、低レベルコマンドの対応は次のとおりです。

| 高レベルコマンド | 低レベルコマンドの組み合わせ |
| --- | --- |
| `skr apprun-dedicated deploy` | `create-application` + `create-application-version` + `update-application` (+ 反映待ちの `list-application-containers` 確認) |
| `skr apprun-dedicated rollback` | `update-application --active-version` (+ 反映待ち) |
| `skr apprun-dedicated deactivate` | `update-application --deactivate` (+ 停止待ち) |
| `skr apprun-dedicated delete` | `update-application --deactivate` + `delete-application` (+ 停止待ちと不在確認) |
| `skr apprun-dedicated status` | `read-application` + `read-application-version` |
| `skr apprun-dedicated versions` | `list-application-versions` |
| `skr apprun-dedicated containers` | `list-application-containers` |
| `skr apprun-dedicated init` | アプリケーションとバージョンの参照から定義を復元 |
| `skr apprun-dedicated diff` / `render` | 対応する単独の API 操作はありません |

このチュートリアルでは、低レベルコマンドでクラスタとオートスケーリンググループを作成し、高レベルコマンドでアプリケーションをデプロイ・更新・ロールバック・削除します。

> [!WARNING]
> この手順ではクラスタ、オートスケーリンググループ (ワーカノード 1 台)、アプリケーションを作成します。AppRun 専有型の料金はワーカノードのスペックと台数、稼働時間に応じて発生します（[料金体系](https://manual.sakura.ad.jp/cloud/apprun-dedicated/about.html)）。実行前に `skr config current` で対象プロファイルとプロジェクトを確認し、テスト専用の名前を使ってください。作成したリソースはクリーンアップの手順で削除します。

## 前提条件

- `skr` が利用でき、`skr apprun-dedicated` と `skr apprun-dedicated-api` の両方が使えること。
- SDK プロファイル、または `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` で認証が設定済みであること。
- クラスタの作成にはサービスプリンシパル ID が必要です。[クイックスタート](https://manual.sakura.ad.jp/cloud/apprun-dedicated/getting_started.html)の「サービスプリンシパルの設定」に従い、サービスプリンシパルを作成し、IAM ポリシーのロールには「さくらのクラウド」の「作成・削除」を付与してください。その ID を `SERVICE-PRINCIPAL-ID` として使います。
- ID の取り込みに `jq` を利用します。事前に導入してください。
- 名前の置き換え: `CLUSTER-NAME`、`ASG-NAME`、`APP-NAME` は英数字・アンダースコア・ハイフンだけで構成した短い名前に置き換えます (例: `skr-tutorial`、`skr-tutorial-a`、`skr-tutorial-app`)。名前の構成文字と長さは API が検証します。クラスタ名はプロジェクト内で、グループ名はクラスタ内で、アプリケーション名はクラスタ内で一意にします。
- `ZONE` はワーカノードを作成するゾーンに置き換えます (例: `tk1a`)。
- サンプルアプリケーション `ghcr.io/sacloud/apprun-samples/tutorial:latest` を使います。このイメージはポート `8080` で応答します（[AppRun クイックスタート](https://manual.sakura.ad.jp/cloud/apprun/getting_started.html)）。公開イメージのためレジストリ認証は不要です。

## 作成するリソースと順序

1. **クラスタ** — AppRun 専有型の最上位の管理単位です。サービスプリンシパル ID を指定して作成します。
2. **オートスケーリンググループ** — ワーカノードをまとめて管理する単位です。ゾーンとワーカノードのスペックを指定してクラスタに作成します。
3. **ワーカノード** — グループの作成後に自動で起動します。`healthy` になるのを待ちます。
4. **アプリケーションとバージョン** — 定義ファイルから高レベルの `deploy` で作成し、コンテナの起動まで待ちます。
5. **更新とロールバック** — 定義を変更して新しいバージョンをデプロイし、前のバージョンへ戻します。

## Step 1: 対象環境とコマンドを確認する

```console
$ skr config current
$ skr apprun-dedicated --help
$ skr apprun-dedicated deploy --help
$ skr apprun-dedicated-api create-cluster --help
$ skr apprun-dedicated-api create-auto-scaling-group --help
```

意図したプロファイルとプロジェクトを確認してください。作成前に、使う名前と同じ名前のリソースがないことを確認します。

```console
$ skr apprun-dedicated-api list-clusters --query '[.items[] | {clusterID, name}]'
[]
```

既存のクラスタが表示された場合は、作成予定の `CLUSTER-NAME` と一致する名前がないことを確認してください。一覧はページ単位のため、結果が 1 ページに収まらない場合は `--cursor` で続きを確認してください。

## Step 2: クラスタを作成する

クラスタの作成リクエストを JSON ファイルとして用意します。`SERVICE-PRINCIPAL-ID` は前提条件で確認した ID に置き換えます。`ports` はクラスタロードバランサの待受ポート、`letsEncryptEmail` は Let's Encrypt の利用設定です。このチュートリアルではどちらも使わないため、`ports` を空配列にします（[クイックスタート](https://manual.sakura.ad.jp/cloud/apprun-dedicated/getting_started.html)）。待受ポートや Let's Encrypt を利用する場合の設定は、該当ページを参照してください。

```json
{
  "name": "CLUSTER-NAME",
  "servicePrincipalID": "SERVICE-PRINCIPAL-ID",
  "ports": []
}
```

```console
$ CLUSTER_ID=$(skr apprun-dedicated-api create-cluster --request @cluster.json --query '.clusterID' | jq -er '.')
$ skr apprun-dedicated-api read-cluster "$CLUSTER_ID" --query '{clusterID, name, servicePrincipalID, hasLetsEncryptEmail}'
```

参照結果に指定した名前とサービスプリンシパル ID が表示されることを確認します。`hasLetsEncryptEmail` は `false` です。以降の手順で使う `CLUSTER_ID` はコマンド出力から取り込んだ値です (`--query` は JSON で出力するため、文字列は `jq -er '.'` でデコードしてから変数に入れます。以降も同じ手順です)。

## Step 3: オートスケーリンググループを作成し、ワーカノードの起動を待つ

グループの作成リクエストを JSON ファイルとして用意します。`workerServiceClassPath` はワーカノードのスペック (CPU とメモリの組み合わせ) を示す値で、次のコマンドで選べる構成を確認します。このチュートリアルでは `1vcpu_2gb` を使い、ノード数を 1 台に固定します。インターフェースは共有セグメント (`upstream: "shared"`) に接続します。これらは[クイックスタート](https://manual.sakura.ad.jp/cloud/apprun-dedicated/getting_started.html)の構成と同じです。

```console
$ skr apprun-dedicated-api list-worker-service-classes --query 'map({path, name})'
```

以下は出力形式の例で、実際に選べる構成はコマンドの出力で確認してください。

```json
[
  {
    "path": "cloud/apprun/dedicated/worker/1vcpu_2gb",
    "name": "ワーカサービスクラスの名前"
  }
]
```

`ASG-NAME` と `ZONE` を置き換えて `asg.json` を保存します。

```json
{
  "name": "ASG-NAME",
  "zone": "ZONE",
  "workerServiceClassPath": "cloud/apprun/dedicated/worker/1vcpu_2gb",
  "minNodes": 1,
  "maxNodes": 1,
  "interfaces": [
    {
      "interfaceIndex": 0,
      "upstream": "shared",
      "connectsToLB": false
    }
  ]
}
```

`interfaceIndex` は 0 が必須です。`connectsToLB` はクラスタのロードバランサへインターフェースを接続する設定で、このチュートリアルでは使わないため `false` にします。

```console
$ ASG_ID=$(skr apprun-dedicated-api create-auto-scaling-group --cluster-id "$CLUSTER_ID" --request @asg.json --query '.autoScalingGroupID' | jq -er '.')
$ skr apprun-dedicated-api read-auto-scaling-group --cluster-id "$CLUSTER_ID" --auto-scaling-group-id "$ASG_ID" --query '{autoScalingGroupID, name, zone, workerServiceClassPath, minNodes, maxNodes}'
```

参照結果に指定した名前、ゾーン、サービスクラス、ノード数が表示されることを確認します。

グループの作成後にワーカノードが自動で起動します。ノードの状態は次のコマンドで確認します。`status` が `healthy` になるまで繰り返し実行して待ってください。起動には時間がかかります。

```console
$ skr apprun-dedicated-api list-worker-nodes --cluster-id "$CLUSTER_ID" --auto-scaling-group-id "$ASG_ID" --query '[.items[] | {status, createErrorMessage}]'
```

以下は出力形式の例です。`createErrorMessage` が設定されている場合はノードの作成に失敗しています。

```json
[
  {
    "status": "healthy",
    "createErrorMessage": null
  }
]
```

## Step 4: 定義ファイルを用意してデプロイする

高レベルコマンドは、定義ファイル 1 つからアプリケーションの作成、バージョンの作成と有効化、反映待ちまでを行います。定義の `cluster` と `name` はデプロイ対象の識別子で、API には送られません。`cpu` は mCPU、`memory` は MB です。`exposedPorts` の `targetPort` はコンテナがリッスンするポートです。`loadBalancerPort` を指定しない場合、そのポートはロードバランサ経由では公開されません。次の内容を `apprun-dedicated.json` として保存し、`cluster` と `name` を実際の値に置き換えてください。

```json
{
  "cluster": "CLUSTER-NAME",
  "name": "APP-NAME",
  "cpu": 100,
  "memory": 128,
  "scalingMode": "manual",
  "fixedScale": 1,
  "image": "ghcr.io/sacloud/apprun-samples/tutorial:latest",
  "exposedPorts": [
    {
      "targetPort": 8080
    }
  ]
}
```

`scalingMode` は `manual` (固定数) と `cpu` (CPU 使用率によるオートスケーリング) を選べます。`manual` では `fixedScale`、`cpu` では `minScale` と `maxScale` が必須です。コンテナイメージの更新や設定の変更は、バージョンとして積まれます。同時にアクティブにできるバージョンは 1 つです（[技術概要](https://manual.sakura.ad.jp/cloud/apprun-dedicated/glossary.html)）。

プライベートレジストリのイメージを使う場合は、定義に `registryUsername` と `registryPassword` を追加します。秘密情報を含む定義ファイルは保護したファイルに保存してください (`umask 077` や `chmod 600` でアクセス権を限定し、リポジトリにコミットしません)。CI などでは `--app -` で標準入力から渡せます。

デプロイの前に、正規化した定義を `render` で確認できます。

```console
$ skr apprun-dedicated render --app apprun-dedicated.json
```

デプロイします。有効化後、このバージョンのコンテナがすべて `running` になるまで待ってから戻ります (`--no-wait` で待機を省略、`--wait-timeout` で期限を指定。既定は 5 分です)。期限を超えた場合も、有効化自体は完了していることがあるため、`status` と `containers` で状態を確認してください。

```console
$ skr apprun-dedicated deploy --app apprun-dedicated.json
```

以下は出力形式の例で、ID は置き換えています。`applicationCreated` が `true` は、この実行でアプリケーションが新規作成されたことを示します。

```json
{
  "cluster": "CLUSTER-NAME",
  "application": "APP-NAME",
  "applicationID": "01a1cb07-1234-75ed-82ff-664020ee6b98",
  "version": 1,
  "applicationCreated": true
}
```

後の手順で使うアプリケーション ID を変数に取り込みます。

```console
$ APP_ID=$(skr apprun-dedicated status --app apprun-dedicated.json --query '.application.applicationID' | jq -er '.')
```

## Step 5: 動作を確認する

`status` でアプリケーションの状態とアクティブバージョンの内容を確認します。

```console
$ skr apprun-dedicated status --app apprun-dedicated.json
```

以下は出力形式の例です。`activeVersion` が 1、`desiredCount` が 1、イメージと CPU、メモリが定義と一致することを確認してください。

```json
{
  "cluster": "CLUSTER-NAME",
  "application": {
    "applicationID": "01a1cb07-1234-75ed-82ff-664020ee6b98",
    "name": "APP-NAME",
    "activeVersion": 1,
    "desiredCount": 1
  },
  "activeVersionDetail": {
    "version": 1,
    "image": "ghcr.io/sacloud/apprun-samples/tutorial:latest",
    "cpu": 100,
    "memory": 128,
    "scalingMode": "manual",
    "activeNodeCount": 1
  }
}
```

`containers` でノードごとのコンテナの状態を確認します。

```console
$ skr apprun-dedicated containers --app apprun-dedicated.json --query 'map({nodeID, states: [.containersStats.containers[].state]})'
```

以下は出力形式の例です。

```json
[
  {
    "nodeID": "01a1cb07-5678-9abc-def0-1234567890ab",
    "states": [
      "running"
    ]
  }
]
```

ワーカノードの IP アドレスを取得し、サンプルアプリケーションにアクセスします。この構成ではワーカノードが共有セグメントに接続され、パケットフィルタを設定していないため、アプリケーションのポートへ外部から直接アクセスできます。[クイックスタート](https://manual.sakura.ad.jp/cloud/apprun-dedicated/getting_started.html)の動作確認も同じ方法です。実際のアプリケーションでは、パケットフィルタを設定して意図しないアクセスを防いでください。

```console
$ NODE_IP=$(skr apprun-dedicated-api list-worker-nodes --cluster-id "$CLUSTER_ID" --auto-scaling-group-id "$ASG_ID" --query '.items[0].networkInterfaces[0].addresses[0].address' | jq -er '.')
$ curl -i "http://$NODE_IP:8080/"
```

アプリケーションが HTTP で応答することを確認してください。確認が終わったら次の手順へ進み、作成したリソースを速やかに削除します。

## Step 6: 定義を更新してロールバックする

定義ファイルに環境変数を追加して保存します。`secret` を `true` にした環境変数の値は API から取得できず、値を指定しない場合は前のバージョンの値が引き継がれます。

```json
  "env": [
    {
      "key": "GREETING",
      "value": "hello",
      "secret": false
    }
  ],
```

`diff` でローカルの定義とデプロイ済みの現状の差分を確認します。

```console
$ skr apprun-dedicated diff --app apprun-dedicated.json
```

以下は出力の例です (hunk の行番号は省略しています)。`registryPassword`、`secret=true` の環境変数の値、`registryPasswordAction` は応答で取得できないため比較から除外され、その旨が標準エラー出力に表示されます。

```diff
--- 定義
+++ 現状
@@
   "cluster": "CLUSTER-NAME",
   "cpu": 100,
+  "env": [
+    {
+      "key": "GREETING",
+      "value": "hello",
+      "secret": false
+    }
+  ],
   "exposedPorts": [
```

再び `deploy` を実行します。今度はアプリケーションが既に存在するため、新しいバージョン (バージョン 2) の作成と有効化だけが行われます。

```console
$ skr apprun-dedicated deploy --app apprun-dedicated.json
```

`versions` でバージョンの一覧を確認します。バージョン 2 が `active: true` になります。

```console
$ skr apprun-dedicated versions --app apprun-dedicated.json
```

以下は出力形式の例です。

```json
[
  {
    "version": 2,
    "image": "ghcr.io/sacloud/apprun-samples/tutorial:latest",
    "activeNodeCount": 1,
    "active": true,
    "created": "2026-11-12T01:00:00Z"
  },
  {
    "version": 1,
    "image": "ghcr.io/sacloud/apprun-samples/tutorial:latest",
    "activeNodeCount": 1,
    "active": false,
    "created": "2026-11-12T00:00:00Z"
  }
]
```

`rollback` で前のバージョンへ戻します。既定の切り替え先は、アクティブバージョンより前のバージョンのうち最大のものです (`--target` でバージョン番号を指定できます)。実行すると対象のクラスタ、アプリケーション、バージョンが表示され、確認を求められます。`y` で続行してください。標準入力が端末でない場合や `--app -` の場合は `--force` が必須です。

```console
$ skr apprun-dedicated rollback --app apprun-dedicated.json
```

`status` で `activeVersion` が 1 に戻ったことを確認します。

## Step 7: アプリケーションを停止・削除する

`deactivate` でアクティブバージョンを解除し、コンテナを停止します。確認プロンプトに対しては `y` で続行してください (非対話環境では `--force`)。

```console
$ skr apprun-dedicated deactivate --app apprun-dedicated.json
```

以下は出力形式の例です。`deactivatedVersion` は解除したバージョン番号です。

```json
{
  "cluster": "CLUSTER-NAME",
  "application": "APP-NAME",
  "applicationID": "01a1cb07-1234-75ed-82ff-664020ee6b98",
  "deactivatedVersion": 1
}
```

`status` で `activeVersion` と `activeVersionDetail` が `null` になったことを確認します。

```console
$ skr apprun-dedicated status --app apprun-dedicated.json --query '{activeVersion: .application.activeVersion, detail: .activeVersionDetail}'
```

`delete` でアプリケーションとすべてのバージョンを削除します。アクティブなバージョンがあれば解除と停止を自動で行い、削除後にアプリケーションが存在しないことを確認してから戻ります。確認プロンプトに対しては `y` で続行してください。このコマンドは標準出力に何も出力しません (`--query` には対応していません)。

```console
$ skr apprun-dedicated delete --app apprun-dedicated.json
$ skr apprun-dedicated-api list-applications --query '[.items[] | select(.name == "APP-NAME") | .applicationID]'
[]
```

## Step 8: クラスタとオートスケーリンググループを削除する

オートスケーリンググループを削除します。削除の前に `read` で ID と名前、ゾーンがこの手順で作成したものと一致することを確認してください。一致しない場合は削除を中止してください。削除は非同期で、ワーカノードの削除も伴います。一覧から消えるまで繰り返し確認してください (`deleting` が `true` の間は削除中です)。

```console
$ skr apprun-dedicated-api read-auto-scaling-group --cluster-id "$CLUSTER_ID" --auto-scaling-group-id "$ASG_ID" --query '{autoScalingGroupID, name, zone}'
$ skr apprun-dedicated-api delete-auto-scaling-group --cluster-id "$CLUSTER_ID" --auto-scaling-group-id "$ASG_ID"
$ skr apprun-dedicated-api list-auto-scaling-groups --cluster-id "$CLUSTER_ID" --query '[.items[] | {autoScalingGroupID, name}]'
[]
```

続けてクラスタを削除します。関連リソース (アプリケーション、オートスケーリンググループ) がないことを確認してから実行してください。

```console
$ skr apprun-dedicated-api read-cluster "$CLUSTER_ID" --query '{clusterID, name}'
$ skr apprun-dedicated-api delete-cluster "$CLUSTER_ID"
$ skr apprun-dedicated-api list-clusters --query '[.items[] | {clusterID, name}]'
[]
```

## 補足: 低レベルコマンドだけでの操作

高レベルコマンドが行うデプロイは、低レベルコマンドでは次の手順に相当します。個別の操作ごとに実行結果を制御したい場合に使ってください。

```console
$ APP_ID=$(skr apprun-dedicated-api create-application --name APP-NAME --cluster-id "$CLUSTER_ID" --query '.applicationID' | jq -er '.')
```

バージョンの作成リクエストには `registryPasswordAction` が必要です。新しいレジストリパスワードを設定する場合は `new` (リクエストに `registryPassword` を含める)、アクティブなバージョンがある状態で既存のパスワードを維持する場合は `keep`、それ以外は `remove` を指定します。公開イメージでレジストリ認証を使わない場合は `remove` です。`registryUsername`、`registryPassword`、`loadBalancerPort`、`healthCheck` は、使わない場合もリクエストの必須項目のため `null` を指定します。`registryPassword` や `secret=true` の環境変数の値など秘密情報を含むリクエストは、`@path.json` または標準入力 (`-`) からのみ指定でき、インラインの JSON は拒否されます。

```json
{
  "image": "ghcr.io/sacloud/apprun-samples/tutorial:latest",
  "cpu": 100,
  "memory": 128,
  "scalingMode": "manual",
  "fixedScale": 1,
  "registryUsername": null,
  "registryPassword": null,
  "registryPasswordAction": "remove",
  "exposedPorts": [
    {
      "targetPort": 8080,
      "loadBalancerPort": null,
      "useLetsEncrypt": false,
      "host": [],
      "healthCheck": null
    }
  ],
  "env": []
}
```

```console
$ skr apprun-dedicated-api create-application-version --application-id "$APP_ID" --request @version.json --query '.version'
$ skr apprun-dedicated-api update-application "$APP_ID" --active-version 1
```

有効化の解除は `update-application --deactivate` です。`--active-version` とは併用できません。反映待ちの判定は `list-application-containers` のコンテナの状態を繰り返し確認して行います。高レベルコマンドはこの待機を自動で行います。

```console
$ skr apprun-dedicated-api update-application "$APP_ID" --deactivate
```

## まとめ

低レベルコマンドでクラスタとオートスケーリンググループを構築し、高レベルコマンドで定義ファイル 1 つからアプリケーションをデプロイ、更新、ロールバック、削除しました。定義の変更は `diff` で確認し、`render` で正規化した内容を確認できます。`registryPassword` などの秘密情報はコマンド引数に含めず、定義ファイルまたは標準入力から渡します。

ロードバランサの待受ポート、Let's Encrypt、証明書、ワーカノードの draining、CPU 使用率によるオートスケーリング (`scalingMode: "cpu"`) はこのチュートリアルでは扱っていません。低レベルコマンドの `--help` と[技術概要](https://manual.sakura.ad.jp/cloud/apprun-dedicated/glossary.html)を参照してください。

## 参考資料

- [AppRun 専有型の基本情報](https://manual.sakura.ad.jp/cloud/apprun-dedicated/about.html)
- [AppRun 専有型 クイックスタート](https://manual.sakura.ad.jp/cloud/apprun-dedicated/getting_started.html)
- [AppRun 専有型 技術概要](https://manual.sakura.ad.jp/cloud/apprun-dedicated/glossary.html)
- [AppRun 専有型 API ドキュメント](https://manual.sakura.ad.jp/api/cloud/apprun-dedicated/)
- [AppRun クイックスタート (サンプルアプリケーション)](https://manual.sakura.ad.jp/cloud/apprun/getting_started.html)
- [sacloud-sdk-go v0.5.0: AppRun Dedicated API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.5.0/api/apprun-dedicated)
- [sacloud-sdk-go v0.5.0: apprun-dedicated service](https://pkg.go.dev/github.com/sacloud/apprun-dedicated-service-go)
