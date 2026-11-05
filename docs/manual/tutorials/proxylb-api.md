# ProxyLB API: HTTP 監視と配下サーバーへのリクエスト振り分けを試す

エンハンスドロードバランサ (ProxyLB) は、クライアントからの HTTP/HTTPS/TCP リクエストを配下サーバーへ振り分けます。この手順では is1b に一時サーバーを1台作成し、plan 100 の ProxyLB を作成してリッスンポートと配下サーバーを登録します。作成直後に割り当てられる仮想 IP アドレス (VIP) への HTTP GET で配下サーバーへ到達することと、稼働状態を確認します（[エンハンスドロードバランサ (ProxyLB)](https://manual.sakura.ad.jp/cloud/appliance/proxylb/index.html)）。

全配下サーバーが監視エラーになった場合の動作は、ソーリーサーバーを設定するかどうかで変わります。この手順ではソーリーサーバーを設定しません（[エンハンスドロードバランサ FAQ](https://manual.sakura.ad.jp/cloud/support/technical/appliance-proxylb.html)）。

> [!WARNING]
> 選択中のプロジェクトに ProxyLB・サーバー・ディスクを作成し、リソースの削除を行います。実行前に `skr config current` と対象プロジェクト、各リソースの料金を確認してください。既存の業務用サーバーでは試さず、この手順で作成したリソースだけを操作します。

## 前提条件

- `skr`、`ssh`、`ssh-keygen`、`openssl`、`curl` を利用できる環境が必要です。
- ProxyLB の操作と、is1b のアーカイブ・ディスクプランの参照、サーバー・ディスクの作成・起動・削除が許可された認証プロファイルを用意してください。
- 以下のコマンドは同じシェルで実行します。作成したリソースの ID はコマンド出力から設定してください。

ProxyLB はグローバルリソースのため、`proxylb-api` の操作にゾーンを指定しません。サーバー、ディスク、アーカイブの操作には `--zone is1b` を指定します。

## 作成するリソースと順序

1. cloud-init 対応 Ubuntu アーカイブを使い、共有セグメントに接続する一時サーバーとディスクを1個ずつ作成して起動します。
2. plan 100 の ProxyLB を作成し、リッスンポートと配下サーバーを登録します。
3. `health-status` と VIP への HTTP GET で振り分けを確認します。
4. 自己署名証明書を設定・取得・削除します。
5. 設定を更新します。
6. ProxyLB、サーバー、ディスク、ローカルの SSH 鍵の順に片付けます。

## Step 1: 対象環境とコマンドを確認する

```console
$ skr config current
$ skr proxylb-api --help
$ skr proxylb-api create --help
$ skr proxylb-api update --help
$ skr proxylb-api delete --help
$ skr proxylb-api set-certificates --help
$ skr iaas-api archive find --help
$ skr iaas-api product disk find --help
$ skr iaas-api server create --help
$ skr iaas-api server boot --help
$ skr iaas-api server delete --help
```

## Step 2: cloud-init 対応アーカイブとディスクプランを選ぶ

Ubuntu の cloudimg に絞り込んで ID が最大 (最新) のアーカイブをシェル変数に取り込みます。通常の Ubuntu アーカイブと cloudimg は別のアーカイブです。cloudimg でなければ cloud-init user-data が処理されません（[cloud-init 対応アーカイブ](https://manual.sakura.ad.jp/cloud/server/cloud-init.html)）。

```console
$ CLOUDIMG_ARCHIVE_ID=$(skr iaas-api archive find --zone is1b \
    --query 'map(select(.Name | test("Ubuntu") and test("cloudimg"))) | sort_by(-.ID) | .[0].ID')
$ echo "$CLOUDIMG_ARCHIVE_ID"
$ skr iaas-api archive read --zone is1b --id "$CLOUDIMG_ARCHIVE_ID" \
    --query '{ID,Name,SizeMB,Tags}'
```

続いて SSDプランのうち ID が最小のものと、プラン内で利用可能な最小の GB 単位のサイズを取り込みます。

```console
$ DISK_PLAN_ID=$(skr iaas-api product disk find --zone is1b \
    --query '.DiskPlans | map(select(.Name | test("SSD"))) | sort_by(.ID) | .[0].ID')
$ DISK_SIZE_GB=$(skr iaas-api product disk read --zone is1b --id "$DISK_PLAN_ID" \
    --query '[.Size[] | select(.Availability == "available" and .DisplaySuffix == "GB") | .DisplaySize] | .[0]')
$ echo "$DISK_PLAN_ID, $DISK_SIZE_GB"
```

`--disk` に指定するディスクのサイズはアーカイブのサイズ以上が必要です。より大きいサイズが必要な場合は、`skr iaas-api product disk read` の結果から選び直してください（[ディスクのプランとサイズ](https://manual.sakura.ad.jp/cloud/storage/disk.html)）。

## Step 3: SSH 鍵と cloud-init 設定を準備する

一時ディレクトリに使い捨ての SSH 鍵と user-data を作成します。秘密鍵の内容は引数や API リクエストに含めず、公開鍵だけを user-data に書き込みます。配下サーバーには、ProxyLB からの監視リクエストに応答する簡単な HTTP サーバーを cloud-init で設定します。

```console
$ umask 077
$ WORK_DIR=$(mktemp -d)
$ KEY_PATH="$WORK_DIR/proxylb-key"
$ USER_DATA_PATH="$WORK_DIR/proxylb-c0nfig.yaml"
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
write_files:
  - path: /var/www/html/index.html
    content: |
      proxylb-tutorial-backend
packages:
  - nginx
runcmd:
  - systemctl enable --now nginx
EOF
```

nginx は HTTP (80 番ポート) で待ち受けます。一時サーバー専用の設定です。

## Step 4: 一時サーバーを作成して起動する

重複しない名前を設定し、1 vCPU／1 GB メモリ、ディスク1個、共有 NIC 1個で作成します。作成時には起動せず、`boot` で user-data を渡します。サーバー入力の詳細は [Server API チュートリアル](iaas-api/server.md)を参照してください。

```console
$ SERVER_NAME="skr-tutorial-proxylb-server-$(date +%s)"
$ PROXYLB_NAME="skr-tutorial-proxylb-$(date +%s)"
$ skr iaas-api server create --zone is1b \
    --name "$SERVER_NAME" --description "Temporary ProxyLB tutorial server" \
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
    "Name": "skr-tutorial-proxylb-server-EXAMPLE",
    "ServerID": 123456789004,
    "SourceArchiveID": 123456789001
  }
]
$ PUBLIC_IP=PUBLIC_IP
$ DISK_ID=DISK_ID
```

出力例は ID と名前を置き換えています。`SERVER_ID` は Create の出力の ID を設定します。`PUBLIC_IP` には Read の出力から共有 NIC の `IPAddress` を設定してください。`DISK_ID` には `disk find` の出力の ID を設定します。`InstanceStatus` が `up` でも OS の初期設定は完了していないことがあるため、SSH でホスト名を取得できるまで待ちます。

```console
$ ssh -i "$KEY_PATH" -o IdentitiesOnly=yes \
    -o UserKnownHostsFile="$WORK_DIR/known_hosts" "skr-tutorial@$PUBLIC_IP" hostname
```

初回接続ではホスト鍵の指紋を確認してください。接続できない場合は ProxyLB の作成へ進まず、アーカイブが cloudimg であること、公開鍵、OS の起動状態、SSH の許可を確認します。

## Step 5: ProxyLB を作成する

plan 100 を指定し、リッスンポート (HTTP 80 番) と配下サーバーを登録します。`--bind-port` と `--server` は繰り返し指定できます。ヘルスチェックのプロトコル、監視間隔、プロトコルが `http` のときの監視パスは API が必須とする設定です。

```console
$ skr proxylb-api create --name "$PROXYLB_NAME" \
    --description "Temporary ProxyLB tutorial" \
    --plan 100 --region anycast \
    --health-check-protocol http --health-check-delay-loop 10 \
    --health-check-path / \
    --bind-port 'proxy-mode=http,port=80' \
    --server "ip-address=$PUBLIC_IP,port=80,enabled=true" \
    --tags tutorial \
    --query '{ID,Name,Plan,Region,HealthCheck,BindPorts,Servers}'
```

`enabled=true` を指定しないと実サーバーは無効で登録され、トラフィックが振り分けられません。

`--bind-port` は `proxy-mode` と `port` が必須です。`redirect-to-https` は `proxy-mode=http` のみ、`support-http2` と `ssl-policy` は `proxy-mode=https` のみ指定できます。HTTPS のリッスンポートを追加する場合は `--bind-port 'proxy-mode=https,port=443'` を繰り返して指定します。

作成した ProxyLB を名前で検索し、ID をシェル変数に取り込みます。`--name` は部分一致検索です。FQDN は API が非同期に割り当てるため、作成直後の応答には含まれません。この手順の振り分け確認は、作成直後に割り当てられる VIP で行います。

```console
$ PROXYLB_ID=$(skr proxylb-api find --name "$PROXYLB_NAME" --query 'map(select(.Name == "'"$PROXYLB_NAME"'")) | sort_by(-.ID) | .[0].ID')
$ skr proxylb-api find --name "$PROXYLB_NAME" --query 'map({ID,Name,Plan})'
[
  {
    "ID": 123456789002,
    "Name": "skr-tutorial-proxylb-EXAMPLE",
    "Plan": 100
  }
]
$ PROXYLB_ID=$(skr proxylb-api find --name "$PROXYLB_NAME" --query 'map(select(.Name == "'"$PROXYLB_NAME"'")) | sort_by(-.ID) | .[0].ID')
$ PROXYLB_VIP=$(skr proxylb-api read --id "$PROXYLB_ID" --query .VirtualIPAddress | tr -d '"')
$ skr proxylb-api read --id "$PROXYLB_ID" \
    --query '{ID,Name,Plan,Region,VirtualIPAddress,FQDN,HealthCheck,BindPorts,Servers}'
```

出力例は ID と名前を置き換えています。手元では `VirtualIPAddress` が設定されていることを確認してください。

## Step 6: 稼働状態と振り分けを確認する

作成直後は ProxyLB の準備が完了していないことがあります。稼働状態が現れるまで待ってから確認します。

`health-status` で配下サーバーの稼働状態を取得します。応答の `Servers` に実サーバーごとの稼働状態が現れます。`Status` が `UP` になるまで、10 秒程度の間隔で再確認します。

```console
$ skr proxylb-api health-status --id "$PROXYLB_ID" --query '.Servers'
[
  {
    "IPAddress": "192.0.2.10",
    "Port": 80,
    "Status": "UP"
  }
]
```

出力例は配下サーバーの IP を文書用の `192.0.2.10` に置き換えています。手元では `$PUBLIC_IP` と一致することを確認してください。

続けて VIP へ HTTP GET でリクエストし、配下サーバーが応答することを確認します。

```console
$ curl -s "http://$PROXYLB_VIP/"
proxylb-tutorial-backend
```

DNS の反映には時間がかかります。解決できない場合は応答が返るまで待って再確認します。

`monitor-connection` で接続数のモニタリングを取得します。`--start` と `--end` は RFC3339 形式です。未指定時は現在時刻の1時間前からの範囲です。

```console
$ skr proxylb-api monitor-connection --id "$PROXYLB_ID" \
    --start "$(date -u -d '10 minutes ago' +%Y-%m-%dT%H:%M:%SZ)" \
    --end "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
```

## Step 7: 証明書を設定・取得・削除する

HTTPS のリッスンポートを使う場合、ProxyLB に証明書を設定します。この手順では `openssl` で自己署名証明書を生成して確認します。実運用では認証局が発行した証明書を使ってください。

秘密鍵を含むファイルは引数に指定せず、`@path` 形式で渡します。`--primary-cert` と `--primary-key` は必須です。API は証明書一式を置き換えるため、追加証明書だけの差し替えでも主証明書の再指定が必要です。

```console
$ openssl req -x509 -newkey rsa:2048 -sha256 -days 1 -nodes \
    -keyout "$WORK_DIR/proxylb-server.key" -out "$WORK_DIR/proxylb-server.crt" \
    -subj "/CN=proxylb.example.test"
$ skr proxylb-api set-certificates --id "$PROXYLB_ID" \
    --primary-cert "@$WORK_DIR/proxylb-server.crt" \
    --primary-key "@$WORK_DIR/proxylb-server.key"
$ skr proxylb-api get-certificates --id "$PROXYLB_ID" --query '.PrimaryCert.CertificateCommonName'
"123456789002.proxylb.jp"
```

出力例は文書用の共通名に置き換えています。`CertificateCommonName` が生成時に指定した CN と一致することを確認してください。

設定した証明書を削除します。

```console
$ skr proxylb-api delete-certificates --id "$PROXYLB_ID"
```

Let's Encrypt による自動更新を使う場合、Create または Update で `--lets-encrypt-enabled true --lets-encrypt-common-name example.jp` を指定します。設定済みの Let's Encrypt 証明書は `renew-lets-encrypt-cert` で更新できます。Let's Encrypt が未設定の ProxyLB に対する `renew-lets-encrypt-cert` は API エラーになります。

## Step 8: 設定を更新する

監視間隔と説明を変更し、Read で反映を確認します。Update の未指定の項目は既存の設定が引き継がれるため、監視間隔だけを変更する場合は他の設定の再指定は不要です。`--health-check-*` を指定すると監視設定全体を置き換えるため、保持したい `--health-check-path` もプロトコルと併せて再指定してください。`--bind-port` と `--server` を指定すると追加ではなく一覧全体の置換になります。`--plan` を指定するとプラン変更 API が呼ばれます。

```console
$ skr proxylb-api update --id "$PROXYLB_ID" \
    --health-check-protocol http --health-check-delay-loop 20 --health-check-path / \
    --description "Updated ProxyLB tutorial"
$ skr proxylb-api read --id "$PROXYLB_ID" \
    --query '{ID,Name,Description,HealthCheck}'
```

## Step 9: 作成したリソースを削除する

途中で失敗した場合も、この手順で作成した ID を確認して後片付けします。まず ProxyLB の ID・名前・説明を照合してから削除します。

```console
$ skr proxylb-api read --id "$PROXYLB_ID" --query '{ID,Name,Description}'
$ skr proxylb-api delete --id "$PROXYLB_ID" --fail-if-not-found
$ skr proxylb-api find --name "$PROXYLB_NAME" --query 'map({ID,Name})'
[]
```

次にサーバーを削除します。`--force` は起動中のサーバーを強制停止します。`--with-disks` は指定しません。対象の ID・名前・説明が今回作成したものか、必ず読み直して確認してください。

```console
$ skr iaas-api server read --zone is1b --id "$SERVER_ID" \
    --query '{ID,Name,Description}'
$ skr iaas-api server delete --zone is1b --id "$SERVER_ID" \
    --force --fail-if-not-found
$ skr iaas-api server find --zone is1b --name "$SERVER_NAME" \
    --query 'map({ID,Name})'
[]
```

次に、作成時に控えたディスク ID を読み直します。名前・プラン・作成元アーカイブ・サイズが今回の指定と一致し、`ServerID` が未接続を示す `0` であることを確認します。一致しない場合は削除せず、対象を確認してください。

```console
$ skr iaas-api disk read --zone is1b --id "$DISK_ID" \
    --query '{ID,Name,ServerID,DiskPlanID,SourceArchiveID,SizeMB}'
$ skr iaas-api disk delete --zone is1b --id "$DISK_ID" --fail-if-not-found
$ skr iaas-api disk find --zone is1b --name "$SERVER_NAME" \
    --query 'map({ID,Name})'
[]
```

Server と Disk の `--name` は部分一致検索です。残った項目があっても一括削除せず、控えた ID と名前を照合してください。

クラウド上の後片付けが完了したら、ローカルの一時鍵と user-data、証明書を削除します。

```console
$ rm "$KEY_PATH" "$KEY_PATH.pub" "$USER_DATA_PATH"
$ rm "$WORK_DIR/proxylb-server.key" "$WORK_DIR/proxylb-server.crt"
$ rm "$WORK_DIR/known_hosts"
$ rmdir "$WORK_DIR"
```

## 実運用での証明書について

Let's Encrypt を使う場合、Create または Update で `--lets-encrypt-enabled true --lets-encrypt-common-name example.jp` を指定します。共通名には ProxyLB で名前解決できる FQDN を指定してください（[エンハンスドロードバランサ (ProxyLB)](https://manual.sakura.ad.jp/cloud/appliance/proxylb/index.html)）。認証局が発行した証明書を使う場合は、この手順の自己署名証明書と同じ `set-certificates` で設定します。

## 参考資料

- [エンハンスドロードバランサ (ProxyLB)](https://manual.sakura.ad.jp/cloud/appliance/proxylb/index.html)
- [エンハンスドロードバランサ FAQ](https://manual.sakura.ad.jp/cloud/support/technical/appliance-proxylb.html)
- [cloud-init 対応パブリックアーカイブ](https://manual.sakura.ad.jp/cloud/server/cloud-init.html)
- [サーバーの電源操作](https://manual.sakura.ad.jp/cloud/server/power.html)
- [ディスクのプランとサイズ](https://manual.sakura.ad.jp/cloud/storage/disk.html)
- [sacloud-sdk-go v0.3.1: ProxyLB service](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.1/service/iaas/proxylb)
