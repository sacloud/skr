# IaaS API: データベースを作成し、運用する

[データベース](https://manual.sakura.ad.jp/cloud/appliance/database/index.html)は、さくらのクラウドが提供する MariaDB と PostgreSQL のマネージドデータベースアプライアンスです。冗長化構成やバックアップ、Web UI、ディスク暗号化などの機能を備え、[スイッチ](https://manual.sakura.ad.jp/cloud/network/switch/about.html)を用いて作成したプライベートネットワークに接続して利用します。

> [!WARNING]
> この手順では、スイッチ 1 件とデータベース 1 件 (10GB プラン) を作成し、設定を更新してから削除します。データベースは作成から削除まで課金対象です。提供料金は[サービスサイト](https://manual.sakura.ad.jp/cloud/appliance/database/index.html)を参照し、実行前に `skr config current` でプロファイルを確認し、影響範囲について必要な承認を得てください。

## 前提条件

- `skr iaas-api database` が利用でき、対象プロジェクトの認証プロファイルが設定されていること。スイッチとデータベースを作成・削除できる権限も必要です。
- データベースを作成するゾーンに、データベースを接続するスイッチを持たないこと。スイッチの作成には `skr iaas-api switch` を使います。
- データベースの接続先となるプライベートネットワークのサブネットを決めること。この手順では `192.168.0.0/24` を使います。

## 作成するリソースと順序

1. データベースの接続先となるスイッチを 1 件作成します。
2. データベース (MariaDB、10GB プラン) を 1 件作成します。作成リクエストは起動の完了を待たないため、作成後に起動状態を確認します。
3. 作成したデータベースを参照し、名前の部分一致検索に含まれることを確認します。
4. 設定パラメータの一覧とモニタリングデータを確認し、説明を更新します。
5. 電源を停止・起動します。
6. ID と所属を照合したうえで、データベースを削除します。
7. スイッチを削除します。

## Step 1: 対象プロファイルとコマンドを確認する

```console
$ skr config current
$ skr iaas-api switch --help
$ skr iaas-api database --help
$ skr iaas-api database create --help
$ skr iaas-api database read --help
$ skr iaas-api database find --help
$ skr iaas-api database list-parameter --help
$ skr iaas-api database monitor-database --help
$ skr iaas-api database monitor-cpu --help
$ skr iaas-api database shutdown --help
$ skr iaas-api database boot --help
$ skr iaas-api database update --help
$ skr iaas-api database delete --help
```

認証は選択中の SDK プロファイルを使います。以降の `SWITCH-NAME`、`DB-NAME` は、他のリソースと重複しない、自分のテスト専用の名前に置き換えてください。

## Step 2: 接続先スイッチを作成する

データベースの接続先となるスイッチを作成します。以下の例では `is1b` を使います。

```console
$ skr iaas-api switch create --zone is1b --name SWITCH-NAME --description "Temporary switch for database tutorial"
```

```json
{
  "ID": "SWITCH-ID",
  "Name": "SWITCH-NAME",
  "Description": "Temporary switch for database tutorial"
}
```

出力例は既存リソースを除いて編集しています。作成結果の `ID` を控え、以降の `SWITCH-ID` に指定してください。作成コマンドがエラーになり ID を取得できなかった場合は、名前だけを手掛かりに削除しないでください。作成された可能性がある場合は、`iaas-api switch find` やコントロールパネルで対象の ID と内容を確認し、安全に特定できるまで操作を止めてください。

スイッチを作成したら、ID をシェル変数に取り込みます。以降のコマンドは `SWITCH_ID` 変数を使います。

```console
$ SWITCH_ID=$(skr iaas-api switch find --zone is1b --query 'map(select(.Name == "SWITCH-NAME")) | map({ID,Name}) | .[0].ID')
```

```json
"SWITCH-ID"
```

## Step 3: データベースプランの ID を決める

データベースプランの ID はディスク容量に対応します。10: 10GB (1core-2gb)、30: 30GB (2core-4gb)、90: 90GB (4core-8gb)、240: 240GB (8core-16gb)、500: 500GB、1000: 1TB です。プラン ID はゾーンによらず同じ値を使えます。この手順では 10GB プラン (`--plan-id 10`) を使います。プランの構成は[サービスサイト](https://manual.sakura.ad.jp/cloud/appliance/database/index.html)を参照してください。

冗長化構成 (レプリケーション) やプロキシ構成のプランは別のプラン ID になります。必要に応じて `skr iaas-api database create --help` の `--request` 項目と[サービスサイト](https://manual.sakura.ad.jp/cloud/appliance/database/database-redundancy.html)を参照してください。

## Step 4: データベースを作成する

パスワードをファイルに保存します。パスワードをコマンド引数の値として渡せないため、保護したファイルから読み込みます。末尾の改行は読み込み時に取り除かれます。`echo` でパスワードを書き込むと値がシェルの履歴に残るため、`read -s` で入力するかエディタで作成してください。

```console
$ umask 077
$ read -rs -p "Password: " DB_PASSWORD && printf '%s' "$DB_PASSWORD" > db-password.txt && unset DB_PASSWORD
```

`read -s` で入力したパスワードは表示されず、シェルの履歴にも残りません。`DB_PASSWORD` は、利用者の責任で十分な強度の値に置き換えてください。エディタで作成する場合も、同じく保護したファイルに保存してください。データベースを作成します。作成リクエストは作成を受け付けた時点で応答を返し、起動の完了は待ちません。起動には数分から十数分かかります。

```console
$ skr iaas-api database create --zone is1b --name DB-NAME \
    --description "Temporary database for tutorial" \
    --tags tutorial,managed \
    --plan-id 10 --switch-id "$SWITCH_ID" \
    --ip-address 192.168.0.11 --network-mask-len 24 \
    --database-type mariadb \
    --username dbuser --password @db-password.txt
```

```json
{
  "ID": "DB-ID",
  "Name": "DB-NAME",
  "Description": "Temporary database for tutorial",
  "Tags": ["tutorial", "managed"],
  "Availability": "available",
  "PlanID": 10,
  "SwitchID": "SWITCH-ID",
  "NetworkMaskLen": 24,
  "IPAddresses": ["192.168.0.11"],
  "CommonSetting": {
    "DefaultUser": "dbuser"
  }
}
```

出力例は `Instance` や `Settings` などの既存項目を除いて編集しています。作成結果の `ID` を控え、シェル変数に取り込みます。以降の参照や変更操作はこの ID を手掛かりに実行します。`作成応答のID` は、控えた作成結果の `ID` に置き換えてください。

```console
$ CREATED_DB_ID=作成応答のID
```

データベースを作成したら、find で検索し、作成応答の `ID` と接続先スイッチが一致する 1 件を確認してから ID を取り込みます。作成が失敗していたり同名の既存データベースが残っていたりした場合に、誤った対象を後続の変更操作に進めないためです。

```console
$ DB_ID=$(skr iaas-api database find --zone is1b --name DB-NAME \
    --query 'map(select(.Name == "DB-NAME" and .ID == '"$CREATED_DB_ID"' and .SwitchID == '"$SWITCH_ID"')) | if length == 1 then .[0].ID else error("created database not found") end')
```

`--name` は部分一致検索のため、結果には名前の一部が一致する別のデータベースも含まれることがあります。作成応答の `ID` と接続先スイッチの `ID` で絞り込み、結果が 1 件のときだけ ID を取り込みます。複数件ある場合や 0 件の場合は、コントロールパネルで対象の ID と内容を確認し、安全に特定できるまで操作を止めてください。

```json
"DB-ID"
```

作成後にパスワードファイルを削除します。

```console
$ rm db-password.txt
```

作成したデータベースが起動するまで待ちます。`read` でトップレベルの `InstanceStatus` が `up` になるまで再実行してください。以降の参照やモニタリングは起動後に行います。

```console
$ skr iaas-api database read --zone is1b --id "$DB_ID" --query '{ID,Name,Status: .InstanceStatus}'
```

```json
{
  "ID": "DB-ID",
  "Name": "DB-NAME",
  "Status": "up"
}
```

### 作成後に変更できる項目と JSON 入力

`create` は、説明やタグ、バックアップ設定、冗長化 (レプリケーション)、Web UI、ディスク暗号化などの項目もフラグで指定できます。フラグで表せない複雑な値は `--request` で JSON を渡します。パスワードを含むため、`--request` は `@path.json` 形式または `-` (標準入力) のみを受け付け、インラインの JSON は指定できません。`--request` と個別フラグは併用できません。

冗長化を有効にする場合は `--enable-replication` とレプリカ側パスワード (`--replica-user-password @replica-password.txt`) が必須で、`--ip-address` を 2 個指定します。バックアップを有効にする場合は `--enable-backup` と取得日 (`--backup-weekday mon,tue` など)、取得開始時刻を指定します。ディスク暗号化を有効にする場合はアルゴリズム (`--disk-encryption-algorithm aes256_xts`) と KMS キー ID (`--disk-encryption-kms-key`) の両方を指定します。

## Step 5: 作成したデータベースを参照する

```console
$ skr iaas-api database read --zone is1b --id "$DB_ID" --query '{ID,Name,Description,Tags,Availability,PlanID,SwitchID,NetworkMaskLen,IPAddresses,Status: .InstanceStatus}'
```

```json
{
  "ID": "DB-ID",
  "Name": "DB-NAME",
  "Description": "Temporary database for tutorial",
  "Tags": ["tutorial", "managed"],
  "Availability": "available",
  "PlanID": 10,
  "SwitchID": "SWITCH-ID",
  "NetworkMaskLen": 24,
  "IPAddresses": ["192.168.0.11"],
  "Status": "up"
}
```

接続先スイッチ、割り当てた IP アドレス、ユーザー名が指定した内容と一致することを確認します。

名前の部分一致検索でも同じデータベースが返ることを確認します。`--name` は部分一致で、1 要素の API `Names` に変換されます。

```console
$ skr iaas-api database find --zone is1b --name DB-NAME --query 'map({ID,Name,Status: .InstanceStatus})'
```

```json
[
  {
    "ID": "DB-ID",
    "Name": "DB-NAME",
    "Status": "up"
  }
]
```

## Step 6: 設定パラメータとモニタリングデータを確認する

データベースの設定パラメータの一覧を取得します。

```console
$ skr iaas-api database list-parameter --zone is1b --id "$DB_ID"
```

```json
[
  {
    "Key": "innodb_buffer_pool_size",
    "Value": "128M",
    "Meta": "global"
  }
]
```

出力例は既存項目を除いて編集しています。パラメータの意味は [MariaDB](https://manual.sakura.ad.jp/cloud/appliance/database/mariadb.html) と [PostgreSQL](https://manual.sakura.ad.jp/cloud/appliance/database/postgresql.html) のサービスドキュメントを参照してください。

モニタリングデータを取得します。取得期間は `--start` と `--end` (RFC3339 形式) で指定できます。未指定の場合は直近 1 時間のデータを取得します。起動直後はまだデータが存在しない場合もあります。

```console
$ skr iaas-api database monitor-database --zone is1b --id "$DB_ID"
```

```json
[
  {
    "Time": "2026-01-01T00:10:00+09:00",
    "TotalMemorySize": 2147483648,
    "UsedMemorySize": 834666496,
    "TotalDisk1Size": 10737418240,
    "UsedDisk1Size": 322122547,
    "BinlogUsedSizeKiB": 0
  }
]
```

出力例は既存項目を除いて編集しています。CPU、ディスク、インターフェースのモニタリングも同じ要領で取得できます。

```console
$ skr iaas-api database monitor-cpu --zone is1b --id "$DB_ID"
$ skr iaas-api database monitor-disk --zone is1b --id "$DB_ID"
$ skr iaas-api database monitor-interface --zone is1b --id "$DB_ID"
```

## Step 7: 説明を更新する

データベースの設定を更新します。`update` は対象を `--zone` と `--id` で指定し、更新内容は `--request` で JSON のみを受け付けます。送信した内容で設定が置き換わるため、既存の設定を維持したい項目は JSON に再指定してください。read の出力を基に現在の設定を含む JSON を組み立てるのが確実です。ここでは read の出力から説明だけを変更した JSON を組み立てます。

```json
{
  "Name": "DB-NAME",
  "Description": "Temporary database for tutorial (updated)",
  "Tags": ["tutorial", "managed"],
  "CommonSetting": {
    "DefaultUser": "dbuser",
    "UserPassword": "DB-PASSWORD",
    "ServicePort": 3306
  }
}
```

`Name`、`Description`、`Tags`、`CommonSetting` は、read の出力で確認した現在の値を指定します。read の応答の `UserPassword` はマスク済みの値で返るため、`CommonSetting` のパスワードには実際の値を指定します。API はマスク済みの値を受け付けません。上記の内容を `DB-NAME` と説明を置き換えて `db-update.json` として保存してください。パスワードを含むため、保存したファイルの取り扱いに注意し、用が済んだら削除してください。

```console
$ skr iaas-api database update --zone is1b --id "$DB_ID" --request @db-update.json
```

更新後の説明を参照で確認します。反映には時間を要することがあります。期待する説明が返るまで再実行してください。

```console
$ skr iaas-api database read --zone is1b --id "$DB_ID" --query '{ID,Description}'
```

```json
{
  "ID": "DB-ID",
  "Description": "Temporary database for tutorial (updated)"
}
```

## Step 8: 電源を停止・起動する

データベースを停止します。`shutdown` は電源 OFF が完了してからコマンドが返ります。停止中はデータベースに接続できません。

```console
$ skr iaas-api database shutdown --zone is1b --id "$DB_ID"
```

データベースを起動します。`boot` は起動が完了してからコマンドが返ります。

```console
$ skr iaas-api database boot --zone is1b --id "$DB_ID"
```

電源操作の反映には時間を要することがあります。`read` でトップレベルの `InstanceStatus` が期待する状態 (`up`、`down`) が返るまで再実行してください。

データベースを再起動する場合は `reset` を使います。再起動中もデータベースには接続できません。

```console
$ skr iaas-api database reset --zone is1b --id "$DB_ID"
```

## Step 9: データベースとスイッチを削除する

削除前に ID と接続先スイッチを照合し、この手順で作成したデータベースであることを確認します。

```console
$ skr iaas-api database read --zone is1b --id "$DB_ID" --query '{ID,Name,SwitchID}'
```

```json
{
  "ID": "DB-ID",
  "Name": "DB-NAME",
  "SwitchID": "SWITCH-ID"
}
```

`delete` は起動中のデータベースを削除できません。`--force` を付けると電源 OFF (強制終了) してから削除します。`delete` は応答本文を出力しません。

```console
$ skr iaas-api database delete --zone is1b --id "$DB_ID" --force
```

削除後、検索に存在しないことを確認します。出力は `[]` です。

```console
$ skr iaas-api database find --zone is1b --name DB-NAME --query 'map({ID,Name})'
```

```json
[]
```

削除の反映には時間を要することがあります。期待する出力が返るまで再実行してください。データベースの削除後に、接続先スイッチを削除します。削除前に ID と名前を照合してください。

```console
$ skr iaas-api switch read --zone is1b --id "$SWITCH_ID" --query '{ID,Name,Description}'
$ skr iaas-api switch delete --zone is1b --id "$SWITCH_ID"
```

## 参考情報

- [データベース](https://manual.sakura.ad.jp/cloud/appliance/database/index.html)
- [MariaDB](https://manual.sakura.ad.jp/cloud/appliance/database/mariadb.html)
- [PostgreSQL](https://manual.sakura.ad.jp/cloud/appliance/database/postgresql.html)
- [データベースの冗長化](https://manual.sakura.ad.jp/cloud/appliance/database/database-redundancy.html)
- [データベース / sacloud-sdk-go v0.3.1](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.1/service/iaas/database)
