# Numeric Addition in Changes Proposal

Status: compiler and repository E2E slice implemented; not part of normative v0

このproposalは、`changes`の右辺へ二項`+`を1個だけ追加し、複数のfield valueから計算した値を
atomic post-stateへ保存できるようにする最小sliceを定める。

```forma
changes {
    stock.reserved = stock.reserved + plan.approvedReserved
}
```

最終対象は`Order.approve`の在庫引当だが、このsliceではcollection、複数assignment、record creationを
同時に導入しない。既に実装したChangesのaction invocation、consistent pre-state、relation binding、
Invariant検査、all-or-nothing commitを再利用し、**exact numeric additionと複数operand bindingだけ**を検証する。

## 決定の要約

- `+`はChanges右辺でだけ受理し、field reference 2個の間に正確に1個だけ書ける。
- operandは既存relation-value sliceと同じく、requiredなself scalarまたはrequiredなto-one relation 1 hop先の
  required scalarに限る。
- 両operandとassignment targetは、同じInt-basedまたはDecimal-basedの**同一nominal type**でなければならない。
- named numeric typeは`Int`または`Decimal`を直接baseにする宣言だけを受理する。named type chainは、
  継承constraintの合成が実装されるまで`F2809`で拒否する。
- Intは数学的な符号付き整数、Decimalは有限桁の正確な10進数として加算する。wrap、binary floating-point rounding、
  暗黙のsaturationを許さない。
- operandはすべて同じconsistent pre-stateから読み、左から右という実行順序を意味にしない。
- mutation targetとoperandが同じrelationを辿る場合は、同じruntime identityを共有する。
- Acceptance Factは単一`valueSubject/valueField`をやめ、canonical Expression treeと各field-reference leafの
  runtime subject bindingを持つ。
- distinctなvalue relationが複数あっても、relationごとに一意な`value-unavailable` Factを導出する。
- repositoryの数値表現が結果を保持できない場合は、wrapせず`failure`として無部分commitにする。その表現境界は
  target-neutralなfixtureから一意に作れないため、machine FactではなくReview Requirementでも確認する。
- `-`、`*`、`/`、numeric literal、parentheses、chained addition、optional traversal、multiple assignmentは含めない。

## なぜAction Preconditionより先か

Action Preconditionは比較、等価、boolean composition、拒否理由、source-stateとの優先順位を同時に必要とする。
一方、Changesには次が既にある。

- action executionという評価時点
- `pre-state`というread model
- target/value relationのruntime binding
- Invariantを含むpost-state validation
- accepted／invalid／failureとsurface feedback
- implicit transitionと明示Changesのatomic boundary

したがって二項treeと複数operand bindingをChangesで先に検証すれば、Preconditionではpredicate固有の意味だけを
追加できる。これはExpressionをChanges専用にする判断ではなく、共有`IRExpression`のconsumerを一段ずつ増やす順序である。

## Repository E2E fixture

既存`StockReservation.commit`を次へ進める。

```forma
type Quantity = Int min 0

entity StockItem {
    onHand   Quantity required
    reserved Quantity required

    invariant stockAvailable: reserved <= onHand
}

entity StockReservation {
    stock             StockItem required
    plan              ReservationPlan required
    requestedReserved Quantity required

    state status Pending | Committed initial Pending
}

entity ReservationPlan {
    approvedReserved Quantity required
}

action StockReservation.commit: Pending -> Committed confirm allow staff {
    changes {
        stock.reserved = stock.reserved + plan.approvedReserved
    }
}
```

fixtureは次のように値を分ける。

```text
stock.reserved                 2
plan.approvedReserved          6
reservation.requestedReserved 3  // self fieldへの誤配線を検出するdecoy
accepted result                8
```

これにより少なくとも次の誤実装を区別できる。

| 誤実装 | 保存値 |
| --- | ---: |
| 正しい加算 | 8 |
| 左operandを捨てたabsolute assignment | 6 |
| distinct relationでなくself decoyを加算 | 5 |
| operatorを実装せず左operandを保存 | 2 |

既存のabsolute relation-value actionを別に残すのではなく、このaction自体を次のsliceへ進める。Resolved Intentと
Acceptance Factsのversioned diffが、absolute assignmentからadditionへの意味変更を明示する。

