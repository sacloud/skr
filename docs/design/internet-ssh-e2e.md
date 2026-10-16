# Internet の SSH ライブ E2E

## 対象範囲

`test/e2e/internet` は is1b にルータ＋スイッチと Server、20 GB の起動ディスクを作成します。Server の先頭 NIC を作成したスイッチに接続し、cloud-init で公開鍵と IPv4 ネットワークを設定して、外部から SSH で `hostname` を実行します。共有セグメントの NIC は作成しません。

Internet の Update、追加サブネット、IPv6、MonitorRouter はこのシナリオの対象外です。

## CLI の入力経路

Server Create は `--disk 'archive-id=...,plan-id=...,size-gb=...'` と `--nic 'upstream=SWITCH_ID'` を使います。Internet、Server、Disk の名前検索は `--name` で API 側の部分一致条件を指定し、削除時の Internet は `--force --fail-if-not-found` を使います。このシナリオでは `--request` は使いません。設計判断は [ADR 0049](../adr/0049-internet-workflow-options.md) に記録しています。

## IPv4 アドレスと OS 設定

ルータ＋スイッチ接続では `Interfaces[].IPAddress` が空のまま Server が作成されるため、自動割り当て済みとは扱いません。`internet list-subnet` の `IPAddresses` から使用する IPv4 アドレスを選びます。作成直後の専用ネットワークに Server を1台だけ接続するシナリオに限定し、既存の共有ネットワークで未使用アドレスを判定する機能にはしません。

サブネットの Internet ID、プライマリーサブネットであること、IPv4 プレフィクスとデフォルトルートを照合します。アドレス候補は API の一覧に含まれるものだけを使い、ネットワーク範囲外、ネットワークアドレス、ゲートウェイ、ブロードキャストアドレスを拒否します。

cloud-init の user-data には、公開鍵に加えて MAC アドレスで NIC を特定するスクリプトを含めます。OS 起動後に `ip address replace` と `ip route replace` で設定します。この設定は一時的であり、永続ネットワーク設定の検証は対象外です。SSH の接続先はこの選択済みアドレスとし、Server API の空の `IPAddress` に依存しません。

## 安全性と証跡

実行確認フラグを必須にし、ランダムな名前と専用の説明を使います。同名の Internet、Server、Disk が存在する場合は作成せず停止します。既存リソースの事前削除は行いません。

後始末は Server と Disk、Internet の順です。削除前に ID、名前、説明、接続先などを読み直して照合し、削除後は名前で検索して空の配列を確認します。識別できない残存 Disk や、Server の削除失敗は明示的にエラーとし、推測で削除しません。

Internet と Server の作成には各35分、通常の操作には90秒の期限を設けます。Internet 作成では、作成直後の404を最大30分リトライする SDK の待機を許容します。SSH 待機は20分で、シナリオ全体は作成2回と通常操作の余裕を含め100分です。

Internet の作成結果を安全に取得できない場合は、シナリオの中断や期限切れとは独立したコンテキストで最大35分間再検索します。空の一覧や一時的な検索・参照失敗だけで不在と判断せず、名前・説明・ネットワーク設定を照合してから削除します。曖昧な一致や識別情報の不一致では削除せず停止し、待機期限まで確認できない場合は手動確認が必要であることを通知します。元の作成エラーは保持します。SIGKILL や実行環境の停止など、後始末自体を実行できない中断は自動回収の対象外です。

通常の後始末には独立した5分の期限を設けます。Server の削除操作が応答受信や証跡保存で失敗しても、Server と Disk の不在を確認できた場合は Internet の後始末を続けます。元の削除エラーと Internet の後始末のエラーはまとめて返し、成功として扱いません。

SSH の秘密鍵と user-data は一時ディレクトリ内で保護し、終了時に削除します。証跡には API 操作と SSH の引数、出力、終了状態を記録し、秘密鍵や user-data の内容は含めません。証跡は Git 管理外の `tmp/internet-api/` に保存します。最終結果は一時ファイルの削除後に記録します。

## 実行

対象プロファイルを確認し、is1b で利用可能な cloud-init 対応アーカイブと、20 GB を利用できるディスクプランを選択します。

```console
$ go build -o ./skr ./cmd/skr
$ ./skr config current
$ go run ./test/e2e/internet --skr ./skr \
    --cloudimg-archive-id ARCHIVE_ID \
    --disk-plan-id DISK_PLAN_ID \
    --disk-size-gb 20 \
    --confirm-is1b-live
```

`ARCHIVE_ID` と `DISK_PLAN_ID` は検索・参照で確認した ID に置き換えます。ルータ＋スイッチ、Server、Disk の課金と外部 SSH 接続を伴うため、対象環境と副作用の承認を得てから実行します。

## 参照

- [利用者向けチュートリアル](../manual/tutorials/iaas-api/internet.md)
- [ルータ＋スイッチの概要](https://manual.sakura.ad.jp/cloud/network/switch/about.html)
- [NIC の設定と表示用 IPv4 アドレス](https://manual.sakura.ad.jp/cloud/server/nic.html)
- [sacloud-sdk-go v0.3.0 Subnet](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/api/iaas#Subnet)
