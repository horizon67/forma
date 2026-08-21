# Changes and Atomic Post-State Proposal

Status: experimental vertical slice implemented and measured; not part of normative v0

この文書は、actionがstate transition以外のdomain dataを変更するとき、その変更集合をFormaでどう表し、
どこまでを1つのatomic post-stateとしてcoding agentへ渡すかを定める。最終対象は注文承認時の在庫引当だが、
最初のcompiler sliceへcollection traversal、加算、複数在庫、record作成を同時に持ち込まない。

規範v0は[`v0-primitives.md`](v0-primitives.md)、Expressionの現在地は
[`expression-proposal.md`](expression-proposal.md)、注文全体のdesign probeは
[`order-approval-proposal.md`](order-approval-proposal.md)にある。本proposalの`changes`構文は比較用candidateであり、
このbounded構文はreference Parserが受理する。ただし規範v0の10 primitivesや完了条件にはまだ含めず、
collection、複数assignment、record作成へ一般化する前のexperimental sliceとして扱う。

## 実装結果（2026-08-21）

Parser、checker、Resolved Intent `v0.9`、Source Map `v0.6`、Acceptance Facts `v0alpha7`、
Outcome Projection `v0alpha3`、Review Requirements `v0alpha3`まで実装した。既存domain actionについても
transition accepted/source rejected、confirmation accepted/declined、surface feedbackを導出し、Changes actionには
atomic accepted、Invariant rejected、target unavailableのdomain/surface Factsを追加した。validatorはResolved Intentから
このaction contractを再導出し、Factの欠落・捏造・result反転を拒否する。

[`order-invariant-agent-e2e`](../experiments/order-invariant-agent-e2e/)では、Forma runtimeを持たない通常のGo applicationへ
`StockReservation.commit`を実装し、275/275 Factsを52 repository testsで再測定した。implicit transitionとrelated
StockItem updateは同じmutex内でのみcommitされ、Invariant違反、target欠落、source state不一致では両entityが不変である。
atomicity、cross-entity authorization、既存Invariant concurrencyの3 Review Requirementsは人間確認待ちである。

## 結論

`Changes`は新しいtop-level primitiveではなく、既存`action`が所有する**同時代入のpost-state宣言**とする。

```forma
action StockReservation.commit: Pending -> Committed allow staff {
    changes {
        stock.reserved = reservedAfter
    }
}
```

このactionが成功したとき、暗黙のstate transitionと明示したfield assignmentは1つのatomic boundaryでcommitする。
右辺はすべて同じpre-stateから評価し、完成したcandidate post-stateに対してfield constraintとInvariantを検査する。
どれか1つでも成立しなければ、stateを含む全変更を破棄する。

`changes`は命令列ではない。宣言順は表示以外の意味を持たず、read-after-write、早期return、条件分岐、loop、
例外処理を持たない。

## 最終対象と最初のbounded sliceを分ける

最終的に表したい注文承認は次の1 operationである。

```text
Order.approve
  Order.status: Submitted -> Approved
  each OrderLine.quantityを対応するStockItem.reservedへ加算
  ApprovalAuditを作成
```

ここには少なくとも、to-many relation、element binding、集約またはfan-out、`+`、複数runtime target、record作成が
必要である。これらを最初から入れると、atomicityの成否とcollection languageの成否を分離できない。

最初のsliceは同じdomain needを、requiredなto-one relationと既存field valueだけで表す。

```forma
type Quantity = Int min 0

entity StockItem {
    onHand   Quantity required
    reserved Quantity required

    invariant stockAvailable: reserved <= onHand
}

entity StockReservation {
    stock         StockItem required
    reservedAfter Quantity required

    state status Pending | Committed initial Pending
}

action StockReservation.commit: Pending -> Committed allow staff {
    changes {
        stock.reserved = reservedAfter
    }
}
```

この例は、暗黙の`StockReservation.status`変更と明示的な`StockItem.reserved`変更を同時に持つ。
`reservedAfter`はabsoluteなcandidate valueであり、`+`を必要としない。実用上のincrementを最終形から削除したのではなく、
operator追加より先にChangesのatomic boundaryを検証するためのprobe inputである。

