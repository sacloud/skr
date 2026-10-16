# cloud-init のトラブルシューティング

この文書は、さくらのクラウドの Server で cloud-init が動作しない場合に、アーカイブ、起動方法、ログを順に確認するための開発・運用メモです。利用者向けマニュアルには掲載しません。

## cloudimg アーカイブを使う

さくらのクラウドで cloud-init を使う Server は、cloud-init 対応のパブリックアーカイブを使います。アーカイブ検索では名前や説明に `cloudimg` が含まれるアーカイブを選び、通常の OS アーカイブを代わりに使わないでください。cloud-init 対応アーカイブ以外では、cloud-init のコマンドやサービス、ログを確認できないことがあります。

アーカイブ選択だけで user-data は適用されません。skr では Server を作成した後、起動時に `iaas-api server boot --user-data @path` で user-data を渡します。作成時に Server を起動せず、起動操作で user-data を渡してください。

## Server 上で確認する

SSH で Server に入り、cloud-init のコマンド、サービス、状態、ログファイルを確認します。

```console
$ command -v cloud-init
$ dpkg-query -W -f='${Status} ${Version}\n' cloud-init 2>/dev/null || true
$ systemctl list-unit-files 'cloud-init*'
$ systemctl status cloud-init-local.service cloud-init.service cloud-config.service cloud-final.service --no-pager
$ sudo cloud-init status --long
$ sudo ls -l /var/log/cloud-init*.log
```

`dpkg-query` の出力だけで動作を判断しないでください。パッケージ情報が表示されても、実行ファイルや systemd unit、ログが存在するとは限りません。`command -v`、unit の一覧、ログファイルを合わせて確認します。unit が見つからない場合やログがない場合は、まず使用したアーカイブが cloudimg かを確認してください。

## ログを確認する

- `/var/log/cloud-init.log` には cloud-init の各段階、データソース検出、設定モジュールの実行状況やエラーが記録されます。
- `/var/log/cloud-init-output.log` には user-data のスクリプトやコマンドによる標準出力・標準エラーを記録します。`runcmd` の結果を確認するときはこちらを見ます。

```console
$ sudo less /var/log/cloud-init.log
$ sudo less /var/log/cloud-init-output.log
$ sudo grep -nE 'ERROR|Traceback|FAILED|DataSource|finished at' /var/log/cloud-init.log
$ sudo journalctl -b --no-pager \
    -u cloud-init-local.service -u cloud-init.service \
    -u cloud-config.service -u cloud-final.service
```

まず最新の起動時刻に対応する `finished at`、データソースの検出、エラーの直前の処理を確認します。`cloud-init-output.log` では `runcmd` やスクリプトの標準出力・標準エラーを探します。現在の起動の journal に記録がない場合は、ログファイルと `/var/lib/cloud/instance/boot-finished` も確認してください。journal の過去起動分が保持されている環境では、`journalctl -b -1` で直前の起動も調べられます。

ログや `/var/lib/cloud/instance/user-data.txt` には user-data が含まれる場合があります。公開鍵以外の秘密情報を user-data に書かず、調査結果を共有するときも認証情報や秘密情報が含まれていないか確認してください。

## この調査で使った検証用 Server の記録

この節の「検証用 Server」は、Networking Suite の動作確認で SSH 接続と cloud-init の設定確認に使った Ubuntu Server 24.04.2 LTS (cloudimg) のことです。共有 NIC を SSH 接続用に使い、起動時に `server boot --user-data` で cloud-init の設定を渡しました。以前の調査記録ではこれを「Server A」と呼んでいましたが、特定のサービス上の役割名ではありません。以下はこの検証用 Server で観測した例であり、読者の環境で同じ状態になることを保証するものではありません。

cloudimg アーカイブに切り替えた後、この Server では次の状態を確認しました。

- `/usr/bin/cloud-init` と `/var/log/cloud-init.log`、`/var/log/cloud-init-output.log` が存在しました。
- `cloud-init.log` には `DataSourceNoCloud` が `/dev/vdb` からデータを読み込んだ記録がありました。
- `config-ssh` の処理が成功し、`/home/ubuntu/.ssh/authorized_keys` へ鍵を書き込んだ記録がありました。ログには鍵の内容を転載していません。
- 同じログには `modules-final` の成功と cloud-init の `finished` が記録されていました。`cloud-init-output.log` にも処理の出力がありました。
- 一方、確認時の `cloud-init status --long` は `disabled` / `disabled-by-generator` と表示しました。ログには cloud-init が実行・完了した記録があるため、この表示だけを根拠に「未実行」と判断できませんでした。この Server で表示が異なる理由は特定できていません。自身の環境で同様の食い違いがあれば、`status`、ログの時刻、`boot-finished`、対象 boot の journal を突き合わせてください。

これ以前に使った cloud-init 非対応の OS アーカイブでは、`cloud-init` コマンド、cloud-init 関連の systemd service、`/var/log/cloud-init.log` が見つかりませんでした。`dpkg-query -W cloud-init` にパッケージ名が出ることだけでは、user-data を処理できるアーカイブかどうかは確認できません。

## 参照

- [サーバの cloud-init 設定](https://manual.sakura.ad.jp/cloud/server/cloud-init.html)
- [cloud-init logging](https://cloudinit.readthedocs.io/en/latest/reference/logging.html)
- [cloud-init debugging](https://cloudinit.readthedocs.io/en/latest/topics/debugging.html)
