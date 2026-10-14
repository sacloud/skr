# Server API チュートリアル

このチュートリアルでは、cloud-init 対応パブリックアーカイブから Server を作成し、共有セグメントへ接続して SSH ログインします。`server create` の個別オプションでディスクと NIC を指定し、cloud-init の user-data は起動時に `server boot` へ渡します。秘密鍵はローカルで生成し、API へ送信しません。

この手順では is1b を使用します。アーカイブ ID、ディスクプラン ID、ディスクサイズは、is1b で利用できる値を指定してください。

## 前提条件

- `skr` と SSH クライアントが利用できること
- さくらのクラウドの認証設定と、is1b で Server・Disk を作成できるプロジェクトを選択していること
- cloud-init 対応パブリックアーカイブの ID と、利用可能なディスクプラン ID・サイズが分かること
- Server の作成、起動、外部 SSH 接続および削除が許可されていること

Server と Disk の利用料金が発生します。共有セグメントへ接続した Server は外部から SSH 接続できるため、公開鍵認証を設定し、不要になったら削除してください。

## コマンドを確認する

```console
$ skr iaas-api server create --help
$ skr iaas-api server boot --help
$ skr iaas-api server read --help
$ skr iaas-api server delete --help
$ skr iaas-api disk find --help
```

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

Server 名には重複しない値を指定します。`CLOUDIMG_ARCHIVE_ID`、`DISK_PLAN_ID`、`DISK_SIZE_GB` は、is1b で利用できる正の整数に置き換えてください。

`--disk` の plan-id と size-gb は必須です。archive-id を省略すると空のディスクを作成します。`--disk` と `--nic` は繰り返して指定でき、それぞれ指定順に接続します。この手順ではディスクと共有 NIC を1個ずつ指定します。`--request` とは併用できません。

```console
$ SERVER_NAME="skr-tutorial-cloudinit-$(date +%s)"
$ skr iaas-api server create \
    --zone is1b \
    --name "$SERVER_NAME" \
    --cpu 1 \
    --memory-gb 1 \
    --disk 'archive-id=CLOUDIMG_ARCHIVE_ID,plan-id=DISK_PLAN_ID,size-gb=DISK_SIZE_GB' \
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

Server と同名の Disk が残っていないことを確認します。出力が空の配列なら該当する Disk はありません。

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
