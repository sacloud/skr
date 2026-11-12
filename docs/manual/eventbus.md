# EventBus の高レベルコマンド (`skr eventbus`)

`skr eventbus` は、EventBus の実行設定、スケジュール、トリガーを宛先別の個別フラグで操作する高レベルコマンドです。実行設定でジョブの宛先を定義し、スケジュール (開始日時からの繰り返し) またはトリガー (イベント検知) から実行します。

このページはコマンドの対象、構造、各サブコマンドの挙動と制約を説明するリファレンスです。最初から順に試す手順は [EventBus: スイッチ作成イベントを SimpleMQ で受信する](tutorials/eventbus.md) を参照してください。フラグと使用例の一覧は `--help` で確認できます。

## 対象と対象外

対象は実行設定 (process-configuration)、スケジュール (schedule)、トリガー (trigger) の一覧・参照・作成・更新・削除と、実行設定への secret の登録です。

対象外はジョブの実行履歴の参照と、EventBus 以外のサービス (SimpleMQ キューの管理など) の操作です。SimpleMQ キューの操作は低レベルの `skr simplemq-api` を使います。

## 概念

EventBus のリソースは次の 3 種類で構成されます。用語の定義は [EventBus の基本情報](https://manual.sakura.ad.jp/cloud/appliance/eventbus/about.html) を参照してください。

- **実行設定 (process-configuration)** — 実行するジョブの内容と宛先を定義します。宛先は `simplenotification` (シンプル通知)、`simplemq` (SimpleMQ キューへメッセージを送信)、`autoscale` (オートスケーラーへアクションを指示) のいずれかです。
- **スケジュール (schedule)** — 開始日時からの繰り返し (間隔指定または Crontab) で実行設定を呼び出します。
- **トリガー (trigger)** — 送信元のイベント (イベントログ、またはモニタリングスイートのアラート) を条件で照合し、実行設定を呼び出します。

EventBus API では、この 3 種類を共通のリソース (CommonServiceItem) として扱い、`Provider.Class` で種別を区別します。低レベルコマンドではこの共通リソースが入出力に直接現れますが、高レベルコマンドでは種別ごとの個別フィールドとして扱えます。

ジョブの実行はベストエフォート型で、厳密なリアルタイム性は保証されません。

## コマンド一覧

| コマンド | 役割 |
| --- | --- |
| `process-configuration list` | 実行設定を一覧表示する |
| `process-configuration read ID` | 実行設定を表示する |
| `process-configuration create` | 実行設定を作成する |
| `process-configuration update ID` | 実行設定を更新する (指定項目のみ変更) |
| `process-configuration delete ID` | 実行設定を削除する |
| `process-configuration set-secret ID` | 実行設定の secret を登録または置き換える |
| `schedule list` | スケジュールを一覧表示する |
| `schedule read ID` | スケジュールを表示する |
| `schedule create` | スケジュールを作成する |
| `schedule update ID` | スケジュールを更新する (指定項目のみ変更) |
| `schedule delete ID` | スケジュールを削除する |
| `trigger list` | トリガーを一覧表示する |
| `trigger read ID` | トリガーを表示する |
| `trigger create` | トリガーを作成する |
| `trigger update ID` | トリガーを更新する (指定項目のみ変更) |
| `trigger delete ID` | トリガーを削除する |

`ID` は位置引数で指定します。ID は `list` の出力や各コマンドの `--help` で確認できます。

高レベルコマンドと低レベル `skr eventbus-api` の対応は次のとおりです。

| `skr eventbus` の操作 | 低レベル `skr eventbus-api` の操作 |
| --- | --- |
| 実行設定の操作 | `--provider-class eventbusprocessconfiguration` の CommonServiceItem 操作 |
| スケジュールの操作 | `--provider-class eventbusschedule` の CommonServiceItem 操作 |
| トリガーの操作 | `--provider-class eventbustrigger` の CommonServiceItem 操作 |

低レベルコマンドでは、共通リソースの `Settings` を種別ごとに組み立てる JSON 入力が必要です。完全な JSON 入力、説明 (description) の消去、トリガーの条件の消去、条件値にカンマを含む入力が必要なときは低レベルコマンドを使います。

## 実行設定 (process-configuration)

### 作成と宛先別のパラメーター

`--name` と `--destination` が必須です。宛先に応じて指定するパラメーターが変わります。

| 宛先 | 指定するフラグ | 内容 |
| --- | --- | --- |
| `simplenotification` | `--group-id`、`--message` | シンプル通知のグループ ID とメッセージ |
| `simplemq` | `--queue-name`、`--content` | 送信先キュー名とメッセージ本文 |
| `autoscale` | `--action`、`--resource-id` | アクション (`scale_up`、`scale_down`) と対象リソース ID |

宛先に属さないフラグを指定すると、空文字列でもエラーになります。これは、他の宛先のパラメーターの取り違えを防止するための検証です。未指定のパラメーターは空文字列として送信します。

宛先ごとのパラメーターの値の範囲は API が検証します。範囲の詳細は [EventBus API ドキュメント](https://manual.sakura.ad.jp/api/cloud/portal/eventbus-api/index.html) を参照してください。

### 更新のマージ規則

`update` は指定した項目だけを変更し、未指定の項目は現状維持します。内部的には現状を取得してから変更項目を上書きした全体を送る (read-modify-write) ため、読み取りと更新の間に他の変更が入った場合の原子性は保証しません。

- 宛先を変更しない場合は、現状のパラメーターの未知のキーも保持したまま、指定フィールドの対応キーだけを上書きします。
- `--destination` を変更した場合は、旧宛先のパラメーターを引き継ぎません。新宛先のパラメーターをすべて指定してください。
- 説明 (`--description`) に空文字列を指定すると空文字列として送信します。説明を null に戻す消去は高レベルコマンドでは行えず、低レベルコマンドの JSON 入力で行います。

### secret の登録 (set-secret)

`set-secret` は実行設定の secret を登録または置き換えます。secret は宛先ごとの認証情報で、`--secret-file` で保護したファイル (`@path`) または標準入力 (`-`) から読み込みます。値を引数やインライン JSON に含めることはできません。

| 宛先 | secret の JSON |
| --- | --- |
| `simplemq` | `{"APIKey":"..."}` |
| `simplenotification`、`autoscale` | `{"AccessToken":"...","AccessTokenSecret":"..."}` |

secret の既存値は読み取れないため、更新ではなく登録または置き換えになります。既存 secret とのマージは行いません。トレースが有効な場合は秘密情報がトレース出力へ現れるため、実行せずエラーになります。

## スケジュール (schedule)

### 繰り返しの指定

`--name`、`--process-configuration-id`、`--starts-at` が必須です。繰り返しは次の 2 方式のどちらか一方を完全に指定します。

- 周期方式: `--recurring-step` と `--recurring-unit` (`min`、`hour`、`day`) の組。
- Crontab 方式: `--crontab`。

両方の指定、周期方式の片方だけの指定、どちらも指定しない場合はエラーになります。

`--starts-at` は RFC3339 形式 (例: `2030-01-10T09:00:00+09:00`、小数秒はミリ秒まで) で指定します。API は開始日時をエポックミリ秒で保持するため、ミリ秒より細かい小数秒は指定できません。

### 更新と繰り返しモードの切り替え

`update` は指定した項目だけを変更し、未指定の項目は現状維持です。繰り返しモードの切り替えは、次のとおりに扱います。

- `--crontab` を指定すると Crontab 方式になり、周期指定は送りません。逆に `--recurring-step` または `--recurring-unit` を指定すると周期方式になり、Crontab は送りません。
- `--crontab` と周期指定の同時指定はエラーになります。
- 周期方式に切り替えたとき、`--recurring-step` と `--recurring-unit` は組で指定します。現状が周期方式なら、指定しなかった片方は現状値を引き継ぎます。現状が Crontab 方式なら、片方だけの指定はエラーになります。
- どちらも指定しないときは、現状のモードと値を維持します。

## トリガー (trigger)

### 送信元と条件

`--name`、`--source`、`--process-configuration-id` が必須です。`--source` は送信元イベントの URI で、次のいずれかです。

- `//eventbus.sakura.ad.jp/eventlog` (イベントログ)
- `//eventbus.sakura.ad.jp/monitoringsuite/alert` (モニタリングスイートのアラート)

送信元ごとのイベントタイプの一覧は [EventBus のイベント](https://manual.sakura.ad.jp/cloud/appliance/eventbus/events.html) を参照してください。

`--type` と `--condition` は省略でき、反復可能なオプションです。省略した場合はすべてのイベントタイプ・すべてのイベントで発火します。値にカンマを含めても分割せず、1 回の指定が 1 要素になります。指定順序を保持します。

`--condition` は `key=名前,op=eq|in,value=値` の形式で指定します。

- `key` は小文字英数字 20 文字以内で、`data` は使えません。
- `op` は `eq` (等号) または `in` (包含) です。`eq` は `value` が 1 件、`in` は `value` を反復指定して 1 件以上必要です。

例: `--condition 'key=zone,op=eq,value=is1b'`、`--condition 'key=region,op=in,value=is1a,value=is1b'`。

### 更新と消去

`update` は指定した項目だけを変更し、未指定の項目は現状維持です。`--type` と `--condition` は指定時に既存値の全体を置き換えます (未指定のときは現状維持)。

イベントタイプや条件を空にする消去 (null への変更) は高レベルコマンドでは行えず、低レベルコマンドの JSON 入力で行います。説明の消去も同様です。

## 出力

`list`、`read`、`create`、`update` は結果を JSON で出力し、グローバルフラグ `--query` で加工できます。`delete` と `set-secret` は何も出力しません。

出力は、低レベルコマンドの共通リソース (CommonServiceItem) を種別ごとの個別フィールドへ射影したものです。共通の `ID`、`Name`、`Description`、`Tags`、`CreatedAt`、`ModifiedAt` に加えて、種別ごとの次のフィールドを含みます。

- 実行設定: `Destination` と、宛先ごとのパラメーター (`Parameters`)。secret などの秘密フィールドは含みません。
- スケジュール: `ProcessConfigurationID`、`StartsAt` (RFC3339 形式、UTC)、`RecurringStep`、`RecurringUnit`、`Crontab`。
- トリガー: `Source`、`Types`、`Conditions`、`ProcessConfigurationID`。

`SettingsHash`、`Status`、`ServiceClass`、`Availability`、`Provider`、`Icon` は出力しません。完全な応答が必要なときは低レベルの `eventbus-api` を使います。

## 秘密情報の扱い

- 実行設定の secret は `--secret-file` の `@path` 形式 (ファイル) または `-` (標準入力) でのみ指定できます。インラインの JSON や環境変数からの読み込みは行いません。
- secret の値はログやエラーメッセージに含まれません。`set-secret` では、API エラーの本文もそのまま表示せず、秘密情報を含まない診断情報を返します。
- トレース (`--trace`、または `SAKURA_TRACE_MODE` などの SDK 設定) が有効な場合、`set-secret` は実行せずエラーになります。トレース出力にはリクエスト本文が含まれるためです。
- secret を書いたファイルは、ファイルの権限に注意してください (例: `chmod 600`)。また、Git などのバージョン管理へコミットしないでください。

## エラーと対処

| 状況 | 挙動 | 対処 |
| --- | --- | --- |
| 宛先に属さないフラグの指定 (simplemq 以外で `--queue-name` など) | API を呼び出す前にエラーになる | `--destination` とフラグの対応を確認する |
| 周期方式と Crontab 方式の両方の指定、周期方式の片方だけの指定 | API を呼び出す前にエラーになる | `--recurring-step` と `--recurring-unit` の組、または `--crontab` のどちらか一方を指定する |
| `--starts-at` の形式エラー (RFC3339 以外、ミリ秒より細かい小数秒) | API を呼び出す前にエラーになる | RFC3339 形式でミリ秒までの精度で指定する |
| `--condition` の構文エラー、`key` の文字数や `data` の違反、`op` と `value` の件数の不整合 | API を呼び出す前にエラーになる | `--condition` の形式を確認する |
| `read`、`update`、`delete`、`set-secret` で ID の種別が一致しない | 変更を行わずにエラーになる | ID が対象種別のリソースか `list` で確認する |
| 宛先変更時に新宛先のパラメーターが未指定 | 空文字列として送信する | 新宛先のパラメーターをすべて指定する |
| 更新中の他の変更 (同時更新) | 検出しない | 原子性は保証しないため、同時に変更しない |
| 説明や条件、イベントタイプの消去 (null 化) | 高レベルコマンドでは行えない | 低レベルの `eventbus-api` で JSON を指定する |
| トレースが有効な状態での `set-secret` | 実行せずにエラーになる | `--trace` を外し、`SAKURA_TRACE_MODE` などの SDK 設定を確認する |

## 参照先

- [EventBus の基本情報](https://manual.sakura.ad.jp/cloud/appliance/eventbus/about.html)
- [EventBus のイベント (送信元とイベントタイプ)](https://manual.sakura.ad.jp/cloud/appliance/eventbus/events.html)
- [EventBus API ドキュメント](https://manual.sakura.ad.jp/api/cloud/portal/eventbus-api/index.html)
- [SimpleMQ の基本情報](https://manual.sakura.ad.jp/cloud/appliance/simplemq/about.html)
- [EventBus のチュートリアル (手順)](tutorials/eventbus.md)
- 低レベル API コマンド: `skr eventbus-api --help`
