# Forma

[en](README.md) / **ja**

**何を作るかを厳密に記述し、どう作るかをAIに委ねるための言語。**

Formaは、coding agentへ渡すapplication specificationを、自然言語だけに委ねず、型付き・検査可能・
review可能なsourceとして記述するための、初期段階の実験的なプログラミング言語です。

現在は次の仮説を、言語設計とend-to-end experimentの両方から検証しています。

> 人間がapplicationの意味をFormaで決め、compilerが曖昧さを取り除けば、AIは通常のsoftware
> repositoryへその意味を安全に実装・更新できるのではないか。

まだproduction-readyなcompilerや完成したecosystemを提供するprojectではありません。

## なぜFormaが必要なのか

AI coding agentは、すでに次のような要求からapplication codeを作れます。

> nameとemailで検索できる、page size 20のUser一覧を追加して。

しかし、自然言語promptは永続的なapplication sourceとしては弱いものです。field名やactionの参照を
機械的に解決できず、typeやstate transitionの矛盾を実装前に検査できません。書かなかったことが
意図的な省略なのか、単なる記述漏れなのかも曖昧です。別のagentや後日のsessionが同じ文章を異なる
behaviorとして解釈する可能性もあります。

### なぜ今なのか

AI-assisted developmentによって、人間がframework固有codeを一行ずつ書く機会は減っています。一方で、
AIが生成したfrontend、backend、database、testのすべてを、変更のたびに精読し続けることも現実的では
ありません。そこで、人間が保守する短く意味密度の高いsourceと、AIが保守する通常のrepository codeを
結ぶ層が必要になります。

### Spec-Driven Developmentとの関係

Spec-Driven Development（SDD、仕様駆動開発）は、実装前に要求を明文化し、仕様・plan・taskを通じて
人間とcoding agentの認識差を減らす有効な方法です。FormaはSDDを否定するものではありません。

ただし、application behaviorの正本が主に自然言語のMarkdownである限り、次の問題は残ります。

- field名やaction参照のtypo、参照切れを実装前に確実には拒否できない。
- type、state transition、permission、constraintの整合性が、人間やagentの解釈に残る。
- 仕様、実装、testの対応を変更のたびに読み直し、同期し続ける必要がある。
- 仕様のどの要求がtestで覆われたかを、機械的に確認しにくい。

SDDが「仕様なしで実装を始める」問題を解くなら、Formaはその仕様の中心をparse・型検査・参照解決・
coverage確認できるsourceにします。背景、design rationale、非機能要件には引き続きMarkdownを使い、
applicationの構造と振る舞いはFormaで固定する、という関係です。

## Formaの答え

先ほどの要求は、Formaでは次のように書けます。

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

このsourceなら、compilerはcoding agentがrepositoryを変更する前に、field名のtypo、type mismatch、
不正なstate transition、未解決action、矛盾したpermissionを検出できます。参照とdefaultも決定的に
解決されるため、人間とAIが同じapplication intentを共有できます。

一方、`list User`は「Userの集合を利用者へ提示する」という意味だけを表します。HTML table、React
component、endpoint、query builderを指定するものではありません。それらはcoding agentが対象の
repositoryを読み、そのarchitectureに合わせて決めます。

## Applicationができるまで

AI生成はFormaの選択可能なbackendではなく、中心となる実行モデルです。

```text
Forma source
  → Go front-end: format / parse / resolve / type check / semantic check
  → Resolved Intent + Acceptance Facts
  → Generation Request
  → AI coding agent + target repository
  → 通常のapplication code
  → build / test
  → failureをagentへfeedback
```

Forma front-endは、applicationの意味を決定するところまでを担当します。framework固有のfileへ直接
loweringしません。coding agentはmachine-readableなrequestを受け取り、実際のrepositoryにある
architecture、library、convention、testを使って実装します。

> 従来のDSLはcode generatorを作る。FormaはAIがcodeを作るための、promptより強い入力を作る。

責務境界とrequest/feedback loopの詳細は
[Agent Generation Model](docs/agent-generation.md)にまとめています。

### 「何を作るか」と「何を使って作るか」

実際の生成では、次の3つを明確に分けます。

| 入力 | 決めること |
| --- | --- |
| `*.forma` | entity、state、action、page、permissionなど、**何を作るか** |
| Implementation Policy Manifest | framework、library、禁止事項など、**何を使って作るか** |
| target repository | 既存code、dependency、architecture、build/test commandなど、**現在どうなっているか** |

たとえば、Formaが「User一覧をnameで検索できる」と決め、manifestが検索libraryとしてRansackを指定し、
repositoryがRails applicationなら、coding agentはその3つを統合してrepository-nativeな実装を作ります。
Forma core自身は`ransack`という技術名を解釈しません。

Implementation Policy Manifestは現在proposal段階です。詳しくは
[設計案](docs/implementation-policy-manifest-proposal.md)を参照してください。

## Formaで記述するもの

1つのcompilation unitが1つのapplication namespaceを表します。現在のv0設計には、次の概念があります。

| 領域 | 記述する概念 |
| --- | --- |
| Data | Type、Entity、Field、Relation |
| Behavior | State、Action |
| Presentation | Page、List、Detail、Form |
| Authorization | Role |

