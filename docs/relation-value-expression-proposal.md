# Relation Value Expression in Changes Proposal

Status: first experimental vertical slice implemented and repository-E2E measured; not part of normative v0

このproposalは、`changes`の右辺でaction entityからrequiredなto-one relationを1回辿り、参照先の
required scalar fieldをpre-state valueとして読めるようにする最小sliceを定める。

```forma
changes {
    observedOnHand = stock.onHand
}
```

最終対象は`Order.approve`の在庫引当である。

```forma
// 後続slice。今回はまだ受理しない。
changes {
    line.product.reserved = line.product.reserved + line.quantity
}
```

ただしrelation traversal、numeric `+`、collection binding、multiple assignmentを一度に導入すると、
「どのruntime subjectを読んだか」「いつ読んだか」「何をatomicにcommitしたか」の失敗を分離できない。
今回は**relation valueを1つ読むことだけ**を追加する。

## 決定の要約

- source syntaxは既存のfield pathを使い、新しいkeywordを追加しない。
- Changes右辺は、既存のself required scalarに加え、action entityからrequiredなto-one relationを1回だけ
  辿った先のrequired scalarを受理する。
- relation identityとscalar valueは、mutation targetおよび他の右辺と同じconsistent pre-state snapshotから読む。
- mutation targetを解決できなければ既存の`target-unavailable`、targetは解決できるが追加のvalue relationを
  解決できなければ`value-unavailable`としてaction全体を拒否する。
- target pathとvalue pathが同じrelation bindingを使う場合は、同じruntime identityを再利用する。そこから独立した
  `value-unavailable` Factは作らない。
- 実行認可はChangesを所有するdomain actionが所有し、参照先entityのpage accessを追加条件として継承しない。
  一方、そのroleがcross-entity valueを利用・再公開してよいかは新しいReview Requirementとして人間へ提示する。
- numeric literal、operator、optional traversal、multi-hop、to-many、Derived Value、Action Preconditionは含めない。
- 最初のChanges sliceと同じくassignmentは1個のままにする。

## なぜDerived ValueよりChangesを先にするか

[`expression-proposal.md`](expression-proposal.md)の初期案は、relation traversalの最初のconsumerをDerived Valueに
置いていた。しかし現在はChanges sliceが実装され、次の意味が既に存在する。

- action invocationという評価時点
- consistent pre-stateというread model
- implicit transitionと明示assignmentのatomic boundary
- `accepted`、`invariant-violated`、`target-unavailable`というoutcome
- page actionの`invalid`／`failure` feedback
- repository固有のtransaction／lockを確認するReview Requirement

一方、Derived Valueから始めると、relation traversalとは別に、宣言owner、保存値か計算値か、再計算時点、依存先変更時の
invalidation、formやprojectionでの提示を同時に決める必要がある。relation traversalそのものを検証するにはChanges右辺の方が
未知が少ない。

したがって実装順序を更新する。

```text
Changes RHSのrequired relation value
  -> numeric +
  -> Action Precondition
  -> multiple assignments / runtime alias
  -> collection binding / fan-out
  -> record creation
  -> full Order.approve
  -> Derived Valueを独立consumerとして設計
  -> Occurrence
  -> Effect
```

Derived Valueを捨てる判断ではない。同じResolved Expressionを再利用するが、最初のconsumerにはしない。

## 比較例

### 1. Inventory snapshot

最初のcompiler／repository fixtureは、書き込み先と読み取り元を分け、self fieldへ誤ってloweringしても観測できる形にする。

```forma
role admin
role staff

type Quantity = Int min 0

entity StockItem {
    onHand Quantity required
}

entity InventorySnapshot {
    stock          StockItem required
    observedOnHand Quantity required default 0
    state status Pending | Captured initial Pending
}

action InventorySnapshot.capture: Pending -> Captured confirm allow staff {
    changes {
        observedOnHand = stock.onHand
    }
}

page StockItems {
    allow admin
    list StockItem {
        columns onHand
    }
}

page Snapshots {
    allow staff
    list InventorySnapshot {
        columns stock, observedOnHand, status
        actions capture
    }
}
```

`observedOnHand`の初期値と`stock.onHand`を異なる値にしたtestで、accepted後にrelation先のpre-state valueが保存されたことを
観測する。targetは`subject/action`、value sourceは`subject/value`であり、同じrecordの別fieldを読んだと偽装できない。
同時に、admin向けStockItemの値がstaff向けSnapshotへcopyされることをsourceだけから自動的に安全とは断定しない。後述の
Review Requirementがこのdata disclosureを人間へ提示する。

