# NFS の高レベルコマンド (`skr nfs`)

`skr nfs` は、ゾーンで利用できる NFS プランの一覧を表示する高レベルコマンドです。NFS の作成に必要なプラン ID を確認できます。

このページはコマンドの対象、構造、各サブコマンドの挙動と制約を説明するリファレンスです。最初から順に NFS を作成する手順は [NFS: 作成から削除まで](tutorials/nfs-api.md) を参照してください。フラグと使用例の一覧は `--help` で確認できます。

## 対象と対象外

対象はゾーンで利用できる NFS プランの一覧表示です。

対象外は NFS アプライアンス自体の作成・参照・更新・削除・電源操作・モニタリングです。これらは低レベルコマンド `skr nfs-api` で操作します。

## 概念

NFS (ファイルサーバアプライアンス) は、スイッチ配下のローカルネットワークに設置するストレージです。NFS の作成では、ストレージ種別 (SSD / HDD) とディスク容量の組み合わせである**プラン**をプラン ID で指定します。

プランの一覧は、さくらのクラウドがゾーンごとに公開する共有リソース Note (`sys-nfs`) の Content に記録されています。`skr nfs plans` はこの Note を取得し、プランの一覧として表示します。利用者は Note の存在を意識する必要はありません。

プランの詳細は [NFS ご利用ガイド](https://manual.sakura.ad.jp/cloud/appliance/nfs/index.html) を参照してください。

## コマンド一覧

| コマンド | 役割 |
| --- | --- |
| `plans` | ゾーンで利用できる NFS プランの一覧を表示する |

高レベルコマンドと低レベル `skr nfs-api` の対応は次のとおりです。

| `skr nfs` の操作 | 低レベル `skr nfs-api` の操作 |
| --- | --- |
| `plans` (Note `sys-nfs` の取得と解析) | 対応する低レベル操作はありません |

プラン ID が決まったあとの NFS の作成は `skr nfs-api create --plan-id PLAN_ID` で行います。

## `plans`

```
skr nfs plans --zone ZONE [--all]
```

ゾーンで利用できる NFS プランの一覧を表示します。出力は JSON 配列で、ストレージ種別が SSD のプランから HDD のプランの順、同じ種別内はディスク容量の昇順に並びます。

```json
[
  {
    "Storage": "SSD",
    "Size": 20,
    "PlanID": 20,
    "Availability": "available",
    "Cloneable": true
  }
]
```

各フィールドは次のとおりです。

- `Storage` — ストレージ種別です。SSD または HDD です。
- `Size` — ディスク容量です (GB)。
- `PlanID` — プラン ID です。`skr nfs-api create` の `--plan-id` に指定します。
- `Availability` — プランの利用可否です。`available` (利用可能) または `disabled` (利用不可) です。
- `Cloneable` — プランの複製可否です。

省略時は利用可能 (`available`) なプランのみを出力します。`--all` を付けると利用不可 (`disabled`) のプランも含めて出力します。

ゾーンで利用できるプランはゾーンごとに異なります。プランの選び方と組み合わせは、出力から選択してください。

### プラン ID の取得例

次の例では、指定ゾーンの SSD プランから最小容量のプラン ID を取得し、`skr nfs-api create` に渡しています。

```console
$ PLAN_ID=$(skr nfs plans --zone "$ZONE" --query-raw 'map(select(.Storage == "SSD")) | sort_by(.Size) | .[0].PlanID')
$ skr nfs-api create --zone "$ZONE" --name example --plan-id "$PLAN_ID" ...
```

`--query-raw` の詳細は [`--query` オプション](query.md) を参照してください。

### エラーと対処

- `ゾーン ZONE の NFS プラン情報が見つかりません` — ゾーン名が正しいか確認してください。ゾーン名は `is1a` などの小文字で指定します。
- `ゾーン ZONE に対応する NFS プラン情報が複数見つかりました` — 同一ゾーンのプラン情報が重複しています。時間を置いて再実行し、解消しない場合はさくらのクラウドのサポートへ問い合わせてください。
