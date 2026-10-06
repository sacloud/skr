# ADR 0023: コンテナの実行用ベースイメージに Distroless を使う

- Status: Accepted
- Date: 2026-10-06

## Context

コンテナ内で実行するのは `CGO_ENABLED=0` でビルドした Go バイナリです。実行時にシェルやパッケージマネージャーは使わないため、Alpine より構成を絞った実行用イメージを使用できます。ビルド時にも lint などの開発用ツールは不要です。

## Decision

実行用ベースイメージを `gcr.io/distroless/static-debian13:latest` に変更します。CA 証明書はベースイメージに含まれるものを使用し、バイナリの配置先 `/usr/bin/skr` と exec 形式のエントリーポイントを維持します。

実行ユーザーの変更は今回の対象外とし、従来どおり root で実行します。ビルドステージは `make build` のみを実行し、`make tools` と不要な `bzr` のインストールを省きます。

## Consequences

実行用イメージからシェルとパッケージマネージャーを除外します。コンテナ内でのシェル操作やパッケージ追加はできなくなります。CLI の起動方法は変更しません。

Debian のメジャーバージョンをイメージ名で指定し、`latest` タグを使って同じ系列の更新を取り込みます。ビルドとコンテナ内での CLI 起動を確認します。

## Alternatives considered

Alpine の継続利用では、使わないシェルやパッケージマネージャーが残るため採用しません。`scratch` は CA 証明書などを個別に配置する必要があるため採用しません。`nonroot` タグは設定ファイルやマウント先のアクセス権に影響するため、今回の変更には含めません。

## 参照

- [Distroless イメージ](https://github.com/GoogleContainerTools/distroless)