### 2. Identity / notification binding

会員登録にも同じaxisがある。

```forma
changes {
    recipient = account.email
}
```

これはEffectそのものではなく、required relationからtarget-neutralなscalar valueを得る部分だけの比較例である。
Inventory固有の構文にしない根拠になる。credentialやverification evidenceのようなsecret semanticはordinary field pathとして
読める対象にはしない。

## Surface syntaxと静的semantics

parserは既にChanges右辺をfield pathとして読めるため、grammarは追加しない。checkerが受理するpath形状を広げる。

```text
self value
  reservedAfter

required relation value
  stock.onHand
```

後者は次をすべて満たさなければならない。

1. rootはaction entityが宣言するfieldである。
2. rootは`required`なto-one entity relationである。
3. pathはrelation 1 hopとterminal fieldの2 segmentで終わる。
4. terminalは`required`なscalar fieldである。
5. terminalはcollection、relation、stateではない。
6. terminalのnominal typeはassignment targetのnominal typeと同じである。

`readonly`はmutation禁止であってread禁止ではないため、terminal value fieldでは許可する。repository固有値、Identityのcredentialや
verification evidenceは普通のentity scalarではなく、引き続きこのpathで参照できない。

次はcompile errorである。

```forma
optionalStock.onHand       // optional relation
warehouse.stock.onHand     // 2 hops
stocks.onHand              // to-many relation
stock                      // relation valueそのもの
stock.tags                 // collection result
stock.status               // state
```

surface syntaxへ`self.stock.onHand`という別表記は追加しない。最初のsliceにはroot bindingがselfしかなく、canonical formを
2つ作る理由がない。

## Pre-state評価とruntime binding

actionの意味順序を次へ拡張する。

1. principal、confirmation、source stateをauthoritative boundaryで検査する。
2. 全mutation targetのruntime identityをconsistent pre-state snapshotから解決する。
3. targetを解決できなければ`target-unavailable`として拒否する。
4. target解決でまだ得ていないvalue relationのruntime identityをcanonical relation path順に解決する。
5. value relationを解決できなければ`value-unavailable`として拒否する。
6. relation先のscalar valueを同じpre-state snapshotから読み、右辺を評価する。
7. implicit transitionを含むcandidate post-stateを作り、field constraintとInvariantを検査する。
8. 全変更を1回でcommitする。2から7のどこかで失敗すれば何もcommitしない。

同じrelation pathがtargetとvalueの両方に現れる場合、2で得たruntime identityを6で再利用する。

```forma
changes {
    stock.reserved = stock.limit
}
```

同じ`stock`を2回repositoryから独立にlookupし、途中のrelation変更によって異なるStockItemを読んで書く実装は不適合である。
relation pathはsource上の文字列ではなく、解決済みfield identityで同一性を判定する。

### Concurrent update

pre-stateで読んだscalarがcommit前に別operationで変更された場合も、宣言順や後読みで結果を変えない。target repositoryは
transaction、lock、version conflictとretry等から、action全体が1つのconsistent snapshotを使う仕組みを選ぶ。

この保証は既存の`atomic-changes-enforcement` Review Requirementが既に「target identity resolution、pre-state reads、
invariant checks、commitを同じ境界に置く」と要求している。relation value fieldとrelation pathをそのrequirementの
`sourceNodes`へ加える。これはatomicityのreviewであり、cross-entity valueをそのroleが利用してよいかというreviewとは
分離する。

## unavailable outcomeの分類

`required`はsource schema上の必須性であり、runtimeで参照先がliveであることまでは保証しない。dangling relationや
concurrent deleteをpanic、zero value、別recordへの暗黙fallbackにしてはならない。

| 状況 | reason | feedback | commit |
| --- | --- | --- | --- |
| mutation target relationが解決不能 | `target-unavailable` | `failure` | 0 |
| targetは解決済み、追加value relationが解決不能 | `value-unavailable` | `failure` | 0 |
| targetとvalueが同じrelationで、そのbindingが解決不能 | `target-unavailable` | `failure` | 0 |

