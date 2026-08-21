# Multiple Assignment Proposal

Status: design review incorporated — implementation deferred until after the fastest alpha cut; not part of normative v0

## 結論

次のP3 sliceでは、1つの明示domain actionにある単一`changes` blockへ、**1件または2件のassignment**を
宣言できるようにする。

```forma
entity StockReservation {
    code                 String required unique label
    stock                StockItem required
    plan                 ReservationPlan required
    requestedReserved    Quantity required
    stockReservedBeforeCommit Quantity required

    state status Pending | Committed initial Pending
}

action StockReservation.commit: Pending -> Committed confirm allow staff {
    precondition withinRequestLimit: stock.reserved + requestedReserved <= plan.requestCeiling

    changes {
        stock.reserved = stock.reserved + plan.approvedReserved
        stockReservedBeforeCommit = stock.reserved
    }
}
```

これは逐次statement 2本ではない。全Preconditionと全RHSを同じconsistent pre-stateから評価し、全targetの
candidate post-stateを組み立て、全Invariantを検査した後、implicit state transitionを含む全mutationを
1つのall-or-nothing outcomeとしてcommitする宣言である。

最初のsliceへcollection fan-out、record creation、conditional assignmentは入れない。assignment数を2件へ閉じるのは、
複数target、atomic outcome、同時評価の観測可能な一方向、Fact completenessを分離して検査するためであり、最終的な
cardinality制限ではない。

## 2026-08-22 design reviewの決着と現在地

このproposalはcompiler実装前のdesign reviewを完了し、次の2点を反映した。

1. Order fixtureはcanonicalな`stock update -> before snapshot`方向の逐次誤実装を値で検出する一方、逆順の逐次実装は
   正しい結果と区別できない。その観測限界を明記し、全RHSのwrite前materializationとwrite-order independenceは
   `atomic-changes-enforcement`のhuman reviewへ移した。compilerのswap fixtureをrepository enforcementの代替証明にはしない。
2. repository E2Eでenforceしない`target-alias-conflict` runtime semanticsは導入しない。異なるtarget bindingが同じ
   resolved entity typeを持つsourceを`F2816`で静的に拒否し、runtime aliasのidentity comparison、outcome、Fact familyは
   それを実applicationで観測できる後続sliceへ送った。

設計成果は、1 block／最大2 assignments、canonical Change order、同一pre-state evaluation、type-disjoint target grouping、
complete candidate Invariant、multi-field atomic Fact、relation別availability ID、Review Requirement拡張までである。
compiler、Order fixture、Generation Request、schema versionはまだ変更しておらず、current executable baselineは
Action Preconditionまでを実装したResolved Intent `v0.12`、Acceptance Facts `v0alpha10`、Outcome Projection `v0alpha5`、
Review Requirements `v0alpha6`、Order 280/280 Factsである。

最速alphaではこのcurrent executable baselineをscope freezeする。multiple assignmentは設計を失わずpost-alpha backlogへ置き、
alpha利用で実際に必要になったapplicationから実装優先度を再確認する。

## なぜcollectionより先か

current compilerは`IRAction.Changes []IRActionChange`を既に持つが、checker、builder、validator、binding plan、
Acceptance Factsはすべてexactly one assignmentへ閉じている。collectionを先に入れると、1つのsource assignmentが
runtimeでN件へ展開されるため、次の未知が同時に入る。

- explicit assignment間の同時評価
- runtime element selectionとcardinality
- element identity、ordering、empty collection
- 同一recordへの重複bindingと競合
- N targetにまたがるtransaction boundary

まずsource上で固定2件を扱えば、target集合がcompile時に有限である状態で同時評価とtarget groupingを決められる。
異なるbindingが同じentity typeをtargetにするsourceはこのsliceで静的に拒否し、runtime aliasはcollectionとともに
後続で扱う。collection bindingはtype-disjointな固定target規則をN件へ一般化する次のsliceとする。

## bounded fixtureが検査する意味