## Source syntaxとgrammar

lexerへ`tokenPlus`を追加する。`+`はstring concatenationやunary signとして扱わない。

最初のChanges value grammarは次で閉じる。

```ebnf
change_assignment = field_path, "=", change_value ;
change_value      = field_path, [ "+", field_path ] ;
field_path        = name, { ".", name } ;
```

受理する。

```forma
reserved = requestedReserved
stock.reserved = stock.reserved + requestedReserved
stock.reserved = stock.reserved + plan.approvedReserved
```

受理しない。

```forma
stock.reserved = 1
stock.reserved = stock.reserved + 1
stock.reserved = stock.reserved + plan.approvedReserved + requestedReserved
stock.reserved = (stock.reserved + plan.approvedReserved)
stock.reserved = stock.reserved - plan.approvedReserved
stock.reserved = stock.reserved * plan.approvedReserved
```

二項`-`を同時に入れないため、現lexerがsigned numberを1 tokenにする規則は今回変更しない。`-` operatorと
signed literalを分けるmigrationは[`expression-proposal.md`](expression-proposal.md)の後続項目に残す。

## 名前解決と型規則

各operandはrelation-value sliceのfield reference規則を独立に満たす。

1. rootはaction entityである。
2. pathはself field、またはrequired to-one relation 1 hopとterminal fieldである。
3. terminalはrequiredなstored scalar fieldである。
4. optional relation、optional terminal、to-many、state、relation terminal、2 hop以上を拒否する。

`left + right`を受理する型条件は次のすべてである。

1. `left`と`right`が同じnominal typeである。
2. そのeffective baseが`Int`または`Decimal`である。
3. assignment targetも同じnominal typeである。
4. targetは`unique`でない。numeric resultのunique collision outcomeはこのsliceで発明しない。
5. named typeならsource declarationのdirect baseがbuiltin `Int`または`Decimal`である。
6. nominal typeのeffective numeric boundsがadditionについて閉じている。

最後の2条件は、validなoperand 2個からtype constraint違反のresultを作らず、このsliceで新しい
result-constraint rejectionを同時に設計しないための境界である。effective intervalについて次を使う。

- 下限が無い、または下限が0以上なら、下側はadditionで閉じている。
- 上限が無い、または上限が0以下なら、上側はadditionで閉じている。
- 両方を満たすtypeだけを最初のsliceで受理する。

したがって、unboundedな`Int`／`Decimal`、`Quantity = Int min 0`、`Debt = Decimal max 0`は受理できる。
`Score = Int min 0 max 100`は`60 + 60`がtype外になるため今回はcompile errorにする。後続sliceで
result constraint rejection Factsを設計した後に制限を外す。

### Chained named typeをこのsliceで拒否する理由

規範v0はnamed scalarのconstraintをtransitiveに合成し、Resolved Intentへeffective constraintを記録すると定める。
しかしcurrent compilerはimmediate declarationのconstraintだけを`IRType.Constraints`へ載せ、`IRType.Base`はbuiltinへ
平坦化する。たとえば次の`Positive`から`max 100`を復元できない。

```forma
type Bounded  = Int max 100
type Positive = Bounded min 0
```

```text
current IRType: name=Positive, base=Int, constraints=[min 0]
declared effective domain: min 0, max 100
```

この状態で「上限なし」と解釈すると、`Positive + Positive`をaddition-closedだと誤って受理する。今回のsliceは
Milestone 1のinherited constraint合成まで便乗して実装しない。代わりに次をschemaへ残す。

```text
IRType.declaredBase             source declarationのimmediate base name
IRType.effectiveNumericBounds  direct builtin numeric typeについて判定に使ったmin/max
```

`Base`は既存どおりflatten済みbuiltin baseを保持し、`declaredBase`を別fieldにする。named typeをbaseにするtypeは、
flatten後の`Base`が`Int`／`Decimal`でもnumeric `+`では`F2809`になる。direct builtin typeでは宣言自身のconstraintが
effective setの全体なので、checkerはそこからboundsを合成し`effectiveNumericBounds`へ記録する。Resolved Intent validatorは
`declaredBase`がbuiltinであることを確認し、`Constraints`からboundsを独立に再計算して記録値とfull compareしてからclosureを判定する。

