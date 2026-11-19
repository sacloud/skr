# DNS API: ゾーンとレコードを作って権威サーバーの応答を確認する

DNS は、ドメイン名 (ゾーン) とそのレコードを管理するサービスです。この手順ではレコード付きの DNS ゾーンを作成し、内容を照合したあと、権威 DNS サーバーに直接問い合わせてレコードの応答を確認します。DNS ゾーン自体は無料で利用できます（[DNS の概要](https://manual.sakura.ad.jp/cloud/dns/index.html)）。

ゾーンを作成しただけでは、そのゾーンはインターネットに公開されません。一般の resolver から応答を得るには、ドメイン登録事業者でネームサーバー (NS) の委譲設定が必要です。この手順では委譲しないため、権威サーバーへの直接問い合わせだけで確認します。

DNS ゾーン名には、実在するトップレベルドメイン (TLD) 配下で、Public Suffix List に掲載されていない名前を指定します。`example.com` や `example.jp` などの文書用に予約されたドメインは Public Suffix List に掲載されているため登録できません（[Public Suffix List](https://publicsuffix.org/)）。この手順では `.cloud` 配下のランダムな名前をダミーゾーンとして使います。既に取得済みのドメインをお持ちの場合は、その名前を使っても構いません。

> [!WARNING]
> 選択中のプロジェクトに DNS ゾーンを作成・削除します。実際に権威を持つドメイン名や、業務で利用中のドメイン名は指定しないでください。既存のゾーンは変更しません。

## 前提条件

- `skr` と `dig` を利用できる環境が必要です。
- DNS の操作が許可された認証プロファイルを用意してください。
- 以下のコマンドは同じシェルで実行します。作成したゾーンの ID はコマンド出力から設定してください。

## 作成するリソースと順序

1. A・TXT・MX・CNAME のレコード付きで DNS ゾーンを作成します。
2. 参照と検索でゾーンの内容を照合します。
3. 権威 DNS サーバーに `dig` で直接問い合わせ、A レコードの応答を確認します。
4. レコード一覧を置き換えて更新し、応答の変化を確認します。
5. ゾーンを削除します。

## Step 1: 対象環境とコマンドを確認する

```console
$ skr config current
$ skr dns-api --help
$ skr dns-api create --help
$ skr dns-api update --help
$ skr dns-api delete --help
```

## Step 2: レコード付きのゾーンを作成する

重複しないゾーン名を設定し、レコードを 4 件指定して作成します。`--record` は `name=レコード名,type=レコード種別,rdata=レコード値,ttl=秒` の形式で、4 つのキーをすべて指定します。全レコードの `ttl` は同じ値にします。レコード名はゾーン名そのもの (ゾーンの頂点) でも、`www` のようなゾーン内の名前でも構いません。MX・CNAME のレコード値は、末尾が `.` の完全修飾ドメイン名で指定します。MX の宛先ホストには、ゾーン内に A または AAAA レコードが必要です。

```console
$ ZONE_NAME="skr-tutorial-dns-$(date +%s).cloud"
$ skr dns-api create --name "$ZONE_NAME" \
    --description "Temporary DNS tutorial zone" \
    --record "name=www,type=A,rdata=192.0.2.10,ttl=3600" \
    --record "name=$ZONE_NAME,type=TXT,rdata=skr-tutorial-dns,ttl=3600" \
    --record "name=$ZONE_NAME,type=MX,rdata=10 www.$ZONE_NAME.,ttl=3600" \
    --record "name=docs,type=CNAME,rdata=www.$ZONE_NAME.,ttl=3600" \
    --query '{ID,Name,Description,Records}'
$ ZONE_ID=ZONE_ID
```

`ZONE_ID` は Create の出力の ID に置き換えます。レコードの種別には A、AAAA、ALIAS、CNAME、NS、MX、TXT、SRV、CAA、HTTPS、SVCB、PTR を指定できます。カンマを含むレコード値 (長い TXT など) は `--record` の形式では表現できないため、`--request` JSON で指定します。

## Step 3: ゾーンを確認する

Read で ID・ゾーン名・説明・レコード一覧と、権威 DNS サーバーの一覧を確認します。

```console
$ skr dns-api read --id "$ZONE_ID" \
    --query '{ID,Name,Description,Records,DNSNameServers}'
```

続いて Find で名前を検索します。DNS ゾーンの `--name` は部分一致検索です。

```console
$ skr dns-api find --name "$ZONE_NAME" --query 'map({ID,Name,Description})'
[
  {
    "ID": 123456789007,
    "Name": "skr-tutorial-dns-EXAMPLE.cloud",
    "Description": "Temporary DNS tutorial zone"
  }
]
```

出力例は ID と名前を置き換えています。

## Step 4: 権威 DNS サーバーの応答を確認する

Read の応答から権威 DNS サーバーを 1 台取り込みます。`--query-raw` は文字列の結果を引用符なしで出力するため、そのままシェル変数に取り込めます。

```console
$ NS=$(skr dns-api read --id "$ZONE_ID" --query-raw '.DNSNameServers[0]')
$ echo "$NS"
```

`dig` で権威サーバーに直接問い合わせます。`@` に取り込んだネームサーバーを指定するため、ゾーンが委譲されていなくても応答を確認できます。

```console
$ dig "@$NS" "www.$ZONE_NAME" A +norecurse +noall +comments +answer
;; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 12345
www.skr-tutorial-dns-EXAMPLE.cloud. 3600 IN A 192.0.2.10
```

出力例はゾーン名を置き換えています。`status: NOERROR` と、Step 2 で指定した IP アドレスと TTL を確認してください。作成直後は権威サーバーへのゾーン情報の反映が完了するまで `status: REFUSED` を返すことがあるため、その場合は数十秒待って再確認します。`dig` に `@` を指定しない場合、このゾーンは委譲されていないため一般の resolver からは応答を得られません。

## Step 5: レコード一覧を更新する

説明とレコード一覧を更新します。`--record` を指定するとレコード一覧全体が置き換わるため、保持したいレコードも再指定します。この手順では A レコードの IP と TTL を変更し、AAAA レコードを追加して、TXT レコードの値を変更します。Step 2 で指定した MX と CNAME は再指定しないため削除されます。

```console
$ skr dns-api update --id "$ZONE_ID" \
    --description "Updated DNS tutorial zone" \
    --record "name=www,type=A,rdata=192.0.2.20,ttl=600" \
    --record "name=www6,type=AAAA,rdata=2001:db8::10,ttl=600" \
    --record "name=$ZONE_NAME,type=TXT,rdata=skr-tutorial-dns-updated,ttl=600" \
    --query '{ID,Name,Description,Records}'
$ skr dns-api read --id "$ZONE_ID" --query '{ID,Name,Description,Records}'
```

## Step 6: 更新後の応答を確認する

同じネームサーバーに再び問い合わせ、A レコードの IP と TTL が更新後の値になっていることを確認します。

```console
$ dig "@$NS" "www.$ZONE_NAME" A +norecurse +noall +comments +answer
;; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 12346
www.skr-tutorial-dns-EXAMPLE.cloud. 600 IN A 192.0.2.20
```

出力例はゾーン名を置き換えています。古い値が応答される場合は、TTL に応じたキャッシュの残りを考慮して待ってから再確認します。

## Step 7: ゾーンを削除する

途中で失敗した場合も、この手順で作成した ID を確認して後片付けします。まずゾーンの ID・名前・説明を照合してから削除します。削除後は検索して、ゾーンが残っていないことを確認します。

```console
$ skr dns-api read --id "$ZONE_ID" --query '{ID,Name,Description}'
$ skr dns-api delete --id "$ZONE_ID" --fail-if-not-found
$ skr dns-api find --name "$ZONE_NAME" --query 'map({ID,Name})'
[]
```

DNS ゾーンは無料のため、この手順では料金が発生しません。

## 独自ドメインで利用する場合

実際に権威を持つドメインで DNS を利用する場合は、ドメイン登録事業者でこのゾーンのネームサーバー (Read の `DNSNameServers`) への NS 委譲を設定します。委譲が反映されると、一般の resolver からもゾーンのレコードが応答されるようになります。委譲の方法はドメインの管理サービスごとに確認してください（[DNS の概要](https://manual.sakura.ad.jp/cloud/dns/index.html)）。

## 参考資料

- [DNS（ドメイン）](https://manual.sakura.ad.jp/cloud/dns/index.html)
- [文書用に予約されたドメイン名 (RFC 2606)](https://www.rfc-editor.org/rfc/rfc2606.html)
- [sacloud-sdk-go v0.7.0: DNS service](https://github.com/sacloud/sacloud-sdk-go/tree/v0.7.0/service/iaas/dns)