repository testでは、commit前を少なくとも次のdistinct値にする。

```text
StockReservation.status               = Pending
StockReservation.stockReservedBeforeCommit = 91     // decoy
StockItem.reserved                     = 2
StockItem.onHand                       = 10
ReservationPlan.approvedReserved       = 6
ReservationPlan.requestCeiling         = 6
StockReservation.requestedReserved     = 3
```

accepted outcomeは次である。

```text
StockReservation.status               = Committed
StockReservation.stockReservedBeforeCommit = 2
StockItem.reserved                     = 8
```

`stockReservedBeforeCommit`は、承認量を反映する前にauthoritative boundaryで観測した在庫予約数を記録する。
91のままなら2本目を落としており、6ならplanへ誤配線し、8なら1本目をcommitした後に2本目のRHSを読んでいる。
これにより、同じpre-stateからの評価を値だけで区別できる。

fixtureのcanonical Change IDは`.../change/stock/reserved`と`.../change/stockReservedBeforeCommit`であり、ID順でも
stock updateが先になる。したがってGeneration Requestのcanonical orderをそのまま逐次実行する誤実装も8を記録して落ちる。
ただし、self fieldを先に2へ保存してからstockを8へ保存する逆順の逐次実装は、このfixtureの値だけでは正しい結果と
区別できない。このE2Eが直接示すのはcanonicalな`stock update -> before snapshot`方向の誤実装までである。
全RHSを最初のwrite前にmaterializeし、write順を逆転しても結果が変わらないことは`atomic-changes-enforcement`の
Review Requirementでimplementation boundaryを確認する。

`ReservationPlan`と`StockReservation`にはpage／viewを追加しない。scalar field追加をCRUD surfaceへ暗黙に公開せず、
multiple assignmentに無関係なpage Factを増やさない。

## source semantics

### cardinality

- action bodyは従来どおり`changes` blockを0件または1件持つ。
- non-empty blockは最初のsliceで1件または2件のassignmentを持つ。
- 既存の1-assignment sourceと意味は互換である。
- 0件、3件以上、複数blockはcompile errorにする。

parserは既にblock内の複数assignmentを構文上保持できる。変更の中心はchecker以降のclosed semanticsである。

### declaration orderは意味ではない

assignmentのsource順は表示上の順序にすぎず、評価順、write順、failure priorityを表さない。compilerはChange IDで
canonical sortしたResolved Intentを作る。次の2 sourceはResolved Intent、Acceptance Facts、Review Requirements、
Outcome Projectionでbyte-identicalなsemantic outputを持つ。Source Mapだけは各nodeを実際のsource spanへ戻す。

```forma
changes {
    left = right
    right = left
}
```

```forma
changes {
    right = left
    left = right
}
```

このswap fixtureではpre-stateが`left=A, right=B`ならpost-stateは常に`left=B, right=A`である。最初の代入結果を
次のRHSが読む逐次実装は言語上許さない。compiler testが固定できるのはdeclaration順に依存しないIR／Factの形までであり、
target repositoryがswapを本当に同時評価することの代替証明にはしない。

### semantic order

accessとconfirmationを通過し、source stateが一致したinvocationについて、authoritative boundaryは次を1つの
consistent snapshotと1つのatomic commitで扱う。

1. action subjectのsource stateを検査する。
2. 全Change target、全RHS、全Preconditionが使うrequired relation identityをpre-stateから解決する。
3. 全Preconditionを同じpre-stateから評価する。
4. 全RHSを同じpre-stateからexactに評価する。assignment間のwriteはまだ行わない。
5. target bindingごとに全assignmentをmergeしたcandidate post-stateを組み立てる。
6. changed fieldを持つ各entityのInvariantをcomplete candidateに対して検査する。
7. implicit destination stateと全explicit field mutationを1回だけcommitする。

途中の失敗ではstate、self field、relation targetのいずれも変更しない。transaction、lock、optimistic conflict retry等の
実装mechanismはtarget repositoryが選ぶが、snapshotとatomic outcomeの意味は変えない。