builtin `Int`／`Decimal`をfield typeとして直接使う場合はIRType nodeが無いため、unbounded domainとして扱う。
chained typeのeffective constraint materializationは引き続きMilestone 1の未実装項目であり、この限定によって
「継承constraintを落として静かに受理する」ことだけを先に閉じる。

型規則は次になる。

```text
T + T -> T
```

ここで`T`は上記条件を満たす同一nominal numeric typeである。`Quantity + Int`、`Quantity + Price`、
`Int + Decimal`、`String + String`はすべてcompile errorであり、base typeへの暗黙変換やstring concatenationを行わない。

diagnosticは既存Changes familyへ閉じる。

| code | 意味 |
| --- | --- |
| `F2805` | field operandのpath、requiredness、scalar性が不正 |
| `F2806` | expression resultとtargetのnominal typeが不一致 |
| `F2807` | Changes valueのoperator数またはtree shapeがこのslice外 |
| `F2808` | `+`のoperandが同一nominal numeric typeでない |
| `F2809` | targetがunique、named typeのbaseがbuiltin直接でない、またはeffective constraintがadditionで閉じていない |

## exact numeric semanticsとrepresentation failure

Formaの既存value semanticsに従い、Int additionは数学的に正確、Decimal additionは10進で正確である。

```text
evaluate(add(left, right)) = exact(left) + exact(right)
```

target repositoryがbounded integerや固定precision decimalを使うこと自体は禁じない。ただし次を要求する。

- wrap、silent saturation、binary floating-point roundingを成功としてcommitしない。
- operandは保存済みの有効値として読み、resultを表現できるかcommit前に検査する。
- 表現できなければ`failure`としてaction全体を拒否し、state、target、他のsubjectを一切変更しない。
- domain上のInvariant違反は既存どおり`invalid`であり、repository representation failureと混同しない。

repository固有の最大値やprecisionはForma sourceから一意に導出できない。そのため全targetに共通する
unrepresentable setupをAcceptance Factへ捏造せず、`exact-numeric-expression-enforcement` Review Requirementを
actionごとに導出する。E2E targetでは、そのrepositoryが採った境界値を使う補助testをrequirement evidenceにする。

## 評価時点とatomicity

action invocationが認可・confirmation・source state検査を通った後、Changesは次の意味順序を持つ。

1. mutation target relationをpre-stateから解決する。
2. operandが使うdistinct value relationを同じconsistent pre-stateから解決する。
3. 全field-reference leafの値を同じsnapshotからcaptureする。
4. capture済み値だけからexact additionを評価する。
5. repository representationがresultを保持できることを確認する。
6. implicit state transitionとassignmentを含むcandidate post-stateを作る。
7. target entityのInvariantをcandidate post-stateで検査する。
8. すべて成立した場合だけ1回commitする。

source上のleft／rightはIR identityを決めるが、runtimeのread順やside effect順を意味しない。両operandのreadは
同じpre-stateであり、片方をcommitした後にもう片方を読み直してはならない。

## Runtime subject binding

二項Expressionは最大2個のvalue relationを持ち得る。単一`valueSubject`では足りないため、field-reference leafごとに
runtime subjectを明示する。

handle規則は次である。

| field reference | runtime handle |
| --- | --- |
| self field | `subject/action` |
| targetと同じrelationを辿るfield | `subject/target` |
| targetとは異なるrelation `plan` | `subject/value/plan` |

同じrelationを複数leafが使う場合は同じhandleを共有する。異なるrelationはrelation field名を末尾に持つため、
`left`／`right`というsource位置でruntime identityを二重化しない。

E2E fixtureでは次になる。

```text
value/left   stock.reserved          -> subject/target
value/right  plan.approvedReserved   -> subject/value/plan
```

## Resolved Intent

共有`IRExpression`の既存shapeを再利用し、Expression fieldは追加しない。

