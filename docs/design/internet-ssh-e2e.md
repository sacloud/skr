# Internet の SSH ライブ E2E

## 対象範囲

`test/e2e/internet` は is1b にルータ＋スイッチと Server、20 GB の起動ディスクを作成します。Server の先頭 NIC を作成したスイッチに接続し、cloud-init で公開鍵と IPv4 ネットワークを設定して、外部から SSH で `hostname` を実行します。共有セグメントの NIC は作成しません。

シナリオは 1 つのテスト関数 (`TestScenarioInternet`) で、ルータ＋スイッチの作成・参照・名前検索・更新と復元・モニタリングを確認したあと、Server の作成と SSH ログインを確認します。続けて、追加サブネットの追加・NextHop 更新・削除、IPv6 の有効化・参照・無効化、後始末をこの順に確認します。

## CLI の入力経路

Server Create は `--disk 'archive-id=...,plan-id=...,size-gb=...'` と `--nic 'upstream=SWITCH_ID'` を使います。Internet、Server、Disk の名前検索は `--name` で API 側の部分一致条件を指定し、削除時の Internet は `--force --fail-if-not-found` を使います。`monitor-router` はゾーンと ID、期間を `--zone`、`--id`、`--start`、`--end` (RFC3339 形式) で指定します。設計判断は [ADR 0049](../adr/0049-internet-workflow-options.md) と [ADR 0066](../adr/0066-internet-monitor-router-flags.md) に記録しています。

## IPv4 アドレスと OS 設定

ルータ＋スイッチ接続では `Interfaces[].IPAddress` が空のまま Server が作成されるため、自動割り当て済みとは扱いません。`internet list-subnet` の `IPAddresses` から使用する IPv4 アドレスを選びます。作成直後の専用ネットワークに Server を1台だけ接続するシナリオに限定し、既存の共有ネットワークで未使用アドレスを判定する機能にはしません。

サブネットの Internet ID、プライマリーサブネットであること、IPv4 プレフィクスとデフォルトルートを照合します。Server の NIC は UpstreamType がルータ＋スイッチ接続の "router" または "switch" で、作成したスイッチの Switch ID と一致することを照合します。ルータ＋スイッチ接続の UpstreamType は 2026-10-28 のライブ実行で "router" と返ることを確認済みです。アドレス候補は API の一覧に含まれるものだけを使い、ネットワーク範囲外、ネットワークアドレス、ゲートウェイ、ブロードキャストアドレスを拒否します。

cloud-init の user-data には、公開鍵に加えて MAC アドレスで NIC を特定するスクリプトを含めます。OS 起動後に `ip address replace` と `ip route replace` で設定します。この設定は一時的であり、永続ネットワーク設定の検証は対象外です。SSH の接続先はこの選択済みアドレスとし、Server API の空の `IPAddress` に依存しません。

## 安全性と証跡

`SKR_E2E` を実行確認フラグとして必須にし、ランダムな名前と専用の説明を使います。同名の Internet、Server、Disk が存在する場合は作成せず停止します。既存リソースの事前削除は行いません。

後始末は Server と Disk、Internet の順です。削除前に ID、名前、説明、接続先などを読み直して照合し、削除後は名前で検索して空の配列を確認します。識別できない残存 Disk や、Server の削除失敗は明示的にエラーとし、推測で削除しません。Server が残っている場合は Internet を削除せず、手動確認を求めます。

テスト対象の操作は skr CLI で実行し、作成前の同名確認、削除前の再照合、削除後の不在確認、起動待ちのポーリングは SDK で直接実行します ([ADR 0064](../adr/0064-e2e-safety-checks-via-sdk.md))。実行記録にはテスト対象の CLI 操作だけが残ります。

Internet と Server の作成には各35分、通常の操作には90秒の期限を設けます。Internet 作成では、作成直後の404を最大30分リトライする SDK の待機を許容します。SSH 待機は20分で、シナリオ全体は作成2回と通常操作の余裕を含め100分です。ライブ実行は `-timeout 120m` 級を指定して `go test` の既定タイムアウトによる強制終了を避けます。

通常の後始末には独立した5分の期限を設けます。Server の削除操作が応答受信や証跡保存で失敗しても、Server と Disk の不在を確認できた場合は Internet の後始末を続けます。元の削除エラーと Internet の後始末のエラーはまとめて返し、成功として扱いません。Internet の削除は非同期で、削除の受理後に応答が喪失してリトライが 404 になることがあります (2026-10-28 のライブ実行で確認)。この場合、SDK で不在を再確認できたときは削除の失敗と扱わず後始末を続けます。リソースが残っている場合は削除の失敗として報告します。

Internet の作成結果を安全に取得できない場合は、シナリオの中断や期限切れとは独立したコンテキストで最大35分間再検索します。空の一覧や一時的な検索・参照失敗だけで不在と判断せず、名前・説明・ネットワーク設定を照合してから削除します。曖昧な一致や識別情報の不一致では削除せず停止し、待機期限まで確認できない場合は手動確認が必要であることを通知します。元の作成エラーは保持します。SIGKILL や実行環境の停止など、後始末自体を実行できない中断は自動回収の対象外です。

SSH の秘密鍵と user-data は一時ディレクトリ内で保護し、終了時に削除します。SSH の待機とログイン確認は `test/e2e/internal/helper` に集約します ([ADR 0063](../adr/0063-reusable-ssh-and-local-command-recording.md))。SSH の接続先は選択済みアドレスです。待機中のポーリングごとの状態確認と失敗した SSH 試行は証跡に記録しません。API 操作の証跡には引数、出力、終了状態を記録し、秘密鍵や user-data の内容は含めません。証跡は Git 管理外の `e2e-output/` 配下に `<識別名>-<実行日時>.md` の Markdown で保存します。最終結果は一時ファイルの削除後に記録します。

## 実行

対象プロファイルを確認し、is1b で利用可能な cloud-init 対応アーカイブと、20 GB を利用できるディスクプランを選択します。アーカイブとディスクプランの ID はシナリオが SDK の検索で自動選択します。

```console
$ go build -o ./skr ./cmd/skr
$ ./skr config current
$ SKR_E2E=1 go test ./test/e2e/internet/ -run TestScenarioInternet -v -timeout 120m
```

認証は `SAKURA_PROFILE` に `skr-e2e` を設定したプロファイルを使い、認証情報を表示しません。ルータ＋スイッチ、Server、Disk の課金と外部 SSH 接続を伴うため、対象環境と副作用の承認を得てから実行します。

## 参照

- [利用者向けチュートリアル](../manual/tutorials/iaas-api/internet.md)
- [ルータ＋スイッチの概要](https://manual.sakura.ad.jp/cloud/network/switch/about.html)
- [NIC の設定と表示用 IPv4 アドレス](https://manual.sakura.ad.jp/cloud/server/nic.html)
- [sacloud-sdk-go v0.3.1 Subnet](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.1/api/iaas#Subnet)