この優先順位により、同じruntime setupへ矛盾する2つのFactを作らない。value relation pathがtarget relation pathと同一なら、
compilerは独立した`value-unavailable` Factを導出しない。異なる場合のvalue-unavailable Factは、target relationが
`resolved`でvalue relationだけが`unavailable`のsetupを持つ。

domain actionでは利用者向けfeedbackを持たず、page action referenceをsubjectにするsurface Factだけが`failure`を要求する。
view自身の`observable-feedback` Factへaction failureを混ぜない既存の所有分離を維持する。

## Authorizationとdata disclosure

relation valueを読むoperationの実行権限はChangesを所有するdomain actionの`allow`が所有する。参照先entityを表示・編集する
pageのallowを追加条件として推測しない。

```forma
page StockItems { allow admin ... }
action InventorySnapshot.capture ... allow staff {
    changes { observedOnHand = stock.onHand }
}
```

この場合staffはcaptureを実行できる。`StockItems` pageへ入れるとは限らず、capture時にadmin roleを追加要求してもならない。
しかし既存のaction access Factsが検査するのはallow／denyどおりに**actionを呼べるか**だけであり、そのroleが参照先fieldの
値を利用し、変更先fieldや後続surfaceから再公開してよいかまでは証明しない。source authorがfield pathを書いた事実だけで
confidentiality判断を代替しない。

そこでrelation value pathを1つ以上持つactionごとに、次を導出する。

```text
id:      review/action/InventorySnapshot/capture/cross-entity-value-read-authorization
kind:    cross-entity-value-read-authorization
subject: action/InventorySnapshot/capture
```

instructionは次の意味を固定する。

> action access、各presenting surfaceのeffective access、relation valueのsource entity／fieldとそれを提示する既存surface、
> 保存先fieldとそれを提示する既存surfaceをreviewし、actionを許可されたroleがsource valueを利用できることと、その値の
> downstream disclosureがintentionalであることを確認する。source entityのsurface roleを追加のaction authorizationとして
> 推測してはならない。

`sourceNodes`は最低でもaction、value expression、relation path、source entity／terminal field、change target field、全action
referenceとそのaccess source、source fieldまたはtarget fieldを提示する全view／pageとそのaccess sourceを含む。同じentityが
cross-entity write targetでもある場合、既存`cross-entity-write-authorization`とこのrequirementの両方を出す。write pathの
許可とread／disclosureの許可は同じ問いではないため統合しない。

Formaにfield confidentialityの宣言がまだ無いため、この判定をAcceptance Factのpassへ偽装しない。将来field-level policyを
導入した場合は、機械判定できる部分だけをReview Requirementから移す。

## Resolved Intent

`IRExpression`のfield referenceへtargetと同形の`relationPath`を追加する。

```text
IRAction
  id: action/InventorySnapshot/capture
  atomicity: all-or-nothing
  changes:
    - id: action/InventorySnapshot/capture/change/observedOnHand
      target:
        binding: self
        field: entity/InventorySnapshot/field/observedOnHand
      value:
        id: action/InventorySnapshot/capture/change/observedOnHand/value
        kind: field-reference
        binding: self
        relationPath:
          - entity/InventorySnapshot/field/stock
        field: entity/StockItem/field/onHand
        resultType: Quantity
      evaluation: pre-state
```

`binding: self`はpath rootがaction entityであることを表す。terminal fieldのownerをbindingと呼ばない。

Change IDは引き続きtarget pathから導出する。valueを`reservedAfter`から`stock.onHand`へ変えても同じChange IDの
semantic changeであり、remove/addへしない。value expression IDもChange IDの`/value`を維持する。

Source Mapは既存の`field-reference` entryをpath全体のspanへ向ける。relation segmentは既存field declaration identityを
参照するだけで、独立したexpression node IDを発明しない。Source Map schemaは変えず、Resolved Intentのschema versionを
更新する。

## Acceptance Facts

accepted Factは、保存値のsource subjectをpathに応じて変える。

| value形状 | `valueSubject` | `valueField` |
| --- | --- | --- |
| self field | `subject/action` | terminal field ID |
| targetと同じrelation path | `subject/target` | terminal field ID |
| targetと異なるrelation path | `subject/value` | terminal field ID |

`subject/value`を使うFact setupは、action subjectからvalue subjectへのresolved relationを持つ。target relationと同一なら
`subject/target`を再利用し、同じruntime identityであることをFact内でも失わない。

最初のsliceで追加するFact familyは次だけである。