```json
{
  "id": "action/StockReservation/commit/change/stock/reserved/value",
  "kind": "binary-expression",
  "resultType": "Quantity",
  "operator": "add",
  "left": {
    "id": "action/StockReservation/commit/change/stock/reserved/value/left",
    "kind": "field-reference",
    "resultType": "Quantity",
    "binding": "self",
    "relationPath": ["entity/StockReservation/field/stock"],
    "field": "entity/StockItem/field/reserved"
  },
  "right": {
    "id": "action/StockReservation/commit/change/stock/reserved/value/right",
    "kind": "field-reference",
    "resultType": "Quantity",
    "binding": "self",
    "relationPath": ["entity/StockReservation/field/plan"],
    "field": "entity/ReservationPlan/field/approvedReserved"
  }
}
```

`operator`はsymbolでなくcanonical name `add`とする。expression rootは従来どおりchangeの`/value`、childは
`/left`と`/right`である。Source MapはrootをRHS全体、各leafをそのfield pathのfull spanへ対応付ける。

一方、type側にはsliceの拒否境界とclosure判定をvalidatorへ運ぶため、`IRType`を次で拡張する。

```json
{
  "id": "type/Quantity",
  "name": "Quantity",
  "kind": "scalar",
  "base": "Int",
  "declaredBase": "Int",
  "constraints": [
    {"id": "type/Quantity/constraint/min", "kind": "min", "value": "0"}
  ],
  "effectiveNumericBounds": {"min": "0"}
}
```

`effectiveNumericBounds`はdirect builtin numeric typeでだけcurrent compilerがmaterializeするclosed contractである。
validatorは単なるchecker自己申告として信用せず、同じIRTypeの`Constraints`から再計算して一致を要求する。
`declaredBase`がnamed typeならboundsの有無にかかわらずnumeric `+`を拒否する。

## Consumerごとのclosed validation

`IRExpression`はInvariantとChangesの共有型だが、consumerの許可範囲は共有しない。

Changes validatorはrootについて次の2 shapeだけを受理する。

1. 既存のcanonical field reference。
2. `kind=binary-expression`、`operator=add`、`resultType=T`、canonicalな`left`／`right` field reference。

各nodeはconsumerが組み立てたcanonical full structと比較し、unknown field、rootのField/RelationPath、leafの
Operator/Left/Rightなどをdefaultで拒否する。relation-value sliceで導入したclosed validationを維持する。

Invariant validatorは従来どおりself-only `less-than-or-equal`だけを受理する。Changesで`add`を許したことを理由に、
Invariant operandへbinary additionやrelation pathを持ち込んではならない。Changes validatorを無効化するmutationと、
Invariant validatorが`add`を受理するよう緩めるmutationの両方をnegative testで落とす。

`cloneIRExpression`とsemantic ID traversalは既にrecursiveだが、FactへExpression treeを複製した後のalias testを
binary tree全体でも固定する。

## Acceptance Facts

### Expression result expectation

現在の`FactFieldExpectation.valueSubject/valueField`は1 leafしか表せない。Acceptance Factsを次へ正規化する。

```text
FactFieldExpectation
  field
  stored = expression-result
  expression
    tree       canonical IRExpressionのdeep copy
    evaluation pre-state
    bindings[]
      node     field-reference expression ID
      subject  runtime subject handle
```

既存のfield-referenceだけのChanges Factも同じshapeへ移し、binary専用の第二形式を作らない。validatorは次を要求する。

- `tree`がResolved Intentのchange valueと完全一致する。
- field-reference leaf集合と`bindings[].node`集合が厳密に一致する。
- bindingのsubjectがsetup済みであり、leafのrelation pathとhandle規則が一致する。
- 同じrelation pathを使うleafは同じsubjectを参照する。
- target fieldはtreeの評価結果を保存し、どちらか一方のoperandを直接保存しない。

これによりoperatorを`add`から別の値へ変える、operandをselfの同名fieldへ差し替える、片方を捨てる、といった
Fact改竄を上位Generation Requestの再導出だけに依存せず拒否できる。

### Availability Facts

target relationの欠落は既存`target-unavailable`が所有する。value operandのrelationはdistinct relationごとに
1件の`value-unavailable` Factを作る。

