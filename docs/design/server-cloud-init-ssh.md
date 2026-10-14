# Server の cloud-init と SSH 接続の設計

## 対象範囲

この設計では、is1b に cloud-init 対応 Server を作成し、起動時に user-data を適用して、SSH 接続を確認する手順を扱います。任意の Server リクエストのネストフィールドを指定する汎用フラグは追加しません。

## CLI の手順

1. `iaas-api server create` は通常の必須スカラーオプションに加え、`--cloudimg-archive-id`、`--disk-plan-id`、`--disk-size-gb` を受け付けます。ディスク用の3オプションはすべて指定する必要があり、選択したアーカイブをソースとする virtio ディスクを作成します。
2. `--network-interface shared|disconnected` は、指定した接続先を持つ NIC を1つ作成します。チュートリアルでは `shared` を選択します。
3. cloud-init 用の手順では Create 時に起動しません。cloud-init ディスク用オプションを指定したときに `--boot-after-create=true` を指定するとエラーになります。
4. `iaas-api server boot` は `--user-data @path.yaml` または `--user-data -` を受け付けます。インラインの user-data は拒否します。Boot はサーバ起動時に user-data を適用する SDK 操作を呼び出します。
5. チュートリアルでは SSH 鍵をローカルで生成し、公開鍵だけを cloud-config に記載します。SSH 接続の前に、共有 NIC の IP アドレスを確認します。

既存の Create の `--request` JSON 入力は、個別スカラーオプションで表せないリクエストにも引き続き利用できます。`--request` と Create の個別フラグは併用できません。

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
