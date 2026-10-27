# Server の cloud-init と SSH 接続の設計

## 対象範囲

この設計では、is1b に cloud-init 対応 Server を作成し、起動時に user-data を適用して、SSH 接続を確認する手順を扱います。任意の Server リクエストのネストフィールドを指定する汎用フラグは追加しません。

cloud-init が動作しない場合の確認手順と検証用 Server でのログ観測結果は、[cloud-init トラブルシューティング](../cloud-init-troubleshooting.md)を参照してください。

## CLI の手順

1. `iaas-api server create` は反復可能な `--disk 'archive-id=123,plan-id=4,size-gb=20'` を受け付けます。各指定は virtio ディスク1個に対応し、plan-id と size-gb は必須です。archive-id 省略時は空のディスクを作成します。
2. `--nic 'upstream=shared'`、`--nic 'upstream=disconnected'`、または `--nic 'upstream=SWITCH_ID'` を反復して NIC を指定します。スイッチ ID は正の整数です。ディスクと NIC はそれぞれ指定順に配列を組み立てます。shared は先頭の NIC にだけ指定でき、2個目以降では API 呼び出し前に拒否します。チュートリアルでは共有 NIC を1個指定します。スイッチ ID の追加は [ADR 0049](../adr/0049-internet-workflow-options.md) に記録しています。
3. cloud-init 用の手順では Create 時に起動しません。archive-id を指定したディスクと `--boot-after-create=true` は併用できません。
4. `iaas-api server boot` は user-data を省略して通常どおり起動できます。指定する場合は `--user-data @path.yaml` または `--user-data -` を受け付けます。明示的な空値やインラインの user-data は拒否します。Boot はサーバ起動時に user-data を適用する SDK 操作を呼び出します。
5. チュートリアルでは SSH 鍵をローカルで生成し、公開鍵だけを cloud-config に記載します。SSH 接続の前に、共有 NIC の IP アドレスを確認します。

既存の Create の `--request` JSON 入力は、個別スカラーオプションで表せないリクエストにも引き続き利用できます。`--request` と Create の個別フラグは併用できません。

## ディスクと NIC の入力処理

[ADR 0041](../adr/0041-server-repeatable-disk-and-nic-options.md) に従い、各オプションの値をカンマで分割し、各要素を `key=value` として解析します。引用符やエスケープは独自に解釈せず、空要素、空値、不明キー、重複キー、必須キーの欠落はエラーにします。ID は正の64ビット整数、size-gb は正の整数として範囲も検証します。入力値そのものはエラーに含めません。

オプションの反復はカンマ区切りと区別して取り込み、各ディスクと NIC を指定順に構築します。ディスク名には従来の単一ディスクと同様にサーバ名を使い、複数ディスクの場合もすべて同じ名前にします。構造化オプションで表せないディスク名や接続先などは `--request` で指定します。空のディスクだけを指定した場合は、作成後の起動を制限しません。

## ライブ E2E の安全性

ライブ E2E は testing.T ベースのシナリオテスト (`test/e2e/server/scenario_server_test.go`) として実装します。配置と実行方法は [ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md) に従います。

シナリオはランダムな Server 名と固定の E2E 説明を使用します。作成前に Server と Disk の両方について同名リソースがないことを確認します。一時的な SSH 鍵を生成し、cloud-init ディスク付き Server を1台作成して起動し、SSH 接続を繰り返し確認します。後始末では Server の ID、名前、説明を照合してから、Disk とともに削除します。最後に両方の名前で検索し、リソースが残っていないことを確認します。検索は `--name` の部分一致でページを最後まで取得します。過去の実行で作成されたリソースは削除しません。

Server 削除前に接続ディスクの ID を取得します。削除後に同名の Disk が1個残った場合は、その ID が取得済みの接続ディスクと一致することを確認します。さらに Disk を読み直し、ID、名前、プラン、ソースアーカイブ、サイズが一致し、他の Server に接続されていない場合だけ削除します。削除後にも不在を確認します。Server の削除操作がエラーを返しても、不在を確認できた場合は残存 Disk の後始末を試み、元のエラーは保持します。

作成失敗後に Server が見つからない場合も Disk を検索します。ただし、今回作成したディスクの ID を確認できない場合や、複数の一致がある場合は推測で削除しません。手動の後始末が必要であることをエラーで通知します。後片付けは `defer` で登録し、途中の失敗でも実行され、その失敗はテストの結果に含まれます。

実行記録にはコマンド情報と API 応答を記録しますが、秘密鍵の内容や cloud-config の内容は含めません。一時的な秘密情報はアクセス制限付きのファイルに保存し、テスト終了時に削除します。

## 実行

`./skr` をビルドし、プロファイルディレクトリに `skr-e2e` プロファイルを用意してから、リポジトリのルートでテストを実行します。is1b で利用可能な cloud-init アーカイブとディスクプランの ID を環境変数で指定します。ディスクサイズは `SKR_E2E_DISK_SIZE_GB` (既定 20) で指定できます。アーカイブかディスクプランが未設定の場合はテストをスキップします。2026-10-27 に移行後のシナリオをライブ実行し、cloud-init 起動・SSH 接続・更新・復元・削除と後片付けを確認しました。

```console
$ make build
$ SKR_E2E=1 \
    SKR_E2E_CLOUDIMG_ARCHIVE_ID=ARCHIVE_ID \
    SKR_E2E_DISK_PLAN_ID=DISK_PLAN_ID \
    go test ./test/e2e/server/ -run TestScenarioServer -v -timeout 40m
```

この実行では Server と Disk を作成し、起動と外部からの SSH 接続を確認してから削除します。利用料金が発生し、外部接続も行うため、環境変数による明示的な同意を必須にします。

## SSH 接続確認の再利用

SSH 鍵の生成、cloud-init の user-data の作成、SSH ログインの確認は、`test/e2e/internal/helper` パッケージに実装し、Server 以外のリソースを扱う E2E でも再利用します ([ADR 0063](../adr/0063-reusable-ssh-and-local-command-recording.md))。

- `helper.WriteSSHMaterial` は ssh-keygen で一時的な鍵ペアを生成し、指定したユーザーでログインするための cloud-init user-data を作成します。公開鍵だけを user-data に記載し、秘密鍵の内容は含めません。sudo を設定すると NOPASSWD の sudo 権限を user-data に含めます。
- `helper.ConnectSSH` は一時的な鍵でリモートの `hostname` を取得し、ログインできることを確認します。`helper.RunSSHCommand` はリモートコマンドを1回実行し、stdout を返します。
- 両者は `helper.RunLocalCommand` を通じて実行結果を実行記録へ保存します。SSH はバッチモードで対話を禁止し、未知のホスト鍵は一時ファイルに記録します。

SSH の接続試行はすべて実行記録に残り、実行引数、標準出力、標準エラー、所要時間を確認できます。`hostname` の出力によってログイン成功を確認できます。接続失敗は期限付きで再試行します。

## 検証

単体テストでは Create リクエストの組み立て、不正なオプションの組み合わせ、Boot の user-data ファイルパス、各種エラーを検証します。シナリオのテストでは偽の操作 (`fakeServerCommands`) を使い、引数の組み立て、SSH 接続の成功と失敗、リソースの識別、後始末を確認します。ライブ API には接続しません。helper の SSH 処理は鍵の生成、user-data の内容、SSH の引数、ローカルコマンドの実行記録を単体テストで検証します。
