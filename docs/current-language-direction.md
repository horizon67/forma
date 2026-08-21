# Current Language Direction

Status: current design decision — language grammar remains experimental

この文書は、これまでのForma実験、会員登録flow probe、外部design research（DR）を踏まえ、
「何を検証したか」「何が決まったか」「次に何を言語へ入れるか」を一か所にまとめる。
個別syntaxの規範は[`v0-primitives.md`](v0-primitives.md)、各proposalの詳細はリンク先を正とする。

## 結論

現在のFormaを完成形として維持するのではない。

維持するのは、**Forma sourceを唯一の正本とし、共通semantic modelから目的別viewを生成し、AIが最終applicationを
repository-nativeに実装するarchitecture**である。現在のgrammarは、そのarchitecture上で最初の実applicationを測るための
subsetであり、表現不能になったapplication semanticsを順番に追加する。

DRで調査した構文例を一括して移植はしない。一方、DRが整理したstructure、state、rule、interaction、flow、event、effect、
constraintというsemantic facetはFormaの設計backlogとして扱う。sourceに追加するのは、既存facetやprojectionから導出できず、
実applicationで必要性とownershipが確認できた意味だけである。

```text
Forma source（唯一の正本、複数の小さなsemantic facet）
  ↓ parse / resolve / type check / semantic check
Resolved Intent + Source Map + Acceptance Facts
  ↓
navigation / outcomes / states / visual flow等のread-only projection
  ↓
Generation Request + target repository + Implementation Policy
  ↓
AI coding agent
  ↓
通常のapplication code + test/build feedback
```

Forma coreが会員登録runtime、framework別generator、画面flow engineを提供するarchitectureにはしない。

## これまで検証したこと

| Probe | 検証した問い | 得られた結果 | 検証していないこと |
| --- | --- | --- | --- |
| Admin agent E2E | Formaの意味をAIへ渡して通常のapplicationを実装できるか | 43 Acceptance Factsを満たすGo applicationを生成・検証できた | あらゆるframeworkでの再現性 |
| Incremental update | 既存repositoryを作り直さず変更できるか | 既存testを保ち、field/page-size変更を適用できた | rename、削除、migration全般 |
| Identity / membership | CRUDを越えるsignup、verification、signin、ownershipをtarget-neutralに表せるか | current 85 Facts、3 policies、3 human review requirementsまで検証できた | 任意のUX flow、任意の外部effect |
| Automated repair | AIの失敗を意味やtestを弱めず修復できるか | test/build failureのrepairとintent-gap handoffを実測した | 一般的なfailure分類の完全性 |
| Navigation/outcome/state projection | 分散した意味を正本を増やさずreviewできるか | stable IDとSource Mapを保つ決定的viewを生成できた | 人間の読解性能 |
| Visual flow projection | 3 viewを一つのoverviewで関連付けられるか | navigationを骨格にOutcome/Stateを型付きで結べた | この表示がsource-onlyより読みやすいという実測 |
| Human evaluation | 人間がsource-onlyとsource+projectionのどちらを速く正確に読めるか | protocolと事前採点基準まで準備した | participant resultはまだ0件 |

直近のprojection実験は「現在のgrammarが完成しているか」を検証したものではない。既にsourceが持つ意味を、第二の正本を
作らずglobal/localに読み分けられるかを検証した。その過程で、projectionでは解決できない言語上の不足も特定した。

## 確認済みの言語gap

### 1. Application default entry（対応済み）

bounded probe前のsourceはpageを宣言できたが、application起動時のentryを宣言できなかった。最初のpage、名前が`Home`のpage、
registration interactionを持つpage等から推測してはならないため、projectionは`unspecified`と表示している。

これは表示上の問題ではなく、正本に意味が存在しない問題だった。bounded probeでtop-levelの`entry Page`を追加し、
`application/entry`としてResolved Intent、Source Map、Acceptance Factsへ解決した。未宣言sourceは引き続き
`unspecified`であり、page順や名前から推測しない。

