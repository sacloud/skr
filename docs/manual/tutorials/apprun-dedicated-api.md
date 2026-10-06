# AppRun Dedicated API: アプリケーションをデプロイする

AppRun 専有型は、自動作成される専用ワーカノード上でコンテナをデプロイ・実行するサービスです。クラスタにオートスケーリンググループを作成し、アプリケーションのバージョンを有効化してデプロイします（[AppRun 専有型の概要](https://manual.sakura.ad.jp/cloud/apprun-dedicated/about.html)）。

> [!WARNING]
> AppRun 専有型ではワーカノードのスペック、台数、利用時間に応じた料金が発生します。この手順ではクラスタ、オートスケーリンググループ、ワーカノード、アプリケーションを作成してバージョンをデプロイし、最後にすべて削除します。実行前に対象プロファイルとプロジェクト、サービスクラス、イメージ、料金を確認してください（[料金体系](https://manual.sakura.ad.jp/cloud/apprun-dedicated/about.html#id15)）。

## 前提条件

- `skr` が利用でき、SDK プロファイルまたは `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` が設定されていること。
- クラスタで使用するサービスプリンシパルが作成され、IAM ポリシーで「さくらのクラウドの作成・削除」ロールを付与されていること（[クイックスタート](https://manual.sakura.ad.jp/cloud/apprun-dedicated/getting_started.html)）。
- `is1b` で利用できるワーカサービスクラス、ネームサーバー、ネットワークインターフェース設定が用意されていること。
- レジストリに保存した、AppRun 専有型から取得できるアプリケーションイメージがあること。イメージは `linux/amd64` 向けに用意してください（[よくある質問](https://manual.sakura.ad.jp/cloud/apprun-dedicated/faq.html)）。
- 以下の JSON 例の `SERVICE-PRINCIPAL-ID`、`WORKER-SERVICE-CLASS-PATH`、`NAMESERVER-IP`、`IMAGE-URI` とアプリケーションの CPU・メモリ設定を、対象プロジェクトで有効な値に置き換えること。ID やサービスクラスはユーザー環境により異なります。

## リソースの作成順

1. クラスタを作成します。クラスタの作成リクエストでサービスプリンシパル ID と待受ポートを指定します。
2. クラスタ内にオートスケーリンググループを作成します。ここで `is1b`、ワーカサービスクラス、ノード数、DNS、ネットワークを指定します。
3. クラスタにアプリケーションを登録し、コンテナイメージとアプリケーションリソースを含むバージョンを作成します。
4. バージョンをアクティブにして、アプリケーションの配置を確認します。

## Step 1: 対象環境とコマンドを確認する

```console
$ skr config current
$ skr apprun-dedicated-api --help
$ skr apprun-dedicated-api cluster create --help
$ skr apprun-dedicated-api auto-scaling-group create --help
$ skr apprun-dedicated-api application create --help
$ skr apprun-dedicated-api version create --help
$ skr apprun-dedicated-api application update --help
$ skr apprun-dedicated-api application containers --help
```

表示されたプロファイルが対象プロジェクトのものであることを確認してください。AppRun Dedicated API は SDK プロファイル、または `SAKURA_ACCESS_TOKEN` と `SAKURA_ACCESS_TOKEN_SECRET` で認証します。共通の `--output` で JSON、YAML、table を選べます。

## Step 2: クラスタを作成する

以下を `cluster.json` として保存します。`SERVICE-PRINCIPAL-ID` と `PORT` は対象環境に合わせて置き換えてください。クラスタ作成では JSON リクエストを使用します。

```json
{
  "name": "dedicated-demo",
  "servicePrincipalID": "SERVICE-PRINCIPAL-ID",
  "ports": [
    {
      "port": 443,
      "protocol": "https"
    }
  ]
}
```

この例では HTTPS の待受ポートに `443` を指定しています。作成結果の `clusterID` を後続コマンドで使います。作成結果はクラスタ ID のみを返すため、ID で読み戻して名前を確認してください。

```console
$ skr apprun-dedicated-api cluster create --request @cluster.json --output json > cluster-result.json
$ CLUSTER_ID=$(jq -er '.clusterID' cluster-result.json)
$ skr apprun-dedicated-api cluster read "$CLUSTER_ID" --output json
```

## Step 3: is1b にオートスケーリンググループを作成する

以下を `auto-scaling-group.json` として保存します。`WORKER-SERVICE-CLASS-PATH` と `NAMESERVER-IP` は、プロジェクトと `is1b` で利用できる実際の値に置き換えてください。`minNodes` と `maxNodes` はワーカノード数の下限と上限です。`interfaces` は配列のため JSON で指定します。インターフェース設定は対象ネットワークの構成に合わせて変更してください。

```json
{
  "name": "dedicated-demo",
  "zone": "is1b",
  "nameServers": ["NAMESERVER-IP"],
  "workerServiceClassPath": "WORKER-SERVICE-CLASS-PATH",
  "minNodes": 1,
  "maxNodes": 1,
  "interfaces": [
    {
      "interfaceIndex": 0,
      "upstream": "shared",
      "ipPool": [],
      "connectsToLB": false
    }
  ]
}
```

ネットワーク構成に応じて `upstream`、`ipPool`、その他のインターフェース項目を設定してください。実際に利用するインターフェース定義とワーカサービスクラスを読み戻して確認してください。作成結果の `autoScalingGroupID` を後続の参照と削除に使います。

```console
$ skr apprun-dedicated-api auto-scaling-group create --cluster-id "$CLUSTER_ID" --request @auto-scaling-group.json --output json > group-result.json
$ GROUP_ID=$(jq -er '.autoScalingGroupID' group-result.json)
$ skr apprun-dedicated-api auto-scaling-group read --cluster-id "$CLUSTER_ID" --auto-scaling-group-id "$GROUP_ID" --output json
```

この操作はワーカノードをプロビジョニングします。AppRun 専有型の料金はワーカノードのスペック、台数、利用時間に基づきます。不要なノードを残さないよう、作業後にオートスケーリンググループを削除してください（[料金体系](https://manual.sakura.ad.jp/cloud/apprun-dedicated/about.html#id15)）。

## Step 4: アプリケーションのバージョンを作成する

まずクラスタにアプリケーションを登録し、返された `applicationID` を使います。

```console
$ skr apprun-dedicated-api application create --name dedicated-demo --cluster-id "$CLUSTER_ID" --output json > application-result.json
$ APPLICATION_ID=$(jq -er '.applicationID' application-result.json)
$ skr apprun-dedicated-api application read "$APPLICATION_ID" --output json
```

次の例を `version.json` として保存し、`IMAGE-URI` を対象レジストリのイメージに置き換えます。アプリケーションのバージョン作成にはイメージ、CPU、メモリ、スケーリング設定を含む `version.CreateParams` JSON が必要です。CPU は mCPU、メモリは MiB で指定します。下記は小規模なアプリケーション向けの値です。ワーカプランごとのアプリケーション割り当て可能リソースを確認し、必要に応じて調整してください（[技術概要](https://manual.sakura.ad.jp/cloud/apprun-dedicated/glossary.html)）。

```json
{
  "cpu": 500,
  "memory": 512,
  "scalingMode": "manual",
  "fixedScale": 1,
  "image": "IMAGE-URI",
  "registryPasswordAction": "keep",
  "exposedPorts": [
    {
      "targetPort": 8080
    }
  ]
}
```

レジストリ認証情報を含める場合も、コマンドライン引数に秘密情報を書かず、JSON ファイルのアクセス権を制限してください。`version create` は `@path.json` または標準入力から読み込みます。作成結果の `version` をアクティブにします。

```console
$ chmod 600 version.json
$ skr apprun-dedicated-api version create --application-id "$APPLICATION_ID" --request @version.json --output json > version-result.json
$ APP_VERSION=$(jq -er '.version' version-result.json)
$ skr apprun-dedicated-api version read --application-id "$APPLICATION_ID" "$APP_VERSION" --output json
$ skr apprun-dedicated-api application update "$APPLICATION_ID" --active-version "$APP_VERSION"
```

## Step 5: デプロイ状態を確認する

`application read` の `activeVersion` が指定したバージョン番号であることを確認し、`application containers` でアプリケーションの配置とコンテナ状態を確認します。ワーカノードの準備と配置には時間を要することがあります。

```console
$ skr apprun-dedicated-api application read "$APPLICATION_ID" --output json
$ skr apprun-dedicated-api application containers "$APPLICATION_ID" --output json
```

アプリケーションバージョンを切り替える際、クラスタには新しいコンテナを配置できる空きリソースが必要です。ワーカノードのリソース不足時は、現在のバージョンを非アクティブにしてから新しいバージョンを有効化してください（[技術概要](https://manual.sakura.ad.jp/cloud/apprun-dedicated/glossary.html)）。

## Step 6: 作成したリソースを削除する

作成した `APPLICATION_ID`、`APP_VERSION`、`GROUP_ID`、`CLUSTER_ID` を確認してから、依存関係を逆にたどって削除します。まずアプリケーションを非アクティブにし、続けてバージョン、アプリケーション、オートスケーリンググループ、クラスタを削除します。

```console
$ skr apprun-dedicated-api application update "$APPLICATION_ID" --deactivate
$ skr apprun-dedicated-api version delete --application-id "$APPLICATION_ID" "$APP_VERSION"
$ skr apprun-dedicated-api application delete "$APPLICATION_ID"
$ skr apprun-dedicated-api auto-scaling-group delete --cluster-id "$CLUSTER_ID" --auto-scaling-group-id "$GROUP_ID"
$ skr apprun-dedicated-api cluster delete "$CLUSTER_ID"
$ rm cluster.json auto-scaling-group.json version.json cluster-result.json group-result.json application-result.json version-result.json
```

削除後は AppRun 専有型のコントロールパネルを開き、この手順で作成したクラスタと関連リソースの不在を確認してください（[コントロールパネル操作ガイド](https://manual.sakura.ad.jp/cloud/apprun-dedicated/operation.html)）。

## 参考資料

- [AppRun 専有型の概要と料金体系](https://manual.sakura.ad.jp/cloud/apprun-dedicated/about.html)
- [AppRun 専有型のクイックスタート](https://manual.sakura.ad.jp/cloud/apprun-dedicated/getting_started.html)
- [AppRun 専有型のコントロールパネル操作ガイド](https://manual.sakura.ad.jp/cloud/apprun-dedicated/operation.html)
- [AppRun 専有型の技術概要](https://manual.sakura.ad.jp/cloud/apprun-dedicated/glossary.html)
- [AppRun 専有型のよくある質問](https://manual.sakura.ad.jp/cloud/apprun-dedicated/faq.html)
- [sacloud-sdk-go v0.3.0 AppRun Dedicated API](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.0/api/apprun-dedicated)
