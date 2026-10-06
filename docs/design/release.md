# リリース設計

## 起点と公開順序

`.github/workflows/tagpr_and_release.yml` は `main` への push で起動します。
tagpr はリリース PR の作成・更新と、マージ後のタグ作成を担当します。
`.tagpr` の `release = false` により、GitHub Release は tagpr ではなく GoReleaser が作成します。

tagpr ジョブの `tag` 出力が空の場合、リリースジョブは実行しません。
タグがある場合は、リリースジョブでそのタグを履歴とともにチェックアウトし、次の順序で公開します。

1. `go.mod` のバージョンで Go をセットアップします。
2. GoReleaser v2 でバイナリをビルドし、GitHub Release に ZIP ファイルとチェックサムを公開します。
3. Docker Buildx でコンテナイメージをビルドし、GHCR に公開します。

`GITHUB_TOKEN` で作成したタグは別の push ワークフローを起動しないため、同じワークフロー内のジョブ依存関係でリリースを開始します。
手動でタグを push してもこのワークフローは起動しません。

## バイナリ

`.goreleaser.yml` で `./cmd/skr` をビルドします。`CGO_ENABLED=0` を指定し、バージョンとコミットの短縮ハッシュをビルド時に埋め込みます。
`skr version` と `skr --version` は、埋め込まれたバージョン、実行 OS・CPU、コミットの短縮ハッシュを表示します。

| OS | CPU |
| --- | --- |
| Linux | amd64、386、arm、arm64 |
| macOS | amd64、arm64 |
| Windows | amd64、386 |

アーカイブ名は `skr_<OS>-<CPU>.zip` です。チェックサムは `skr_SHA256SUMS` にまとめます。
プレリリースタグは GitHub Release でもプレリリースとして公開します。
Homebrew、deb、rpm、GPG 署名は設定しません。

## コンテナイメージ

既存の Dockerfile を使い、Linux の amd64 と arm64 をまとめたイメージを `ghcr.io/sacloud/skr` に公開します。

ビルドステージは `make build` で `CGO_ENABLED=0` のバイナリを生成します。開発用ツールをインストールする `make tools` は実行しません。
実行用ベースイメージには `gcr.io/distroless/static-debian13:latest` を使い、バイナリを `/usr/bin/skr` に配置します。CA 証明書はベースイメージに含まれるものを使用します。
実行ユーザーは従来どおり root です。実行用イメージにはシェルとパッケージマネージャーがないため、コンテナ内でのシェル操作やパッケージ追加はできません。

| タグ | 更新条件 |
| --- | --- |
| `v` 付きリリースタグ | tagpr がタグを作成したときに公開します。 |
| `latest` | プレリリースではないタグの公開時に更新します。 |
| `dev` | 既存の `publish_dev_docker_image.yaml` で `main` への push ごとに更新します。 |

リリースジョブはタグをチェックアウトしているため、バイナリとイメージのソースは一致します。
Docker metadata action へリリースタグを渡します。起動元の `main` ブランチ名はイメージタグとして使用しません。

## 権限と障害時の対応

tagpr ジョブは `contents: write`、`pull-requests: write`、`issues: write` を使います。
リリースジョブは GitHub Release 用の `contents: write` と GHCR 用の `packages: write` を使います。
認証には `GITHUB_TOKEN` のみを使い、Homebrew 用トークンや署名用シークレットは不要です。
リポジトリでは GitHub Actions による PR 作成を許可し、既存の GHCR パッケージがある場合はリポジトリからの書き込みを許可してください。

公開処理が失敗した場合は、対象ワークフローの失敗したジョブを再実行します。
tagpr ジョブまで含む全ジョブの再実行では、タグ出力は空になる場合があります。この場合、リリースはスキップされます。
バイナリ公開後にイメージ公開が失敗した場合も、リリースジョブの再実行で同じタグの成果物を公開し直します。

## ローカルでの確認

公開せずに設定と成果物を確認するには、GoReleaser v2 と actionlint を使います。

```console
$ goreleaser check
$ actionlint .github/workflows/tagpr_and_release.yml
$ goreleaser release --snapshot
```

スナップショットの成果物は `dist/` に出力します。リリースには使用しないでください。
`dist/` がすでに存在する場合は、内容を確認してから `--clean` を指定してください。

## 参照

- [usacloud の tagpr・リリースワークフロー](https://github.com/sacloud/usacloud/blob/main/.github/workflows/tagpr_and_release.yml)
- [tagpr の設定](https://github.com/Songmu/tagpr#configuration)
- [GoReleaser](https://goreleaser.com/)
- [Distroless イメージ](https://github.com/GoogleContainerTools/distroless)
- [GITHUB_TOKEN によるワークフロー起動の制約](https://docs.github.com/en/actions/how-tos/writing-workflows/choosing-when-your-workflow-runs/triggering-a-workflow#triggering-a-workflow-from-a-workflow)
