# Server API チュートリアル

このチュートリアルでは、cloud-init 対応パブリックアーカイブから Server を作成し、共有セグメントへ接続して SSH ログインします。`server create` の個別オプションでディスクと NIC を指定し、cloud-init の user-data は起動時に `server boot` へ渡します。秘密鍵はローカルで生成し、API へ送信しません。

この手順では is1b を使用します。アーカイブ ID、ディスクプラン ID、ディスクサイズは、作成前に同じゾーンの検索・参照結果から選びます。

## 前提条件

- `skr` と SSH クライアントが利用できること
- さくらのクラウドの認証設定と、is1b で Server・Disk を作成できるプロジェクトを選択していること
- is1b で Archive・DiskPlan を検索・参照できること
- Server の作成、起動、外部 SSH 接続および削除が許可されていること

Server と Disk の利用料金が発生します。共有セグメントへ接続した Server は外部から SSH 接続できるため、公開鍵認証を設定し、不要になったら削除してください。

## コマンドを確認する

```console
$ skr iaas-api archive find --help
$ skr iaas-api archive read --help
$ skr iaas-api disk-plan find --help
$ skr iaas-api disk-plan read --help
$ skr iaas-api server create --help
$ skr iaas-api server boot --help
$ skr iaas-api server read --help
$ skr iaas-api server delete --help
$ skr iaas-api server find --help
$ skr iaas-api disk find --help
```

## アーカイブとディスクプランを選ぶ