### target group

compile時のtarget binding keyを次で定める。

```text
self                         -> subject/action
required relation field R    -> relation field semantic ID
```

同じbinding keyへ複数fieldを書ける。例えば`self.a`と`self.b`、または`stock.reserved`と`stock.onHand`は、
同じruntime subjectの1 candidateへmergeし、そのentityのInvariantをmerge後に1回評価する。

同じcanonical target path、すなわち同じbinding keyと同じfieldへのassignmentを2件書くsourceはcompile errorにする。
RHSが同一でもdedupeせず、source順でlast-write-winsにもfirst-write-winsにもしない。

## distinct target bindingの静的境界

source上で異なるrequired relation fieldが同じentity typeをtargetにすると、runtimeでは同じrecordを指す可能性がある。
そのidentity比較と拒否を実applicationで観測しないままruntime outcomeだけを新設しない。

最初のsliceでは次へ閉じる。

- 同じtarget binding keyの別fieldは、intentionalな同一subjectとして1 candidateへmergeできる。
- 異なるtarget binding keyは、resolved target entity typeがpairwise distinctでなければcompile errorにする。
- selfとself-relationが同じaction entity typeをtargetにする場合も後者に含む。

これにより、受理したsourceではdistinct binding間のruntime aliasが型上起きない。`target-alias-conflict` outcome、reason、
surface feedback、Acceptance Fact familyは作らない。same-type targetが必要なapplicationが現れた後、明示alias contract、
identity comparison、safe merge／rejectionをrepository E2Eできる独立sliceとして設計する。

E2E fixtureのtargetはself `StockReservation`とrelation `StockItem`でtypeが異なるため受理される。compiler fixtureでは
same-type relation 2本とself＋same-type relationをそれぞれ拒否し、validatorもtampered IRから同じ形を受理しない。

## binding ownershipとhandle

action-wide binding planをsingularなtarget/valueから、全Changeを含む集合へ広げる。同じrequired relationが複数用途を
持つ場合のowner priorityは次を維持する。

```text
target > Changes value > Precondition-only value
```

canonical handleは次とする。

```text
self                               subject/action
target relation stock              subject/target/stock
value-only relation plan           subject/value/plan
Precondition-only relation limit   subject/precondition/limit
```

同じrelationが1件目のtarget、2件目のRHS、Precondition operandを兼ねてもsetup subjectは1件であり、すべて
`subject/target/<relation>`を使う。複数targetを導入した後もsingularな`subject/target`をfirst declarationへ割り当てると、
source順でFactが変わるため使用しない。

fixtureでは`stock`がtarget owner、`plan`がvalue ownerである。2本目のself targetは`subject/action`を使い、
そのRHSの`stock.reserved`は1本目と同じ`subject/target/stock`へbindingする。

## authorizationとdata access

Changesを持つactionの実行認可ownerは、assignment数が増えてもaction自身の`allow`である。target entityを提示するpageの
roleを暗黙に継承しない。pageから呼ぶ場合は既存どおりsource page、action、destination pageのaccessを合成する。

- cross-entity targetを1件以上持つactionには`cross-entity-write-authorization`をaction単位で1件導出し、全targetを列挙する。
- cross-entity RHS／Precondition leafを持つactionには`cross-entity-value-read-authorization`をaction単位で1件導出し、
  全readとdownstream disclosure surfaceを列挙する。
- assignmentごとに同じReview Requirementを重複生成しない。

## grammar boundary

EBNF shapeは現行を変えない。

```ebnf
ChangesBlock     = "changes", "{", ChangeAssignment, { ChangeAssignment }, "}" ;
ChangeAssignment = FieldPath, "=", Expression ;
```

semantic checkerが最初のsliceを1..2 assignmentsへ閉じる。各assignmentのtargetとRHSは既存sliceをそのまま使う。

