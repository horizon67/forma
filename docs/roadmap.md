# Forma Roadmap

Status: living roadmap — non-normative

この文書はFormaの現在地と、おおまかな実装順序を共有するためのroadmapである。言語の規範的な
syntax、semantics、v0完了条件は[`v0-primitives.md`](v0-primitives.md)に定める。本roadmapと仕様が
矛盾する場合は規範仕様を優先する。

日付を先に固定せず、前段の不確実性を閉じてから次へ進む。特に、target固有実装を急いで作る前に、
Semantic IR、conformance、profile、artifactの境界を固定する。

## 現在地

| 領域 | 状態 | 現在の内容 |
| --- | --- | --- |
| 言語思想 | 形になっている | AI時代のsource of truth、target非依存、可読性の設計原則を文書化 |
| v0言語仕様 | design draft | 10 primitives、閉じたmodifier、EBNF、静的検査、runtime semanticsを定義 |
| reference front-end | 部分実装 | Lexer、Parser、syntax AST、主要Checker、diagnostic、`forma/v0.3` core Semantic IR |
| 完全例 | check可能 | `examples/users.forma`をparse・検査し、golden IRを生成可能 |
| conformance | 未実装 | contract schema、fixture、adapter、実行protocolが未決定 |
| target generation | 未実装 | profile manifest、artifact protocol、reference generatorが未決定 |
| architecture selection | exploratory | Architecture Manifest案を記録したが未決定 |
| end-to-end application | 未着手 | 生成、build、conformance、再生成を通したartifactはまだない |

## Milestone 0 — 言語設計の基準を固定する

目的は、新しいsyntaxを好みで追加せず、proposalを一貫した基準で判断できる状態にすること。

### 状態

ほぼ完了。以後も実例から問題が見つかれば更新する。

### 完了済み

- Formaを作る理由とAIとの役割分担を日英READMEへ記載
- 人間が保守するsourceをFormaだけに限定
- [Language Design Principles](language-design-principles.md)と可読性contractを作成
- `users.forma`を可読性、EBNF、参照解決の観点で監査
- 初期stateを`initial Value`で明示
- target codeを破棄・再生成可能なartifactと定義

### Exit criteria

- 新構文のproposalを可読性、semantic necessity、target neutralityで評価できる
- README、規範仕様、完全例が同じ中心仮説を説明している
- v0のscope外を生成コードへの手修正で補わない

## Milestone 1 — Language/front-end v0

目的は、Forma sourceからtarget-neutralな意味と検査contractを決定的に得ること。

### 実装する

- design draftとParser、AST、Checkerのsurface syntaxを一致させる
- 省略された`columns`、`detail fields`、form fieldsを決定的に展開する
- inherited constraintを合成し、defaultと`required readonly` producerを検査する
- formを`SubmitIntent`へ解決し、成功後navigationと認可を合成する
- string/regex escape setを仕様どおり検査する
- semantic nodeのstable identityとSource Mapを生成する
- Semantic IRからConformance Contractを決定的に生成する
- default、projection、action resolution、navigationを人間向けに展開して確認できるようにする

### Tooling候補

- `forma check`: syntaxとtarget-neutral semanticsの検査
- `forma explain`: 暗黙に展開された意味と参照解決の説明
- `forma fmt`: canonicalなsource layoutへの整形

command名と出力形式は未決定である。可読性のために必要なcapabilityを先に定め、CLI shapeは実装時に
固定する。

### Exit criteria

- 同じsourceとfront-end versionからbyte-identicalなSemantic IRとConformance Contractを得る
- 全semantic nodeをsourceへ追跡できる
- 完全例の暗黙semanticsをtarget generatorなしで説明できる
- front-endとtest oracleがAIまたはnetwork inferenceへ依存しない

## Milestone 2 — 生成境界contractを固定する

目的は、特定framework向けgeneratorを作る前に、targetを交換可能にするinterfaceを決めること。

### 固定するcontract

- Target Profile Manifest
  - profile ID/version
  - 対応IR version
  - 必須・提供capability
  - generatorとtoolchain設定
- Artifact Protocol
  - generatorが書ける範囲
  - file manifestとdependency lock
  - build entrypoint
  - diagnosticとfailure形式
- Conformance Schema
  - fixture、principal、operation、期待値、否定case、interaction state
- Profile Conformance Adapter
  - data reset、role注入、operation実行、target固有観測の共通interface
- Source Map Protocol
  - generator/build/runtime failureをForma declarationへ返す形式

### Architecture Manifestの検証

[Architecture Manifest Proposal](architecture-manifest.md)は、このMilestoneで実例により検証する。

1. Ruby/Rails + AWSを人間が固定するPinned構成を書く。
2. 異なるframework/provider、またはrequirementだけを指定するConstrained構成を書く。
3. Application ProfileとDeployment Profileを分ける必要があるか確認する。
4. direct libraryをcapabilityへ関連付けられるか確認する。

