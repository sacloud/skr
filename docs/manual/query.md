# API 出力の jq 加工

API コマンドの結果は、グローバルな `--query` に jq 式を指定して加工できます。式は [jq のマニュアル](https://jqlang.github.io/jq/manual/)を参考にしてください。

```console
$ skr iaas-api switch find --zone ZONE --query '.[].Name'
```

ゾーン一覧を取得して名前の配列を表示する例は、[IaaS Zone API チュートリアル](tutorials/iaas-api/zone.md)を参照してください。

クエリは API の結果全体に適用されます。複数の値を返す式では、各結果を JSON として順に出力します。結果が 0 個の場合は何も出力しません。クエリを指定すると `--output` とプロファイルの既定出力形式は使われず、クエリ結果を JSON で出力します。

`--query` は jq 式を Go で実装した [gojq](https://github.com/itchyny/gojq) で評価します。jq コマンド本体のインストールは不要です。jq と実装の差異や未対応機能については [gojq の説明](https://github.com/itchyny/gojq#differences-from-jq)を確認してください。

`--query` は API コマンドの結果に適用されます。`skr http` のレスポンス本文は加工されず、そのまま出力されます。
