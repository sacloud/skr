# ADR 0095: NFS API コマンドを追加する

- Status: Accepted
- Date: 2026-11-21
- SDK: `github.com/sacloud/sacloud-sdk-go v0.7.0` (`api/iaas`)

## Context

さくらのクラウド NFS は、スイッチ配下のローカルネットワークに設置する NFS ファイルサーバアプライアンスです ([NFS のマニュアル](https://manual.sakura.ad.jp/cloud/appliance/nfs/index.html))。作成には名前、プラン ID、接続先スイッチ ID、IPv4 アドレス (1〜2 個)、ネットマスク長が必須で、接続先スイッチ、プラン、IP アドレス、ネットマスク長は作成後の変更ができません。プラン ID は共通仕様ノート `sys-nfs` のプラン一覧から解決します。削除はアプライアンスの電源を OFF にしてから行う必要があり、電源 ON のまま削除 API を呼ぶと失敗します。

SDK は `iaas.NFSAPI` に Find / Read / Create / Update / Delete / Boot / Shutdown / Reset / MonitorCPU / MonitorFreeDiskSize / MonitorInterface の 11 操作を公開し、`service/iaas/nfs` には起動・停止待ちや削除ワークフローを実装した service 層ライブラリがあります。

NFS は IaaS のゾーンを持つアプライアンスで、データベース ([設計](../design/database-api.md)) と同じく生成コマンドに加えて手書きのワークフローを含む構成になります。sakumock の IaaS モックは NFS を含みません。

## Decision

- `skr nfs-api <operation>` を追加します。11 操作すべてを公開します。`skr iaas-api nfs <operation>` ではなく独立した `<domain>-api` グループとします。NFS は単一リソースの操作群をそのままコマンドツリーにでき、手書きワークフローの比重が大きいためです。コマンドの実装は `internal/iaas/nfsapi/` に置き、生成コードと手書きのリクエスト・ワークフローを同じパッケージに同居させます。
- 操作の大半 (find / read / delete / boot / shutdown / reset / monitor-cpu / monitor-free-disk-size / monitor-interface) は `api/commands/nfs.json` から生成器で生成します。create と update は手書きです。create は手書きの `CreateRequest` から API の作成リクエストへ変換する必要があり、update は未指定項目をポインタの nil として区別する入力処理が必要なためです。
- create は個別フラグと `--request` JSON の両経路を設け、相互排他とします。個別フラグは名前、プラン ID、スイッチ ID、ネットマスク長を必須とし、説明、タグ (カンマ区切り)、アイコン ID、デフォルトルートを任意とします。IP アドレスは `--ip-address` を反復して指定し、1〜2 個の IPv4 アドレスだけを受け付けます。反復可能な単純値フラグであり、[ADR 0041](0041-server-repeatable-disk-and-nic-options.md) の key=value 構文は使いません。データベースの `--ip-address` と同じ方式です。
- create の応答は起動の完了を待たずに返します (API の挙動どおり)。起動待ちは利用者や後続の作業で行います。データベースの create と同じ挙動です。
- update の個別フラグは名前、説明、タグ、アイコン ID です。SDK の更新リクエストは未指定項目を空の値として送る挙動のため、既存の値を維持する項目もあわせて指定する必要があることをヘルプと設計文書に明記します。プランとスイッチは更新リクエストに含まれません。
- find の `--name` は [ADR 0049](0049-internet-workflow-options.md) と同じく SDK の `Names` への 1 要素変換 (部分一致 AND) とし、`--tags` はカンマ区切りを複数要素の `Tags` に変換します ([ADR 0035](0035-iaas-tags-flag.md))。
- delete は SDK service 層と同じ条件のワークフローを api 層に実装します。電源が ON で `--force` が無い場合はエラーにし、`--force` のときは強制終了と停止待ちのうえ削除します。`--fail-if-not-found` は対象が存在しない場合のエラー化を切り替えます。この 2 つは API 本文ではなく削除の動作オプションとして [ADR 0049](0049-internet-workflow-options.md) の拡張範囲に含まれます。
- boot と shutdown は `--no-wait` に対応します。省略時は SDK の waiter で起動・停止の完了を待ちます。shutdown は `--force-shutdown` (強制終了) にも対応します。低レベルコマンドの `--no-wait` は SDK パラメータの公開であり、[ADR 0082](0082-high-level-command-wait-options.md) の高レベル層の待機オプションとは区別されます (データベースと同じ方式)。
- ライブ E2E は `test/e2e/nfs/` に testing.T ベースのシナリオとして配置します ([ADR 0062](0062-e2e-testing-t-based-scenario.md))。tk1a でテスト用スイッチと NFS (Note から解決した SSD プランの最小サイズ) を作成し、作成、起動待ち、参照、名前・タグ検索、モニタリング、更新、電源操作、削除と不在確認までを検証します。作成したリソースはテスト内の `defer` で ID と識別情報を再照合してから削除します。実行ごとに NFS の課金が発生します (作成して削除する)。tk1a を選んだのは、他の E2E で使う is1b (switch)、is1c (database) に加えて is1a もスイッチのゾーン内上限に達していたためです。2026-11-20 に全ステップをライブ検証済みです。
- ライブ検証で確認した仕様として、空きディスク容量のモニタリング API はデータベースと共通の route を使う日単位の API で、`--start` / `--end` に正時でない時刻を指定すると API エラー (400) になります。コマンドのヘルプに正時指定を案内し、シナリオでは正時の期間を指定します。また、update で送信したタグの順序は API 側で並べ替えられて返ります。
- sakumock は NFS を含まないため、チュートリアルの CLI 操作ハーネステストは sakumock の NFS 対応を待って提供します。差分は sakumockiaas の差分管理文書に記録します。

## Consequences

- NFS のライフサイクル (作成から削除まで) を CLI で完結できます。プラン ID は Note (sys-nfs) から解決する必要があり、ヘルプとチュートリアルで案内します。
- create と update の入力変換と検証、delete のワークフローを、パッケージ内の単体テストで固定します。
- チュートリアルの自動検証は sakumock の NFS 対応まで延期されます。

## Alternatives considered

- `skr iaas-api nfs <operation>` にする案: データベースと同じリソースコマンド形式です。iaas-api 配下にも配置できますが、NFS は削除ワークフローや電源操作の待ち合わせなど手書き処理の比重が大きく、単一リソースの 11 操作をフラットに公開できるため、独立グループにしました。
- create を service 層 (`service/iaas/nfs`) に依存させる案: service 層の create は起動待ちを内製しますが、`iaas-api` の低レベルコマンドは API 層の操作と 1 対 1 で対応させる方針 ([アーキテクチャ](../design/cli-command-architecture.md)) に反します。API 層の操作をそのままラップし、待ち合わせは `--no-wait` の挙動として明示的に公開します。
- IP アドレスを `--ip-address 'key=value'` 型の構造化オプションにする案: 要素が単純な文字列 (IPv4 アドレス) であり、構造化の余地がありません。反復可能な単純値フラグで十分です。
