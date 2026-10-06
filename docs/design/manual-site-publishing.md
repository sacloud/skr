# 利用者向けドキュメントの HTML 公開

## 目的

`docs/manual/` の Markdown と CLI の `--help` を、GitHub Pages で閲覧できる静的 HTML として公開します。コマンドヘルプは実行中の `skr` バイナリから取得し、実際の利用者向け出力と一致させます。

## 生成

- `make docs-site` は `skr` を一時ディレクトリにビルドし、`cmd/docsite` で `_site/` に HTML を生成します。
- Markdown は Go の Goldmark で変換します。生成物の CSS と検索スクリプトは HTML に含め、サイト閲覧時に外部 CDN や npm パッケージを必要としません。
- 全ページのナビゲーション直下に「開発中バージョン」のバナーを表示し、コマンドや仕様が変更される可能性と `usacloud` との互換性がないことを知らせます。
- `docs/manual/README.md` をサイトの `index.html` にし、その他の Markdown は相対パスを保った HTML に変換します。内部の Markdown リンクは HTML のリンクへ変換します。
- `skr --help` のコマンド一覧から全コマンドパスを取得し、各パスと親コマンドに対して `--help` を実行します。結果は `commands/index.html` にまとめ、ブラウザー内検索で絞り込めるようにします。

## 公開

`.github/workflows/publish-docs.yml` は `main` への push または手動実行でサイトを生成し、`actions/upload-pages-artifact` と `actions/deploy-pages` を使って公開します。公開用ブランチは作成しません。リポジトリの GitHub Pages 設定では、初回に Source を `GitHub Actions` に設定する必要があります。

Pull Request では `make docs-site` を実行し、サイト生成が壊れていないことを確認します。生成物はリポジトリにコミットしません。

## 運用上の制約

- コマンドヘルプの抽出は Kong が出力する `skr --help` のコマンド一覧を使います。CLI のコマンド構造を変更したときは、サイト生成テストも更新します。
- GitHub Pages の公開 URL はリポジトリ設定に従います。Markdown とサイト内ナビゲーションには相対リンクを使い、プロジェクトページのベースパスに依存させません。
- Go の依存関係には Markdown 変換用の Goldmark を追加しますが、npm や `package.json` は導入しません。
