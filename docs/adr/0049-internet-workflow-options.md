# ADR 0049: ルータ＋スイッチの操作を個別オプションで完結させる

- Status: Accepted
- Date: 2026-10-15

## Context

ルータ＋スイッチを作成し、Server の NIC を接続して SSH でログインする手順では、スイッチ ID の指定、単一名での検索、削除動作の指定に JSON が必要でした。いずれも限定的な入力であり、複雑な構成を公開せずに個別オプションで表現できます。

## Decision

- Server Create の `--nic` に `upstream=SWITCH_ID` を追加します。値は正の整数とし、既存の `shared` と `disconnected` を維持します。指定順は変えず、OS の IP アドレス設定や名前によるスイッチ検索は追加しません。ADR 0041 の初期範囲をこの点だけ拡張します。
- Internet、Server、Disk の Find に、単一名の部分一致検索用の `--name` を追加します。値を分割せず、1要素の `Names` に変換します。未指定時は `Names` を設定せず、明示された空文字列はそのまま渡します。複数名は JSON に残し、任意の配列フラグには一般化しません。
- 生成設定の `singleton: true` は、Find の `name` / `Names` / `string` の組み合わせだけに許可します。`pointer`、`conversion`、`separator` との併用を拒否します。生成器は CLI 入力の意味を推測しません。
- Internet Delete に `--force` と `--fail-if-not-found` を追加します。これらは API 本文ではなく削除の動作オプションです。`--force` は IPv6 の無効化と `NextHop` のある追加サブネットの削除を伴います。`--fail-if-not-found` を指定すると、対象不在時もエラーを返します。省略時はいずれも false とする既存動作を維持します。
- すべての変更で JSON 入力を維持し、個別オプションとの併用を拒否します。E2E とチュートリアルには新しい個別オプションを使います。

## Consequences

SDK v0.3.0 の `FindRequest.ToRequestParameter` は `Names` を `search.AndEqual` に変換します。これは部分一致の AND 検索であり、単一名でも完全一致ではありません。フラグ経路でも同じ変換を維持します。E2E の削除では検索結果だけを信用せず、ID、名前、説明、接続先を読み直して照合します。

単純なネットワーク作成から SSH 接続、後始末までの手順で JSON リクエストが不要になります。cloud-init の内容は引き続き保護したファイルから渡します。生成設定・生成物・入力処理・ヘルプのテストと、is1b の再実行可能な SSH ライブ E2E で確認します。

## Alternatives considered

- 名前をカンマ区切りにする方法は、名前自体のカンマと区別できないため採用しません。
- 名前検索を `--query` のみで行う方法は、API 側の検索条件にならないため採用しません。
- 任意の配列をフラグ化する方法は、今回の用途に不要なため採用しません。
