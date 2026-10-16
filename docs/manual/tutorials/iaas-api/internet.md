# IaaS Internet API チュートリアル

このチュートリアルでは、is1b にルータ＋スイッチを作成し、そのスイッチに接続した Server を cloud-init で設定して、NIC のグローバル IPv4 アドレスへ SSH 接続します。作成後は Server、Disk、ルータ＋スイッチを削除します。

ルータ＋スイッチは[インターネットへ直接接続されるグローバルネットワーク](https://manual.sakura.ad.jp/cloud/network/switch/about.html)です。Server の先頭 NIC を接続し、割り当てられたサブネット内の IP アドレスとデフォルトルートを Server の OS に設定します。[NIC の表示用 IPv4 アドレスは OS の設定には反映されません](https://manual.sakura.ad.jp/cloud/server/nic.html)。この手順では、新しく作成したルータ＋スイッチの `IPAddresses` から使用する IPv4 アドレスを選びます。

## 前提条件

- `skr` と `ssh` が利用できること
- さくらのクラウドの認証設定で、is1b を操作できるプロジェクトを選択していること
- is1b でルータ＋スイッチ、Server、Disk を作成・削除できること
- ルータ＋スイッチと Server の利用料金が発生することを確認していること

ルータ＋スイッチは選択した帯域に応じた料金が発生します。この手順では `/28`、`100 Mbps` のルータ＋スイッチを作成します。Server と Disk の料金も発生します。SSH 接続のため Server の NIC を一時的にインターネットへ公開します。不要になったら必ず削除してください。

## Step 1: コマンドと対象プロファイルを確認する

```console
$ skr config current
$ skr iaas-api --help
$ skr iaas-api internet --help
$ skr iaas-api internet find --help
$ skr iaas-api internet create --help
$ skr iaas-api internet list-subnet --help
$ skr iaas-api internet delete --help
$ skr iaas-api server create --help
$ skr iaas-api server boot --help
$ skr iaas-api server read --help
$ skr iaas-api server delete --help
```

## Step 2: Archive と DiskPlan を選ぶ

cloud-init 対応パブリックアーカイブを is1b で検索します。名前と説明から `cloudimg` を選び、Archive の `ID` を `CLOUDIMG_ARCHIVE_ID` に設定します。Archive API の結果を参照して、アーカイブの ID、名前、サイズも確認してください。

```console
$ skr iaas-api archive find --zone is1b \
    --query 'map({ID,Name,Description,DiskPlanID,DiskPlanName,SizeMB})'
$ CLOUDIMG_ARCHIVE_ID=CLOUDIMG_ARCHIVE_ID
$ skr iaas-api archive read --zone is1b --id "$CLOUDIMG_ARCHIVE_ID"
```

次に、同じゾーンで利用可能な DiskPlan とサイズを検索し、`DISK_PLAN_ID` と `DISK_SIZE_GB` を設定します。

```console
$ skr iaas-api disk-plan find --zone is1b \
    --query 'map({ID,Name,Availability,Size})'
$ DISK_PLAN_ID=DISK_PLAN_ID
$ DISK_SIZE_GB=DISK_SIZE_GB
$ skr iaas-api disk-plan read --zone is1b --id "$DISK_PLAN_ID"
```

各代入の右辺は実際に選んだ正の整数に置き換えます。Archive と DiskPlan の選び方は [Archive API チュートリアル](archive.md)と[DiskPlan API チュートリアル](disk-plan.md)も参照してください。

## Step 3: ルータ＋スイッチとサブネットを確認する

既存リソースと重複しない名前を付けて作成します。作成結果の `ID` と `Switch.ID` を控えます。後者は Server NIC の接続先として使います。

```console
$ ROUTER_NAME="skr-internet-tutorial-$(date +%s)"
$ skr iaas-api internet create \
    --zone is1b \
    --name "$ROUTER_NAME" \
    --description "Temporary Router + Switch for tutorial" \
    --network-mask-len 28 \
    --band-width-mbps 100 \
    --query '{ID,Name,Description,NetworkMaskLen,BandWidthMbps,Switch}'
$ INTERNET_ID=INTERNET_ID
$ SWITCH_ID=SWITCH_ID
```

`INTERNET_ID` と `SWITCH_ID` は作成結果の値に置き換えます。作成時に割り当てられた IPv4 サブネットのネットワークアドレス、プレフィクス長、デフォルトルートを確認します。

```console
$ skr iaas-api internet list-subnet --zone is1b --id "$INTERNET_ID" \
    --query 'map({ID,InternetID,NetworkAddress,NetworkMaskLen,DefaultRoute,StaticRoute,NextHop,IPAddresses})'
```

この手順ではルータ＋スイッチ作成時に割り当てられたサブネットを使用します。追加の IP アドレスブロックやスタティックルートは作成しません。

## Step 4: SSH 鍵を作成する

秘密鍵はローカルだけに保管し、コマンドの引数や API リクエストに含めません。

```console
$ umask 077
$ WORK_DIR=$(mktemp -d)
$ KEY_PATH="$WORK_DIR/internet-tutorial-key"
$ USER_DATA_PATH="$WORK_DIR/internet-tutorial-c0nfig.yaml"
$ ssh-keygen -t rsa -b 3072 -N '' -f "$KEY_PATH"
```

## Step 5: Server の先頭 NIC をスイッチに接続する

`--nic "upstream=$SWITCH_ID"` で Server の先頭 NIC をルータ＋スイッチに接続します。`--disk` には選択した Archive、DiskPlan、サイズを指定します。これらのオプションは繰り返し指定できますが、この手順では NIC と Disk をそれぞれ1個だけ作成します。

ルータ＋スイッチに接続できるのは[先頭の NIC だけ](https://manual.sakura.ad.jp/cloud/server/nic.html)です。`--nic` は接続先を指定するもので、ゲスト OS の IP アドレスは設定しません。Server は停止状態で作成し、次の手順でネットワーク設定と公開鍵を渡して起動します。

```console
$ SERVER_NAME="skr-internet-server-$(date +%s)"
$ skr iaas-api server create \
    --zone is1b \
    --name "$SERVER_NAME" \
    --description "Temporary Server for Router + Switch tutorial" \
    --cpu 1 \
    --memory-gb 1 \
    --disk "archive-id=$CLOUDIMG_ARCHIVE_ID,plan-id=$DISK_PLAN_ID,size-gb=$DISK_SIZE_GB" \
    --nic "upstream=$SWITCH_ID" \
    --query '{ID,Name,Description,Interfaces,Disks}'
$ SERVER_ID=SERVER_ID
```

`SERVER_ID` は作成結果の ID に置き換えます。Server API が返す先頭 NIC の `MACAddress` を控えます。`SwitchID` が `SWITCH_ID` と一致することも確認してください。

```console
$ skr iaas-api server read --zone is1b --id "$SERVER_ID" \
    --query '{ID,Name,InstanceStatus,Interfaces}'
```

ルータ＋スイッチ接続では、Server の `Interfaces[].IPAddress` は空の場合があります。OS に設定する IPv4 アドレスは `internet list-subnet` の `IPAddresses[].IPAddress` から選びます。この手順では新しく作成したルータ＋スイッチに Server を1台だけ接続するため、その一覧の先頭のアドレスを使います。既存ネットワークでは、他の機器と重複しないアドレスを選ぶ必要があります。

## Step 6: cloud-init で IP アドレスを設定して SSH 接続する

次の値を前のコマンド出力から設定します。`NIC_MAC` は Server の `MACAddress`、`NIC_IP` はサブネットの `IPAddresses` から選んだアドレス、`NIC_PREFIX` と `DEFAULT_ROUTE` はサブネットの `NetworkMaskLen` と `DefaultRoute` です。代入の右辺は実際の値に置き換えます。

```console
$ NIC_MAC=SERVER_NIC_MAC
$ NIC_IP=SUBNET_IP_ADDRESS
$ NIC_PREFIX=28
$ DEFAULT_ROUTE=ROUTER_DEFAULT_ROUTE
```

cloud-init user-data に公開鍵を登録し、MAC アドレスに一致する NIC へ IP アドレスとデフォルトルートを設定します。値はこの手順で作成したルータ＋スイッチと Server の出力から取得してください。

この設定は起動中の OS へ適用する一時的な設定です。再起動後も使う Server では、OS のネットワーク設定にも保存してください。

```console
$ cat > "$USER_DATA_PATH" <<EOF
#cloud-config
users:
  - default
  - name: skr-tutorial
    shell: /bin/bash
    ssh_authorized_keys:
      - $(cat "$KEY_PATH.pub")
runcmd:
  - |
    set -eu
    expected_mac="$NIC_MAC"
    iface=
    for path in /sys/class/net/*/address; do
      if [ "\$(cat "\$path")" = "\$expected_mac" ]; then
        iface=\${path%/address}
        iface=\${iface##*/}
        break
      fi
    done
    [ -n "\$iface" ]
    ip link set "\$iface" up
    ip address replace "$NIC_IP/$NIC_PREFIX" dev "\$iface"
    ip route replace default via "$DEFAULT_ROUTE" dev "\$iface"
EOF
$ skr iaas-api server boot \
    --zone is1b \
    --id "$SERVER_ID" \
    --user-data "@$USER_DATA_PATH"
```

`--user-data` は `@path` または標準入力の `-` で指定します。user-data の内容を引数に直接渡さないでください。

起動後、SSH クライアントで接続し、`hostname` の出力でログインを確認します。初回接続時は表示されたホスト鍵の指紋を確認してください。

```console
$ ssh -i "$KEY_PATH" -o IdentitiesOnly=yes "skr-tutorial@$NIC_IP" hostname
```

## Step 7: Server、Disk、ルータ＋スイッチを削除する

削除前に ID と名前を再確認します。次のコマンドは Server と接続中の Disk を削除します。

```console
$ skr iaas-api server read --zone is1b --id "$SERVER_ID" \
    --query '{ID,Name,InstanceStatus}'
$ skr iaas-api server delete \
    --zone is1b \
    --id "$SERVER_ID" \
    --with-disks \
    --force \
    --fail-if-not-found
```

Server と Disk が残っていないことを検索結果で確認してから、作成したルータ＋スイッチを削除します。`--name` は API 側で単一名を部分一致検索します。カンマを含む値も1つの名前として扱い、複数名の条件は `--request` の `Names` 配列で指定します。`--request` と個別フラグは併用できません。

```console
$ skr iaas-api server find --zone is1b --name "$SERVER_NAME" \
    --query 'map({ID,Name})'
```

```json
[]
```

```console
$ skr iaas-api disk find --zone is1b --name "$SERVER_NAME" \
    --query 'map({ID,Name})'
```

```json
[]
```

`--force` は IPv6 の無効化と、`NextHop` が設定された追加サブネットの削除も行います。この手順ではどちらも追加していませんが、削除範囲を確認したうえで指定します。`--fail-if-not-found` は対象が存在しない場合もエラーにする動作オプションです。

```console
$ skr iaas-api internet read --zone is1b --id "$INTERNET_ID" \
    --query '{ID,Name,Description,Switch}'
$ skr iaas-api internet delete --zone is1b --id "$INTERNET_ID" \
    --force --fail-if-not-found
$ skr iaas-api internet find --zone is1b --name "$ROUTER_NAME" \
    --query 'map({ID,Name})'
```

```json
[]
```

検索結果がすべて空の配列であることを確認します。最後に鍵と作業ファイルを削除します。

```console
$ rm "$KEY_PATH" "$KEY_PATH.pub" "$USER_DATA_PATH"
$ rmdir "$WORK_DIR"
```

## 参考資料

- [ルータ＋スイッチの概要](https://manual.sakura.ad.jp/cloud/network/switch/about.html)
- [NIC の作成・削除・接続先編集](https://manual.sakura.ad.jp/cloud/server/nic.html)
- [スタティックルート機能](https://manual.sakura.ad.jp/cloud/network/switch/static-routing.html)
- [cloud-init を利用したサーバ作成](https://manual.sakura.ad.jp/cloud/server/cloud-init.html)
- [sacloud-sdk-go v0.3.0 Internet API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/internet)
- [sacloud-sdk-go v0.3.0 Server API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/server)