## 2つ目のapplicationで意味を照合する

Changesのatomicityは注文だけに固有ではない。会員登録のemail verificationでも、成功時にはverification evidenceの
consumeと`Pending -> Active`を同じoutcomeにし、invalid、expired、consumed済みならどちらも変更しない必要がある。
これは[`email-verified-membership-probe.md`](email-verified-membership-probe.md)と
[`identity-semantic-model-proposal.md`](identity-semantic-model-proposal.md)で既に要求している。

```text
verification accepted
  evidence: available -> consumed
  membership: Pending -> Active

verification rejected
  evidence: unchanged
  membership: Pending
```

したがって、`all changes committed`／`no changes committed`というoutcome vocabularyは、在庫引当とは別のapplicationでも
成立する。一方で、Identityのcredentialやverification evidenceを通常のdomain fieldとして公開したり、既存の専用operationを
機械的に`changes`構文へ書き換えたりはしない。ここで照合するのはatomic outcomeの意味であり、同じconceptへ汎用構文と
Identity専用構文を二重に与えることではない。

## Action ownership

Changesはaction bodyに置く。別のtop-level宣言にはしない。

```forma
// 採用候補
action StockReservation.commit: Pending -> Committed allow staff {
    changes {
        stock.reserved = reservedAfter
    }
}

// 採らない
changes StockReservation.commit {
    stock.reserved = reservedAfter
}
```

actionから分離すると、認可、confirmation、source state、navigationとatomic boundaryのownerが二重になる。
action bodyなら「1 action invocation = 1 atomic boundary」が構造から一意になる。既存の`confirm`、`allow`、`goto`は
action headerのmodifierとして維持し、bodyへ移さない。

bodyを持たない既存actionの意味とidentityは変えない。`changes`は0個または1個で、空blockと複数blockはcompile errorにする。

### 構文候補の比較

| 候補 | 判定 | 理由 |
| --- | --- | --- |
| action bodyの`changes { ... }` | 現candidate | transition、認可、navigation、atomicityのownerをactionへ集約できる |
| top-level `changes ActionName { ... }` | 採らない | actionとの対応とatomic boundaryを別宣言間で再構成する必要がある |
| header modifierへ1代入を直書き | 採らない | 最初のsliceには短いが、複数assignmentへ広げるとheaderが構造を失う |
| `set`、`if`等を持つstatement body | 採らない | 宣言順と途中状態をlanguage semanticsへ持ち込む |

この比較はaction bodyを最終構文として確定するものではない。compiler sliceと生成applicationの人間評価で、headerの可読性や
page-local actionからの追跡性に問題が出た場合は、意味論を維持したままsurface syntaxを再比較する。

## 認可の所有者

cross-entity writeを許可するroleは、Changesを所有するdomain action自身の`allow`が所有する。変更対象entityを表示・編集する
別page、view、formの`allow`をactionへ暗黙に継承しない。それらは別のinvocation surfaceに対する認可であり、entityやfieldへ
付与されたglobal write policyではない。

ただしaction referenceを実際のpageから呼ぶときは、既存v0どおりsource page、domain action、固定destination pageのaccessを
合成する。Changesがこの合成を緩めることはない。authoritativeなmutation境界では少なくともdomain action自身の`allow`を
再検査し、source／destination pageの境界も既存規則どおり守る。

したがって、`StockItemEdit`が`allow admin`でも、`StockReservation.commit`の`allow staff`はstaff向けの別write pathを
意図的に宣言する。対象pageの`allow admin`を追加条件として推測してはならない。一方、この差が意図したものかは機械的に
断定できないため、action entity以外を変更するactionごとに次のReview Requirementを導出する。

```text
review/action/StockReservation/commit/cross-entity-write-authorization
```

Review Requirementは、action access、参照元surfaceのeffective access、変更対象entity／field、同じfieldを変更できる既存surfaceを
人間へ提示する。role集合が一致することは要求せず、意図しないwrite pathを作っていないことを確認させる。

