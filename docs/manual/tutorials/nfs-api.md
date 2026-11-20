# NFS API: スイッチ配下に NFS を作ってライフサイクルを確認する

NFS は、スイッチ配下のローカルネットワークに設置する NFS ファイルサーバアプライアンスです。この手順ではテスト用のスイッチと NFS を作成し、起動の完了を待って内容を照合したあと、説明の更新と電源操作を確認してから削除します ([NFS の概要](https://manual.sakura.ad.jp/cloud/appliance/nfs/index.html))。

> [!WARNING]
> NFS は課金リソースです。作成から削除までの期間に、選んだプランの料金が発生します。料金は [NFS の料金](https://manual.sakura.ad.jp/cloud/appliance/nfs/index.html#nfs-appliance-tariff) を確認してください。この手順では SSD プランの最小サイズを選ぶことで、費用を抑えます。スイッチは無料です。

接続先スイッチ、プラン、IP アドレス、ネットマスク長は作成後の変更ができません。削除はアプライアンスの電源を OFF にしてから行う必要があります。電源が ON のまま削除するときは `--force` で、強制終了と停止待ちを伴う削除をします。

## 前提条件

- `skr` を利用できる環境が必要です。
- NFS とスイッチの操作が許可された認証プロファイルを用意してください。
- 以下のコマンドは同じシェルで実行します。作成したリソースの ID はコマンド出力から設定してください。

## 作成するリソースと順序

1. テスト用のスイッチ (無料) を作成します。
2. `skr nfs plans` でプラン ID を確認します。
3. NFS を作成し、起動の完了を待ちます。
4. 参照と検索で内容を照合します。
5. 説明とタグを更新します。
6. 電源操作 (shutdown、boot) を確認します。
7. NFS を削除し、不在を確認してからスイッチを削除します。

## Step 1: 対象環境とコマンドを確認する

```console
$ skr config current
$ skr nfs-api --help
$ skr nfs-api create --help
$ skr nfs-api delete --help
$ skr nfs plans --help
```

この手順ではゾーンに `tk1a` を使います。別のゾーンを使う場合は、以降の `ZONE` を置き換えてください。

```console
$ ZONE=tk1a
```

## Step 2: 接続先スイッチを作成する

NFS はスイッチへの接続が必須です。テスト用のスイッチを作成します。

```console
$ SWITCH_NAME="skr-tutorial-nfs-switch"
$ skr iaas-api switch create --zone "$ZONE" --name "$SWITCH_NAME" \
    --description "Temporary switch for NFS tutorial" \
    --query '{ID,Name,Description}'
{
  "ID": 112900500001,
  "Name": "skr-tutorial-nfs-switch",
  "Description": "Temporary switch for NFS tutorial"
}
$ SWITCH_ID=$(skr iaas-api switch find --zone "$ZONE" --name "$SWITCH_NAME" --query-raw 'map(select(.Name == "'"$SWITCH_NAME"'")) | sort_by(-.ID) | .[0].ID')
$ echo "$SWITCH_ID"
```

出力例は ID と名前を置き換えています。`SWITCH_ID` は検索結果から取り込むため、手動での置き換えは不要です。

## Step 3: プラン ID を確認する

NFS のプランは、ストレージ種別とディスク容量の組み合わせです。`skr nfs plans` で、ゾーンで利用できるプランの一覧を出力します。デフォルトでは利用可能 (available) なプランのみが表示されます。

```console
$ skr nfs plans --zone "$ZONE" --query 'map(select(.Storage == "SSD")) | map({Size,PlanID})'
[
  {
    "Size": 20,
    "PlanID": 20
  },
  {
    "Size": 100,
    "PlanID": 1001014001
  }
]
```

出力例はプラン ID を置き換えています。表示された一覧から、作成したいサイズのプラン ID を選んで設定します。この手順では費用を抑えるため、一覧の最小サイズのプラン ID をコマンド置換で取り込みます。

```console
$ PLAN_ID=$(skr nfs plans --zone "$ZONE" --query-raw 'map(select(.Storage == "SSD")) | sort_by(.Size) | .[0].PlanID')
$ echo "$PLAN_ID"
20
```

標準 (HDD) プランを確認する場合は、`select(.Storage == "SSD")` を `select(.Storage == "HDD")` に置き換えてください。利用不可 (disabled) のプランを含めて確認する場合は `--all` を付けます。

## Step 4: NFS を作成する

スイッチ配下に NFS を作成します。ローカルネットワークの IPv4 アドレスを 1 つ割り当てます。ネットワーク内で他のリソースと重複しないアドレスを指定してください。

```console
$ NFS_NAME="skr-tutorial-nfs"
$ skr nfs-api create --zone "$ZONE" --name "$NFS_NAME" \
    --description "Temporary NFS for tutorial" \
    --tags "tutorial,managed" \
    --plan-id "$PLAN_ID" \
    --switch-id "$SWITCH_ID" \
    --ip-address 192.168.0.11 \
    --network-mask-len 24 \
    --query '{ID,Name,PlanID,SwitchID,IPAddresses,NetworkMaskLen}'
{
  "ID": 112800500001,
  "Name": "skr-tutorial-nfs",
  "PlanID": 100001,
  "SwitchID": 112900500001,
  "IPAddresses": ["192.168.0.11"],
  "NetworkMaskLen": 24
}
$ NFS_ID=$(skr nfs-api find --zone "$ZONE" --name "$NFS_NAME" --query-raw 'map(select(.Name == "'"$NFS_NAME"'")) | sort_by(-.ID) | .[0].ID')
$ echo "$NFS_ID"
```

出力例は ID を置き換えています。`NFS_ID` は検索結果から取り込むため、手動での置き換えは不要です。作成リクエストは起動の完了を待たずに応答を返します。2 台目の IP アドレスが必要な場合は `--ip-address` を繰り返して指定します。

## Step 5: 起動の完了を待つ

作成した NFS が起動 (`up`) するまで待ちます。アプライアンスの構築には時間がかかるため、ループで状態を確認します。

```console
$ until [ "$(skr nfs-api read --zone "$ZONE" --id "$NFS_ID" --query-raw '.InstanceStatus')" = "up" ]; do
    sleep 30
    echo "waiting for NFS $NFS_ID to become up..."
  done
```

この待ちには時間がかかります。`InstanceStatus` の確認は `--query-raw` で文字列として取り込むため、手動での置き換えは不要です。

## Step 6: 内容を確認する

参照で作成時の設定を照合します。

```console
$ skr nfs-api read --zone "$ZONE" --id "$NFS_ID" \
    --query '{ID,Name,Description,Tags,PlanID,SwitchID,IPAddresses,NetworkMaskLen,InstanceStatus}'
{
  "ID": 112800500001,
  "Name": "skr-tutorial-nfs",
  "Description": "Temporary NFS for tutorial",
  "Tags": ["tutorial", "managed"],
  "PlanID": 100001,
  "SwitchID": 112900500001,
  "IPAddresses": ["192.168.0.11"],
  "NetworkMaskLen": 24,
  "InstanceStatus": "up"
}
```

続いて検索を確認します。`--name` は部分一致、`--tags` はカンマ区切りのタグでの検索です。

```console
$ skr nfs-api find --zone "$ZONE" --name "$NFS_NAME" --query 'map({ID,Name})'
[
  {
    "ID": 112800500001,
    "Name": "skr-tutorial-nfs"
  }
]
$ skr nfs-api find --zone "$ZONE" --tags "tutorial,managed" --query 'map({ID,Name})'
[
  {
    "ID": 112800500001,
    "Name": "skr-tutorial-nfs"
  }
]
```

出力例は ID と名前を置き換えています。

## Step 7: 説明とタグを更新する

説明とタグを更新します。更新リクエストは未指定の項目を空の値として送信するため、保持したい項目もあわせて指定します。

```console
$ skr nfs-api update --zone "$ZONE" --id "$NFS_ID" \
    --name "$NFS_NAME" \
    --description "Updated NFS for tutorial" \
    --tags "tutorial,managed,updated" \
    --query '{ID,Name,Description,Tags}'
{
  "ID": 112800500001,
  "Name": "skr-tutorial-nfs",
  "Description": "Updated NFS for tutorial",
  "Tags": ["tutorial", "managed", "updated"]
}
```

出力例は ID と名前を置き換えています。

## Step 8: 電源操作を確認する

シャットダウンします。デフォルトでは停止の完了を待つため、完了後に次のコマンドへ進みます。電源 OFF を待たずに次の作業へ進む場合は `--no-wait` を指定します。

```console
$ skr nfs-api shutdown --zone "$ZONE" --id "$NFS_ID"
$ skr nfs-api read --zone "$ZONE" --id "$NFS_ID" --query '{ID,Name,InstanceStatus}'
{
  "ID": 112800500001,
  "Name": "skr-tutorial-nfs",
  "InstanceStatus": "down"
}
```

出力例は ID と名前を置き換えています。再度起動します。デフォルトでは起動の完了を待ちます。

```console
$ skr nfs-api boot --zone "$ZONE" --id "$NFS_ID"
$ skr nfs-api read --zone "$ZONE" --id "$NFS_ID" --query '{ID,Name,InstanceStatus}'
{
  "ID": 112800500001,
  "Name": "skr-tutorial-nfs",
  "InstanceStatus": "up"
}
```

出力例は ID と名前を置き換えています。

## Step 9: 削除する

途中で失敗した場合も、この手順で作成した ID を確認して後片付けします。まず ID と名前を照合してから削除します。電源が ON のまま削除するため `--force` を指定します。強制終了と停止待ちのうえ削除されます。

```console
$ skr nfs-api read --zone "$ZONE" --id "$NFS_ID" --query '{ID,Name,Description,SwitchID}'
$ skr nfs-api delete --zone "$ZONE" --id "$NFS_ID" --force --fail-if-not-found
$ skr nfs-api find --zone "$ZONE" --name "$NFS_NAME" --query 'map({ID,Name})'
[]
```

削除後は検索で NFS が残っていないことを確認します。最後にテスト用のスイッチを削除します。

```console
$ skr iaas-api switch delete --zone "$ZONE" --id "$SWITCH_ID" --fail-if-not-found
```

## 参考資料

- [NFS アプライアンス](https://manual.sakura.ad.jp/cloud/appliance/nfs/index.html)
- [API キーの発行](https://manual.sakura.ad.jp/cloud/management/api/api-key.html)
- [sacloud-sdk-go v0.7.0: NFS service](https://github.com/sacloud/sacloud-sdk-go/tree/v0.7.0/service/iaas/nfs)
