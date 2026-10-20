# GSLB API: ping 監視による DNS 応答先の停止・復帰を試す

GSLB は、実サーバーのヘルスチェック結果に応じて DNS 応答先を制御します。この手順では is1b に実サーバーとソーリーサーバーを各1台作成し、実サーバーを ping で監視します。停止するとソーリーサーバーへ応答先が切り替わり、再起動すると実サーバーへ戻ることを確認します（[GSLB の利用手順](https://manual.sakura.ad.jp/cloud/appliance/gslb/index.html)）。

全実サーバーが監視エラーになった場合、ソーリーサーバーを設定していなければ全実サーバーの IP を応答します。設定済みの場合はソーリーサーバーの IP を応答します。この手順では後者を確認します（[GSLB FAQ](https://manual.sakura.ad.jp/cloud/support/technical/appliance-gslb.html)）。

> [!WARNING]
> 選択中のプロジェクトに GSLB・サーバー・ディスクを作成し、サーバーの停止・再起動とリソースの削除を行います。実行前に `skr config current` と対象プロジェクト、各リソースの料金を確認してください。既存の業務用サーバーでは試さず、この手順で作成したリソースだけを操作します。

## 前提条件

- `skr`、`ssh`、`ssh-keygen`、`dig` を利用できる環境が必要です。
- GSLB の操作と、is1b のアーカイブ・ディスクプランの参照、サーバー・ディスクの作成・起動・削除が許可された認証プロファイルを用意してください。
- 作業端末から一時サーバーへの SSH 接続と、GSLB からの ping を許可してください。共有セグメントに接続するサーバーはインターネットから到達可能になります。
- 以下のコマンドは同じシェルで実行します。`CLOUDIMG_ARCHIVE_ID` など代入の右辺にあるプレースホルダーは、実際の値に置き換えてください。

独自ドメインは不要です。GSLB が生成する FQDN に直接問い合わせるため、この手順では DNS CNAME を作成・変更しません。

## 作成するリソースと順序

1. cloud-init 対応 Ubuntu アーカイブを使い、共有セグメントに接続する一時サーバーとディスクを各2個作成します。両サーバーを起動します。
2. 実サーバーの IP を宛先、もう1台の IP をソーリーサーバーに指定して GSLB を作成します。
3. 実サーバーの稼働中・停止中・再起動後の DNS 応答を比較します。ソーリーサーバーは起動したままにします。
4. GSLB、各サーバー、各ディスク、ローカルの SSH 鍵の順に片付けます。

## Step 1: 対象環境とコマンドを確認する

```console
$ skr config current
$ skr gslb-api --help
$ skr gslb-api create --help
$ skr gslb-api update --help
$ skr gslb-api delete --help
$ skr iaas-api archive find --help
$ skr iaas-api disk-plan read --help
$ skr iaas-api server create --help
$ skr iaas-api server boot --help
$ skr iaas-api server delete --help
```

GSLB 自体にはゾーンを指定しません。サーバー、ディスク、アーカイブの操作には `--zone is1b` を指定します。

## Step 2: cloud-init 対応アーカイブとディスクプランを選ぶ

`cloud-init` タグで絞り、Ubuntu Server 24.04 の名前に `(cloudimg)` が付いたアーカイブを選びます。通常の Ubuntu アーカイブと cloudimg は別のアーカイブです。`--os-type ubuntu2404` だけで選ばず、必ず `Tags` と名前を確認してください（[cloud-init 対応アーカイブ](https://manual.sakura.ad.jp/cloud/server/cloud-init.html)）。

```console
$ skr iaas-api archive find --zone is1b --tags cloud-init --count 100 \
    --query 'map(select(.Name | contains("Ubuntu Server 24.04")) | {ID,Name,SizeMB})'
[
  {
    "ID": 123456789001,
    "Name": "Ubuntu Server 24.04.2 LTS 64bit (cloudimg)",
    "SizeMB": 20480
  }
]
$ CLOUDIMG_ARCHIVE_ID=CLOUDIMG_ARCHIVE_ID
$ skr iaas-api archive read --zone is1b --id "$CLOUDIMG_ARCHIVE_ID" \
    --query '{ID,Name,SizeMB,Tags}'
```

出力例は検索結果から1件を抜粋し、ID を置き換えています。アーカイブの提供状況は変わるため、実際の検索結果から選んでください。選択したアーカイブに `cloud-init` タグがなければ、先へ進まないでください。

次に SSD プラン ID 4 の利用可否とサイズを確認します。この手順では 20 GB を使います。プランと `Size` 内の 20 GB の `Availability` が `available` であることを確認してください。アーカイブの `SizeMB` が 20480 を超える場合は、より大きい利用可能なサイズを選びます（[ディスクのプランとサイズ](https://manual.sakura.ad.jp/cloud/storage/disk.html)）。

```console
$ skr iaas-api disk-plan read --zone is1b --id 4 \
    --query '{ID,Name,Availability,Size}'
$ DISK_PLAN_ID=4
$ DISK_SIZE_GB=20
```

## Step 3: SSH 鍵と cloud-init 設定を準備する

一時ディレクトリに使い捨ての SSH 鍵と user-data を作成します。秘密鍵の内容は引数や API リクエストに含めず、公開鍵だけを user-data に書き込みます。

```console
$ umask 077
$ WORK_DIR=$(mktemp -d)
$ KEY_PATH="$WORK_DIR/gslb-key"
$ USER_DATA_PATH="$WORK_DIR/gslb-c0nfig.yaml"
$ ssh-keygen -t rsa -b 3072 -N '' -f "$KEY_PATH"
$ cat > "$USER_DATA_PATH" <<EOF
#cloud-config
users:
  - default
  - name: skr-tutorial
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    ssh_authorized_keys:
      - $(cat "$KEY_PATH.pub")
EOF
```

`skr-tutorial` には、後でサーバーを停止するための sudo 権限を設定します。一時サーバー専用の設定です。

## Step 4: 一時サーバーを作成して起動する

重複しない名前を設定し、1 vCPU／1 GB メモリ、ディスク1個、共有 NIC 1個で作成します。作成時には起動せず、`boot` で user-data を渡します。サーバー入力の詳細は [Server API チュートリアル](iaas-api/server.md)を参照してください。

```console
$ SERVER_NAME="skr-tutorial-gslb-server-$(date +%s)"
$ GSLB_NAME="skr-tutorial-gslb-$(date +%s)"
$ skr iaas-api server create --zone is1b \
    --name "$SERVER_NAME" --description "Temporary GSLB tutorial server" \
    --cpu 1 --memory-gb 1 \
    --disk "archive-id=$CLOUDIMG_ARCHIVE_ID,plan-id=$DISK_PLAN_ID,size-gb=$DISK_SIZE_GB" \
    --nic 'upstream=shared'
$ SERVER_ID=SERVER_ID
$ skr iaas-api server boot --zone is1b --id "$SERVER_ID" \
    --user-data "@$USER_DATA_PATH"
$ skr iaas-api server read --zone is1b --id "$SERVER_ID" \
    --query '{ID,Name,Description,InstanceStatus,Interfaces}'
$ skr iaas-api disk find --zone is1b --name "$SERVER_NAME" \
    --query 'map({ID,Name,ServerID,SourceArchiveID})'
[
  {
    "ID": 123456789003,
    "Name": "skr-tutorial-gslb-server-EXAMPLE",
    "ServerID": 123456789004,
    "SourceArchiveID": 123456789001
  }
]
$ PUBLIC_IP=PUBLIC_IP
```

出力例は ID と名前を置き換えています。`SERVER_ID` は Create の出力の ID を設定します。`PUBLIC_IP` には Read の出力から共有 NIC の `IPAddress` を設定してください。`InstanceStatus` が `up` でも OS の初期設定は完了していないことがあるため、SSH でホスト名を取得できるまで待ちます。

```console
$ ssh -i "$KEY_PATH" -o IdentitiesOnly=yes \
    -o UserKnownHostsFile="$WORK_DIR/known_hosts" "skr-tutorial@$PUBLIC_IP" hostname
```

初回接続ではホスト鍵の指紋を確認してください。接続できない場合は GSLB の作成へ進まず、アーカイブが cloudimg であること、公開鍵、OS の起動状態、SSH の許可を確認します。

同じアーカイブ・プラン・user-data を使い、ソーリーサーバーを別に作成します。各代入の右辺は今回の出力に置き換えてください。`DISK_ID` と `SORRY_DISK_ID` には、それぞれのサーバーへ接続されたディスクの ID を控えます。

```console
$ DISK_ID=DISK_ID
$ SORRY_NAME="$SERVER_NAME-sorry"
$ skr iaas-api server create --zone is1b \
    --name "$SORRY_NAME" --description "Temporary GSLB tutorial sorry server" \
    --cpu 1 --memory-gb 1 \
    --disk "archive-id=$CLOUDIMG_ARCHIVE_ID,plan-id=$DISK_PLAN_ID,size-gb=$DISK_SIZE_GB" \
    --nic 'upstream=shared'
$ SORRY_ID=SORRY_ID
$ skr iaas-api server boot --zone is1b --id "$SORRY_ID" \
    --user-data "@$USER_DATA_PATH"
$ skr iaas-api server read --zone is1b --id "$SORRY_ID" \
    --query '{ID,Name,Description,InstanceStatus,Interfaces}'
$ skr iaas-api disk find --zone is1b --name "$SORRY_NAME" \
    --query 'map({ID,Name,ServerID,SourceArchiveID})'
[
  {
    "ID": 123456789005,
    "Name": "skr-tutorial-gslb-server-EXAMPLE-sorry",
    "ServerID": 123456789006,
    "SourceArchiveID": 123456789001
  }
]
$ SORRY_IP=SORRY_IP
$ SORRY_DISK_ID=SORRY_DISK_ID
$ ssh -i "$KEY_PATH" -o IdentitiesOnly=yes \
    -o UserKnownHostsFile="$WORK_DIR/known_hosts" "skr-tutorial@$SORRY_IP" hostname
```

出力例は ID と名前を置き換えています。両サーバーの IP が異なり、SSH 接続できることを確認してください。ソーリーサーバーは今回の DNS 応答先確認用です。HTTP のメンテナンスページなどは設定しません。

## Step 5: ping 監視の GSLB を作成する

監視方法を `ping`、チェック間隔を 10 秒にして、実サーバーとソーリーサーバーの IP を登録します。

```console
$ skr gslb-api create --name "$GSLB_NAME" \
    --description "Temporary GSLB ping tutorial" \
    --health-check-protocol ping --delay-loop 10 \
    --destination-server "ip-address=$PUBLIC_IP" \
    --sorry-server "$SORRY_IP" \
    --tags tutorial \
    --query '{ID,Name,FQDN,HealthCheck,DestinationServers,SorryServer}'
$ GSLB_ID=GSLB_ID
$ GSLB_FQDN=GSLB_FQDN
```

`GSLB_ID` と `GSLB_FQDN` は Create の出力に置き換えます。FQDN を組み立てず、返された値をそのまま使ってください。

`--destination-server` の `enabled` と `weight` を省略すると、有効状態と重み 1 になります。複数の宛先を指定する場合はオプションを繰り返します。チェック間隔は 10～60 秒、重みは 1～10000 の整数です（[実サーバーと監視の設定](https://manual.sakura.ad.jp/cloud/appliance/gslb/index.html)）。

作成した GSLB を名前で検索し、ID と FQDN を照合します。GSLB の `--name` は完全一致検索です。

```console
$ skr gslb-api find --name "$GSLB_NAME" --query 'map({ID,Name,FQDN})'
[
  {
    "ID": 123456789002,
    "Name": "skr-tutorial-gslb-EXAMPLE",
    "FQDN": "site-123456789002.gslbN.sakura.ne.jp"
  }
]
$ skr gslb-api read --id "$GSLB_ID" \
    --query '{ID,Name,FQDN,HealthCheck,DestinationServers,SorryServer}'
```

出力例は ID、名前、FQDN を置き換えています。

## Step 6: 稼働中・停止中・復帰後の DNS 応答を比較する

### 稼働中の応答

GSLB FQDN の A レコードを問い合わせます。以下の出力例は実際のサーバー IP を文書用の `192.0.2.10` に置き換えています。手元では `$PUBLIC_IP` の値と一致することを確認してください。作成直後に `NXDOMAIN` などが返る場合は、反映を待って再確認します。`dig "$GSLB_FQDN" A` で `status: NOERROR` と期待する IP の応答を確認してから、停止操作へ進んでください。

```console
$ dig +short "$GSLB_FQDN" A
192.0.2.10
```

### サーバーを停止する

SSH 経由で OS を正常に停止し、API でも電源状態を確認します。この操作は一時サーバーだけに実行してください（[サーバーの電源操作](https://manual.sakura.ad.jp/cloud/server/power.html)）。

```console
$ ssh -i "$KEY_PATH" -o IdentitiesOnly=yes \
    -o UserKnownHostsFile="$WORK_DIR/known_hosts" "skr-tutorial@$PUBLIC_IP" \
    'sudo -n systemctl poweroff --no-block'
$ skr iaas-api server read --zone is1b --id "$SERVER_ID" \
    --query '{ID,Name,InstanceStatus}'
```

`InstanceStatus` が `down` になるまで待ちます。停止に伴って SSH が切断された場合も、エラーを無視せず Read で停止したことを確認してください。

ソーリーサーバーには引き続き SSH 接続できることを確認します。続けて同じ FQDN に問い合わせ、応答先が `$SORRY_IP` だけになるまで待ちます。以下の出力例はソーリーサーバーの IP を文書用の `192.0.2.20` に置き換えています。

```console
$ ssh -i "$KEY_PATH" -o IdentitiesOnly=yes \
    -o UserKnownHostsFile="$WORK_DIR/known_hosts" "skr-tutorial@$SORRY_IP" hostname
$ dig +short "$GSLB_FQDN" A
192.0.2.20
```

`dig "$GSLB_FQDN" A` で `status: NOERROR` も確認してください。実サーバーの IP の消失だけでは成功と判断せず、ソーリーサーバーの IP が応答されることを確認します。空出力、タイムアウト、`SERVFAIL` は切り替え成功ではありません。

### サーバーを再起動する

同じサーバーを起動します。初回起動で作成したユーザーと公開鍵を使うため、user-data は再指定しません。

```console
$ skr iaas-api server boot --zone is1b --id "$SERVER_ID"
$ skr iaas-api server read --zone is1b --id "$SERVER_ID" \
    --query '{ID,Name,InstanceStatus,Interfaces}'
$ ssh -i "$KEY_PATH" -o IdentitiesOnly=yes \
    -o UserKnownHostsFile="$WORK_DIR/known_hosts" "skr-tutorial@$PUBLIC_IP" hostname
$ dig +short "$GSLB_FQDN" A
192.0.2.10
```

SSH 接続でき、実サーバーの IP だけが DNS 応答に戻ることを確認します。監視間隔や DNS キャッシュの影響があるため、停止・復帰直後に結果が変わるとは限りません。10 秒程度の間隔で再確認し、変化しない場合はサーバー状態、GSLB の宛先 IP、ping の許可を確認してください。ping の成功は HTTP などのアプリケーションの正常性までは保証しません。

## Step 7: 設定を更新する

監視間隔と説明を変更し、Read で反映を確認します。省略した設定は変更されません。ただし `--health-check-*` を指定すると、監視設定全体を置き換えます。たとえば HTTP 監視のパスを変える場合、保持したいポートやホストヘッダー、応答コードもプロトコルと共に再指定してください。Update で `--destination-server` を指定すると、追加ではなく一覧全体の置換になります。

```console
$ skr gslb-api update --id "$GSLB_ID" --delay-loop 20 \
    --description "Updated GSLB ping tutorial"
$ skr gslb-api read --id "$GSLB_ID" \
    --query '{ID,Name,Description,DelayLoop,HealthCheck,DestinationServers}'
```

## Step 8: 作成したリソースを削除する

途中で失敗した場合も、この手順で作成した ID を確認して後片付けします。まず GSLB の ID・名前・説明を照合してから削除します。

```console
$ skr gslb-api read --id "$GSLB_ID" --query '{ID,Name,Description}'
$ skr gslb-api delete --id "$GSLB_ID" --fail-if-not-found
$ skr gslb-api find --name "$GSLB_NAME" --query 'map({ID,Name})'
[]
```

次に各サーバーを削除し、ディスクは残します。`--force` は起動中のサーバーを強制停止します。`--with-disks` は指定しません。対象の ID・名前・説明が今回作成したものか、必ず読み直して確認してください。

```console
$ skr iaas-api server read --zone is1b --id "$SERVER_ID" \
    --query '{ID,Name,Description}'
$ skr iaas-api server delete --zone is1b --id "$SERVER_ID" \
    --force --fail-if-not-found
$ skr iaas-api server read --zone is1b --id "$SORRY_ID" \
    --query '{ID,Name,Description}'
$ skr iaas-api server delete --zone is1b --id "$SORRY_ID" \
    --force --fail-if-not-found
$ skr iaas-api server find --zone is1b --name "$SERVER_NAME" \
    --query 'map({ID,Name})'
[]
```

次に、作成時に控えた各ディスク ID を読み直します。名前・プラン・作成元アーカイブ・サイズが今回の指定と一致し、`ServerID` が未接続を示す `0` であることを確認します。一致しない場合は削除せず、対象を確認してください。

```console
$ skr iaas-api disk read --zone is1b --id "$DISK_ID" \
    --query '{ID,Name,ServerID,DiskPlanID,SourceArchiveID,SizeMB}'
$ skr iaas-api disk delete --zone is1b --id "$DISK_ID" --fail-if-not-found
$ skr iaas-api disk read --zone is1b --id "$SORRY_DISK_ID" \
    --query '{ID,Name,ServerID,DiskPlanID,SourceArchiveID,SizeMB}'
$ skr iaas-api disk delete --zone is1b --id "$SORRY_DISK_ID" --fail-if-not-found
$ skr iaas-api disk find --zone is1b --name "$SERVER_NAME" \
    --query 'map({ID,Name})'
[]
```

Server と Disk の `--name` は部分一致検索です。残った項目があっても一括削除せず、控えた ID と名前を照合してください。作成途中の失敗でディスクだけが残った場合も、[Disk API チュートリアル](iaas-api/disk.md)を参照し、今回のディスクだと確認してから削除します。

クラウド上の後片付けが完了したら、ローカルの一時鍵と user-data を削除します。

```console
$ rm "$KEY_PATH" "$KEY_PATH.pub" "$USER_DATA_PATH"
$ rm "$WORK_DIR/known_hosts"
$ rmdir "$WORK_DIR"
```

## 独自ドメインで利用する場合

GSLB を継続利用する場合は、利用したいホスト名の CNAME に生成 FQDN を設定します。たとえば `www.example.jp CNAME <GSLB-FQDN>` とします。このデモでは設定不要です。CNAME を設定した場合は GSLB の削除前に解除してください（[FQDN の DNS 登録](https://manual.sakura.ad.jp/cloud/appliance/gslb/index.html)）。

## 参考資料

- [GSLB（広域負荷分散）](https://manual.sakura.ad.jp/cloud/appliance/gslb/index.html)
- [GSLB FAQ](https://manual.sakura.ad.jp/cloud/support/technical/appliance-gslb.html)
- [cloud-init 対応パブリックアーカイブ](https://manual.sakura.ad.jp/cloud/server/cloud-init.html)
- [サーバーの電源操作](https://manual.sakura.ad.jp/cloud/server/power.html)
- [ディスクのプランとサイズ](https://manual.sakura.ad.jp/cloud/storage/disk.html)
- [sacloud-sdk-go v0.3.1: GSLB service](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.1/service/iaas/gslb)
- [sacloud-sdk-go v0.3.1: Server service](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.1/service/iaas/server)
