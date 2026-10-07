# API 出力の jq 加工

API コマンドの結果は、グローバルな `--query` に jq 式を指定して加工できます。式は [jq のマニュアル](https://jqlang.github.io/jq/manual/)を参考にしてください。

```console
$ skr iaas-api switch find --zone ZONE --query '.[].Name'
```

ゾーン一覧を取得して名前の配列を表示する例は、[IaaS Zone API チュートリアル](tutorials/iaas-api/zone.md)を参照してください。

クエリは API の結果全体に適用されます。複数の値を返す式では、各結果を JSON として順に出力します。結果が 0 個の場合は何も出力しません。クエリ結果も JSON で出力します。

一覧から確認に必要な項目だけを表示するには、配列を返す式を使います。

```console
$ skr iaas-api switch find --zone ZONE --query 'map({ID,Name})'
```

`map({ID,Name})` は各要素の ID と名前だけを残します。空一覧は `[]` として出力するため、削除後の確認にも使えます。項目名は対象 API の JSON 出力に合わせて指定してください。

`--query` は jq 式を Go で実装した [gojq](https://github.com/itchyny/gojq) で評価します。jq コマンド本体のインストールは不要です。jq と実装の差異や未対応機能については [gojq の説明](https://github.com/itchyny/gojq#differences-from-jq)を確認してください。

`--query` は API コマンドの結果に適用されます。`skr http` のレスポンス本文は加工されず、そのまま出力されます。
