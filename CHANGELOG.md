# Changelog

## [v0.0.2](https://github.com/sacloud/skr/compare/v0.0.1...v0.0.2) - 2026-10-09

### 🚀 Changes
- build: Docker ビルドを簡素化し実行用を Distroless に変更 by @tokuhirom in https://github.com/sacloud/skr/pull/46
- feat: add IaaS Disk API commands by @tokuhirom in https://github.com/sacloud/skr/pull/53
- docs: require live E2E and tutorials for IaaS commands by @tokuhirom in https://github.com/sacloud/skr/pull/54
- feat: add version command by @tokuhirom in https://github.com/sacloud/skr/pull/55
- Add jq query filtering and IaaS zone listing by @tokuhirom in https://github.com/sacloud/skr/pull/57
- fix: make default checks work for module layout by @tokuhirom in https://github.com/sacloud/skr/pull/56
- Add AppRun Dedicated API commands by @tokuhirom in https://github.com/sacloud/skr/pull/58
- Add config profile list and show commands by @tokuhirom in https://github.com/sacloud/skr/pull/63
- Remove YAML API output by @tokuhirom in https://github.com/sacloud/skr/pull/64
- Add service principal authentication profile management by @tokuhirom in https://github.com/sacloud/skr/pull/65
- config use コマンドを追加 by @tokuhirom in https://github.com/sacloud/skr/pull/66
- table 出力を廃止し JSON と --query に統一する by @tokuhirom in https://github.com/sacloud/skr/pull/67
- IAM API の中核3リソースを追加 by @tokuhirom in https://github.com/sacloud/skr/pull/69
- IaaS の全ゾーン検索を廃止する by @tokuhirom in https://github.com/sacloud/skr/pull/68
- E2E 実行レポートを Markdown で生成 by @tokuhirom in https://github.com/sacloud/skr/pull/71
- Potential fix for code scanning alert no. 6: Workflow does not contain permissions by @tokuhirom in https://github.com/sacloud/skr/pull/72
- Add remaining IAM API commands by @tokuhirom in https://github.com/sacloud/skr/pull/70
- Set identifiable User-Agent headers by @tokuhirom in https://github.com/sacloud/skr/pull/74
- Document command argument guidelines by @tokuhirom in https://github.com/sacloud/skr/pull/76
- docs: API コマンド対応計画を更新 by @tokuhirom in https://github.com/sacloud/skr/pull/77
- Container Registry API コマンドとライブ E2E を追加 by @tokuhirom in https://github.com/sacloud/skr/pull/78
- Show help for incomplete commands by @tokuhirom in https://github.com/sacloud/skr/pull/79
- Add --tags flags to IaaS API commands by @tokuhirom in https://github.com/sacloud/skr/pull/80
- Simplify README authentication and output sections by @tokuhirom in https://github.com/sacloud/skr/pull/81
- AGENTS.md にレビュー指摘の再発防止方針を追加する by @tokuhirom in https://github.com/sacloud/skr/pull/94
### 📦 Dependency Updates
- ci: bump actions/deploy-pages from 4.0.5 to 5.0.1 by @dependabot[bot] in https://github.com/sacloud/skr/pull/62
- ci: bump actions/configure-pages from 5.0.0 to 6.0.0 by @dependabot[bot] in https://github.com/sacloud/skr/pull/61
- ci: bump actions/upload-pages-artifact from 4.0.0 to 5.0.0 by @dependabot[bot] in https://github.com/sacloud/skr/pull/60

## [v0.0.1](https://github.com/sacloud/skr/commits/v0.0.1) - 2026-10-06