- target: self mutable stored scalar、またはrequired to-one relation 1 hop先のmutable stored scalar
- RHS: required self／relation scalar field reference、またはfield reference 2個のexact numeric `+`
- targetとRHSは同じnominal scalar type
- optional／collection／2 hop／literal／record constructorは不可

## diagnostics

既存codeを次の責務で維持する。

- `F2801`: Changes blockが2件以上。
- `F2802`: non-empty blockのassignmentが0件または3件以上。hintを「1件または2件」へ更新する。
- `F2803`–`F2809`: 各assignmentのtarget、value、type、numeric closureを従来どおり検査する。
- `F2815`: 同じcanonical target pathへのduplicate assignment。
- `F2816`: 異なるtarget binding keyが同じresolved entity typeをtargetにする。

Resolved Intent validatorはcardinality、Change ID、canonical order、duplicate target key、target binding間のpairwise
distinct entity type、各target／RHS shape、typeを独立に再導出する。parserを通らないtampered IRで3件目、重複target、
same-type distinct binding、非canonical orderを持ち込めないようにする。

## Resolved Intent

container shapeは既存のまま利用する。

```text
IRAction
  atomicity = all-or-nothing
  changes[] = canonical Change ID order

IRActionChange
  id
  evaluation = pre-state
  target
  value
```

implicit state transitionは`changes[]`へ重複格納しない。`IRAction.Changes`がsliceであることだけを根拠に受理せず、
validatorが1..2件と全要素をfull compareする。

Change IDは従来どおりaction IDとresolved target pathから作る。duplicate target pathは同じIDになる前にsource diagnosticで
拒否し、process boundary後もvalidatorが拒否する。builder、clone、semantic ID traversal、Source Map coverageは全Changeを
走査する。Source Map schemaは変えず、各assignment、target、expression treeを個別nodeとしてsourceへ戻す。

## Acceptance Facts

### 1 actionに1つのatomic accepted Factを作る

assignmentごとのaccepted Factは作らない。別testで各assignmentが成功しても、同じinvocationで同時commitした証明に
ならないためである。各source stateについて既存のdomain／surface `changes-accepted`を1件ずつ維持し、expected subjectsへ
全field mutationを入れる。

```text
expected:
  outcome = accepted
  atomicity = all-changes-committed
  appliedMutations = 3
  subject/action.status = Committed
  subject/action.stockReservedBeforeCommit = expression-result(pre-state stock.reserved)
  subject/target/stock.reserved = expression-result(pre-state stock.reserved + plan.approvedReserved)
```

`AppliedMutations`はそのFactの`Expected.Subjects`が主張するchanged state／field数とする。したがってcomplete
Changes Factではimplicit transition 1 + explicit assignment 2 = 3である。独立した`transition-accepted` Factはstate facet
1件だけを主張する既存contractを維持し、両者を足し合わせない。

各`FactFieldExpectation.expression`はcomplete IR tree、`evaluation=pre-state`、全leaf bindingを持つ。Subjects、Fields、
bindings、sourceNodesはsemantic ID／handleでcanonical sortし、source declaration orderを保存しない。

### Invariant rejectionはcomplete candidateを使う

target bindingごとにchanged field集合を作り、そのentityのInvariantが集合のいずれかを参照するとき1件のrejection Factを
導出する。同じInvariantを2 assignmentが触ってもFactは1件である。

Fact inputは全target groupのpost-state Invariant evaluationを持ち、該当Invariantだけをfalse、他をtrueにする。expectedは
implicit transition、全self field、全relation targetをunchangedにし、`appliedMutations=0`とする。

これにより、1件目だけのcandidateでInvariantを検査してから2件目を足す実装や、あるtargetをcommitしてから別targetの
Invariantで失敗する実装を満たせない。

### availabilityはrelation ownerごとに1件

target relationが複数になり得るため、target-unavailable IDをrelation別へ正規化する。

```text
<subject>/changes/target-unavailable/via/<relation>/from/<source-state>
<subject>/changes/value-unavailable/via/<relation>/from/<source-state>
```

