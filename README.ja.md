# Forma

[en](README.md) / **ja**

**アプリケーションそのものを高水準に記述する言語。**

Formaは、coding agentへ渡す自然言語promptを、型付き・検査可能・review可能な言語へ置き換えます。

> 人間はFormaでapplication intentを決めます。compilerはその意味を解決し、AI coding agentが
> 通常のsoftware repositoryへ実装します。

```forma
type Email = String matches /.+@.+/

entity User {
    name  String required
    email Email required unique

    state status Pending | Confirmed | Active | Suspended initial Pending
}

action User.activate: Confirmed -> Active

page Users {
    allow admin

    list User {
        columns name, email, status
        search name, email
        filter status
        sort name asc
        paginate 20
        actions activate
    }
}
```

`list User`は「Userの集合を利用者へ提示する」という意味です。HTML table、React component、endpoint、
query builderを指定するものではありません。coding agentが対象repositoryを読み、解決済みintentを
保つ実装を選びます。

## 中心となる実行モデル

AI生成はFormaで選択可能なbackendではありません。end-to-endの中心となる実行モデルです。

```text
Forma source
  → parse / check / resolve
  → Resolved Intent + Acceptance Facts
  → Generation Request
  → AI coding agent + target repository
  → 通常のapplication code
  → build / test
  → failureをagentへfeedback
```

決定的なForma front-endの責務は、意味を確定するところまでです。その意味をframework固有fileへ
loweringしません。coding agentはmachine-readableなrequestを受け取り、実際のrepositoryにある
architecture、library、convention、testを使って実装します。

この境界がFormaの独自性です。

> 従来のDSLはcode generatorを作る。FormaはAIがcodeを作るための、promptより強い入力を作る。

責務境界とrequest/feedback loopは[Agent Generation Model](docs/agent-generation.md)にまとめています。

## Formaが記述するもの

1つのcompilation unitが1つのapplication namespaceを表します。現在のv0設計には次の概念があります。

```text
Application
├── Data
│   ├── Type
│   ├── Entity
│   ├── Field
│   └── Relation
├── Behavior
│   ├── State
│   └── Action
├── Presentation
│   ├── Page
│   ├── List
│   ├── Detail
│   └── Form
└── Authorization
    └── Role
```

Relationは独立primitiveではなくentity型のFieldとして表します。Actionは現在、許可されたentityの
state transitionを表します。正確なsyntaxとsemanticsは
[Forma v0仕様](docs/v0-primitives.md)に定めています。

次の概念はまだ設計中です。

```text
Under design
├── Expression
│   ├── Derived value
│   ├── Invariant
│   └── Precondition
├── Changes
├── Occurrence
├── Effect
└── Identity
```

[最小式レイヤ案](docs/expression-proposal.md)、
[注文承認・在庫probe](docs/order-approval-proposal.md)、
[表側の会員登録案](docs/public-membership-proposal.md)で実例から検討しています。

## なぜFormaなのか

AI coding agentは、すでに次のような高水準の要求を受け取っています。

> nameとemailで検索できる、page size 20のUser一覧を追加して。

自然言語promptは便利ですが、永続的なapplication sourceとしては弱いものです。名前解決や型がなく、
省略と意思決定の区別も曖昧で、後のpromptが以前の意味を再解釈できます。散文の仕様書にも同じ同期問題が
あり、人間は継続的に読み直して実装と比較しなければなりません。

Formaは同じ要求を、parse・検査・diff・reviewできるsourceへ変えます。

```forma
page Users {
    list User {
        search name, email
        paginate 20
    }
}
```

compilerはcoding agentがrepositoryを変更する前に、field名のtypo、type mismatch、不正なstate
transition、未解決action、矛盾したpermissionを検出します。その後、参照と決定的なdefaultをすべて
解決したapplicationの意味を出力します。

## Resolved Intent

compilerの主出力は、lowering用の中間表現ではなく**Resolved Intent**です。coding agentが実装すべき
application intentを、target-neutralかつmachine-readableに列挙します。

解決済みentity、field、constraint、state、action、permission、page、capability、navigation、stableな
semantic identityを含みます。React component、HTTP verb、SQL、directory、package名、framework APIは
含みません。loading widget、relation picker、submission tokenなどの実現機構も含みません。

Source Mapは各resolved nodeをForma sourceへ結び、compilerとrepositoryのfailureを人間がreviewできる
Forma上の位置へ戻します。

## Acceptance Facts

Formaは、実装後に成立すべきstructuredかつtarget-neutralな事実も導出します。以下はその人間向け表示です。

```text
- User.activateはConfirmedからだけ成功する
- adminはUsers pageを閲覧できる
- Usersはnameとemailで検索できる
- Usersのlogical page sizeは20
- 不正なtransitionはstateを変えずに拒否される
```

coding agentはこれを、対象repositoryで通常使われているunit、integration、request、browser testへ
変換します。Forma自身はframework adapterを持たず、HTTP statusやDOM selectorを標準化しません。

