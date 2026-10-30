# Networking Suite API: 別サブネット間の通信を確認する

このチュートリアルでは、サブネットグループ配下に2つのサブネットを作成し、別々のサーバ NIC を接続して IPv4 通信を確認します。ネットワークスイートは L3 接続、IP アドレス管理、DHCP、DNS フォワーダーを提供します。同一サブネットグループ内の異なるサブネットに接続したサーバ間でも通信できます。

> [!WARNING]
> この手順では is1b にサブネットグループ、サブネット、Server 2台と Disk 2台を作成し、最後に削除します。実行前に `skr config current` と対象プロジェクトを確認し、テスト専用のリソース名を使ってください。Server と Disk の利用料金が発生します。料金は Server のプランと Disk のプラン・サイズなどで異なります。作成前に [Server 作成の料金内訳](https://manual.sakura.ad.jp/cloud/server/create-delete.html) と [Disk](https://manual.sakura.ad.jp/cloud/storage/disk.html) を確認してください。

## 前提条件

- `skr`、`ssh`、`ssh-keygen` が利用でき、さくらのクラウドの認証プロファイルが設定済みであること
- is1b で Server と Disk を作成・起動・削除でき、ネットワークスイートのサブネットグループ、サブネット、インターフェースコネクションを管理できること
- is1b で利用可能な Ubuntu の `(cloudimg)` Archive と、20 GB の Disk を作成できる DiskPlan があること
- `SAKURA_ENDPOINTS_NETWORKING_SUITE` が is1b のネットワークスイート API endpoint を指していること。SDK の既定 endpoint は is1c です
- 操作対象として使い捨ての Server を2台作成できること。Server A は `shared` NIC と未接続 NIC を1つずつ、Server B は未接続 NIC を1つ持ちます

Archive と DiskPlan は、is1b の検索結果からコマンド置換でシェル変数に取り込みます。次の例では両 Server に20 GB の Disk を作成します。

```console
$ skr config current
$ export SAKURA_ENDPOINTS_NETWORKING_SUITE=https://secure.sakura.ad.jp/cloud/zone/is1b/api/cloud/1.1/
$ CLOUDIMG_ARCHIVE_ID=$(skr iaas-api archive find --zone is1b \
    --query 'map(select(.Name | test("Ubuntu") and test("cloudimg"))) | sort_by(-.ID) | .[0].ID')
$ DISK_PLAN_ID=$(skr iaas-api product disk find --zone is1b \
    --query '.DiskPlans | map(select(.Name | test("SSD"))) | sort_by(.ID) | .[0].ID')
$ echo "$CLOUDIMG_ARCHIVE_ID, $DISK_PLAN_ID"
```

## 作成するリソースと順序

1. SSH 接続用の一時鍵と Server 用 user-data を作成します。
2. Server A と B を作成し、ネットワークスイートのサブネットグループとサブネット2個を作成します。
3. Server の未接続 NIC をそれぞれ別のサブネットへ接続します。
4. Server を起動し、割り当てられた IPv4 アドレスを使って Server A から Server B へ TCP 接続します。
5. Server を停止し、接続、Server と Disk、サブネット、サブネットグループの順に削除します。

## Step 1: コマンドヘルプと Archive を確認する

```console
$ skr networking-suite-api --help
$ skr iaas-api archive read --help
$ skr networking-suite-api subnet-group list --help
$ skr networking-suite-api subnet-group read --help
$ skr networking-suite-api subnet-group create --help
$ skr networking-suite-api subnet-group update --help
$ skr networking-suite-api subnet-group delete --help
$ skr networking-suite-api subnet list --help
$ skr networking-suite-api subnet read --help
$ skr networking-suite-api subnet create --help
$ skr networking-suite-api subnet update --help
$ skr networking-suite-api subnet delete --help
$ skr networking-suite-api interface-connection create --help
$ skr networking-suite-api interface-connection delete --help
$ skr networking-suite-api address list --help
$ skr networking-suite-api address read --help
$ skr iaas-api server create --help
$ skr iaas-api server boot --help
$ skr iaas-api server read --help
$ skr iaas-api server delete --help
$ skr iaas-api server find --help
$ skr iaas-api disk find --help
```

使用する Archive を読み取り、`Name` に `(cloudimg)` を含む Ubuntu アーカイブであることを確認します。通常の OS Archive では cloud-init user-data が処理されないことがあります。

```console
$ skr iaas-api archive read --zone is1b --id "$CLOUDIMG_ARCHIVE_ID" \
    --query '{ID,Name,Description,Tags,SizeMB}'
```

表示された Archive の `ID`、`Name`、`Description` を確認してください。通常の OS Archive や他の OS の cloudimg は使わないでください。

ネットワークスイートの SDK endpoint は is1c が既定です。ここでは環境変数で is1b に切り替え、IaaS の Server コマンドにも `--zone is1b` を指定します。

## Step 2: SSH 鍵と user-data を用意する

秘密鍵はローカルだけに保存します。Server A には OpenSSH server を導入し、Server B には疎通確認用の TCP listener を設定します。user-data ファイルには公開鍵だけを含めます。

```console
$ umask 077
$ WORK_DIR=$(mktemp -d)
$ RUN_ID=$(date +%s)
$ KEY_PATH="$WORK_DIR/networking-suite-key"
$ ssh-keygen -t rsa -b 3072 -N '' -f "$KEY_PATH"
$ SSH_PUBLIC_KEY=$(cat "$KEY_PATH.pub")
$ ENTRY_USER_DATA="$WORK_DIR/entry-user-data.yaml"
$ PEER_USER_DATA="$WORK_DIR/peer-user-data.yaml"
```

Server B の listener は TCP 38172 番で `ping` を受け取り、`PONG networking-suite` を返します。`shutdown` を受け取ると Server を停止します。

```console
$ cat > "$PEER_USER_DATA" <<'EOF'
#cloud-config
hostname: nstest6-peer
timezone: Asia/Tokyo
ssh_pwauth: false
chpasswd:
  expire: false
write_files:
  - path: /usr/local/bin/skr-networking-suite-listener.py
    permissions: "0755"
    content: |
      #!/usr/bin/python3
      import socket
      import subprocess

      with socket.socket() as listener:
          listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
          listener.bind(("0.0.0.0", 38172))
          listener.listen()
          while True:
              conn, _ = listener.accept()
              with conn:
                  command = conn.makefile("r").readline().strip()
                  if command == "ping":
                      conn.sendall(b"PONG networking-suite\n")
                  elif command == "shutdown":
                      conn.sendall(b"stopping\n")
                      subprocess.Popen(["/usr/bin/systemctl", "poweroff"])
                      break
  - path: /etc/systemd/system/skr-networking-suite-listener.service
    permissions: "0644"
    content: |
      [Unit]
      Description=Temporary Networking Suite listener
      After=network-online.target
      [Service]
      ExecStart=/usr/bin/python3 /usr/local/bin/skr-networking-suite-listener.py
      Restart=always
      [Install]
      WantedBy=multi-user.target
runcmd:
  - [systemctl, daemon-reload]
  - [systemctl, enable, --now, skr-networking-suite-listener.service]
EOF
```

## Step 3: Server A と Server B を作成する

名前には重複しない値を設定します。Server A は共有セグメントに接続する NIC が先頭で、2つ目の NIC はネットワークスイート接続用です。Server B はネットワークスイート接続用 NIC だけを持ちます。

```console
$ ENTRY_NAME="skr-nws-tutorial-$RUN_ID-entry"
$ PEER_NAME="skr-nws-tutorial-$RUN_ID-peer"
$ DESCRIPTION="Temporary Networking Suite tutorial resource"
$ skr iaas-api server create \
    --zone is1b --name "$ENTRY_NAME" --description "$DESCRIPTION" \
    --cpu 1 --memory-gb 1 \
    --disk "archive-id=$CLOUDIMG_ARCHIVE_ID,plan-id=$DISK_PLAN_ID,size-gb=20" \
    --nic 'upstream=shared' --nic 'upstream=disconnected'
$ skr iaas-api server create \
    --zone is1b --name "$PEER_NAME" --description "$DESCRIPTION" \
    --cpu 1 --memory-gb 1 \
    --disk "archive-id=$CLOUDIMG_ARCHIVE_ID,plan-id=$DISK_PLAN_ID,size-gb=20" \
    --nic 'upstream=disconnected'
```

各作成結果から Server ID を控え、`ENTRY_ID` と `PEER_ID` に設定します。`server read` で NIC ID を確認し、`UpstreamType` が `none` の NIC を控えます。サーバ API の `none` は、作成フラグで指定した未接続 NIC に対応します。

```console
$ ENTRY_ID=ENTRY_SERVER_ID
$ PEER_ID=PEER_SERVER_ID
$ skr iaas-api server read --zone is1b --id "$ENTRY_ID" \
    --query '{ID,Name,InstanceStatus,Interfaces}'
$ skr iaas-api server read --zone is1b --id "$PEER_ID" \
    --query '{ID,Name,InstanceStatus,Interfaces}'
```

`ENTRY_NIC_ID` と `PEER_NIC_ID` をそれぞれの `UpstreamType: "none"` の NIC の ID に置き換えます。Server A の同じ NIC に表示された `MACAddress` も `ENTRY_NWS_MAC` に設定します。

```console
$ ENTRY_NIC_ID=ENTRY_DISCONNECTED_NIC_ID
$ ENTRY_NWS_MAC=ENTRY_DISCONNECTED_NIC_MAC_ADDRESS
$ PEER_NIC_ID=PEER_DISCONNECTED_NIC_ID
$ ENTRY_NWS_MAC=$(printf '%s' "$ENTRY_NWS_MAC" | tr '[:upper:]' '[:lower:]')
```

Server A は共有 NIC と Networking Suite NIC を持つため、Networking Suite NIC の MAC アドレスに一致する DHCP 設定を user-data に含めます。SSH 公開鍵は cloudimg Archive のデフォルトユーザー `ubuntu` に設定します。NIC の MAC アドレスを条件にすることで、共有 NIC の設定を変えずに NWS 側の NIC を DHCP で構成します。

```console
$ cat > "$ENTRY_USER_DATA" <<EOF
#cloud-config
hostname: nstest6
timezone: Asia/Tokyo
ssh_pwauth: false
chpasswd:
  expire: false
packages:
  - openssh-server
users:
  - default
ssh_authorized_keys:
  - ${SSH_PUBLIC_KEY}
write_files:
  - path: /etc/netplan/60-networking-suite.yaml
    permissions: "0600"
    content: |
      network:
        version: 2
        ethernets:
          networking-suite:
            match:
              macaddress: "${ENTRY_NWS_MAC}"
            dhcp4: true
            dhcp4-overrides:
              route-metric: 200
runcmd:
  - [netplan, generate]
  - [netplan, apply]
EOF
```

## Step 4: サブネットグループとサブネットを作成する

サブネットグループはリージョン単位、サブネットはその配下のゾーン別ネットワークです。サブネットの IPv4 範囲は親の範囲内に置き、他のサブネットと重複させません。

```console
$ GROUP_NAME="skr-nws-tutorial-$RUN_ID"
$ skr networking-suite-api subnet-group create \
    --name "$GROUP_NAME" \
    --description "$DESCRIPTION" \
    --ipv4-address-range-cidr 10.0.0.0/16 \
    --region is1
```

作成結果の `SRN` を `GROUP_SRN` に設定します。グループの名前を更新し、`read` で値を確認します。

```console
$ GROUP_SRN=SUBNET_GROUP_SRN
$ skr networking-suite-api subnet-group update "$GROUP_SRN" \
    --name "$GROUP_NAME-updated" --description "$DESCRIPTION"
$ skr networking-suite-api subnet-group read "$GROUP_SRN" \
    --query '{SRN,Name,Description,IPv4AddressRangeCIDR,Region}'
```

次に、同じ `/16` 内で重複しない `/24` サブネットを2つ作成します。

```console
$ skr networking-suite-api subnet create \
    --name "$GROUP_NAME-subnet-1" --description "$DESCRIPTION" \
    --ipv4-address-range-cidr 10.0.1.0/24 --zone is1b \
    --subnet-group-srn "$GROUP_SRN"
$ skr networking-suite-api subnet create \
    --name "$GROUP_NAME-subnet-2" --description "$DESCRIPTION" \
    --ipv4-address-range-cidr 10.0.2.0/24 --zone is1b \
    --subnet-group-srn "$GROUP_SRN"
```

作成結果の `SRN` を `ENTRY_SUBNET_SRN` と `PEER_SUBNET_SRN` に設定します。1つ目のサブネットの名前を更新し、一覧で両方のサブネットを確認します。

```console
$ ENTRY_SUBNET_SRN=FIRST_SUBNET_SRN
$ PEER_SUBNET_SRN=SECOND_SUBNET_SRN
$ skr networking-suite-api subnet update "$ENTRY_SUBNET_SRN" \
    --name "$GROUP_NAME-subnet-1-updated" --description "$DESCRIPTION"
$ skr networking-suite-api subnet list "$GROUP_SRN" \
    --query 'map({SRN,Name,Description,IPv4AddressRangeCIDR,Zone,SubnetGroup})'
```

一覧は配列で返ります。以下は実際のライブ応答から ID、SRN、名前を置き換えた出力例です。

```json
[
  {
    "SRN": "<ENTRY-SUBNET-SRN>",
    "Name": "skr-nws-tutorial-<RUN-ID>-subnet-1-updated",
    "Description": "Temporary Networking Suite tutorial resource",
    "IPv4AddressRangeCIDR": "10.0.1.0/24",
    "Zone": {"Code": "is1b"},
    "SubnetGroup": {"SRN": "<SUBNET-GROUP-SRN>"}
  },
  {
    "SRN": "<PEER-SUBNET-SRN>",
    "Name": "skr-nws-tutorial-<RUN-ID>-subnet-2",
    "Description": "Temporary Networking Suite tutorial resource",
    "IPv4AddressRangeCIDR": "10.0.2.0/24",
    "Zone": {"Code": "is1b"},
    "SubnetGroup": {"SRN": "<SUBNET-GROUP-SRN>"}
  }
]
```

## Step 5: NIC を各サブネットへ接続する

NIC の SRN は、ゾーンと `server read` で確認した NIC ID から組み立てます。接続先サブネットを指定し、IPv4 アドレスを省略すると空きアドレスが割り当てられます。

```console
$ ENTRY_INTERFACE_SRN="srnv1:sakura-is1b:sakura.iaas.interface:$ENTRY_NIC_ID"
$ PEER_INTERFACE_SRN="srnv1:sakura-is1b:sakura.iaas.interface:$PEER_NIC_ID"
$ skr networking-suite-api interface-connection create \
    --interface-srn "$ENTRY_INTERFACE_SRN" --subnet-srn "$ENTRY_SUBNET_SRN"
$ skr networking-suite-api interface-connection create \
    --interface-srn "$PEER_INTERFACE_SRN" --subnet-srn "$PEER_SUBNET_SRN"
```

Create の応答にはインターフェースコネクションの SRN と割り当てられた IPv4 アドレスが含まれます。次はライブ API 応答から値を置き換えた例です。

```json
{
  "SRN": "<PEER-INTERFACE-CONNECTION-SRN>",
  "EphemeralIPv4Address": "<PEER-NWS-IP>"
}
```

各結果の `SRN` と `EphemeralIPv4Address` を控えます。`ENTRY_CONNECTION_SRN` と `PEER_CONNECTION_SRN`、`PEER_NWS_IP` を設定します。アドレス一覧と参照コマンドでも割り当てを確認できます。

```console
$ ENTRY_CONNECTION_SRN=ENTRY_INTERFACE_CONNECTION_SRN
$ PEER_CONNECTION_SRN=PEER_INTERFACE_CONNECTION_SRN
$ PEER_NWS_IP=PEER_EPHEMERAL_IPV4_ADDRESS
$ skr networking-suite-api address list "$ENTRY_SUBNET_SRN" \
    --interface-connection-srn "$ENTRY_CONNECTION_SRN" \
    --query 'map({SRN,IPAddress,Subnet})'
$ skr networking-suite-api address list "$PEER_SUBNET_SRN" \
    --interface-connection-srn "$PEER_CONNECTION_SRN" \
    --query 'map({SRN,IPAddress,Subnet})'
$ PEER_ADDRESS_SRN=PEER_ADDRESS_SRN
$ skr networking-suite-api address read "$PEER_ADDRESS_SRN" \
    --query '{SRN,IPAddress,Subnet}'
```

次の出力例は、上から Server A、Server B のサブネットを検索したライブ API 応答です。SRN と IPv4 アドレスは置き換えています。

```json
[
  {
    "SRN": "<ENTRY-ADDRESS-SRN>",
    "IPAddress": "<ENTRY-NWS-IP>",
    "Subnet": {"SRN": "<ENTRY-SUBNET-SRN>"}
  }
]
```

```json
[
  {
    "SRN": "<PEER-ADDRESS-SRN>",
    "IPAddress": "<PEER-NWS-IP>",
    "Subnet": {"SRN": "<PEER-SUBNET-SRN>"}
  }
]
```

## Step 6: Server を起動し、別サブネット間の TCP 通信を確認する

user-data を設定して Server A と B を起動します。`@path.yaml` は user-data ファイルを読み込む構文です。公開鍵以外の秘密情報を user-data や引数へ含めないでください。Server A では netplan が DHCP から Networking Suite の NIC アドレスとサブネットグループ向け経路を設定します。共有 NIC のデフォルトルートを優先するため、Networking Suite NIC の route metric を `200` にしています。

```console
$ skr iaas-api server boot --zone is1b --id "$ENTRY_ID" \
    --user-data "@$ENTRY_USER_DATA"
$ skr iaas-api server boot --zone is1b --id "$PEER_ID" \
    --user-data "@$PEER_USER_DATA"
```

Server A の `shared` NIC に割り当てられた `IPAddress` を `ENTRY_SHARED_IP` に設定し、`InstanceStatus` が起動状態になるまで待ちます。次のコマンドは Server A へ SSH 接続し、別サブネットにある Server B の listener へ TCP 接続します。

```console
$ ENTRY_SHARED_IP=ENTRY_SHARED_IPV4_ADDRESS
$ ssh -i "$KEY_PATH" -o IdentitiesOnly=yes \
    -o StrictHostKeyChecking=accept-new "ubuntu@$ENTRY_SHARED_IP" \
    python3 - "$PEER_NWS_IP" <<'PY'
import socket
import sys

with socket.create_connection((sys.argv[1], 38172), timeout=10) as connection:
    connection.sendall(b"ping\n")
    print(connection.makefile("r").readline().strip())
PY
```

応答が `PONG networking-suite` なら、Server A から別サブネットに接続した Server B へ TCP 通信できています。サブネットグループに対応するルーティング情報と DHCP によるネットワーク設定を利用します。

## Step 7: 作成したリソースを削除する

Server B の listener に `shutdown` を送信し、Server A を停止します。停止状態は `server read` の `InstanceStatus` で確認してください。Server が停止してからインターフェースコネクションを削除します。

```console
$ ssh -i "$KEY_PATH" -o IdentitiesOnly=yes \
    "ubuntu@$ENTRY_SHARED_IP" python3 - "$PEER_NWS_IP" <<'PY'
import socket
import sys

with socket.create_connection((sys.argv[1], 38172), timeout=10) as connection:
    connection.sendall(b"shutdown\n")
    print(connection.makefile("r").readline().strip())
PY
$ ssh -i "$KEY_PATH" -o IdentitiesOnly=yes \
    "ubuntu@$ENTRY_SHARED_IP" \
    "sudo sh -c 'sleep 2; systemctl poweroff' >/dev/null 2>&1 </dev/null &"
```

両 Server の `InstanceStatus` が停止状態になったら、接続を解除し、アドレスが残っていないことを確認します。

```console
$ skr networking-suite-api interface-connection delete "$PEER_CONNECTION_SRN"
$ skr networking-suite-api interface-connection delete "$ENTRY_CONNECTION_SRN"
$ skr networking-suite-api address list "$PEER_SUBNET_SRN" \
    --query 'map({SRN,IPAddress,Subnet})'
$ skr networking-suite-api address list "$ENTRY_SUBNET_SRN" \
    --query 'map({SRN,IPAddress,Subnet})'
```

どちらのアドレス一覧も空配列になります。ネットワークスイート側の接続を削除してから、この手順で作成した Server と Disk を削除します。対象の ID と名前を再確認して `--with-disks` を指定してください。

```json
[]
```

```json
[]
```

```console
$ skr iaas-api server delete --zone is1b --id "$PEER_ID" \
    --with-disks --fail-if-not-found
$ skr iaas-api server delete --zone is1b --id "$ENTRY_ID" \
    --with-disks --fail-if-not-found
$ skr iaas-api server find --zone is1b --name "$PEER_NAME" \
    --query 'map({ID,Name,Description})'
```

```json
[]
```

```console
$ skr iaas-api server find --zone is1b --name "$ENTRY_NAME" \
    --query 'map({ID,Name,Description})'
```

```json
[]
```

両 Server が見つからないことを確認してから、サブネットを削除します。

```console
$ skr networking-suite-api subnet delete "$PEER_SUBNET_SRN"
$ skr networking-suite-api subnet delete "$ENTRY_SUBNET_SRN"
$ skr networking-suite-api subnet list "$GROUP_SRN" \
    --query 'map({SRN,Name,Description,IPv4AddressRangeCIDR,Zone,SubnetGroup})'
```

削除後のサブネット一覧は `[]` です。これを確認してからサブネットグループを削除します。

```json
[]
```

```console
$ skr networking-suite-api subnet-group delete "$GROUP_SRN"
$ skr networking-suite-api subnet-group list \
    --query "map(select(.SRN == \"$GROUP_SRN\") | {SRN,Name,Description,IPv4AddressRangeCIDR,Region})"
```

今回作成した `GROUP_SRN` に絞った一覧が `[]` であることを確認します。他のグループは削除しません。

```json
[]
```

最後に、Disk が残っていないことを検索します。

```console
$ skr iaas-api disk find --zone is1b --name "$PEER_NAME" \
    --query 'map({ID,Name,ServerID})'
```

```json
[]
```

```console
$ skr iaas-api disk find --zone is1b --name "$ENTRY_NAME" \
    --query 'map({ID,Name,ServerID})'
```

```json
[]
```

削除確認の各検索結果が `[]` であることを確認します。SSH 鍵と user-data は不要になったらローカルから削除してください。

```console
$ rm "$KEY_PATH" "$KEY_PATH.pub" "$ENTRY_USER_DATA" "$PEER_USER_DATA"
$ rmdir "$WORK_DIR"
```

## 参考資料

- [ネットワークスイートの概要と仕様](https://manual.sakura.ad.jp/cloud/network/networking-suite/about.html)
- [ネットワークスイートの基本操作](https://manual.sakura.ad.jp/cloud/network/networking-suite/instruction-connecting-nic.html)
- [ネットワークスイート](https://manual.sakura.ad.jp/cloud/network/networking-suite/index.html)
- [Cloud-init 対応パブリックアーカイブから Server を作成する](https://manual.sakura.ad.jp/cloud/server/cloud-init.html)
- [Server API チュートリアル](iaas-api/server.md)
- [sacloud-sdk-go v0.3.0 Networking Suite API](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/api/networking-suite)
- [sacloud-sdk-go v0.3.0 Networking Suite API models](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/api/networking-suite/apis/v1)