## Assignmentの意味

左辺と右辺は役割が異なる。

```text
stock.reserved = reservedAfter
^^^^^^^^^^^^^^   ^^^^^^^^^^^^^
mutation target  pre-state expression
```

最初のsliceで許す左辺は次だけである。

- action entity自身のstored scalar field。
- action entityからrequiredなto-one relationを1回辿った先のstored scalar field。
- collection、optional relation、readonly／derived field、relation fieldそのもの、state fieldは対象外。
- stateは既存`A -> B`だけが所有し、`changes`から二重に書かない。

右辺はExpression treeへ解決する。最初のsliceでは、action entity自身のrequired scalar field参照だけでよい。
relation traversal、literal、算術は必要になった順に追加する。したがって最初のcandidateは
`stock.reserved = reservedAfter`を受理するが、`stock.reserved = stock.reserved + quantity`はまだ受理しない。

## Pre-state snapshotと同時代入

action実行時の意味順序を次に固定する。

1. principal、confirmation、source stateをauthoritative boundaryで検査する。認可は前節のownerとv0のaccess合成に従う。
2. 左辺targetのruntime identityと、全右辺が読む値を同じpre-state snapshotから解決する。
3. targetを解決できなければ`target-unavailable`として拒否し、何もcommitしない。
4. 全右辺を評価し、implicit state transitionを含むcandidate post-stateを組み立てる。
5. changed fieldの型とconstraintを検査する。
6. changed entityが所有するすべてのInvariantをcandidate post-stateで検査する。
7. 全変更を1回でcommitする。2から6のどこかで失敗すれば何もcommitしない。

例えば将来、次の同一entity assignmentを許しても、結果は宣言順に依存しない。

```forma
changes {
    left  = right
    right = left
}
```

両右辺はpre-stateを読むので値はswapされる。上から逐次実行して両方が同じ値になる実装は不適合である。

## Relation targetの固定

`stock.reserved`の`stock` bindingはpre-stateで解決する。target relationを同じactionで差し替えながら、その新旧どちらを
更新するかを暗黙に決めてはならない。最初のsliceでは、Changesのtarget pathに使ったrelation field自体を同じblockで
変更することをcompile errorにする。

source上で同じtarget pathへ2回assignmentした場合もcompile errorにする。

```forma
changes {
    stock.reserved = reservedAfter
    stock.reserved = otherValue // duplicate target
}
```

異なるrelation pathがruntimeで同じentity instanceへaliasする問題は、source pathの比較だけでは解決しない。
最初のsliceは明示assignmentを1個に限定してこの問題を持ち込まない。複数assignmentへ広げる前に、runtime target identityで
衝突を拒否するか、同値assignmentだけを統合するかを別途決める。

## Runtime targetを解決できない場合

`required`はsource model上、そのrelation valueが必須であることを表すが、参照先entityが実行時にも存在し続けることまでは
保証しない。v0の標準`delete`にはreferential integrityの一般規則がないため、dangling relationやconcurrent deleteを
実装依存のpanic／部分commitにしてはならない。

pre-state snapshotでtarget identityをlive entityへ解決できない場合、またはcommitまで同じtargetを保持できない場合は、
action全体を`target-unavailable`として拒否する。implicit transitionと明示Changesはどちらもcommitせず、page actionでは
存在の詳細を開示しない`failure` feedbackを観測可能にする。別のtargetへ暗黙に差し替えて成功させてはならない。

参照先の削除を禁止するかcascade／restrictを選ぶ一般的なreferential integrityは、このsliceでは決めない。どのpolicyでも、
Changes実行時の安全なoutcomeは上記で固定する。

## ConstraintとInvariantの再検査

変更されたentityごとに、そのentity自身のfield constraintとself-only Invariantをすべて再検査する。
`StockItem.reserved`を変更するので、次を必ずcandidate post-stateで評価する。

```forma
invariant stockAvailable: reserved <= onHand
```