各factはstable IDを持ち、repository固有testは対応するIDを参照します。generation成功時には、requestの
fact ID集合とtestがcoverした集合の一致、および全factの成功を機械的に検査します。

期待する意味はForma front-endが決定します。repository固有の実装方法と観測方法だけをagentが決めます。

## Formaとtarget repository

target repositoryは破棄専用artifactではなく、通常のapplication sourceです。coding agentは既存systemへ
機能を追加し、手書きcodeを保ち、既存architectureに従ってincrementalに変更できます。人間も引き続き
repositoryで作業できます。

FormaはFormaに記述されたapplication intentを所有し、repositoryは実装を所有します。意味の変更を
target codeだけへ加えるとdriftするため、次のagent requestまでにForma sourceへ反映します。Formaは
生成fileのbyte-identical性を要求しません。

具体的な実装判断はagentとrepositoryが所有します。

- componentとUI構造
- route、API、transport
- database schema、persistence、migration
- frameworkとlibraryの使用方法
- file layoutと命名
- target固有test
- 既存codeへのincremental integration

## 設計原則

- framework語彙から逆算せず、application intentを直接表す。
- 一つの概念に一つのcanonical formを持つ。
- 実装詳細は省略しても、意味を決める事実は省略しない。
- defaultと参照を決定的に解決し、理由を説明可能にする。
- semantic identityからdependencyと変更影響を追跡できるようにする。
- 実装shapeはrepository contextを読んだcoding agentへ任せる。
- build/test feedbackは実装修正に使い、intentの再定義には使わない。

詳細なreview基準は[Forma Language Design Principles](docs/language-design-principles.md)にあります。

## 目標

- 壊れやすいcoding promptを、型付きで永続的なapplication intentへ置き換える。
- domain ruleと利用者に見えるcapabilityを明示し、検査可能にする。
- stableなResolved Intentとmachine-readableなGeneration Requestを出力する。
- 新規・既存repositoryの両方へcoding agentが実装できるようにする。
- build/test failureを反復的な修正loopへ戻す。
- framework別generatorを保有せず、Forma変更をincrementalに適用する。

## 非目標

Formaは次を目指しません。

- built-inの決定的lowererで各frameworkを生成すること
- target profile capability matrixやframework adapter suiteを保守すること
- route、SQL、component、directory、test frameworkを標準化すること
- application codeのbyte-identicalな再生成を要求すること
- 未検査の自然言語を実行syntaxにすること
- parse、名前解決、型検査、Forma semanticsをLLMへ委ねること
- すべての低水準・systems programming languageを置き換えること

## 現在地

Formaは初期設計段階で、compilerは未releaseです。現在のGo front-endはdesign draft v0.4のgrammar、
parser、名前解決、型検査、semantic validation、stable identity、Resolved Intent、Source Mapを部分実装
し、管理画面flow向けの最小Acceptance Facts／Generation Request sliceも実装しています。最初のcontrolled
agent runではstandalone Go repositoryへ管理画面を実装し、43件すべてのAcceptance Factsを検証しました。
v0外のself-only Invariant sliceも実験的に含みます。

`experiments/`配下のGo管理画面generatorとtarget-neutral conformance adapterは、今後の正式architecture
ではなく、**意味を発見するための凍結済みprototype**です。front-endに不足していた情報の確認には
役立ちましたが、次に二つ目のframework generatorや共通runtime adapterを作ることはしません。

- [Forma v0仕様](docs/v0-primitives.md)
- [Agent generation model](docs/agent-generation.md)
- [Implementation Policy Manifest案](docs/implementation-policy-manifest-proposal.md)
- [開発ロードマップ](docs/roadmap.md)
- [言語設計原則](docs/language-design-principles.md)
- [ユーザー管理の完全例](examples/users.forma)
- [注文承認・在庫probe](examples/orders.forma)
- [最小式レイヤ案](docs/expression-proposal.md)
- [進行中の管理画面agent-generation experiment](experiments/admin-agent-e2e/README.md)
- [凍結済み管理画面生成prototype](experiments/admin-e2e/README.md)
- [凍結済みconformance prototype](experiments/conformance/README.md)

Go 1.24以上でcheckerを実行できます。

```bash
go run ./cmd/forma check examples/users.forma
go run ./cmd/forma check examples/orders.forma
go run ./cmd/forma resolve examples/users.forma
go run ./cmd/forma request experiments/admin-agent-e2e/app.forma
go run ./cmd/forma verify internal/agentrequest/testdata/admin.request.json experiments/admin-agent-e2e/target/generation-feedback.json
go test ./...
```

1回の`forma check`へ渡したfileとdirectoryが1つのcompilation unitになります。二つのexampleは独立した
applicationなので、上記のように個別に検査します。

`forma resolve`はcanonicalなResolved Intent JSON、`forma request`は現在のfull Generation Request
sliceを出力します。`forma verify`はimmutableなrequestに対してagent feedbackを検査します。次のmilestone
は既存target repositoryへのincremental changeと、自動repair loopです。