```text
<subject>/changes/value-unavailable/via/<relation-field-name>/from/<source-state>
```

各Factのsetupは、対象relationだけを`value-unavailable`、targetと他のvalue relationを`resolved`にする。
これにより宣言順やleft/right順による優先順位を作らず、各欠落経路を独立に到達可能にする。

同じrelationを2 leafが使う場合は1件にdedupeする。target relationと同じなら`target-unavailable`だけを作り、
到達不能なvalue-unavailable Factを作らない。複数value relationが同時に欠落したruntime invocationは同じ
`value-unavailable` outcomeであり、どちらを先に読んだかを観測可能な意味にしない。

### Outcome table

| 条件 | outcome/reason | page feedback | commit |
| --- | --- | --- | ---: |
| 全binding解決、exact result、Invariant成立 | `accepted` | success navigation | 1 atomic outcome |
| source state不一致 | `rejected` / `source-state-mismatch` | `invalid` | 0 |
| target relation欠落 | `rejected` / `target-unavailable` | `failure` | 0 |
| distinct value relation欠落 | `rejected` / `value-unavailable` | `failure` | 0 |
| resultがInvariant違反 | `rejected` / `invariant-violated` | `invalid` | 0 |
| repository representationがexact resultを保持不能 | `rejected` / implementation failure | `failure` | 0 |
| confirmation decline | action dispatchなし | confirmation surface | 0 |

最後の行から1つ上のrepresentation failureはtarget固有なのでcanonical Fact familyを新設しないが、既存action surfaceの
`failure` vocabularyから外してはならない。

## Review Requirements

既存requirementを再帰Expressionへ追随させる。

- `atomic-changes-enforcement`: value rootだけでなく全operand node、field、relation、type constraintをsourceNodesへ含める。
- `cross-entity-value-read-authorization`: 全relation operandを列挙し、source fieldを提示するpage/viewとaccess sourceを含める。
- `cross-entity-write-authorization`: target側の既存規則を維持する。

加えてnumeric Changesを持つactionごとに次を1件導出する。

```text
kind: exact-numeric-expression-enforcement
subject: action/<Entity>/<action>
```

instructionは、Int/Decimalのrepresentation、加算処理、保存境界、failure pathを確認し、wrap、binary rounding、
silent saturationが成功としてcommitされず、保持不能なresultもstate/targetを部分変更しないことを要求する。

E2EのReview Requirementsは4件から5件になった。新requirementのsourceNodesはaction、change、value tree、
operand field/relation、そのnominal typeとeffective min/max constraintを含む。

## Projection

Outcome Projectionは`valueSubject.valueField`という単一値の表示をやめ、Expression treeをruntime handleで展開する。

```text
subject/target.reserved=add(subject/target.reserved,subject/value/plan.approvedReserved)
```

source orderはIRのleft/rightを維持するが、実行順序とは説明しない。Factsを埋め込むprojection schemaも変わるため、
Outcome Projection versionを上げる。flow/navigation/state projectionはversion文字列以外の意味差分を持たない。

## Schema versioning

compiler sliceで予定するversionは次である。

- Resolved Intent: `v0.10` → `v0.11`（Changes `add` semantics、`IRType.declaredBase`とdirect numeric typeのeffective bounds）
- Acceptance Facts: `v0alpha8` → `v0alpha9`（Expression resultとleaf binding、relation別availability ID）
- Source Map: schema shapeは維持し、`intentVersion`だけ`v0.11`へ更新
- Outcome Projection: `v0alpha3` → `v0alpha4`（Expression resultの表示とembedded expectation shape）
- Review Requirements: `v0alpha4` → `v0alpha5`（exact numeric requirementとrecursive sourceNodes）
- Generation Request: envelope schemaは維持し、component versionとdigestを更新

version bumpはmembership golden、両Generation Request、retry baseline、integrity artifactを同じrunで再検証する。

## 最初のcompiler slice

設計review後の実装範囲を次に固定する。

1. lexer/parserがChanges RHSのfield reference 2個と`+` 1個を受理する。
2. checkerが各operandのrequired 1-hop path、同一nominal numeric type、target typeを検査し、named type chainを
   `F2809`で拒否したうえでdirect builtin typeのeffective boundsとconstraint closureを検査する。