```text
pre-state
  StockReservation.status = Pending
  StockItem.onHand         = 10
  StockItem.reserved       = 2

reservedAfter = 8
  -> status = Committed, reserved = 8 を同時commit

reservedAfter = 12
  -> Invariant false
  -> status = Pending, reserved = 2 のまま
```

Invariantをaction entityにだけ適用する実装や、先にstatusをcommitしてからStockItemを検査する実装は不適合である。

relationを読むInvariantは本sliceでも許可しない。Changesが複数entityを変更できることと、任意のrelation依存Invariantを
再検査できることは別問題である。後者にはdependency graphと逆方向の再検査規則が要る。

## Resolved Intent candidate

既存`IRAction`のtransition、認可、navigationを維持し、action所有のatomic mutation情報を追加する。

```text
IRAction
  id: action/StockReservation/commit
  sources: [Pending]
  destination: Committed
  atomicity: all-or-nothing
  changes:
    - id: action/StockReservation/commit/change/stock/reserved
      target:
        root: self
        relationPath: [entity/StockReservation/field/stock]
        field: entity/StockItem/field/reserved
      value:
        FieldReference
          binding: self
          field: entity/StockReservation/field/reservedAfter
          resultType: Quantity
      evaluation: pre-state
```

このactionを含む明示domain actionを提示する各`IRActionRef`には、既存`access`に加えてcompiler-ownedな
`interactionStates: [invalid, failure]`を持たせる。標準`delete`のreferenceは`[failure]`だけを持つ。mutationを所有せず
別surfaceへ移動する標準`create`、`view`、`edit`のreferenceにはaction-owned `interactionStates`を持たせない。
create／editのmutation feedbackは遷移先formのsubmitが所有する。`invalid`と`failure`の原因対応は後述の閉じた表から導出し、
agentに分類させない。actionがcross-entity targetを持つことや対象fieldもResolved Intentから決定でき、Review Requirementが
source textを再parseせず表示できなければならない。

implicit state transitionを`changes`配列へ重複格納しない。ただしGeneration Requestでは、1 atomic outcomeの
post-stateとしてtransitionと明示Changesの両方を列挙する。coding agentに別transactionだと解釈させないためである。

Change nodeのstable IDはaction IDと解決済みtarget pathから導出する。右辺だけを変えた場合は同じChange IDのsemantic diff、
targetを変えた場合はremove/addになる。Source MapはChange全体、target path、value expressionを別nodeとしてsourceへ戻せる
必要がある。

## Acceptance Facts candidate

Changes固有Factの前に、既存domain actionのtransition outcomeをFactへする。現在のcompilerはaction referenceに
accessとnavigationのFactだけを生成しており、v0が最低要件とするsource precondition、confirmation、遷移後stateを
機械的に要求していない。最初のChanges sliceはこの既存gapも閉じる。

### 0. domain actionのtransition outcomeを先に固定する

Changesの有無にかかわらず、すべての明示domain actionから最初の2つを導出する。confirmationは、明示domain actionに
`confirm`がある場合だけでなく、v0で常に確認必須と定めた標準`delete`の各referenceからも3つ目を導出する。

- `transition-accepted`: 各source stateから1回dispatchし、destination stateへ1回だけ遷移する。
- `transition-source-rejected`: source集合外の各stateからdispatchし、outcomeは`rejected`、stateと他のmutationは不変にする。
- `confirmation-required`: `confirm`付きdomain actionまたは標準`delete`の各page action referenceでconfirmationを提示し、
  declineではdispatch 0回、stateと他のmutationを不変にする。acceptではdispatch 1回から通常outcomeへ到達する。

最初の2つは`action/...`をsubjectとするdomain outcome Factである。confirmationはsurfaceでしか観測できないため、
各`page/.../action/...`をsubjectとし、明示actionならそのdeclarationもsource nodeに含める。この追加で標準`delete`を含む
既存172 Factsのbaselineが変わることは意図した結果であり、実装後のGeneration Requestは275 Factsへ更新した。