各Factは対象relationだけをunavailable、他のtarget／value／Precondition relationをresolvedにする。target ownerがvalueや
Preconditionも兼ねる場合はtarget-unavailableだけを作り、到達不能なvalue-unavailableを作らない。

fixtureでは`stock`のtarget-unavailableと`plan`のvalue-unavailableを維持するため、Fact総数は増えない。
複数relationが同時に欠落したruntime invocationでもreasonはowner priorityで決まり、relation traversal順は外部意味にしない。

### completeness

`ValidateAcceptanceFacts`はResolved Intentから少なくとも次を再導出する。

- accepted domain／surface Factが全Changeを1 atomic expectationへ過不足なく含む。
- field expectation、expression tree、leaf binding、target handle、pre-state evaluationが各Changeと一致する。
- `appliedMutations`がimplicit transitionと全explicit assignmentを数える。
- affected Invariantごとのdomain／surface rejection Factが存在し、complete candidateと全subject unchangedを持つ。
- distinct target/value relationごとのavailability Factが存在し、owner priorityにより重複しない。
- Changeの欠落、捏造、target差し替え、expression交換、assignment順だけの差を検出する。

上位Generation Requestのcanonical再導出だけに依存せず、exported validator単体でも欠落・捏造を拒否する。

## outcome priority

| 条件 | outcome / reason | page feedback | expression evaluation | commit |
| --- | --- | --- | --- | ---: |
| access denied | existing access-denied | access outcome | none | 0 |
| confirmation declined | cancelled / dispatch none | confirmation surface | none | 0 |
| source state不一致 | rejected / source-state-mismatch | `invalid` | none | 0 |
| target relation欠落 | rejected / target-unavailable | `failure` | none | 0 |
| value／Precondition-only relation欠落 | rejected / value-unavailable | `failure` | none | 0 |
| Precondition false | rejected / precondition-unsatisfied | `invalid` | exact pre-state predicate | 0 |
| Precondition true、complete candidateのInvariant違反 | rejected / invariant-violated | `invalid` | all RHS from pre-state | 0 |
| repositoryがいずれかのexact result／atomic writeを保持不能 | rejected / implementation failure | `failure` | all RHS from pre-state | 0 |
| 全条件成立 | accepted | success navigation | all RHS from pre-state | 1 atomic outcome |

同じpriority内で複数relationが欠落した場合、診断対象集合はsemantic ID順にcanonicalizeし、最初に読んだrelationを
observable reasonにしない。Acceptance Factは各relationを単独で欠落させ、全経路を独立に検査する。

## Human Review Requirements

新しいkindは追加せず、既存requirementを複数assignmentへ広げる。

### atomic-changes-enforcement

instructionとsourceNodesへ全Change、全target、全RHS、全Precondition、全affected Invariantを含める。人間は次を確認する。

- 全RHSとpredicateが同じconsistent pre-stateから読まれる。
- 全RHS resultが最初のwriteより前にmaterializeされ、write順を逆転しても保存結果が変わらない。
- targetごとに別transactionを開かず、implicit transitionを含めて1 commitになる。
- 途中のInvariant、representation、write、conflict failureで先に書いたfieldを残さない。
- concurrent invocationでtarget集合の一部だけがstaleにならない。

Order E2Eはcanonicalな`stock update -> before snapshot`の逐次mutationを落とすが、逆順の逐次実装は値が正解と一致するため
単独では落とせない。human reviewは実装がたまたま安全なwrite順を選んでいることではなく、capture／candidate構築が全writeより
前にあることを確認する。

### 既存requirements

- `cross-entity-write-authorization`: 全cross-entity Change targetを列挙する。
- `cross-entity-value-read-authorization`: 全RHS／Precondition relation leafを列挙する。
- `exact-numeric-expression-enforcement`: 全Change／Precondition numeric treeを列挙し、どれか1つでもexact resultを
  保持できない場合に全mutationが拒否されることを含める。
