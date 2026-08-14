# Forma

[English](README.md) | **日本語**

**アプリケーションを構築するための、より高水準なプログラミング言語。**

Formaは、framework固有の実装方法ではなく、entity、state、relation、action、page、list、form
といったアプリケーション概念によってソフトウェアを記述する、実験的なプログラミング言語です。

> Formaのsourceは、アプリケーションが「何であるか」を表現します。
> 「どう実装するか」はcompilerが決定します。

```forma
type Email = String matches /.+@.+/

entity User {
    name  String required
    email Email required unique

    state status Pending | Confirmed | Active | Suspended
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

Formaは直接的なsource-to-source変換ではなく、target-neutralなSemantic IRを中心に設計します。

```text
自然言語（任意）
      │
      ▼
 Forma source
      │
      ▼
Parser / Typed AST
      │
      ▼
Semantic Model / Application IR
  ├─ Web target ──── React
  ├─ Server target ─ Ruby / Go
  └─ Native target ─ Binary
```

compilerは、type、relation、state transition、permission、action、navigationを解決してから、
target profileにcomponent、transport、persistence、runtime behaviorの選択を委ねます。

コンパイルは決定論的であることを目指します。

```text
Forma source + target profile + compiler version -> generated application
```

AIは、この規範的なcompile pathには含まれません。

## FormaとAI

AI-assisted developmentによって、developerは既存のプログラミング言語より高い抽象度で
アプリケーション変更を説明できることが明らかになりました。

> ユーザー一覧に、名前とメールアドレスの検索とページングを追加して。

現在はcoding agentが、この指示を多数のframework固有コードへ展開します。Formaは、同じ
高水準の指示を、永続的でreview可能かつ決定論的なsource codeにできるかを探究します。

```forma
page Users {
    list User {
        search name, email
        paginate 20
    }
}
```

AIは自然言語からFormaへの翻訳を支援できます。一方、Forma自体は決定論的なsyntax、semantics、
validation、compilationを持つことを目指します。

## 目標

- アプリケーションの意思決定を、高い意味密度で表現する
- domain ruleと利用者に見えるcapabilityを、明示的かつ検査可能にする
- frontend、backend、persistence、testを通じて一貫した振る舞いを生成する
- Forma sourceをtarget frameworkから独立させる
- LLMなしで生成結果を再現可能にする

## 目標にしないこと

Formaは、次のものを目指しません。

- あらゆる低水準言語やsystems programming languageを置き換える
- すべてのtarget frameworkの全機能を公開する
- 自然言語を実行可能なsyntaxとして使う
- 決定論的なcompilationをLLMに依存させる
- framework固有のshortcut集になる

## 現在の状況

Formaは初期設計段階であり、compilerはまだreleaseされていません。

- [Forma v0仕様](docs/v0-primitives.md)
- [ユーザー管理の完全例](examples/users.forma)

初期compiler interfaceは次の形を予定しています。

```text
forma check
forma build
forma run
```