### 1. valid post-stateは全変更をcommitする

```text
subject: action/StockReservation/commit
setup:
  source state Pending
  StockItem(onHand=10, reserved=2)
  reservedAfter=8
expected:
  outcome accepted
  evaluation pre-state
  atomicity all-changes-committed
  StockReservation.status=Committed
  StockItem.reserved=8
```

### 2. invalid post-stateは全変更を拒否する

```text
setup:
  source state Pending
  StockItem(onHand=10, reserved=2)
  reservedAfter=12
expected:
  outcome rejected
  violated entity/StockItem/invariant/stockAvailable
  atomicity no-changes-committed
  StockReservation.status=Pending
  StockItem.reserved=2
```

### 3. 解決できないtargetは全変更を拒否する

```text
setup:
  source state Pending
  StockReservation.stockが参照するStockItemは存在しない
  reservedAfter=8
expected:
  outcome rejected
  reason target-unavailable
  atomicity no-changes-committed
  StockReservation.status=Pending
```

このFactはInvariant rejectionと混ぜない。target identityの解決または保持に失敗した経路を明示的に通し、panicや
stateだけのpartial commitでは満たせないようにする。page action counterpartは同じoutcomeに`failure` feedbackを要求する。

### 4. assignmentはpre-stateから同時評価する

複数assignmentを許すsliceでswap caseを追加し、宣言順を逆転しても同じpost-stateになることを要求する。
最初の1-assignment sliceではFactを捏造せず、Resolved Intentの`evaluation: pre-state`とvalidatorを先に固定する。

### 5. page actionは実surfaceを通る

明示domain actionがlist/detailへ公開される場合、action-level Factだけで完了にしない。各`page/.../action/...` subjectへ、
実invocationが同じatomic outcomeへ到達する正常系と拒否系Factを導出する。repository/store testだけでpage Factを
満たせないことはorder invariant E2Eで確認済みである。

action referenceのcompiler-owned feedback vocabularyは最初のsliceでは次へ閉じる。新しいsource syntaxは追加しない。

| 原因 | surface feedback | mutation |
| --- | --- | --- |
| 明示domain actionのsource state不一致、field constraint／Invariant違反 | `invalid` | 0 |
| Changes target unavailable、明示domain action／標準`delete`の永続化失敗 | `failure` | 0 |
| confirmation decline | なし。dispatch自体が0回 | 0 |
| access denied | feedbackではなく既存access-denied outcome | 0 |

既存`IRView.interactionStates`と`observable-feedback` Factは、そのview自身のload、empty、query、form submit等の結果だけを
所有する。子であるaction invocationのfeedbackを集約した閉集合ではない。したがってlist／detailのview Factが
`[empty, failure]`のままでも、明示domain actionのpage referenceは独立したFactと`IRActionRef.interactionStates`により
`invalid`を必ず表示できなければならない。view Factを根拠にactionの`invalid`を省略してはならず、同じsurfaceを通るtestで
両方を観測する。

action-level Factはdomain outcomeを、page action Factは対応するfeedbackを実surfaceで観測する。これにより、拒否理由を
agentがHTTP status、例外、無言のredirectのどれかへ勝手に割り当てる余地をなくす一方、具体的な表示文言は規定しない。

## Human Review Requirement

単一threadの正常・拒否testだけでは、複数entityのtransaction boundaryとconcurrent conflictを証明できない。
明示Changesを持つactionごとに、次のcompiler-owned Review Requirementを候補とする。

```text
review/action/StockReservation/commit/atomic-changes-enforcement
```

人間は、全targetのidentity解決、pre-state read、Invariant検査、commitが同じtransaction／lock／conflict retry境界にあり、
concurrent invocationやprocess failureで一部だけcommitされないことを確認する。これは既存の
`concurrent-invariant-enforcement`と重なるが同一ではない。前者はaction全体の複数entity atomicity、後者はInvariantを
変更可能な全mutation boundaryで守ることを対象にする。