### 2. Surface-only transition（対応済み）

bounded probe前のIdentity interactionは、operation成功先と、その成功先からの1回の`continue`を持てた。しかし次のような、
domain operationを伴わない任意のsurface chainは表現できない。

```text
RegistrationComplete -> OnboardingGuide -> SignIn
```

pageを追加するだけ、または生成diagramへedgeを足すだけでは正本の意味にならない。bounded probeではpage-localな
`continue Page`を、trigger/capability、source surface、destinationを持つsurface transition semanticへ解決した。
最初のsliceは固定pageへの`continue`だけを受理し、parameterized pageへのbindingなし遷移と二重ownershipを拒否する。

### 3. CRUD/state transitionを越えるdomain behavior

注文、在庫、承認、通知等には、値を読むExpression、atomicなChanges、発生した事実を表すOccurrence、外部作用を表すEffectが
必要である。self-only Invariantのfield参照と`<=`、成立・違反Acceptance Facts、Generation Request差分を
実装し、続いてChangesをaction-attachedな同時代入とatomic post-stateとして実装した。通常のGo applicationを使う
repository E2Eで280/280 Factsを実測し、Invariant concurrency、Changes atomicity、cross-entity write/value-read authorization、
exact numeric enforcement、concurrent Action Precondition enforcementの6 Review Requirementsは人間確認待ちである。Changes右辺の
required relation value 1 hop、field reference 2個のexact binary `+`、named Action Preconditionを各proposalどおり実装した。
Preconditionはsource state不一致、exactなpre-state predicate、post-state Invariant違反を別outcomeとして保つ。

## DRのsemantic facetをどう扱うか

| DRで整理されたfacet | Formaの現在地 | 方針 |
| --- | --- | --- |
| Structure / relation | `type`、`entity`、field、entity reference | 現行を維持し、必要なrelation semanticsだけ追加する |
| State / transition | entity `state`、`action A -> B` | 現行を維持し、Changesとpreconditionへ接続する |
| Interaction | `page`、`list`、`detail`、`form`、`interact` | 現行のpage-local ownershipを基本にする |
| Navigation / task flow | action/submit/interaction destinationのみ | `entry`とsurface transitionを直近で追加検討する |
| Policy / authorization | `allow`、`require authenticated/owner`、Implementation Policy | application ruleとimplementation policyを混同せず、汎用ruleはExpression利用者として拡張する |
| Invariant / declarative constraint | self-only Invariantの最小Expression slice | P3で型付きExpressionと利用contextを段階的に拡張する |
| Mandatory / possible / forbidden | 一部をcompiler invariant、Acceptance Fact、`must not`として導出 | 汎用modifierを先に入れず、導出不能なsafety/recovery要件が現れた時点でsource syntaxを設計する |
| Scenario / example | Acceptance Factsとtest scenarioを原則生成 | 重複するscenario正本は作らない。導出不能な補助exampleだけを将来候補にする |
| Event / occurrence | Identity operationとnoticeに専用semanticがある | P3でdomain-neutralなOccurrenceへ一般化する |
| Effect / recovery | Identity notice emission/delivery failureに専用semanticがある | P3でEffect bindingとdelivery contractへ一般化する |
| State table / task tree / diagram | states、outcomes、flow projection | 原則viewとして生成する。layoutをsourceへ入れない |
| Nested / parallel / interruptible flow | 未対応 | 具体的applicationで必要になるまでgrammarへ入れない |

DRの「複数の小さな記法を共通semantic modelへ接続する」という結論は採用する。ただし、これは`.forma`内に無関係な言語を
何個も埋め込むという意味ではない。各facetが独立したidentity、ownership、型、failure semanticsを持ち、compiler内で同じ
semantic graphへ解決されることを意味する。

## 直近の実施順序

### Track A — Navigation semantics follow-up（完了）

`flow`所有とpage-local所有を同じmembership変更で比較し、page-localを採用した。