Relationは独立primitiveではなく、entity型のFieldとして表します。Actionは現在、許可されたentityの
state transitionを表します。正確なsyntaxとsemanticsは
[Forma v0仕様](docs/v0-primitives.md)に定めています。

次の概念は、実例を使いながら設計中です。

| 概念 | 解決したいこと |
| --- | --- |
| Expression | field参照、比較、算術、条件 |
| Derived Value | 他の値から導出される値 |
| Invariant / Precondition | 常に守る制約、action実行前の条件 |
| Changes | actionによる事後状態 |
| Occurrence / Effect | 発生した事実と、email・通知などの外部効果 |
| Identity | 利用者本人とdomain dataの関係 |

[最小式レイヤ案](docs/expression-proposal.md)、
[注文承認・在庫probe](docs/order-approval-proposal.md)、
[表側の会員登録案](docs/public-membership-proposal.md)で検討しています。

## CompilerがAIへ渡すもの

Forma sourceは、そのまま長いpromptへ変換されるわけではありません。front-endが意味を解決し、次の
機械可読な出力を作ります。

### Resolved Intent

Resolved Intentは、coding agentが実装すべきapplicationの意味です。解決済みのentity、field、constraint、
state、action、permission、page、capability、navigation、stable semantic identityを含みます。

React component、HTTP verb、SQL、directory、package名、framework API、loading widget、relation picker、
submission tokenなどの実現方法は含みません。Source Mapが各nodeを元のForma sourceへ結び、compilerや
repositoryのfailureを人間が確認できる位置へ戻します。

### Acceptance Facts

Acceptance Factsは、実装後に成立すべきtarget-neutralな事実です。

```text
- User.activateはConfirmedからだけ成功する
- adminはUsers pageを閲覧できる
- Usersはnameとemailで検索できる
- Usersのlogical page sizeは20
- 不正なtransitionはstateを変えずに拒否される
```

coding agentは各factを、対象repositoryで通常使われているunit、integration、request、browser testへ
変換します。FormaはHTTP statusやDOM selectorを標準化せず、各factへstable IDを付けます。生成後は、
要求されたfact IDとtestがcoverしたIDの集合が一致し、すべて成功したことを機械的に確認します。

つまり、**何を保証するか**はFormaが決め、**そのrepositoryでどう実装・観測するか**はagentが決めます。

## Target repositoryとの関係

target repositoryは破棄専用artifactではなく、通常のapplication sourceです。coding agentは既存systemへ
機能を追加し、手書きcodeを保ち、既存architectureに従ってincrementalに変更できます。人間も引き続き
repositoryで作業できます。

FormaはFormaに記述されたapplication intentを所有し、repositoryは具体的な実装を所有します。

- componentとUI構造
- route、API、transport
- database schema、persistence、migration
- frameworkとlibraryの使い方
- file layoutと命名
- target固有test
- 既存codeへのintegration

意味の変更をtarget codeだけへ加えるとForma sourceとdriftします。その変更は次のagent requestまでに
Formaへ反映します。Formaはapplication codeのbyte-identicalな再生成を要求しません。

## 現在地

Formaは初期設計段階で、compilerは未releaseです。現在のGo front-endはdesign draft v0.4のgrammar、
parser、名前解決、型検査、semantic validation、stable identity、Resolved Intent、Source Mapを部分実装し、
管理画面flow向けの最小Acceptance Facts／Generation Request sliceも実装しています。v0外のself-only
Invariant sliceも実験的に含みます。

最初のcontrolled agent runでは、Formaから生成したrequestをAI coding agentへ渡し、standalone Go
repositoryへ管理画面を実装しました。そこで導出した43件すべてのAcceptance Factsを検証できました。
これは「FormaがGo管理画面を生成した」のではなく、「Formaが意味を決め、AIが通常のGo applicationを
実装した」というexperimentです。

`experiments/`配下の旧Go管理画面generatorとtarget-neutral conformance adapterは、正式architecture
ではなく、**意味を発見するための凍結済みprototype**です。次に二つ目のframework generatorや共通
runtime adapterを作る予定はありません。

### Sourceから試す

Go 1.24以上でcheckerと現在のgeneration workflowを実行できます。

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

`forma resolve`はcanonicalなResolved Intent JSON、`forma request`はGeneration Request、
`forma verify`はimmutableなrequestに対するagent feedbackの検査結果を出力します。次のmilestoneは、
既存target repositoryへのincremental changeと自動repair loopです。

## 設計資料

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

## 設計上の境界

Formaは、application intentを直接表し、defaultと参照を決定的に解決し、semantic identityから変更の
影響を追跡できることを目指します。build/test feedbackは実装修正に使い、intentの再定義には使いません。

一方、Formaは次を目指しません。

- built-inの決定的lowererで各frameworkを生成すること
- target profile capability matrixやframework adapter suiteを保守すること
- route、SQL、component、directory、test frameworkを標準化すること
- application codeのbyte-identicalな再生成を要求すること
- 未検査の自然言語を実行syntaxにすること
- parse、名前解決、型検査、Forma semanticsをLLMへ委ねること
- すべての低水準・systems programming languageを置き換えること

詳細なreview基準は
[Forma Language Design Principles](docs/language-design-principles.md)にまとめています。
