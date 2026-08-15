# Forma

[English](README.md) | **日本語**

**アプリケーションを構築するための、より高水準なプログラミング言語。**

Formaは、framework固有の実装方法ではなく、entity、state、relation、action、page、list、form
といったアプリケーション概念によってソフトウェアを記述する、実験的なプログラミング言語です。

> Formaのsourceは、アプリケーションが「何であるか」を表現します。
> compilerはその意味を決定し、target generatorが実装へ変換します。

```forma
type Email = String matches /.+@.+/

entity User {
    name  String required
    email Email required unique

    state status Pending | Confirmed | Active | Suspended initial Pending
}

action User.activate: Confirmed -> Active

page Users {
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

`list User`は「ユーザーの集合を提示する」という意味です。HTMLのtable、React component、
database queryを指定するものではありません。target profileが、宣言されたアプリケーションの
意味を保ちながら、platformに適した実装を選択します。

## なぜFormaなのか

アプリケーションのsource codeには、性質の異なる2種類の情報が含まれています。

1. そのアプリケーションに固有の意思決定
2. frameworkやruntimeが要求する実装上の仕組み

Formaは、前者の密度を最大化し、後者の記述量を最小化することを目指します。

上の例でdeveloperが決めたのは、どのentityを表示するか、どのfieldを見せて検索可能にするか、
どのfieldで絞り込めるか、そしてlogical page sizeです。requestのencoding、data fetching、
loading/failure state、queryの組み立て、widget、cache更新、routingは実装上の仕組みです。

従来の実装では、これらをfrontend、backend、database、API contract、testの間で連携させる
必要があります。Formaでは、それらを一つのapplication-level declarationから生じる結果として
扱います。

## なぜ今Formaなのか

AI-assisted developmentによって、人間とsource codeの関係は変わりました。developerが実装コードを
一行ずつ書く機会は減り、生成された大量のコードをすべて読むことも現実的ではなくなっています。
それでも現在の開発では、frontend、backend、database、schema、testなどに異なる言語が使われ、
アプリケーションの仕様は設計書、実装コード、API spec、issue、promptへ分散したままです。

Kiroの`design.md`に代表されるspec-drivenな方法は、この分散を減らす有効な試みです。しかし、散文の
仕様書はそれ自体を継続的に読み、実装との対応を確認し、変更のたびに同期しなければなりません。
機械が実装の大部分を担う時代に、人間が長い仕様書と生成コードの両方を運用し続けるのは退屈で、
driftも起こりやすい作業です。

Formaの仮説は、必要なのが「さらに詳しい仕様書」ではなく、設計図として読め、programとしてparse・
検査・実行できる、より高水準な言語ではないか、というものです。人間が保守するのは短く意味密度の
高いForma sourceです。compilerはそこから意味を確定し、AIを含むtarget generatorが実装を作り、
conformanceが生成結果を検証します。

```text
人間が読む・reviewする       Forma source
機械が生成し保守する         target code
両者の意味の一致を検証する   compiler + conformance
```

Formaは自然言語promptを保存する仕組みでも、設計書から一度だけscaffoldを作る仕組みでもありません。
散在していた設計図、実装上の意思、検査可能なspecを、一つの実行可能なapplication languageへ
集約することを目指します。

## 思想

### コードはモデルに似ているべき

アプリケーションコードは、developerが実際に考えている概念に近い形であるべきです。
Userにはstateがあり、Orderにはlifecycleがあり、Pageには検索可能なListがあり、Actionが
systemを変化させます。これらの概念を言語で直接表現できるようにします。

### 抽象度を一段上げる

既存のプログラミング言語は、machine instruction、memory address、CPU registerを抽象化して
きました。Formaはさらに一段上へ進み、より大きなアプリケーションの意味を言語に組み込みます。

### 複雑さは抽象化の下に置く

Formaは、ソフトウェアが単純だとは考えません。繰り返し現れる実装上の複雑さを言語概念の背後へ
移し、compilerとruntimeが一貫して処理できるようにします。

### Syntax sugarではなくsemanticsを優先する

Formaは、Ruby、Go、TypeScriptを短く書くためのsyntaxではありません。`entity`、`state`、
`action`、`list`、`page`は、compilerが理解するアプリケーション上の意味を持ちます。

### 一つのsourceから複数のtargetへ

Forma sourceは特定のframeworkに結び付きません。同じアプリケーションモデルを、観測可能な
振る舞いを保ったまま、異なるtarget profileへ変換できることを目指します。

### 人間が読むsourceはFormaだけ

生成されたReact、Ruby、Goなどのtarget codeは、人間が継続的に編集するsourceではありません。
そのためForma source自体が、diff、review、検索、履歴管理に耐える、アプリケーション唯一の
source of truthでなければなりません。図が有用な場合も、UMLなどを別の入力として管理するのではなく、
Forma sourceから常に再生成できるviewとして扱います。

Formaでまだ表現できない要求を、生成コードへの手修正で補ってはいけません。宣言済みのintentを
profileが実装できない場合はcompile errorとし、言語自体で表現できない要求はv0のscope外として
明示します。将来のescape hatchも、version管理されたprofileまたはextensionとしてsource側に
残る形だけを認めます。

### 人間が正確に、長く読める

Forma sourceは、短いことや自然言語のように見えることより、applicationの意味を正確に読めることを
優先します。target frameworkを知らなくても、何が存在し、誰が何を見て変更でき、どのconstraintと
state transitionが適用され、変更によって何が変わるかを説明できなければなりません。

実装詳細は省略しますが、意味上重要な事実は隠しません。同じ概念は同じ形で表し、暗黙のdefaultと
参照解決は閉じた規則から決定的に導出し、compilerが展開して説明できるようにします。詳しい判断基準は
[Forma Language Design Principles](docs/language-design-principles.md)にまとめています。

### 意味は固定し、実装の形は固定しない

決定性が必要なのは、構文、名前解決、型、認可、遷移、navigation、conformance上の期待値です。
component分割、ファイル配置、framework APIの使い方まで同一である必要はありません。target
generatorがAIを利用して生成コードが変化しても、解決済みSemantic IRの意味を変えず、同じ
conformance contractを通過することを要求します。

## 一つの宣言から複数の結果へ

次のstate transitionを考えます。

```forma
action User.activate: Confirmed -> Active
```

これは一つの`if`文を短く書いたものではありません。次のアプリケーションルールを宣言します。

> UserはConfirmed stateからだけActiveになれる。

このルールから、targetはauthoritativeなbackend guard、frontendでのaction availability、
API contract、stateの再取得、正常・不正遷移のtest、documentationを生成できます。一つのForma
宣言が複数の生成物へ影響するのは、それがcode templateではなく「意味」を表すためです。

## アーキテクチャ

Formaは、target-neutralなSemantic IRと、そこから決定的に導出されるconformance contractを
中心に設計します。

```text
自然言語（任意）
      │
      ▼ AI（任意）
 Forma source
      │
      ▼ deterministic front-end