- `entry SignUp`はapplication-levelに一度だけ宣言する。
- operationを伴わないedgeはsource pageが`continue Destination`として所有する。
- operation成功先は従来どおり、そのoperationを提示するsurfaceが所有する。
- pageとinteractionが同じcontinuationを二重宣言するsourceはcompile errorにする。
- global overviewは編集可能な`flow`正本ではなく、navigation/flow projectionから得る。

[`../examples/email-verified-membership.forma`](../examples/email-verified-membership.forma)で
`RegistrationComplete -> OnboardingGuide -> SignIn`を実際にcompileし、current Resolved Intent `v0.12`、Source Map `v0.6`、
Acceptance Facts `v0alpha10`、navigation/flow projection、incremental semantic diffまで通した。destinationだけを変える
mutationは、owner pageとtransition node、および対応Factだけを変更する。既存admin CRUD sourceには新しい記述を要求しない。

`flow` blockは、同じdestinationをpageと二重管理するか、既存のpage-owned action/submit navigationを全移動する必要があり、
このsliceでは局所理解とCRUDの簡潔さを悪化させた。nested、parallel、interruptible flowの実例が現れた場合は再評価する。

### Track B — Human readability evaluation（Track Aをblockしない）

source-onlyとsource+projectionの比較を実施する。これはprojectionのprogressive disclosure、label、trace情報を改善するための
評価である。既に確認されたentry/surface transitionの表現力不足を、participant resultが出るまで保留する試験ではない。

Candidate Cの完全な`flow` DSLを採用する判断には人間評価を使うが、最小surface transition semanticの必要性は既に確認済みである。

### Track C — P3 Domain behavior（次の本線）

Navigationのbounded probeが完了したため、[`expression-proposal.md`](expression-proposal.md)とroadmapに従い、次の順序で進める。

1. Expression coreとInvariant（最初のslice完了）
2. Changesとatomic post-state（最初のslice完了）
3. Changes RHSのrequired relation value（最初のslice完了）
4. numeric `+`（最初のslice完了）
5. Action Precondition（最小slice完了）、multiple assignment、collection binding、record creationをfull Order approvalまで段階化
6. Derived Valueを独立したExpression consumerとして検証
7. Occurrence
8. Effect binding / delivery contract

これは一般的な機能階層ではなく、実例を分離して検証する実装順序である。bounded Changesは既存field valueだけで
atomicityを検査できるため、Derived Valueや算術を先に一般化しない。

Expressionの最初のvertical sliceでは、`StockItem.stockAvailable`の`reserved <= onHand`からentity単位の
正常系・否定系2 Factsに加え、参照fieldを編集するform submitのauthoritativeな拒否Factを導出した。
post-stateでの全commit／無commitをGeneration Requestへ運び、concurrent operationの保証は独立Review Requirementとして
人間へ表示する。[`order-invariant-agent-e2e`](../experiments/order-invariant-agent-e2e/)では、このrequestを
Formaに依存しない通常のGo applicationのauthoritative mutation境界へ落とした。続く
[`changes-proposal.md`](changes-proposal.md)では、最終対象を`Order.approve`の在庫引当に保ちつつ、最初のsliceを
requiredなto-one relationへの1 assignmentとimplicit state transitionに分離し、bounded syntax、Resolved Intent、
atomic Facts、Review Requirementsをreference compilerへ実装した。v0が要求しながら現compilerに無かったdomain actionの
source precondition、遷移後state Fact、明示`confirm` actionと標準`delete`のconfirmation Fact、action拒否時のsurface
feedbackも同じsliceで追加した。cross-entity writeの認可はactionが所有し、target entityの別surface accessは継承しない。
通常applicationでは52 mapped testsから280/280 Factsを測定し、その差とatomic boundaryは人間Review Requirementへ提示する。