is1b のアーカイブを検索し、名前と説明から [cloud-init 対応パブリックアーカイブ（cloudimg）](https://manual.sakura.ad.jp/cloud/server/cloud-init.html)を選びます。通常の OS アーカイブではなく、cloudimg のアーカイブを選んでください。検索結果の ID を `CLOUDIMG_ARCHIVE_ID` に設定し、参照結果でも名前、説明、サイズを確認します。

```console
$ skr iaas-api archive find --zone is1b \
    --query 'map({ID,Name,Description,DiskPlanID,DiskPlanName,SizeMB,Tags})'
$ CLOUDIMG_ARCHIVE_ID=CLOUDIMG_ARCHIVE_ID
$ skr iaas-api archive read --zone is1b --id "$CLOUDIMG_ARCHIVE_ID" \
    --query '{ID,Name,Description,DiskPlanID,DiskPlanName,SizeMB,Tags}'
```

代入の右辺の `CLOUDIMG_ARCHIVE_ID` は、選んだアーカイブの実際の ID に置き換えます。検索条件の指定方法は [Archive API チュートリアル](archive.md)を参照してください。

次に、同じゾーンのディスクプランを検索します。プランの `Availability` と `Size` 内の各サイズの `Availability` を確認し、利用可能なプランとサイズを選びます。[ディスクプランや作成先のゾーンによって選択可能なサイズは異なります](https://manual.sakura.ad.jp/cloud/storage/disk.html)。

```console
$ skr iaas-api disk-plan find --zone is1b \
    --query 'map({ID,Name,StorageClass,Availability,Size})'
$ DISK_PLAN_ID=DISK_PLAN_ID
$ skr iaas-api disk-plan read --zone is1b --id "$DISK_PLAN_ID" \
    --query '{ID,Name,StorageClass,Availability,Size}'
$ DISK_SIZE_GB=DISK_SIZE_GB
```

代入の右辺の `DISK_PLAN_ID` は、選んだプランの実際の ID に置き換えます。`DISK_SIZE_GB` は、`Size` 内で `DisplaySuffix` が `GB` の利用可能なサイズの `DisplaySize` に置き換えます。アーカイブの `SizeMB` と、選んだディスクサイズの `SizeMB` も確認してください。各フィールドの説明は [DiskPlan API チュートリアル](disk-plan.md)を参照してください。

ここまでの操作は検索・参照のみで、クラウドリソースを変更しません。

## SSH 鍵を作成する

作業用ディレクトリを作成し、その中に鍵を生成します。秘密鍵はローカルに保管し、コマンドの引数や API リクエストに含めません。

```console
$ umask 077
$ WORK_DIR=$(mktemp -d)
$ KEY_PATH="$WORK_DIR/server-tutorial-key"
$ USER_DATA_PATH="$WORK_DIR/server-c0nfig.yaml"
$ ssh-keygen -t rsa -b 3072 -N '' -f "$KEY_PATH"
```

## cloud-init の user-data を作成する

公開鍵を cloud-config に設定します。次のコマンドは公開鍵だけをファイルへ書き込みます。

```console
$ cat > "$USER_DATA_PATH" <<EOF
#cloud-config
users:
  - default
  - name: skr-tutorial
    shell: /bin/bash
    ssh_authorized_keys:
      - $(cat "$KEY_PATH.pub")
EOF
```

## Server を作成して起動する

Server 名には重複しない値を指定します。`CLOUDIMG_ARCHIVE_ID`、`DISK_PLAN_ID`、`DISK_SIZE_GB` には、前の手順で選んだ正の整数を設定しておきます。

`--disk` の plan-id と size-gb は必須です。archive-id を省略すると空のディスクを作成します。`--disk` と `--nic` は繰り返して指定でき、それぞれ指定順に接続します。`upstream=shared` は先頭の NIC にだけ指定できます。この手順ではディスクと共有 NIC を1個ずつ指定します。`--request` とは併用できません。

`--nic "upstream=$SWITCH_ID"` で、正の整数の ID を持つスイッチへの接続も指定できます。ルータ＋スイッチに接続し、ゲスト OS の IP アドレスを設定して SSH 接続する手順は [Internet API チュートリアル](internet.md)を参照してください。

```console
$ SERVER_NAME="skr-tutorial-cloudinit-$(date +%s)"
$ skr iaas-api server create \
    --zone is1b \
    --name "$SERVER_NAME" \
    --cpu 1 \
    --memory-gb 1 \
    --disk "archive-id=$CLOUDIMG_ARCHIVE_ID,plan-id=$DISK_PLAN_ID,size-gb=$DISK_SIZE_GB" \
    --nic 'upstream=shared'
```

作成結果の Server ID を控え、`SERVER_ID` に設定します。cloud-init の user-data を適用するため、作成時には Server を起動せず、次の `boot` 操作で起動します。

`--user-data` 自体は任意ですが、この手順では SSH 公開鍵とユーザーを設定するために指定します。user-data が不要な通常の起動では省略できます。指定する場合は `@path` または `-` を使い、内容を直接引数に渡さないでください。

```console
$ SERVER_ID=SERVER_ID
$ skr iaas-api server boot \
    --zone is1b \
    --id "$SERVER_ID" \
    --user-data "@$USER_DATA_PATH"
```

Server の状態と NIC の IP アドレスを確認します。`Interfaces` の共有セグメントに割り当てられた `IPAddress` を `PUBLIC_IP` に設定してください。

```console
$ skr iaas-api server read --zone is1b --id "$SERVER_ID" \
    --query '{ID, Name, InstanceStatus, Interfaces}'
$ PUBLIC_IP=PUBLIC_IP
```

`InstanceStatus` が起動状態になり、IP アドレスが割り当てられるまで待ってから SSH 接続します。初回接続では表示されるホスト鍵の指紋を確認してください。

```console
$ ssh -i "$KEY_PATH" \
    -o IdentitiesOnly=yes \
    "skr-tutorial@$PUBLIC_IP"
```

ログインできたら、次のコマンドで Server のホスト名を確認します。

```console
$ hostname
```

## Server と Disk を削除する

SSH セッションで `exit` を実行し、削除前に Server ID と名前を確認します。`--with-disks` は接続中の Disk も削除します。対象がこの手順で作成した Server であることを必ず確認してください。

```console
$ skr iaas-api server read --zone is1b --id "$SERVER_ID" \
    --query '{ID, Name, InstanceStatus}'
$ skr iaas-api server delete \
    --zone is1b \
    --id "$SERVER_ID" \
    --with-disks \
    --force \
    --fail-if-not-found
```

Server が残っていないことを ID と名前で確認します。出力が空の配列なら該当する Server はありません。

```console
$ skr iaas-api server find --zone is1b \
    --query "map(select((.ID | tostring) == \"$SERVER_ID\" or .Name == \"$SERVER_NAME\") | {ID, Name})"
```

```json
[]
```

続いて Server と同名の Disk が残っていないことを確認します。出力が空の配列なら該当する Disk はありません。

```console
$ skr iaas-api disk find --zone is1b \
    --query "map(select(.Name == \"$SERVER_NAME\") | {ID, Name})"
```

```json
[]
```

ローカルの鍵と user-data も不要になったら削除してください。秘密鍵を失うと同じ鍵では再接続できません。必要なデータを退避してから削除します。

```console
$ rm "$KEY_PATH" "$KEY_PATH.pub" "$USER_DATA_PATH"
$ rmdir "$WORK_DIR"
```

## 参考資料

- [cloud-init 対応パブリックアーカイブからサーバを作成する](https://manual.sakura.ad.jp/cloud/server/cloud-init.html)
- [サーバの作成と削除](https://manual.sakura.ad.jp/cloud/server/create-delete.html)
- [共有セグメントを利用する](https://manual.sakura.ad.jp/cloud/design-pattern/ex01.html)
- [公開鍵認証の設定](https://manual.sakura.ad.jp/cloud/controlpanel/settings/public-key.html)
- [sacloud-sdk-go v0.3.0 Server API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/server)