検証前にTOML schema、profile分割、Pinned/Constrained/Automaticを決定事項にはしない。

### Exit criteria

- profile非対応をgeneration前に検出できる
- generatorがAIを使用しても、入力、出力、合否判定の境界が機械可読である
- architectureとgenerator設定がbuild keyへ含まれる
- artifactを破棄して同じversioned inputsから再生成できる

## Milestone 3 — 一つ目のreference profile

目的は、完全例から実行可能なapplicationを一つ生成し、境界contractが現実に機能するか確認すること。

具体的なframework、provider、modelはMilestone 2の検証後に選ぶ。最初の実装を容易にするためだけに
Forma semanticsをそのstackへ寄せない。

### 生成するcapability

- Userのcreate、list、view、edit、delete
- search、filter、stable sort、pagination
- Team relationのlabel表示、filter、picker
- state transitionと不正遷移拒否
- page/action authorization
- create/edit/delete後の決定的navigation
- loading、empty、invalid、pending、failure

### Exit criteria

- generated applicationがbuildできる
- generated codeを手編集せず完全例を実行できる
- Conformance Contractの正常caseと否定caseをすべて通過する
- failureをForma sourceへ対応付けて報告できる
- source変更からartifactの破棄・再生成・再検証を繰り返せる

## Milestone 4 — End-to-end v0

目的は、prototypeの一度きりの成功ではなく、Forma projectとして運用可能な最小loopを完成させること。

```text
edit Forma source
  → check
  → build with profile
  → generate artifact
  → compile/package target
  → run conformance
  → accept or return source-addressed diagnostics
```

### Exit criteria

- clean environmentで同じpipelineを再実行できる
- architecture/profile/generatorのversionがlockされている
- application semanticsとarchitecture変更を区別してreviewできる
- stale artifactや手修正target codeをsource of truthにしない
- 規範仕様§14.3と§14.4の条件をすべて満たす

ここまでを最初の利用可能なv0 release候補とする。

## Milestone 5 — Formaの中心仮説を検証する

目的は、「一つのprofileに都合のよいDSL」ではなく、application intentを記述する言語になっているかを
検証すること。

### 実施する

- 同じSemantic IRを、実装モデルの異なる二つ目のprofileへ生成する
- たとえばSPAとserver-rendered applicationのように、表面的なcode structureが離れた組を選ぶ
- 同じtarget-neutral Conformance Contractを両artifactへ適用する
- generated codeの類似性ではなく、観測可能な意味の同等性を比較する

### Exit criteria

- `.forma`へtarget固有の条件分岐を追加せず、二profileで完全例を実装できる
- 両artifactが同じconformanceを通過する
- profile差によって意味が変わる箇所があれば、言語またはcontractの穴として説明できる

一つのreference profileでend-to-end v0を完成しても、この比較を通るまでは中心仮説を検証済みとは
みなさない。

## Milestone 6 — v1候補を実例から設計する

v1 syntaxを先に考えず、v0では表現できない現実的なapplicationから必要なsemantic axisを抽出する。

### 最初の追加例候補

注文、注文明細、在庫、価格を持つapplicationを記述し、次を確認する。

- derived value
- entity invariant
- state以外のaction precondition
- 複数entityをまたぐeffect
- transaction boundary
- runtime由来field

### その後に検討する領域

- expression layer、invariant、derived query
- domain actionのargumentとeffect model
- aggregate、join、inverse relation、cascade rule
- [表側の会員登録とidentity](public-membership-proposal.md)、identity provider、login/logout、ownership
- notification、background job、schedule、retry、file
- schema/data migration
- i18n copyとdesign intent
- safeなprofile extension

一つのexampleだけでgeneral syntaxを決めない。複数のdomainで同じsemantic needが現れ、既存primitiveの
modifierでは表現できない場合にだけ、新しいprimitiveまたはexpressionを検討する。

## Roadmap全体を通じた原則

- Forma sourceの可読性を、生成速度より優先する。
- 短さのためだけに暗黙semanticsを増やさない。
- application intent、architecture policy、resolved lock、generated artifactの責務を混ぜない。
- AIは翻訳、提案、artifact生成に使えるが、parse、意味、期待値、合否判定を所有しない。
- target codeへ手修正せず、足りない表現は言語またはprofileの問題として扱う。
- 未決定事項をgeneratorの推測で埋めない。

## 今は行わないこと

- boundary contractを決める前にprovider固有promptを規範interfaceとして固定する
- 一つ目のreference frameworkへForma semanticsを最適化する
- v0の完全例を通す前にv1の式やeffectを実装する
- generated target codeとForma sourceを共同編集する運用を導入する
- Architecture Manifest案を実例なしでcore grammarへ取り込む

この順序は、実装から得た知見に応じて更新してよい。ただしMilestoneを飛ばす場合は、後段で何を
推測または仮置きすることになるかを明示する。