```text
changes-value-unavailable
action-changes-value-unavailable
```

value relationがtarget relationと異なるactionについて、source stateごとにdomain Factと各page action surface Factを導出する。
assignmentが1個のsliceなので、Fact IDは既存target-unavailableと対になる次の形で一意になる。

```text
<subject>/changes/value-unavailable/from/<source-state>
```

multiple assignmentへ進み、1 actionが複数のdistinct value relationを持てるようにする際は、relation pathをFact IDへ
含めるschema revisionを先に行う。現在のIDを暗黙に多重化しない。

```text
setup:
  subject/action state = Pending
  target relation = resolved       # relation targetを持つ場合
  value relation condition = value-unavailable
input:
  action dispatches = 1
expected:
  outcome = rejected
  reason = value-unavailable
  atomicity = no-changes-committed
  appliedMutations = 0
  action and resolved target = unchanged
  surface feedback = [failure]
```

accepted、Invariant rejected、target unavailableの既存Factsにもvalue relation、terminal value field、value entityを
`sourceNodes`として追加する。accepted Factは`valueSubject`を通じ、action entityの同名fieldを読んだ実装を許さない。

Acceptance Factsは具体的なscalar fixture valueを規定しない。repository testはtargetとsourceへ異なる値を置き、relation先の
値を観測していることを示す。compilerが任意のnominal scalar値を合成するruntime evaluatorにはならない。

## IR validationとdiagnostic

`IRExpression`はInvariant predicateとChanges valueが共有する型である。したがって`relationPath`追加はChanges validatorだけの
変更ではない。同じcompiler sliceで、**新しいfieldを許さない既存consumerも含めて全validatorを更新する**。

closed expression sliceのvalidatorは、個別fieldを部分的に検査して未知のfieldを見落とす形にしない。各nodeについて、その
consumerが許すfieldだけを埋めたcanonical `IRExpression`を組み立て、元nodeとの全field比較でshapeを検査する。

- Changes field referenceは`relationPath`が0または1件のcanonical shapeを許す。
- self-only Invariantのbinary rootと左右field referenceは`relationPath == nil`だけをcanonicalとする。
- 将来`IRExpression`へfieldを追加した際、既存consumerのcanonical valueはそのfieldがzeroのままなので、non-zeroなtampered IRを
  defaultで拒否する。新しいconsumerが明示的に許可するまで素通りさせない。

特に`validateInvariantSemantics`と`validateInvariantFieldReference`を同じrunで更新し、predicate rootまたはoperandに
`relationPath`を持つInvariantを拒否する。`invariantFieldReferences`はself terminal fieldだけを歩く現状を維持できるが、
Facts導出前の`ValidateResolvedIntent`がrelation-reading Invariantを必ず止めることをnegative testで固定する。

validator以外の共有helperも同じrunで棚卸しする。`cloneIRExpression`は`relationPath` sliceをdeep copyし、Changesの
Source Map／Fact／Review Requirement walkerはpath field IDsをprovenanceへ含める。`appendExpressionSemanticIDs`はexpression
node identityだけを集める責務を維持し、relation field declarationを新しいexpression nodeとして数えない。

process boundaryを越えたChanges validatorは、parserを通らないtampered IRについて次を拒否する。

- `relationPath`が2件以上。
- relation path fieldがaction entityのrequired to-one relationでない。
- terminal fieldがrelation target entityに属さない。
- terminal fieldがrequired scalarでない。
- `binding`、expression identity、result typeがcanonicalでない。
- target fieldとvalue fieldのnominal typeが異なる。

source checkerは既存`F2805`をChanges value pathのshape／requiredness／scalar性に使い、`F2806`をnominal type mismatchに
維持する。別のfailure classを表さない限りdiagnostic codeを増やさない。helpはself fieldだけを案内せず、
「required self scalarまたはrequired to-one relation 1 hop先のrequired scalar」を示す。

negative testは少なくともself、valid relation、optional relation、to-many、multi-hop、state terminal、relation terminal、
foreign terminal owner、tampered binding、tampered result typeを固定する。加えて、実在するrequired relationとterminal fieldを
Invariant operandの`relationPath`／`field`へ注入したtampered Resolved Intentがvalidatorで落ちるcaseを
`TestResolvedInvariantValidationRejectsUnsupportedOrTamperedIR`へ追加する。Invariant validatorからrelationPathのzero判定を
外すmutationと、Changes validatorを無効化するmutationの両方で、それぞれ対応testが落ちなければならない。