- `concurrent-action-precondition-enforcement`: predicate判定から全assignment commitまでを同じboundaryとして確認する。
- affected entityの既存`concurrent-invariant-enforcement`は別ownerとして維持する。

fixtureではkindの追加がないためReview Requirementsは6件のままである。

## Outcome Projection

Outcome Projectionはaccepted rowのsubjectsを全て表示し、assignmentを独立したsuccess rowへ分割しない。

```text
accepted / all-changes-committed
  subject/action.status=Committed
  subject/action.stockReservedBeforeCommit=subject/target/stock.reserved@pre-state
  subject/target/stock.reserved=add(subject/target/stock.reserved@pre-state,
                                    subject/value/plan.approvedReserved@pre-state)
```

Invariant、availability rowは、どのtarget bindingがreasonを所有するかを表示する。表示順はcanonical Change IDであり、
実行順を意味しない。Navigation、Flow、Domain State Projectionはversion文字列以外の意味差分を持たない。

## Schema versioning

compiler sliceで予定するversionは次である。

- Resolved Intent: `v0.12` → `v0.13`（Changes cardinality、canonical order、distinct target type restriction）
- Acceptance Facts: `v0alpha10` → `v0alpha11`（multi-field atomic expectation、target handle／availability ID）
- Source Map: schema shapeは維持し、`intentVersion`だけ`v0.13`へ更新
- Outcome Projection: `v0alpha5` → `v0alpha6`（multi-target atomic row）
- Review Requirements: `v0alpha6` → `v0alpha7`（全Changeを含むatomic／authorization source contract）
- Generation Request: envelope schemaは維持し、canonical componentとrequest digestを更新

version bumpはmembership golden、両Generation Request、retry baseline、membership repair integrity artifactを同じrunで再検証する。

## 最初のcompiler slice

設計review後の実装範囲を次に固定する。

1. checkerが1 block／1..2 assignmentsを受理し、各assignmentへ既存target／value／type検査を適用する。
2. duplicate canonical target pathを`F2815`、same-type distinct target bindingを`F2816`で拒否する。
3. builderが全Changeを構築し、Change IDでcanonical sortする。Source Mapはsource spanを保つ。
4. Resolved Intent validatorがcardinality、order、identity、duplicate target、pairwise distinct target type、全expressionを独立に再導出する。
5. action-wide binding planを全Changeへ広げ、target > value > Preconditionのowner priorityとcanonical handleを固定する。
6. accepted、Invariant rejected、relation別unavailable Factを全Changeの1 atomic contractとして導出する。
7. Fact validatorがChange completeness、subject grouping、expression binding、mutation count、availability completenessを再導出する。
8. Review RequirementsとOutcome Projectionを全Changeへ拡張する。
9. order fixtureへ`stockReservedBeforeCommit`と2件目のassignmentを追加し、target repositoryを1 snapshot／1 commitへ更新する。
10. schema、golden、Generation Request、repair／integrity artifactを更新する。
11. orderの280/280 Facts、52 mapped tests、page subject 244件のHTTP coverage、6 Reviewsを維持する。

compiler testでは少なくとも、self field swap、同じbindingの別field、duplicate target、same-type distinct binding rejection、
assignment順逆転、3 assignments、tampered IR／Factを別fixtureで固定する。

## E2Eとmutation判定

repository targetでは少なくとも次を観測する。

- acceptedでreservedが8、stockReservedBeforeCommitが2、stateがCommittedになる。
- accepted Factを担う同じtestが3 mutationをすべて観測し、assignmentごとの別unit testだけへ分割しない。
- Precondition false、Invariant false、両方false、target/value unavailable、source mismatch、confirmation decline、access deny、
  numeric representation failureで3箇所すべて不変になる。
- snapshot後にbacking stock／plan／reservationを変えてもcapture済み値から8と2を作る。
- concurrent commitでも成功invocationのstockReservedBeforeCommitが、そのinvocationがlock内で観測したstock pre-stateと一致する。
- page actionのinvalid／failureは実HTTP surfaceを通り、success redirectを行わない。

