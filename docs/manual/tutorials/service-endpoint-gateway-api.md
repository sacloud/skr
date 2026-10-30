# Service Endpoint Gateway API: ゲートウェイを作成し、設定を更新する

[サービスエンドポイントゲートウェイ](https://manual.sakura.ad.jp/cloud/network/switch/seg/about.html)は、[スイッチ](https://manual.sakura.ad.jp/cloud/network/switch/about.html)を用いて作成したプライベートネットワーク内から、オブジェクトストレージやコンテナレジストリなどのマネージドサービスへ閉域経路で安全に接続できるようにする、スイッチの拡張機能です。利用するサーバは参照する DNS をゲートウェイの IP アドレスへ変更するだけでよく、ルーティング設定の変更は不要です。スイッチ単体でのみ利用でき、ルータ＋スイッチでは作成できません。1 つのスイッチに対して複数のゲートウェイは有効化できません。

> [!WARNING]
> この手順では、スイッチ 1 件とサービスエンドポイントゲートウェイ 1 件を作成し、設定を更新してから削除します。サービスエンドポイントゲートウェイは作成から削除まで課金対象です。提供料金は[サービスサイト](https://manual.sakura.ad.jp/cloud/network/switch/seg/about.html)を参照し、実行前に `skr config current` でプロファイルを確認し、影響範囲について必要な承認を得てください。

## 前提条件

- `skr service-endpoint-gateway-api` が利用でき、対象プロジェクトの認証プロファイルが設定されていること。スイッチとゲートウェイを作成・削除できる権限も必要です。
- ゲートウェイを作成するスイッチを持たないこと。スイッチの作成には `skr iaas-api switch` を使います。
- 接続先のマネージドサービスと、接続先マネージドサービスを決めること。オブジェクトストレージ、コンテナレジストリ、モニタリングスイートなど、接続できるマネージドサービスは[サービスエンドポイントゲートウェイの概要](https://manual.sakura.ad.jp/cloud/network/switch/seg/about.html)を参照してください。
- API エンドポイントのゾーンは SDK 認証設定のゾーン (プロファイルの `Zone` または `SAKURA_ZONE` 環境変数) を使います。未設定の場合の既定値は is1a です。エンドポイントは `SAKURA_ENDPOINTS_SERVICE_ENDPOINT_GATEWAY` 環境変数で変更できます。

## 作成するリソースと順序

1. ゲートウェイの接続先となる通常のスイッチを 1 件作成します。
2. スイッチにサービスエンドポイントゲートウェイを 1 件作成します。
3. 作成したゲートウェイを参照し、一覧に含まれることを確認します。
4. ゲートウェイのインターフェースと電源状態を確認し、電源を停止・起動します。
5. 接続先マネージドサービスの設定を更新し、`apply` で反映します。
6. ID と所属を照合したうえで、ゲートウェイを削除します。
7. スイッチを削除します。

## Step 1: 対象プロファイルとコマンドを確認する

```console
$ skr config current
$ skr iaas-api switch --help
$ skr service-endpoint-gateway-api --help
$ skr service-endpoint-gateway-api appliance create --help
$ skr service-endpoint-gateway-api appliance read --help
$ skr service-endpoint-gateway-api appliance list --help
$ skr service-endpoint-gateway-api appliance read-interface --help
$ skr service-endpoint-gateway-api appliance read-power-status --help
$ skr service-endpoint-gateway-api appliance shutdown --help
$ skr service-endpoint-gateway-api appliance power-on --help
$ skr service-endpoint-gateway-api appliance update --help
$ skr service-endpoint-gateway-api appliance apply --help
$ skr service-endpoint-gateway-api appliance delete --help
```

認証は選択中の SDK プロファイルを使います。以降の `SWITCH-NAME`、`UPDATED-SWITCH-NAME` は、他のリソースと重複しない、自分のテスト専用の名前に置き換えてください。

## Step 2: 接続先スイッチを作成する

ゲートウェイの接続先となる通常のスイッチを作成します。ルータ＋スイッチではゲートウェイを利用できないため、スイッチ単体を作成してください。ゲートウェイは、SDK 認証設定のゾーン (プロファイルの `Zone` または `SAKURA_ZONE` 環境変数、未設定なら is1a) に作成されるため、スイッチも同じゾーンに作成してください。以下の例では `is1a` を使います。

```console
$ skr iaas-api switch create --zone is1a --name SWITCH-NAME --description "Temporary switch for service endpoint gateway tutorial"
```

```json
{
  "ID": "SWITCH-ID",
  "Name": "SWITCH-NAME",
  "Scope": "user",
  "Description": "Temporary switch for service endpoint gateway tutorial"
}
```

出力例は既存リソースを除いて編集しています。作成結果の `ID` を控え、以降の `SWITCH-ID` に指定してください。作成コマンドがエラーになり ID を取得できなかった場合は、名前だけを手掛かりに削除しないでください。作成された可能性がある場合は、`iaas-api switch find` やコントロールパネルで対象の ID と内容を確認し、安全に特定できるまで操作を止めてください。

スイッチを作成したら、ID をシェル変数に取り込みます。以降のコマンドは `SWITCH-ID` 変数を使います。

```console
$ SWITCH_ID=$(skr iaas-api switch find --zone is1a --query 'map(select(.Name == "SWITCH-NAME")) | map({ID,Name,Description}) | .[0].ID')
```

```json
"SWITCH-ID"
```

## Step 3: サービスエンドポイントゲートウェイを作成する

接続先スイッチにゲートウェイを作成します。`--switch-id` と `--network-mask-len` が必須で、`--server-ip` はゲートウェイに割り当てる IPv4 アドレス (ゲートウェイアドレス) を繰り返し指定できます。ネットマスク長と IP アドレスは、スイッチで構成するプライベートネットワークのサブネットを定義します。ここでは[クイックスタート](https://manual.sakura.ad.jp/cloud/network/switch/seg/getting_started.html)と同じ `192.168.2.0/24` を使います。

```console
$ skr service-endpoint-gateway-api appliance create --switch-id "$SWITCH_ID" --network-mask-len 24 --server-ip 192.168.2.110
```

```json
{
  "Appliance": {
    "ID": "APPLIANCE-ID",
    "Class": "serviceendpointgateway",
    "Availability": "available",
    "Remark": {
      "Switch": {
        "ID": "SWITCH-ID"
      },
      "Network": {
        "NetworkMaskLen": 24
      },
      "Servers": [
        {
          "IPAddress": "192.168.2.110"
        }
      ]
    }
  },
  "is_ok": true
}
```

出力例は `Instance` や `Interfaces` などの既存項目を除いて編集しています。作成結果の `Appliance.ID` を控え、以降の `APPLIANCE-ID` に指定してください。`Class` と `Plan` は自動設定されるため、指定する項目はありません。

接続先スイッチ、ネットマスク長、サーバー IP を JSON で指定する場合は、`--request` に `Appliance.Remark` を含めます。`--request` と個別フラグは併用できません。例えば次の内容を `seg-create.json` として保存し、ID とネットマスク長を置き換えてください。

```json
{
  "Appliance": {
    "Remark": {
      "Switch": {
        "ID": "SWITCH-ID"
      },
      "Network": {
        "NetworkMaskLen": 24
      },
      "Servers": [
        {
          "IPAddress": "192.168.2.110"
        }
      ]
    }
  }
}
```

```console
$ skr service-endpoint-gateway-api appliance create --request @seg-create.json
```

ゲートウェイを作成したら、ID をシェル変数に取り込みます。以降のコマンドは `APPLIANCE_ID` 変数を使います。

```console
$ APPLIANCE_ID=$(skr service-endpoint-gateway-api appliance read "$APPLIANCE_ID" --query .Appliance.ID)
```

初回は `APPLIANCE_ID` が未設定のため、作成コマンドの出力から `Appliance.ID` の値を指定してください。

## Step 4: 作成したゲートウェイを参照する

```console
$ skr service-endpoint-gateway-api appliance read "$APPLIANCE_ID" --query '.Appliance | {ID,Class,Availability,Remark,Status: .Instance.Status,ServiceClass}'
```

```json
{
  "ID": "APPLIANCE-ID",
  "Class": "serviceendpointgateway",
  "Availability": "available",
  "Remark": {
    "Switch": {
      "ID": "SWITCH-ID"
    },
    "Network": {
      "NetworkMaskLen": 24
    },
    "Servers": [
      {
        "IPAddress": "192.168.2.110"
      }
    ]
  },
  "Status": "up",
  "ServiceClass": "cloud/appliance/serviceendpointgateway/1"
}
```

一覧でも同じ内容が返ることを確認します。

```console
$ skr service-endpoint-gateway-api appliance list --query '.Appliances | map(select(.ID == "'"$APPLIANCE_ID"'") | {ID,Name,Switch: .Remark.Switch.ID,Status: .Instance.Status})'
```

```json
[
  {
    "ID": "APPLIANCE-ID",
    "Name": "Service Endpoint Gateway (APPLIANCE-ID)",
    "Switch": "SWITCH-ID",
    "Status": "up"
  }
]
```

## Step 5: 電源状態を確認する

ゲートウェイを利用するサーバは、参照する DNS をゲートウェイの IP アドレスへ設定します。

電源状態を確認し、電源を停止してから起動します。停止中はゲートウェイを経由した通信が行えません。

```console
$ skr service-endpoint-gateway-api appliance read-power-status "$APPLIANCE_ID" --query '{Status: .Instance.Status}'
```

```json
{
  "Status": "up"
}
```

```console
$ skr service-endpoint-gateway-api appliance shutdown "$APPLIANCE_ID"
$ skr service-endpoint-gateway-api appliance read-power-status "$APPLIANCE_ID" --query '{Status: .Instance.Status}'
```

```json
{
  "Status": "down"
}
```

```console
$ skr service-endpoint-gateway-api appliance power-on "$APPLIANCE_ID"
```

```json
{
  "Success": true,
  "is_ok": true
}
```

電源操作の反映には時間を要することがあります。`read-power-status` で期待する状態が返るまで再実行してください。

## Step 6: 接続先マネージドサービスの設定を更新する

ゲートウェイの接続先マネージドサービス設定を更新します。設定は `Settings` を全体置換するため、既存の設定を維持したい項目は JSON に再指定してください。オブジェクトストレージを接続先にする例を `seg-settings.json` として保存します。

```json
{
  "Appliance": {
    "Settings": {
      "ServiceEndpointGateway": {
        "EnabledServices": [
          {
            "Type": "ObjectStorage",
            "Config": {
              "Endpoints": ["s3.isk01.sakurastorage.jp"]
            }
          }
        ],
        "MonitoringSuite": {
          "Enabled": "True"
        }
      }
    }
  }
}
```

```console
$ skr service-endpoint-gateway-api appliance update "$APPLIANCE_ID" --request @seg-settings.json
$ skr service-endpoint-gateway-api appliance apply "$APPLIANCE_ID"
```

`update` は `Settings` の更新だけで、`apply` で設定が反映されます。接続先マネージドサービスの変更時、ゲートウェイを経由した通信が一時的に中断することがあります。反映後の設定は `read` で確認できます。

```console
$ skr service-endpoint-gateway-api appliance read "$APPLIANCE_ID" --query '{Settings: .Appliance.Settings,SettingsHash: .Appliance.SettingsHash}'
```

```json
{
  "Settings": {
    "ServiceEndpointGateway": {
      "EnabledServices": [
        {
          "Type": "ObjectStorage",
          "Config": {
            "Endpoints": ["s3.isk01.sakurastorage.jp"]
          }
        }
      ],
      "MonitoringSuite": {
        "Enabled": "True"
      }
    }
  },
  "SettingsHash": "SETTINGS-HASH"
}
```

接続先マネージドサービスごとの設定方法は、[クイックスタート](https://manual.sakura.ad.jp/cloud/network/switch/seg/getting_started.html)と[モニタリングスイート連携](https://manual.sakura.ad.jp/cloud/network/switch/seg/monitoring-suite.html)を参照してください。

## Step 7: ゲートウェイとスイッチを削除する

削除前に ID と接続先スイッチを照合し、この手順で作成したゲートウェイであることを確認します。`delete` は応答本文を出力しません。

```console
$ skr service-endpoint-gateway-api appliance read "$APPLIANCE_ID" --query '{ID: .Appliance.ID,Switch: .Appliance.Remark.Switch.ID}'
```

```json
{
  "ID": "APPLIANCE-ID",
  "Switch": "SWITCH-ID"
}
```

```console
$ skr service-endpoint-gateway-api appliance delete "$APPLIANCE_ID"
```

削除後、一覧に存在しないことを確認します。出力は `[]` です。

```console
$ skr service-endpoint-gateway-api appliance list --query '.Appliances | map(select(.ID == "'"$APPLIANCE_ID"'") | {ID,Name})'
```

```json
[]
```

削除の反映には時間を要することがあります。期待する出力が返るまで再実行してください。ゲートウェイの削除後に、接続先スイッチを削除します。削除前に ID と名前を照合してください。

```console
$ skr iaas-api switch read --zone is1a "$SWITCH_ID" --query '{ID,Name,Description}'
$ skr iaas-api switch delete --zone is1a "$SWITCH_ID"
```

## 参考情報

- [サービスエンドポイントゲートウェイの概要](https://manual.sakura.ad.jp/cloud/network/switch/seg/about.html)
- [クイックスタート](https://manual.sakura.ad.jp/cloud/network/switch/seg/getting_started.html)
- [モニタリングスイート連携](https://manual.sakura.ad.jp/cloud/network/switch/seg/monitoring-suite.html)
- [コントロールパネル操作ガイド](https://manual.sakura.ad.jp/cloud/network/switch/seg/operation.html)
- [サービスエンドポイントゲートウェイ / sacloud-sdk-go v0.3.1](https://github.com/sacloud/sacloud-sdk-go/tree/v0.3.1/api/service-endpoint-gateway)