また、cross-entity targetを1つでも持つactionには、認可節で定めた
`cross-entity-write-authorization` Review Requirementを別に生成する。transactionがatomicであることと、そのwrite pathを
誰に開くかが妥当であることは別の判断なので、1件へ統合しない。

## 必須diagnostic

実装時に少なくとも次をcompile errorへする。

- action bodyに未知member、空`changes`、複数`changes`がある。
- 暗黙の標準action（`create`、`view`、`edit`、`delete`）へbodyまたはChangesを宣言しようとする。
- left targetが存在しない、stored scalar fieldでない、またはstate fieldである。
- target pathがoptional、to-many、2 hop以上である。
- 同じtarget pathへの二重assignmentがある。
- target bindingに使うrelationを同じChangesで変更する。
- 右辺を解決できない、またはresult typeをtarget fieldへ代入できない。
- changed entityのInvariant dependencyをcompilerが決定できない。

diagnostic codeはparser/checker実装時に既存familyとの衝突を確認して割り当てる。proposal段階で番号だけを予約しない。

## 採らない案

### Imperative statement block

```forma
changes {
    if stock.available {
        stock.reserved += quantity
    }
}
```

control flow、順序、途中状態、target runtime例外をlanguage semanticsへ持ち込むため採らない。条件はAction Precondition、
値はExpression、拒否はInvariantとして分離する。

### ChangesをEffectとして扱う

在庫引当はactionをrollbackするなら一緒にrollbackすべきdomain stateである。通知のような外向きEffectではない。

### Repository transaction構文

`transaction`、SQL isolation level、lock、unit-of-work名をsourceへ書かない。Formaはall-or-nothingとconcurrent outcomeを
要求し、mechanismはtarget repositoryとcoding agentが選ぶ。

### Full order approvalから始める

最終目標ではあるが、collection fan-out、算術、record creationを一度に導入するため、最初の言語sliceにはしない。

## 最初のcompiler slice（実装済み）

設計review後の実装範囲を次に固定する。

1. action-attachedな`changes` blockと`target = expression`をParser／ASTへ追加する。
2. targetはself scalarまたはrequired to-one relation 1 hop先のscalar、明示assignmentは1個に限定する。
3. valueはaction entity自身のrequired scalar field参照だけを受理する。
4. Resolved Intent、Source Map、stable Change IDを生成する。
5. 全明示domain actionへsource preconditionと遷移後state Factを追加し、`confirm`付きactionと標準`delete`の全surfaceへ
   confirmation Factを追加する。
6. 明示domain action referenceへ`invalid`／`failure`、標準`delete` referenceへ`failure`のfeedback vocabularyとsurface
   Factを追加する。標準`create`、`view`、`edit` referenceには追加しない。
7. transitionとChangeを含む正常、Invariant拒否、target-unavailableのatomic Acceptance Factsを生成する。
8. `atomic-changes-enforcement`と`cross-entity-write-authorization` Review Requirementsを生成する。
9. Generation Request diff、validator、projectionの非回帰を固定する。既存actionのFact追加によるbaseline更新も含む。
10. `StockReservation.commit`を通常applicationへ実装するE2Eで、partial commit、target欠落、surface bypass、
   cross-entity access bypass、confirmation bypassのmutationを行う。

このsliceが成立してから、次の順で広げる。

```text
required relation value read
  -> numeric +
  -> multiple assignmentsとruntime alias rule
  -> collection element binding / fan-out
  -> record creation（ApprovalAudit）
  -> full Order.approve
```

OccurrenceとEffectはfull Order.approveのatomic outcomeが定まるまで実装しない。

## Review判定基準

- action headerとChangesのownerが1つか。
- 宣言順を変えても意味が変わらないか。
- valid／invalidの両方で、transitionを含む全post-stateを一意に説明できるか。
- Invariant対象entityをcoding agentに推測させていないか。
- page actionをrepository内部testだけで満たす穴がないか。
- actionが別entityを変更するとき、認可ownerとtarget-unavailable outcomeが一意か。
- collection、Effect、target frameworkのmechanismを最初のsliceへ混ぜていないか。