このfixtureはcanonicalな`A -> B`逐次評価を観測するが、値が同じになる`B -> A`逐次評価までrepository mutationで
区別しない。その限界をcompiler swap testで代替したとは主張せず、全RHS materializationとwrite-order independenceを
`atomic-changes-enforcement`のreview evidenceに残す。

少なくとも次のmutationで対応testが落ちなければならない。

1. 2件目のassignmentをbuilder、Fact、target implementationのいずれかで落とす。
2. 1件目だけcommitしてstate／2件目を落とす、または2件目だけcommitする。
3. 1件目を保存した後に2件目の`stock.reserved`を読み、stockReservedBeforeCommitへ8を保存する。
4. stockReservedBeforeCommitへdecoy 91、plan value 6、またはrequestedReserved 3を保存する。
5. Precondition falseまたはInvariant falseの前にstockReservedBeforeCommitだけ書く。
6. second write failure後にstock=8またはstate=Committedを残す。
7. compilerがassignment宣言順をIR／Fact orderへ保存する。
8. Factからsecond field、expression binding、または`appliedMutations=3`のいずれかを落とす。
9. 同じtarget pathを2回書くsource／tampered IRを受理する。
10. same-type distinct target bindingをcheckerまたはResolved Intent validatorが受理する。
11. 同じbindingの別fieldを別runtime subjectとして解決する。
12. affected Invariantを1 assignmentずつのpartial candidateで検査する。
13. target owner relationをvalue-unavailableとして重複発行する、またはtarget-unavailable Fact IDを衝突させる。
14. atomic／authorization Review RequirementのsourceNodesから2件目を落とす。

## このsliceへ入れないもの

- assignment 3件以上と最終的なunbounded cardinality
- collection element binding、fan-out、aggregate、quantifier
- record creation、delete、upsert、relation replacement
- conditional assignment、branch、early return、loop、statement順序
- optional traversal、fallback、missing valueのdefault
- dynamic target path、computed field name
- distinct target bindingが同じentity typeを持つsource
- runtime alias identity comparison、merge／rejection outcome、明示alias contract
- literal、`-`、`*`、`/`、chained numeric expression
- multiple Precondition、AND／OR／NOT
- Derived Value、relation-reading Invariant
- Occurrence、Effect

これらを不要と判断したのではない。type-disjointなfixed target集合で同時評価とatomic Factを実測してから、runtime aliasを
観測できるapplication、collection、record creationへ進むための境界である。

## Review判定基準

- assignmentが逐次statementではなく、全RHSのpre-state評価と1 atomic post-stateになっているか。
- source declaration順を逆転してもsemantic outputが変わらないか。
- fixtureがsecond assignmentの欠落、誤binding、post-write rereadを異なる値で区別できるか。
- fixtureが逆順の逐次実装を値で区別できない限界を明記し、Review Requirementが全RHSのwrite前materializationを確認するか。
- same bindingのmultiple fieldsを許しつつ、same-type distinct bindingを`F2816`で拒否するか。
- duplicate targetをsource orderで解決せずcompile errorにしているか。
- runtimeでしかenforceできないalias outcome／Factをこのsliceへ捏造していないか。
- target／value／Precondition relation ownerが全Changeを通じて一意で、Fact handleが宣言順に依存しないか。
- accepted Factがassignment別に分裂せず、全fieldとimplicit transitionを同じatomic expectationへ含めるか。
- complete candidateに対するInvariant rejectionが全subject unchangedを要求するか。
- target-unavailable IDがrelation別で、value-unavailable／Precondition-only availabilityと重複しないか。
- Resolved Intent／Fact validatorがcardinality、order、target、expression、pair completenessを独立に再導出するか。
- Review Requirementが2件目のtarget/readとpartial failure pathを人間へ提示するか。
- collection、record creation、conditional control flowを先回りで発明していないか。