続く[`relation-value-expression-proposal.md`](relation-value-expression-proposal.md)では、relation traversalの最初のconsumerを
Derived ValueではなくChanges右辺に置いた。Changesには評価時点、pre-state、atomic boundary、failure outcomeが既にあり、
Derived Valueの保存／再計算／dependency semanticsを同時に発明せずrelation readだけを検査できるためである。最初は
selfまたはrequired relation 1 hop先のrequired scalar field referenceだけを受理し、targetとは別のvalue relationが
runtimeで解決不能な場合を`value-unavailable`として全変更拒否へ閉じる。actionのallowが実行認可を所有する一方、
cross-entity valueの利用とdownstream disclosureは`cross-entity-value-read-authorization` Review Requirementとして
人間へ提示する。共有`IRExpression`の拡張時にはself-only Invariant validatorも同時更新し、relation-reading Invariantを
tampered Resolved Intentから持ち込めないようにした。repository E2Eではdistinct target/value relation、relation先とselfの
異なる値、`value-unavailable`、HTTP feedback、無部分commitを既存`StockReservation.commit`へ統合した。

次の[`numeric-addition-expression-proposal.md`](numeric-addition-expression-proposal.md)では、同じactionを
`stock.reserved = stock.reserved + plan.approvedReserved`へ進める。`+`はfield reference 2個の間の1回だけに限定し、
同一nominal numeric type、consistent pre-state、target/value relation identity共有を維持する。複数operandを単一
`valueSubject`へ潰さず、Expression treeと全leafのruntime subject bindingをAcceptance Factsへ運ぶ。Int／Decimalの
wrap・rounding防止はtarget固有representationを伴うため、machine Factを捏造せず独立Review Requirementでも確認する。
current compilerはchained named typeのinherited constraintをまだ合成しないため、最初のsliceはbuiltinを直接baseにする
numeric typeへ限定した。immediate declared baseとclosure判定に使うeffective boundsをIRへ残し、validatorが同じboundsを
再計算する。Expression treeと全leaf bindingをFactsへ運び、repository E2Eでは2+6=8、同一snapshot、overflow failure、
無部分commitを実測した。[`action-precondition-proposal.md`](action-precondition-proposal.md)の実装では、named predicateを
source stateとpost-state Invariantから分離し、required relationを共有するexact pre-state評価、
`precondition-unsatisfied`／`invalid`、relation unavailableとの優先順位、concurrent enforcementを最小sliceとして実装・実測した。
compiler実装はこのproposalのdesign review後に行う。

Effectから先に設計しない。recipient、発生条件、payload bindingにはExpressionが必要であり、Effectを発生させる事実には
ChangesとOccurrenceの境界が必要だからである。

## Navigation syntaxを決める判定基準

`flow` blockかpage-local syntaxかは、見た目の好みではなく次で決める。

- semantic factの正本が一か所である。
- 同じinteraction/actionのdestinationが競合しない。
- normal pathとexternal entryを区別できる。
- operationを伴わないtransitionを表せる。
- source diffから変更したedgeを一意に特定できる。
- page単体の局所理解とapplication全体のoverviewを往復できる。
- 単純なCRUDへ新しいboilerplateを強制しない。
- 将来のback/cancel/interrupt/parallelを、modifier追加だけで破綻させない。
- Resolved Intentと全projectionがdeclaration順・source path・layoutから独立して決定的である。

## 現時点で採らないもの

- DRに登場したSCR、LSC、Declare、CTTのsyntaxをそのまま全部実装すること。
- Mermaid、diagram layout、生成tableを編集可能な正本にすること。
- membership専用flow runtimeや専用generatorをForma coreへ追加すること。
- scenarioを全operationについて手書きし、Acceptance Factsと二重管理すること。
- `hot`、`cold`、`forbidden`、parallel operator等を利用例なしで予約・実装すること。
- Formaを通常のprogramming languageへ近づけるstatement、loop、汎用I/Oを追加すること。

## 次の更新条件

Track Aのsyntax probe、人間評価、P3の各vertical sliceが完了するたび、この文書の「現在地」「実施順序」「採らないもの」を更新する。
実験結果と設計判断を分け、未実測の期待を「検証済み」と書かない。
