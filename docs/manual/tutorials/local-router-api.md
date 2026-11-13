# Local Router API: スイッチ接続とスタティックルートを設定する

ローカルルータはスイッチに接続して使うルータです。この手順では is1b にスイッチを1台作成し、スイッチ接続、VRRP 用のインターフェース設定、スタティックルートを登録したローカルルータを作成します。作成した設定を参照・更新し、ピアの稼働状態とトラフィックモニタリングの取得を確認したうえで片付けます（[ローカルルータ](https://manual.sakura.ad.jp/cloud/network/localrouter.html)）。

> [!WARNING]
> 選択中のプロジェクトにスイッチとローカルルータを作成し、リソースの削除を行います。実行前に `skr config current` と対象プロジェクトを確認してください。既存のリソースでは試さず、この手順で作成したリソースだけを操作します。

## 前提条件

- `skr` と `jq` を利用できる環境が必要です。
- ローカルルータの操作と、is1b のスイッチの作成・削除が許可された認証プロファイルを用意してください。
- 以下のコマンドは同じシェルで実行します。作成したリソースの ID はコマンド出力から設定してください。

ローカルルータはグローバルリソースのため、`local-router-api` の操作にゾーンを指定しません。接続先スイッチはゾーンリソースのため、スイッチの操作には `--zone is1b` を指定します。

## 作成するリソースと順序

1. is1b にスイッチを1台作成します。
2. スイッチ接続とスタティックルートを登録したローカルルータを作成します。
3. 説明を更新し、接続設定が維持されることを確認します。
4. `update-settings` でスタティックルートとインターフェースを置き換えます。
5. ピアの稼働状態とトラフィックモニタリングを取得します。
6. ローカルルータ、スイッチの順に片付けます。

## Step 1: 対象環境とコマンドを確認する

```console
$ skr config current
$ skr local-router-api --help
$ skr local-router-api create --help
$ skr local-router-api update --help
$ skr local-router-api update-settings --help
$ skr iaas-api switch create --help
```

`local-router-api` には `--zone` がありません。作成するスイッチの操作だけが `--zone is1b` を必要とします。

## Step 2: 接続先スイッチを作成する

重複しない名前で is1b にスイッチを作成し、ID をシェル変数に取り込みます。

```console
$ SWITCH_NAME="skr-tutorial-local-router-switch-$(date +%s)"
$ skr iaas-api switch create --zone is1b \
    --name "$SWITCH_NAME" --description "Temporary switch for local router tutorial" \
    --query '{ID,Name,Zone}'
$ SWITCH_ID=$(skr iaas-api switch find --zone is1b --name "$SWITCH_NAME" \
    --query 'map(select(.Name == "'"$SWITCH_NAME"'")) | sort_by(-.ID) | .[0].ID' | jq -er '.')
$ echo "$SWITCH_ID"
```

出力例は ID と名前を置き換えています。手元では作成したスイッチの ID が表示されることを確認してください。

## Step 3: ローカルルータを作成する

スイッチ接続、インターフェース、スタティックルートを指定して作成します。スイッチのフラグを指定した場合は `--switch-id` が必須で、カテゴリとゾーンの既定値補完はないため `--switch-category cloud` と `--switch-zone-id is1b` も明示します。インターフェースのフラグを指定した場合は 4 項目すべてが必須です。

インターフェースの IP アドレスは、接続するスイッチのネットワーク内で VRRP を構成するための値です。VRRP の構成には仮想 IP と物理 IP 2 つの合計 3 つのアドレスを使うため、`--interface-ip-address` は 2 つの IP アドレスを指定します。カンマ区切りと繰り返し指定のどちらでも受け付けます。この手順ではスイッチにサブネットを接続していないため、プライベートアドレス `192.168.1.0/24` を使います。実際に運用するネットワークに合わせて読み替えてください。

```console
$ LOCAL_ROUTER_NAME="skr-tutorial-local-router-$(date +%s)"
$ skr local-router-api create --name "$LOCAL_ROUTER_NAME" \
    --description "Temporary local router tutorial" \
    --switch-id "$SWITCH_ID" --switch-category cloud --switch-zone-id is1b \
    --interface-virtual-ip-address 192.168.1.1 \
    --interface-ip-address 192.168.1.2,192.168.1.3 \
    --interface-network-mask-len 24 --interface-vrid 100 \
    --static-route 'prefix=192.168.10.0/24,next-hop=192.168.1.254' \
    --query '{ID,Name,Description,Switch,Interface,StaticRoutes}'
```

出力例は ID と名前を置き換えています。

```json
{
  "ID": 123456789005,
  "Name": "skr-tutorial-local-router-EXAMPLE",
  "Description": "Temporary local router tutorial",
  "Switch": {
    "Code": "123456789004",
    "Category": "cloud",
    "ZoneID": "is1b"
  },
  "Interface": {
    "VirtualIPAddress": "192.168.1.1",
    "IPAddress": ["192.168.1.2", "192.168.1.3"],
    "NetworkMaskLen": 24,
    "VRID": 100
  },
  "StaticRoutes": [
    {
      "Prefix": "192.168.10.0/24",
      "NextHop": "192.168.1.254"
    }
  ]
}
```

`--static-route` は `prefix=CIDR,next-hop=IP` 形式で繰り返し指定できます。`prefix` と `next-hop` は必須で、一覧全体の置換です。

ピア設定 (`Peers`) は秘密情報 (`SecretKey`) を含むため個別フラグがなく、`--request` の JSON で指定します。`--request` はインライン JSON を受け付けず、`@path.json` か `-`（標準入力）だけを受け付けます。詳細は Step 6 を参照してください。

作成したローカルルータを名前で検索し、ID をシェル変数に取り込みます。`--name` は部分一致検索です。

```console
$ LOCAL_ROUTER_ID=$(skr local-router-api find --name "$LOCAL_ROUTER_NAME" \
    --query 'map(select(.Name == "'"$LOCAL_ROUTER_NAME"'")) | sort_by(-.ID) | .[0].ID' | jq -er '.')
$ skr local-router-api read --id "$LOCAL_ROUTER_ID" \
    --query '{ID,Name,Switch,Interface,StaticRoutes}'
```

## Step 4: 説明を更新する

説明だけを更新します。Update の未指定の設定は現在値が引き継がれるため、接続設定の再指定は不要です。

```console
$ skr local-router-api update --id "$LOCAL_ROUTER_ID" \
    --description "Updated local router tutorial"
$ skr local-router-api read --id "$LOCAL_ROUTER_ID" \
    --query '{ID,Name,Description,Switch,Interface,StaticRoutes}'
```

Read の結果で、説明が更新され、スイッチ接続とスタティックルートが維持されていることを確認してください。

## Step 5: 接続設定を部分更新する

`update-settings` はスイッチ、インターフェース、スタティックルート、ピアの設定だけを更新します。省略した設定は現在値が引き継がれます。この手順ではスタティックルートを置き換え、インターフェースの VRID を変更します。

```console
$ skr local-router-api update-settings --id "$LOCAL_ROUTER_ID" \
    --switch-id "$SWITCH_ID" --switch-category cloud --switch-zone-id is1b \
    --interface-virtual-ip-address 192.168.1.1 \
    --interface-ip-address 192.168.1.4 --interface-ip-address 192.168.1.5 \
    --interface-network-mask-len 24 --interface-vrid 200 \
    --static-route 'prefix=192.168.20.0/24,next-hop=192.168.1.253' \
    --static-route 'prefix=192.168.21.0/24,next-hop=192.168.1.253' \
    --query '{ID,Name,Switch,Interface,StaticRoutes}'
$ skr local-router-api read --id "$LOCAL_ROUTER_ID" \
    --query '{ID,Switch,Interface,StaticRoutes}'
```

スイッチとインターフェースのフラグは、それぞれのグループ内で設定全体を組み立てます。スイッチ設定を変えずにインターフェースだけを更新する場合は、`--switch-*` を省略してインターフェースの 4 項目をすべて指定できます。逆にスタティックルートだけを変更する場合は、`--static-route` だけを指定します。

`--request` と個別フラグは併用できません。未指定の設定を消す (ピアやスタティックルートの一覧を空にする) には、JSON で空配列を指定します。スイッチ接続そのものの解除は本コマンドの対応範囲外で、生の API 操作が必要です。

## Step 6: ピアを設定する (任意)

ローカルルータ同士をピア接続する場合、`Peers` を JSON で指定します。`SecretKey` は相手側ローカルルータの認証鍵にあたる秘密情報のため、リクエスト JSON は保護したファイルか標準入力から渡します。詳細は `skr local-router-api update-settings --help` を参照してください。

```console
$ umask 077
$ WORK_DIR=$(mktemp -d)
$ cat > "$WORK_DIR/peers-request.json" <<'EOF'
{
  "Peers": [
    {
      "ID": 100,
      "SecretKey": "PEER_SECRET_KEY",
      "Enabled": true,
      "Description": "Peer to 123456789006"
    }
  ]
}
EOF
```

`ID` はピアのルータ ID、`SecretKey` はピア接続の相手と取り決めた認証鍵です。`PEER_SECRET_KEY` は実際の値に置き換えてください。編集が完了したら、`--request` に `@path` 形式で渡して適用します。

```console
$ skr local-router-api update-settings --id "$LOCAL_ROUTER_ID" \
    --request "@$WORK_DIR/peers-request.json" \
    --query '{ID,Name,Peers}'
```

`--request` を指定すると個別フラグは使えません。未指定の設定は現在値が引き継がれます。ピアの一覧を消す場合は `"Peers": []` を指定します。確認が終わったらリクエストファイルを削除します。

```console
$ rm "$WORK_DIR/peers-request.json"
```

## Step 7: 稼働状態とモニタリングを取得する

`health-status` でピアの稼働状態と学習したルートを取得します。ピアを設定していない場合は `Peers` が空配列で返ります。

```console
$ skr local-router-api health-status --id "$LOCAL_ROUTER_ID"
{
  "Peers": []
}
```

`monitor-local-router` でトラフィックのモニタリングを取得します。`--start` と `--end` は RFC3339 形式です。未指定時は現在時刻の1時間前からの範囲です。トラフィックがない期間は空配列で返ります。

```console
$ skr local-router-api monitor-local-router --id "$LOCAL_ROUTER_ID" \
    --start "$(date -u -d '10 minutes ago' +%Y-%m-%dT%H:%M:%SZ)" \
    --end "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
[]
```

## Step 8: 作成したリソースを削除する

途中で失敗した場合も、この手順で作成した ID を確認して後片付けします。まずローカルルータの ID・名前・説明を照合してから削除し、不在を確認します。

```console
$ skr local-router-api read --id "$LOCAL_ROUTER_ID" --query '{ID,Name,Description}'
$ skr local-router-api delete --id "$LOCAL_ROUTER_ID" --fail-if-not-found
$ skr local-router-api find --name "$LOCAL_ROUTER_NAME" --query 'map({ID,Name})'
[]
```

次にスイッチを削除します。対象の ID・名前・説明が今回作成したものか、必ず読み直して確認してください。

```console
$ skr iaas-api switch read --zone is1b --id "$SWITCH_ID" --query '{ID,Name,Description}'
$ skr iaas-api switch delete --zone is1b --id "$SWITCH_ID" --fail-if-not-found
$ skr iaas-api switch find --zone is1b --name "$SWITCH_NAME" --query 'map({ID,Name})'
[]
```

Step 6 で一時ディレクトリを作成した場合は、空になったディレクトリを削除します。

```console
$ rmdir "$WORK_DIR"
```

## 参考資料

- [ローカルルータ](https://manual.sakura.ad.jp/cloud/network/localrouter.html)
- [さくらのクラウド ネットワーク](https://manual.sakura.ad.jp/cloud/network/index.html)
- [sacloud-sdk-go: LocalRouter service](https://github.com/sacloud/sacloud-sdk-go/tree/main/service/iaas/localrouter)
