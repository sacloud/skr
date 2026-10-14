# Server の cloud-init と SSH 接続の設計

## 対象範囲

この設計では、is1b に cloud-init 対応 Server を作成し、起動時に user-data を適用して、SSH 接続を確認する手順を扱います。任意の Server リクエストのネストフィールドを指定する汎用フラグは追加しません。

## CLI の手順

1. `iaas-api server create` は反復可能な `--disk 'archive-id=123,plan-id=4,size-gb=20'` を受け付けます。各指定は virtio ディスク1個に対応し、plan-id と size-gb は必須です。archive-id 省略時は空のディスクを作成します。
2. `--nic 'upstream=shared'` または `--nic 'upstream=disconnected'` を反復して NIC を指定します。ディスクと NIC はそれぞれ指定順に配列を組み立てます。チュートリアルでは共有 NIC を1個指定します。
3. cloud-init 用の手順では Create 時に起動しません。archive-id を指定したディスクと `--boot-after-create=true` は併用できません。
4. `iaas-api server boot` は `--user-data @path.yaml` または `--user-data -` を受け付けます。インラインの user-data は拒否します。Boot はサーバ起動時に user-data を適用する SDK 操作を呼び出します。
5. チュートリアルでは SSH 鍵をローカルで生成し、公開鍵だけを cloud-config に記載します。SSH 接続の前に、共有 NIC の IP アドレスを確認します。

既存の Create の `--request` JSON 入力は、個別スカラーオプションで表せないリクエストにも引き続き利用できます。`--request` と Create の個別フラグは併用できません。

## ディスクと NIC の入力処理

[ADR 0041](../adr/0041-server-repeatable-disk-and-nic-options.md) に従い、各オプションの値をカンマで分割し、各要素を `key=value` として解析します。引用符やエスケープは独自に解釈せず、空要素、空値、不明キー、重複キー、必須キーの欠落はエラーにします。ID は正の64ビット整数、size-gb は正の整数として範囲も検証します。入力値そのものはエラーに含めません。

オプションの反復はカンマ区切りと区別して取り込み、各ディスクと NIC を指定順に構築します。ディスク名には従来の単一ディスクと同様にサーバ名を使い、複数ディスクの場合もすべて同じ名前にします。構造化オプションで表せないディスク名や接続先などは `--request` で指定します。空のディスクだけを指定した場合は、作成後の起動を制限しません。

## ライブ E2E の安全性

is1b の runner はランダムな Server 名と固定の E2E 説明を使用します。作成前に Server と Disk の両方について同名リソースがないことを確認します。一時的な SSH 鍵を生成し、cloud-init ディスク付き Server を1台作成して起動し、SSH 接続を繰り返し確認します。後始末では Server の ID、名前、説明を照合してから、Disk とともに削除します。最後に両方の名前で検索し、リソースが残っていないことを確認します。過去の実行で作成されたリソースは削除しません。

証跡にはコマンド情報と API 応答を記録しますが、秘密鍵の内容や cloud-config の内容は含めません。一時的な秘密情報はアクセス制限付きのファイルに保存し、runner の終了時に削除します。

SSH の接続試行は `test-ssh-login` として独立した証跡に記録します。実行引数、標準出力、標準エラー、実行結果を保存し、`hostname` の出力によってログイン成功を確認できます。接続失敗は再試行しますが、証跡の保存に失敗した場合は E2E を中止し、作成済みリソースを後始末します。

`./skr` をビルドし、使用する SDK プロファイルを選択してから、リポジトリのルートで runner を実行します。

```console
$ go run ./test/e2e/server --skr ./skr \
    --cloudimg-archive-id ARCHIVE_ID \
    --disk-plan-id DISK_PLAN_ID \
    --disk-size-gb 20 \
    --confirm-is1b-live
```

選択したアーカイブとディスクプランは is1b で利用可能である必要があります。この実行では Server と Disk を作成し、起動と外部からの SSH 接続を確認してから削除します。利用料金が発生し、外部接続も行うため、明示的な確認を必須にします。

## 検証

単体テストでは Create リクエストの組み立て、不正なオプションの組み合わせ、Boot の user-data ファイル／標準入力、各種エラーを検証します。runner のテストでは偽の CLI を使い、引数の組み立て、SSH 接続の成功と失敗、リソースの識別、後始末を確認します。ライブ API には接続しません。