## Schema versioning

実装したschema versionは次である。

- Resolved Intent: `v0.9` → `v0.10`（`IRExpression.relationPath`）
- Acceptance Facts: `v0alpha7` → `v0alpha8`（value subjectとvalue-unavailable family）
- Source Map: schema形状は変えず、`intentVersion`だけ`v0.10`へ更新
- Review Requirements: `v0alpha3` → `v0alpha4`（`cross-entity-value-read-authorization` kind）
- Generation Request: envelope schemaは維持し、canonical componentとrequest digestを更新

これらはcurrent compiler constant、golden artifact、Generation Requestで固定する。

## 最初のcompiler slice

設計review後の実装範囲を次に固定する。

1. Changes checkerが1 hopのrequired relation valueを受理する。
2. `IRExpression.relationPath`、Source Map、consumerごとの全field canonical validatorを実装する。Changesとself-only Invariantの
   validatorを同時に更新する。
3. accepted Factの`valueSubject`をself／shared target／distinct valueで正しく分ける。
4. distinct value relationについてdomain／surfaceのvalue-unavailable Factsを導出する。
5. targetとvalueが同じrelation pathならruntime subjectを共有し、到達不能なvalue-unavailable Factを作らない。
6. action authorization ownershipとpage subjectのHTTP coverage規則を維持する。
7. atomic Changes Review Requirementへrelation value source nodesを加え、別に
   `cross-entity-value-read-authorization` Review Requirementを導出する。
8. Generation Request、projection、既存membership／repair artifactのversion非回帰を更新する。
9. repository E2EへInventory snapshot相当のactionを追加し、relation先の値、value-unavailable、confirmation、access、
   no-partial-commitをsurface経由で観測する。
10. self valueへ誤配線、optional fallback、post-boundary再読込、surface feedback省略、参照先page accessの誤継承を
    mutationで検出する。

## 実装結果

reference compilerはResolved Intent `v0.10`、Acceptance Facts `v0alpha8`、Review Requirements `v0alpha4`として
このsliceを実装した。Changes RHSはself fieldとrequired to-one relation 1 hopを受理し、self／shared target／distinct valueの
subject binding、distinct relationの`value-unavailable` domain/surface Facts、value-read authorization reviewを導出する。
self-only Invariant validatorは同じ共有`IRExpression`の`relationPath`をdefaultで拒否する。

repository E2Eでは既存`StockReservation.commit`のtargetを`stock`、value sourceをdistinctな`plan` relationとし、
`stock.reserved = plan.approvedReserved`を実装した。self上の`requestedReserved`は異なる値にして誤配線を検出する。
target欠落、value欠落、Invariant違反、source state不一致、confirmation decline、access denyはいずれもHTTP surfaceと
authoritative store boundaryで無部分commitを観測する。current artifactは52 mapped tests、278/278 Acceptance Facts、
4 human Review Requirementsである。

## このsliceへ入れないもの

- numeric literal、`+`、`-`、`*`
- 2 hop以上のrelation traversal
- optional traversal、absence test、fallback
- to-many relation、collection element binding、aggregate
- assignment 2件以上、runtime alias resolution
- record creation
- relationを読むInvariant
- Derived Value、Action Precondition
- stateを変えないaction
- Occurrence、Effect

これらをerrorにすることは将来不要という判断ではなく、最初のrelation readを独立に検証する境界である。

## Review判定基準

- relation pathのrootとterminal ownerをIRだけから一意に説明できるか。
- targetとvalueが同じrelationなら、同じruntime identityを読むことがFactへ残るか。
- target-unavailableとvalue-unavailableに重複または到達不能なFactがないか。
- pre-state valueをatomic boundary外で読み直す実装をReview Requirementが許していないか。
- 参照先entityのpage accessをdomain actionへ誤って継承していないか。
- action accessの正しさだけでcross-entity valueの利用／再公開を安全と断定せず、人間へ提示しているか。
- `IRExpression`の共有field追加が、relation traversalを許さないInvariant validatorを開けていないか。
- relation traversal以外のoperator、collection、Derived Value semanticsを先回りで発明していないか。
- repository E2Eがrelation先とselfへ異なる値を置き、誤ったbindingでもgreenになる経路を閉じているか。
