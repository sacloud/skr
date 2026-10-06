# ADR 0022: tagpr を起点に GoReleaser とコンテナイメージを公開する

- Status: Accepted
- Date: 2026-10-06

## Context

tagpr は `main` ブランチでリリース PR とタグを作成します。一方、既存のコンテナイメージ公開はタグの push に依存していました。`GITHUB_TOKEN` で作成したタグでは別の push ワークフローが起動しないため、tagpr を起点に成果物を公開できる構成が必要です。GoReleaser の既存設定も、現在の `cmd/skr` エントリーポイントと v2 の設定形式に合わせる必要があります。

## Decision

- [usacloud のワークフロー](https://github.com/sacloud/usacloud/blob/main/.github/workflows/tagpr_and_release.yml)を参考に、tagpr とリリースを同じワークフローに配置します。
- tagpr の `tag` 出力がある場合だけリリースジョブを実行し、そのタグをチェックアウトして公開します。
- GitHub Release は GoReleaser が作成します。tagpr の Release 作成は無効にします。
- GoReleaser v2 で各 OS・CPU 向けの ZIP ファイルと SHA-256 チェックサムを公開します。
- コンテナイメージは Docker Buildx で Linux の amd64 と arm64 向けにビルドし、GHCR に公開します。
- Homebrew、OS パッケージ、GPG 署名は今回の対象外です。既存の `dev` イメージ公開は維持します。

## Consequences

タグの push イベントや追加の GitHub App トークンに依存せず、`GITHUB_TOKEN` で公開できます。リリースジョブには `contents: write` と `packages: write` が必要です。

バイナリとイメージは同じタグのソースからビルドします。両方の公開は単一のトランザクションではないため、イメージ公開に失敗しても先に公開した GitHub Release は残ります。失敗したジョブを再実行して復旧します。

## Alternatives considered

- タグの push で別ワークフローを起動する方法: GitHub App トークンなどを追加で管理する必要があるため採用しません。
- GoReleaser でコンテナイメージも公開する方法: 既存の Dockerfile と Buildx を使う公開方式を維持するため採用しません。