Lexer / Parser / AST / Checker
      │
      ▼
Semantic IR + Conformance Contract
      │
      ▼ target profile + generator（AI可）
Generated Application
      │
      ▼ build + deterministic conformance
Accepted Artifact
```

compilerは、type、relation、state transition、permission、action、navigationを解決してから、
target profileにcomponent、transport、persistence、runtime behaviorの選択を委ねます。

決定性の境界は、Forma sourceからSemantic IRとconformance contractまでです。

```text
Forma source + front-end version -> Semantic IR + Conformance Contract
```

target generatorはAIを利用でき、生成コードは実行ごとに異なって構いません。ただし、生成物は
人間が編集するsourceではなく、破棄・再生成可能なartifactです。正しさはコードの同一性ではなく、
build成功と、決定的に生成されたconformance contractへの適合で判定します。

## FormaとAI

AI-assisted developmentによって、developerは既存のプログラミング言語より高い抽象度で
アプリケーション変更を説明できることが明らかになりました。

> ユーザー一覧に、名前とメールアドレスの検索とページングを追加して。

現在はcoding agentが、この指示を多数のframework固有コードへ展開します。Formaは、同じ
高水準の指示を、永続的でreview可能かつ機械的に検査できるsource codeにできるかを探究します。

```forma
page Users {
    list User {
        search name, email
        paginate 20
    }
}
```

AIは、自然言語からFormaへの翻訳、target profileの作成、Semantic IRからtarget artifactへの
生成を支援できます。一方、Formaのsyntax、semantics、validation、Semantic IR、conformance
contractはAIに依存せず決定的に扱います。

AI target generatorへ渡すのは未検査のForma sourceではありません。解決済みSemantic IR、
versioned target profile、出力契約を渡します。生成物はbuildとconformanceを通過した場合だけ
採用し、失敗した場合はdiagnosticとともに再生成します。

## 目標

- アプリケーションの意思決定を、高い意味密度で表現する
- domain ruleと利用者に見えるcapabilityを、明示的かつ検査可能にする
- frontend、backend、persistence、testを通じて一貫した振る舞いを生成する
- Forma sourceをtarget frameworkから独立させる
- LLMに依存せず、意味と合否判定を再現可能にする
- target codeを編集不要な生成artifactとして扱う

## 目標にしないこと

Formaは、次のものを目指しません。

- あらゆる低水準言語やsystems programming languageを置き換える
- すべてのtarget frameworkの全機能を公開する
- 自然言語を実行可能なsyntaxとして使う
- 構文解析、意味解決、conformance oracleをLLMの判断に委ねる
- 生成されたtarget codeを手編集し、Forma sourceと真実を二重化する
- framework固有のshortcut集になる

## 現在の状況

Formaは初期設計段階であり、compilerはまだreleaseされていません。現在の未release Go
front-endは、design draft v0.4のsurface syntaxを部分実装し、lexer、parser、syntax AST、名前解決、
型検査、semantic validation、diagnostic、`forma/v0.3` core Semantic IRまで実装しています。

規範仕様の全機能が実装済みという意味ではありません。conformance contract、IR source map、
target profile capability check、artifact生成・検証protocolは未実装です。規範文書はこれらを含む
design draft v0.4で、reference実装はその一部です。

- [Forma v0仕様](docs/v0-primitives.md)
- [開発ロードマップ](docs/roadmap.md)
- [ユーザー管理の完全例](examples/users.forma)
- [Architecture Manifest案（検討中）](docs/architecture-manifest.md)
- [表側の会員登録・identity案（検討中）](docs/public-membership-proposal.md)

Go 1.24以降でcheckerを実行できます。

```bash
go run ./cmd/forma check examples/users.forma
go test ./...
```

`forma build`、`forma conformance`、`forma run`は今後実装します。
