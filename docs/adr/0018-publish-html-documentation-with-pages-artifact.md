# ADR 0018: GitHub Pages artifact で HTML ドキュメントを公開する

- Status: Accepted
- Date: 2026-10-06

## Context

利用者向けドキュメントは `docs/manual/` の Markdown と CLI の `--help` に分散しています。これらをブラウザーで読める1つのサイトへまとめます。CLI の最新ヘルプもコマンド単位で掲載します。さらに、`package.json` や JavaScript ツール群を導入せず、公開用ブランチも作成しないことが求められています。

## Decision

- `make docs-site` で Go 製の生成器を実行し、利用者向け Markdown と実際の `skr` バイナリが出力するコマンドヘルプから静的 HTML を生成します。
- Markdown の変換には Goldmark を使い、生成サイトは `_site/` に出力します。
- `main` への push と手動実行で GitHub Actions を起動し、Pages artifact をアップロードして GitHub Pages にデプロイします。公開用ブランチは使いません。
- `package.json` と npm の依存関係は導入しません。
- HTML サイトの全ページ上部とリポジトリ README に「開発中バージョン」と明示し、利用者に仕様変更の可能性を伝えます。

## Consequences

- `docs/manual/` の内容と CLI のヘルプが同じサイトから読めます。コマンドヘルプ一覧は検索できます。
- 利用者はどのページから閲覧を始めても、開発中で仕様変更があり得ることを確認できます。
- Pull Request の CI でサイトを生成するため、HTML 変換とヘルプ収集の失敗を公開前に検知できます。
- GitHub Pages の Source は初回設定で `GitHub Actions` を選択する必要があります。
- Go module に Goldmark が追加されますが、Node.js のパッケージ依存は増えません。
- サイト生成は `skr --help` が列挙する全コマンドを実行して確認します。新しいコマンドも自動でヘルプ一覧に含まれます。

## Alternatives considered

- Markdown を Node.js の静的サイトジェネレーターで変換する方法: npm と `package.json` が必要になるため採用しません。
- ヘルプを Markdown に手動転記する方法: 実際の `--help` と説明が一致しなくなるため、採用しません。
- `gh-pages` などのブランチに生成物を push する方法: 公開物を artifact としてデプロイする要件に合わせ、採用しません。

生成方式の詳細は [利用者向けドキュメントの HTML 公開](../design/manual-site-publishing.md) に記録します。