### 🚀 Changes
- Add EventBus API commands and tutorial by @tokuhirom in https://github.com/sacloud/skr/pull/9
- docs: record project architecture decisions by @tokuhirom in https://github.com/sacloud/skr/pull/13
- docs: write skill files in Japanese by @tokuhirom in https://github.com/sacloud/skr/pull/14
- docs: clarify Japanese writing and text lint rules by @tokuhirom in https://github.com/sacloud/skr/pull/15
- docs: define CLI command architecture by @tokuhirom in https://github.com/sacloud/skr/pull/18
- Add IaaS Switch API commands and tutorial by @tokuhirom in https://github.com/sacloud/skr/pull/19
- Add all-zone IaaS search support by @tokuhirom in https://github.com/sacloud/skr/pull/20
- Support configurable JSON, YAML, and table output by @tokuhirom in https://github.com/sacloud/skr/pull/21
- Add SimpleMQ API commands and E2E by @tokuhirom in https://github.com/sacloud/skr/pull/22
- Clarify first-party documentation voice by @tokuhirom in https://github.com/sacloud/skr/pull/23
- Remove CLI help from tutorial references by @tokuhirom in https://github.com/sacloud/skr/pull/24
- Add EventBus SimpleMQ live E2E tutorial by @tokuhirom in https://github.com/sacloud/skr/pull/25
- README を現状に合わせて整理 by @tokuhirom in https://github.com/sacloud/skr/pull/26
- CI: push-triggered tests only on main by @tokuhirom in https://github.com/sacloud/skr/pull/27
- Unify E2E evidence and document table output by @tokuhirom in https://github.com/sacloud/skr/pull/28
- docs: organize user manuals under docs/manual by @tokuhirom in https://github.com/sacloud/skr/pull/29
- ci: categorize release notes by @tokuhirom in https://github.com/sacloud/skr/pull/30
- feat: add IaaS API command generator by @tokuhirom in https://github.com/sacloud/skr/pull/33
- sacloud-sdk-go API対応計画を追加 by @tokuhirom in https://github.com/sacloud/skr/pull/34
- GitHub Pages artifact でドキュメントを公開 by @tokuhirom in https://github.com/sacloud/skr/pull/35
- Add IaaS Server API commands by @tokuhirom in https://github.com/sacloud/skr/pull/36
- Add shared Ogen API generator and migrate SimpleMQ by @tokuhirom in https://github.com/sacloud/skr/pull/38
- Add expand/collapse controls to command help by @tokuhirom in https://github.com/sacloud/skr/pull/37
- Add authenticated HTTP request command by @tokuhirom in https://github.com/sacloud/skr/pull/39
- Add -H short option for skr http --header by @tokuhirom in https://github.com/sacloud/skr/pull/40
- feat: generate EventBus API commands by @tokuhirom in https://github.com/sacloud/skr/pull/41
- Add global SDK HTTP tracing option by @tokuhirom in https://github.com/sacloud/skr/pull/42
- Move CLI package under cmd/skr and internal/cli by @tokuhirom in https://github.com/sacloud/skr/pull/43
- Update Docker Go image to 1.27.1 by @tokuhirom in https://github.com/sacloud/skr/pull/44
- ci: tagpr を起点にバイナリとコンテナイメージをリリース by @tokuhirom in https://github.com/sacloud/skr/pull/45
### 📦 Dependency Updates
- docker: bump golang from 1.26.1 to 1.27.0 by @dependabot[bot] in https://github.com/sacloud/skr/pull/4
- ci: bump docker/login-action from 3.7.0 to 4.6.0 by @dependabot[bot] in https://github.com/sacloud/skr/pull/8
- docker: bump alpine from 3.23.3 to 3.24.2 by @dependabot[bot] in https://github.com/sacloud/skr/pull/6
- ci: bump docker/setup-qemu-action from 3.7.0 to 4.4.0 by @dependabot[bot] in https://github.com/sacloud/skr/pull/7
- ci: bump docker/metadata-action from 5.10.0 to 6.2.0 by @dependabot[bot] in https://github.com/sacloud/skr/pull/5
- ci: bump actions/setup-go from 6.2.0 to 7.0.0 by @dependabot[bot] in https://github.com/sacloud/skr/pull/3
- ci: bump actions/checkout from 6.0.2 to 7.0.1 by @dependabot[bot] in https://github.com/sacloud/skr/pull/2
- ci: bump Songmu/tagpr from 1.9.0 to 1.21.0 by @dependabot[bot] in https://github.com/sacloud/skr/pull/10
- ci: bump docker/build-push-action from 6.18.0 to 7.4.0 by @dependabot[bot] in https://github.com/sacloud/skr/pull/11
- ci: bump docker/setup-buildx-action from 3.12.0 to 4.4.1 by @dependabot[bot] in https://github.com/sacloud/skr/pull/12
- go: bump google.golang.org/grpc from 1.82.1 to 1.83.2 by @dependabot[bot] in https://github.com/sacloud/skr/pull/16
- go: Bump go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp from 1.44.0 to 1.45.0 by @dependabot[bot] in https://github.com/sacloud/skr/pull/17
- go: bump github.com/sacloud/sakumock from 0.9.1 to 0.12.0 by @dependabot[bot] in https://github.com/sacloud/skr/pull/32
- ci: bump Songmu/tagpr from 1.21.0 to 1.21.1 by @dependabot[bot] in https://github.com/sacloud/skr/pull/31