3. canonical `IRExpression(binary-expression/add)`とfull-span Source Mapを構築する。
4. `IRType.declaredBase`と`effectiveNumericBounds`を構築し、validatorがConstraintsからboundsを再計算して一致を要求する。
5. Changes／Invariant両validatorのclosed shapeとbinary deep-copy／semantic ID traversalを固定する。
6. Fact expectationをExpression tree＋leaf subject bindingsへ正規化する。
7. target／self／複数distinct value relationのhandle共有とrelation別value-unavailable Factsを実装する。
8. recursive atomic/value-read Review Requirementとexact numeric requirementを導出する。
9. Outcome Projectionをruntime-bound expressionとして表示する。
10. schema、golden、Generation Request、repair/integrity artifactを更新する。
11. order E2Eのcommitを`stock.reserved + plan.approvedReserved`へ変更し、278 Factsの既存surface coverageを維持する。

## E2Eとmutation判定

repository targetでは次を観測する。

- accepted後のreservedが8であり、2、5、6のいずれでもない。
- target、plan、両operandを同じlock／transaction内のpre-stateからcaptureする。
- snapshot後にbacking valueを変えるtest seamでもcapture済み値を使う。
- target欠落、plan欠落、Invariant違反、source state不一致、confirmation decline、access denyで無部分commitになる。
- repository numeric boundaryでwrapせずfailureになる。
- page subject Factが引き続きすべてHTTP testを持つ。

少なくとも次のmutationで対応testが落ちなければならない。

1. `+`を捨ててright operandをabsolute assignmentする。
2. right operandを`requestedReserved`へ誤配線する。
3. left operandを捨てる。
4. snapshot後にstockまたはplanを読み直す。
5. unchecked additionでrepository integerをwrapさせる。
6. distinct relation欠落をself fallbackにする。
7. value-unavailableまたはrepresentation failureを`invalid`として表示する。
8. Changes validatorを無効化する。
9. Invariant validatorへ`add`を許す。
10. chained typeの継承`max`を落としたままnumeric `+`を受理する。
11. IRのeffective maxを削除、またはConstraintsと異なる値へ変える。

Fact数はrelation別availabilityのdedupe規則により278件のまま、page subjectは243件のまま全件HTTP testへ対応した。
52 mapped testsから278/278を再測定し、Review Requirementsはexact numeric enforcementを加えて5件になった。

## このsliceへ入れないもの

- numeric literal
- 二項`-`、`*`、`/`
- unary operator
- parenthesesとoperator precedence
- chained addition
- string concatenation
- optional traversal、absence、fallback
- additionで閉じていないconstraintのresult rejection
- chained named scalarのinherited constraint合成
- unique target collision
- 2 hop以上のrelation traversal
- multiple assignment、runtime alias resolution
- collection binding、aggregate、record creation
- Action Precondition、Derived Value
- relationを読むInvariant
- Occurrence、Effect

## Review判定基準

- `+`を加えたことでExpression全体を一般式として誤って開いていないか。
- left/rightとtargetが同じnominal numeric typeへ決定的に解決されるか。
- type constraint closureの判定が到達不能なrejection Factを避けているか。
- chained typeの継承constraintを「boundsなし」と誤認して過小に受理せず、direct builtin base以外を`F2809`で拒否するか。
- direct builtin numeric typeで、checkerとvalidatorが同じeffective boundsを独立に再計算しているか。
- 全field-reference leafがFact上のruntime subjectへ過不足なくbindingされるか。
- 同じrelation identityをtarget／複数operandで二重にsetupしていないか。
- distinct value relationが2件でもFact ID衝突や宣言順依存がないか。
- target-unavailableとrelation別value-unavailableに重複・到達不能caseがないか。
- exact Int/Decimal semanticsとrepository representation failureを混同していないか。
- Invariant consumerがrelation pathや`add`をdefaultで拒否し続けるか。
- E2Eがabsolute assignment、self誤配線、wrap、post-snapshot rereadでもgreenにならないか。
- Action Precondition、multiple assignment、collection semanticsを先回りで発明していないか。
