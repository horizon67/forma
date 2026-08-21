# Action Precondition Proposal

Status: experimental vertical slice implemented and measured; not part of normative v0

このproposalは、明示domain actionを実行してよいかをruntime valueから判定するAction Preconditionの
最小sliceを定める。既存の`A -> B`はsource stateだけを、`allow`／page accessはprincipalだけを検査する。
在庫上限、承認額、残高、期限等のdomain predicateはどちらにも縮退させない。

```forma
action StockReservation.commit: Pending -> Committed confirm allow staff {
    precondition withinRequestLimit: stock.reserved + requestedReserved <= plan.requestCeiling

    changes {
        stock.reserved = stock.reserved + plan.approvedReserved
    }
}
```

最初のsliceは、既に実装したrequired field reference、required relation 1 hop、exact binary numeric `+`、`<=`を
再利用する。boolean composition、literal、collection、aggregate、clock、multiple preconditionは同時に導入しない。

## 決定の要約

- Action Preconditionは新しいtop-level primitiveではなく、明示domain actionが所有するnamed memberとする。
- source syntaxは`precondition <name>: <predicate>`とし、最初のsliceではactionごとに0件または1件を許す。
- predicateは`<=`をrootとし、required scalar field referenceを比較する。predicate全体でnumeric `+`は最大1個だけ
  使える。
- field referenceはaction entity自身、またはrequiredなto-one relation 1 hop先に限る。
- predicateはactionのconsistent pre-stateで評価し、Changesやimplicit state transitionのcandidate post-stateを読まない。
- surface access、confirmation、source stateを通過し、全runtime bindingを解決した後にpredicateを評価する。
- predicateがfalseなら`precondition-unsatisfied`として`invalid`を返し、state、Changes target、他のsubjectを一切変更しない。
- relationを解決できずpredicateを評価できない場合はfalseとみなさず、`value-unavailable`／`failure`として拒否する。
- source state不一致、Precondition不成立、Invariant違反を別outcomeとして維持する。
- Preconditionがtrueであることは既存のtransition／Changes accepted Factへ入力条件として載せ、falseのdomain／surface Factを
  別に導出する。内部predicateだけを検査する孤立unit testではpage Factを満たせない。
- predicateのrelation bindingはChanges target/valueとruntime identityを共有し、同じrelationを二重setupしない。
- action accessが実行認可を所有する既存規則を維持する。cross-entity readの利用／再公開は既存Review Requirementへ
  Precondition sourceを追加して人間へ提示する。
- concurrent operationがpredicateをfalseにできる場合でもcommitしないことを、actionごとの新しいReview Requirementで確認する。

## なぜsource stateとInvariantでは足りないか

3つのguardは評価対象と時点が異なる。

| guard | 問い | 評価時点 | false時のreason |
| --- | --- | --- | --- |
| transition source | action entityが許可されたsource stateか | invocation開始時 | `source-state-mismatch` |
| Action Precondition | 現在のdomain valueでactionを実行してよいか | consistent pre-state | `precondition-unsatisfied` |
| Invariant | candidate post-stateが常に守るべき条件を満たすか | commit直前のpost-state | `invariant-violated` |

Preconditionをsource stateへ押し込むと、値の組合せごとにstateを増やす必要がある。Invariantへ押し込むと、
「このactionだけに必要な条件」と「すべてのmutationで常に守る条件」を区別できない。UIのdisabled stateだけにすると、
stale request、別surface、直接repository callで回避できる。

したがってAction Preconditionは独立したstable identityとfailure semanticsを持つ。ただしactionの外へ独立primitiveとして
置かず、認可、confirmation、transition、Changesと同じinvocation ownerへ属する。

current Acceptance Factはsource不一致を`kind=transition-source-rejected`で区別する一方、`expected.reason`は空である。
Action Preconditionとの機械的な優先順位を曖昧にしないため、このsliceでは既存の全domain／surface source rejectionへ
`reason=source-state-mismatch`を追加する。新しいruntime outcomeではなく、既存kindが既に持つ意味のcanonical明示である。

## Repository E2E fixture

既存の`StockReservation.commit`へ、approved quantityに加えて、そのplanが申請を受け付ける上限を置く。
Preconditionは既存予約と申請量`requestedReserved`の合計を`requestCeiling`で判定し、通過後にplannerが決めた
`approvedReserved`を実際の引当量としてChangesへ使う。この2つは意図的に異なるdomain valueである。

```forma
type Quantity = Int min 0

entity StockItem {
    onHand   Quantity required
    reserved Quantity required

    invariant stockAvailable: reserved <= onHand
}

entity ReservationPlan {
    code               String required unique label
    approvedReserved   Quantity required
    requestCeiling     Quantity required
}

entity StockReservation {
    stock             StockItem required
    plan              ReservationPlan required
    requestedReserved Quantity required

    state status Pending | Committed initial Pending
}

action StockReservation.commit: Pending -> Committed confirm allow staff {
    precondition withinRequestLimit: stock.reserved + requestedReserved <= plan.requestCeiling

    changes {
        stock.reserved = stock.reserved + plan.approvedReserved
    }
}
```

accepted fixtureは既存numeric sliceの値を維持する。

```text
stock.reserved                2
reservation.requestedReserved 3
plan.approvedReserved         6
plan.requestCeiling           6
stock.onHand                 10

2 + 3 <= 6             true
candidate reserved      8
8 <= 10                true
```

PreconditionとInvariantを別々に壊せる値を必ず持つ。

| case | requestCeiling | onHand | Precondition | candidate Invariant | outcome |
| --- | ---: | ---: | --- | --- | --- |
| accepted | 6 | 10 | `2 + 3 <= 6` true | `8 <= 10` true | commit 8 |
| equality boundary | 5 | 10 | `2 + 3 <= 5` true | `8 <= 10` true | commit 8 |
| precondition false | 4 | 10 | `2 + 3 <= 4` false | `8 <= 10` true | `precondition-unsatisfied` |
| invariant false | 6 | 7 | `2 + 3 <= 6` true | `8 <= 7` false | `invariant-violated` |
| both false | 4 | 7 | `2 + 3 <= 4` false | `8 <= 7` false | `precondition-unsatisfied` |

predicateの左辺5とChanges candidate 8を分けるため、Preconditionを削除してpost-state Invariantだけを検査する実装も、
candidate reservedをrequest ceilingと比較してPrecondition相当にする実装もgreenにならない。`both false`はPreconditionを先にする
優先順位だけを観測するrepository testであり、新しいFactは増やさない。Invariant rejection Factは引き続き
Precondition=trueのsetupだけを持つ。`equality boundary`も`<=`を`<`へ取り違えないことを観測するrepository testであり、
独立したAcceptance Factは追加しない。

### exact predicateを観測するboundary case

Formaの`Int`は数学的な整数であり、predicateの中間値はrepositoryの保存表現へ書く値ではない。

```text
stock.reserved                MaxInt
reservation.requestedReserved 1
plan.approvedReserved         0
plan.requestCeiling           MaxInt
stock.onHand                  MaxInt

MaxInt + 1 <= MaxInt    false
```

targetがpredicateの`MaxInt + 1`をnative integerでunchecked加算して負数へwrapするとtrueになり得る。Changes側の
`MaxInt + 0`は保存可能なので、この誤りを後段のrepresentation failureで隠せない。正しい結果は
`precondition-unsatisfied`／`invalid`である。Changes resultを保存できない`failure`へ分類しても不適合であり、
predicateはarbitrary precision、checked comparison、または同値なoverflow-free変形でexactに評価する。

Changes representation failureは別caseで維持する。`stock.reserved=MaxInt`、`requestedReserved=0`、
`approvedReserved=1`、`requestCeiling=MaxInt`ならpredicateはtrueだがChanges resultは保存不能なので、
`failure`として全subjectを不変にする。

## 2つ目のapplicationで意味を照合する

email verificationは、evidenceがavailable、未失効、未consumeである場合だけ`Pending -> Active`を許し、falseなら
evidenceとmembershipを変更しない。現在はIdentity専用semanticとして表しているが、次のaxisは同じである。

```text
source state satisfied
+ runtime domain/evidence predicate satisfied
-> action outcomeをcommit

predicate unsatisfied
-> outcomeを拒否し、全subjectを不変にする
```

Identity operationを通常fieldとgeneric actionへ書き換えない。ここで照合するのは、action固有predicateのtrue/falseが
atomic outcomeをguardする意味である。在庫とverificationの双方で、UI表示やroleだけでは代替できない。

## Source syntaxとownership

Action bodyへnamed memberとして置く。

```forma
action StockReservation.commit: Pending -> Committed confirm allow staff {
    precondition withinRequestLimit: stock.reserved + requestedReserved <= plan.requestCeiling

    changes {
        stock.reserved = stock.reserved + plan.approvedReserved
    }
}
```

action bodyのmember順は意味を持たない。sourceで`changes`を先に書いても、Preconditionは必ずpre-stateでChangesより先に
評価される。bodyはstatement listではなく、action contractのnamed facet集合である。

### 構文候補の比較

| candidate | 判定 | 理由 |
| --- | --- | --- |
| `precondition name: predicate` | 採用candidate | stable identity、failure trace、action ownershipを同時に持てる |
| `require predicate` | 採らない | pageの`require authenticated/owner`と認可規則を共有しているように読める |
| `when predicate` | 採らない | conditional Changesやbranchを導入するkeywordに見え、失敗identityも無い |
| `if predicate { ... }` | 採らない | statement順、branch、早期returnをaction bodyへ持ち込む |
| top-level `precondition Action.name` | 採らない | invocation ownerとpredicate ownerを別宣言間で再構成する必要がある |

名前はaction内で一意とする。最初のsliceは0件または1件だけを受理する。将来複数を許す場合も宣言順でshort-circuitせず、
各named predicateを独立に観測できるAND集合とするが、そのFact cardinalityと複数failure表示は今回決めない。

action bodyはPreconditionだけ、Changesだけ、または両方を持てる。bodyを持つactionに必ずChangesを要求するcurrent checkerは
更新する。空bodyは意味を持たないためerrorとし、bodyの無い既存actionの意味は変えない。

## 最初のpredicate grammar

概念上のgrammarは次である。

```ebnf
action_member       = precondition_decl | changes_decl ;
precondition_decl   = "precondition", lower_name, ":", precondition_expr ;
precondition_expr   = additive_expr, "<=", additive_expr ;
additive_expr       = field_path, [ "+", field_path ] ;
field_path          = lower_name, [ ".", lower_name ] ;
```

最初のsliceではInvariant declarationと同じく、`:`とpredicateを同じlogical lineに置く。汎用のline continuationや
indentation semanticsをこの機能に便乗して導入しない。

checkerはpredicate tree全体で`+`が最大1個であることを要求する。したがって次を受理する。

```forma
precondition simple: requested <= approved
precondition leftAdd: stock.reserved + requestedReserved <= plan.requestCeiling
precondition rightAdd: minimum <= stock.reserved + requested
```

次は拒否する。

```forma
precondition chained: a + b + c <= limit
precondition twoAdds: a + b <= c + d
precondition equality: status == Ready
precondition boolean: a <= b and c <= d
precondition literal: amount <= 10
precondition collection: lines.count <= limit
```

right-side additionも同じ1-operator semanticsで説明できるため、fixtureのleft-side形だけへgrammarを非対称に固定しない。
一方、2つのadditionやboolean compositionは評価順ではなくても、追加のtype/error/Fact caseを必要とするため閉じる。

## Field pathと型規則

各field-reference leafは次をすべて満たす。

1. action entity自身、またはrequiredなto-one relationを1 hopだけ辿る。
2. terminalはrequiredなstored scalar fieldである。
3. optional relation、optional terminal、collection、state、relation terminal、readonly derived value、2 hop以上を拒否する。
4. `<=`の左右は同じnominal ordered scalar typeである。
5. additionを含まない比較では、Invariantと同じInt／Decimal／Date／DateTime-based typeを許す。
6. additionはInt／Decimal-basedの同一nominal typeだけを許し、numeric addition proposalのdirect builtin base、
   effective bounds、closure規則をそのまま適用する。

comparison内のaddition resultはrepositoryへ保存しないが、規則4により比較の左右は同じnominal typeであり、add nodeの
`resultType`もそのtypeになる。closureを外すと、たとえば`Score min 0 max 100`の`60 + 60`を`resultType=Score`の値120として
IRへ運ぶことになる。nominal typeより広いintermediate numeric typeはこのsliceで導入しないため、Changesと同じclosure規則を
適用する。保存しないことだけを理由に緩めず、widened intermediate typeを別sliceで決めた後に再検討する。

additionのcheckerをChanges専用実装から共有helperへ移しても、consumerごとのclosed shapeは共有しない。

- Invariant: self field `<=` self fieldだけ。relation pathと`add`を引き続き拒否する。
- Changes value: field reference、またはfield reference `+` field referenceだけ。comparisonを拒否する。
- Action Precondition: root `less-than-or-equal`、field/add child、relation 1 hop、predicate全体でadd最大1個だけ。

`IRExpression`へ新fieldは追加しないが、3 consumerのcanonical validatorを同じrunでmutation検査する。共有parser/helperを
理由に、InvariantやChangesの受理範囲を暗黙に広げない。

## 認可とdata disclosure

Preconditionを満たすか否かはactionの実行認可を変更しない。既存どおりdomain action自身の`allow`が実行認可を所有し、
page action referenceではsource page、action、destination pageのaccessを合成する。参照先entityを表示する別pageのroleを
追加条件として暗黙に継承しない。

access deniedではPreconditionを評価せず、predicateのtrue/falseやrelation欠落を開示しない。UIがbuttonをdisabled／hiddenに
することは許すが、それをauthoritative enforcementの代わりにしてはならない。またFormaは最初のsliceで、predicate falseの
理由文やoperand値を利用者へ開示することを要求しない。

relation先の値をaction roleが利用してよいかは実行認可とは別である。既存
`cross-entity-value-read-authorization` Review Requirementの導出対象をChanges RHSだけでなくPreconditionの全relation leafへ
広げる。同じactionで既にrequirementがある場合は1件へdedupeし、predicate、relation／terminal field、値を提示するsurface、
actionを提示するsurfaceとeffective accessをsourceNodesへ追加する。

## Authoritative evaluation order

surfaceでconfirmationをacceptして1回dispatchした後、authoritative action boundaryは次の順序を持つ。

1. principal/action accessを検査する。deniedならpredicateを評価しない。
2. action entityのsource stateを検査する。不一致なら`source-state-mismatch`で拒否する。
3. PreconditionとChangesが必要とする全required relation identityを同じconsistent pre-stateで解決する。
4. いずれかのbindingが解決不能なら`value-unavailable`または`target-unavailable`で拒否し、predicateをfalse扱いしない。
5. Preconditionの全field valueを同じsnapshotからcaptureし、exactに評価する。
6. falseなら`precondition-unsatisfied`で拒否し、stateと全related subjectを不変にする。
7. trueならChanges RHSを同じsnapshotのcapture済みbindingから評価し、implicit transitionを含むcandidate post-stateを作る。
8. changed field constraintと、変更対象entityのInvariantをcandidate post-stateで検査する。
9. すべて成立すれば1回でcommitする。

confirmation declineはdispatch自体が0回なので、このauthoritative順序へ入らない。success navigationは9のcommit後だけに行う。
Precondition rejectionでsuccess destinationへ移動してはならない。

step 3から9は、Preconditionが参照する値を別operationが変更できる場合も同じtransaction、lock、またはconflict-retry境界に
置く。predicateをlock外でtrueと判定してからstaleな結果でcommitする実装は不適合である。

全bindingをpredicateより先に解決するのは、relation resolution自体をChangesの副作用とみなさず、1 invocationの
consistent snapshotを確立する操作とするためである。これによりPrecondition falseとChanges-only relation欠落が同時にある
場合もavailability failureが先になり、action bodyのmember順や遅延読込の都合でreasonが変わらない。

### source stateとの優先順位

source state不一致ではrelationを読まない。たとえばCommitted recordのplanが同時に欠落していても、結果は
`source-state-mismatch`である。これは診断の好みではなく、無効なstateからpredicateを評価してdata存在を漏らさず、
既存transition Factの意味を維持する境界である。

### PreconditionとInvariantの優先順位

Preconditionはcandidateを作る前に評価する。falseと、仮に作ったcandidateのInvariant違反が同時に成立しても、結果は
`precondition-unsatisfied`である。Invariant rejection FactはPrecondition=trueのsetupだけを持つ。

## Runtime binding identityとavailability

PreconditionとChangesはaction invocationごとに1つのbinding planを共有する。field-reference leafのruntime handleは、
同じrelation IDならconsumerやtree位置にかかわらず同じになる。

| leaf | runtime handle |
| --- | --- |
| action entityのself field | `subject/action` |
| Changes mutation target relation | `subject/target` |
| target以外でChanges RHSにも現れるrelation | `subject/value/<relation-field>` |
| Preconditionだけに現れるrelation | `subject/precondition/<relation-field>` |

ownerの優先順位はtarget、Changes value、Precondition-onlyである。これはrelationを解決する実行順ではなく、同じidentityへ
複数handleと到達不能なFactを作らないためのcanonical namingである。

availability Factもrelation IDでdedupeする。

- Changes target relationなら既存`changes/target-unavailable`だけを作る。
- targetでなくChanges RHSにも現れるrelationなら既存`changes/value-unavailable/via/<relation>`だけを作る。
- Preconditionだけに現れるrelationなら`precondition/<name>/value-unavailable/via/<relation>`を作る。

最後のFactもreasonは`value-unavailable`、surface feedbackは`failure`である。required relationのruntime欠落をpredicate falseの
`invalid`へ変換しない。複数のdistinct relationが同時に欠落するruntime caseは同じfailure outcomeであり、source宣言順や
expression traversal順をobservable priorityにしない。Acceptance Factsでは各relationだけをunavailable、他をresolvedにした
独立caseを作る。

最初のrepository E2E fixtureで使う`stock`はChanges target、`plan`はChanges valueでもあるため、
`subject/precondition/<relation>`とPrecondition-only `value-unavailable` Factは生成されない。この3つ目のowner経路は
Changesを持たず、predicateだけがrequired relationを読む明示actionのcompiler fixtureとFact validatorで固定する。このfixtureは
Precondition-only handle、availability Fact、cross-entity read requirement、numeric `add` requirementを同時に検査する。
repository E2Eはtarget／Changes valueと共有する2経路だけを実測する。
新relationとapplication semanticsを足してFact数を増やすことは、このpredicate sliceへ混ぜない。

## Resolved Intent candidate

`IRAction`へnamed preconditionを追加する。

```text
IRAction
  id: action/StockReservation/commit
  sources: [Pending]
  destination: Committed
  preconditions:
    - id: action/StockReservation/commit/precondition/withinRequestLimit
      name: withinRequestLimit
      predicate:
        id: .../expression
        kind: binary-expression
        operator: less-than-or-equal
        resultType: Bool
        left:
          kind: binary-expression
          operator: add
          resultType: Quantity
          left:  field-reference stock.reserved
          right: field-reference requestedReserved
        right:
          kind: field-reference
          field: plan.requestCeiling
          resultType: Quantity
      evaluation: pre-state
  atomicity: all-or-nothing
  changes: ...
```

candidate Go shapeは次とする。

```go
type IRAction struct {
    // existing fields
    Preconditions []IRActionPrecondition `json:"preconditions,omitempty"`
}

type IRActionPrecondition struct {
    ID         SemanticID   `json:"id"`
    Name       string       `json:"name"`
    Predicate  IRExpression `json:"predicate"`
    Evaluation string       `json:"evaluation"`
}
```

`evaluation`は最初のsliceで常に`pre-state`だが、agentにaction bodyのsource順やpost-stateを推測させず、validatorがtamperを
拒否するため明示する。failure reasonやfeedbackはcompiler-ownedなFact規則から一意なのでIRへ重複格納しない。

Precondition IDはaction IDとsource nameから導出する。predicate変更は同じPrecondition IDのsemantic diff、renameはremove/addに
なる。expression node IDは既存どおりrootから`/left`、`/right`を再帰的に付ける。

## Source Mapとclosed validation

Source MapはPrecondition declaration、predicate root、全operator、全field-reference leaf、relation／terminal fieldをsourceへ
戻せるようにする。schema shapeは変えず、Resolved Intent versionだけ更新する。

`ValidateResolvedIntent`はsourceを再parseせず、次を独立に再導出・検査する。

- actionごとのPrecondition件数が0または1で、ID／nameがcanonicalである。
- `evaluation == pre-state`である。
- predicate rootが`less-than-or-equal`、resultTypeが`Bool`である。
- childがconsumerのclosed field/add treeで、predicate全体のaddが最大1個である。
- relation path、required scalar terminal、nominal type、ordered/numeric baseがResolved Intentのentity/typeから一致する。
- addのdirect builtin baseとeffective numeric boundsを`IRType.Constraints`から再計算し、宣言値を信頼しない。
- expression ID、field ID、relation IDがcanonicalで、unknown／duplicate nodeがない。
- Invariant validatorはrelation pathとaddを、Changes validatorはcomparisonを引き続き拒否する。

negative testは少なくともoperator、resultType、evaluation、field、relationPath、optional field、型、2個目のadd、duplicate
Preconditionを個別に改竄する。Precondition validatorをreturn nilにするmutationだけでなく、Invariantへrelation/addを許すmutation、
Changesへcomparisonを許すmutationも落ちなければならない。

## Acceptance Facts

### Predicate input

`FactInput`へaction-ownedなpredicate評価を追加する。

```text
FactInput.preconditions[]
  precondition       semantic ID
  subject            subject/action
  expression         complete canonical IRExpression tree
  bindings[]
    node              field-reference expression ID
    subject           runtime subject handle
  evaluation         pre-state
  result              true | false
```

既存`FactPredicateInput`を意味の異なるanonymous fieldとして流用せず、named action ownerとruntime bindingを持つ
`FactActionPreconditionInput`を使う。binding node集合はexpressionのfield-reference leaf集合と厳密一致し、同じrelationは
Changes expectationと同じsubject handleを使う。source stateとrelation conditionはFact setupが既に一意に表すため、
それを再掲する`otherRequirements` fieldは新設しない。

### trueは既存accepted Factの前提にする

Precondition単体のpure unit evaluation Factは作らない。実applicationでaction outcomeをguardすることが意味だからである。
各source stateについて、既存のdomain／surface `transition-accepted`と、Changesを持つ場合の`changes-accepted`へ
Precondition result=trueを含める。

```text
setup:
  source state Pending
  stock, plan relations resolved
input:
  action dispatches = 1
  precondition withinRequestLimit result = true
expected:
  transition and Changes accepted
```

これにより、predicate helperのunit testだけをcoverageへ割り当て、実action handlerがpredicateを呼ばない実装はFactを満たさない。
actionがChangesを持たない場合も、`transition-accepted`がtrue predicateを観測する。

### falseは独立したdomain／surface Factにする

各source state、各Precondition、各action referenceについて次を導出する。最初のsliceは1 Preconditionなのでcardinalityは
source数に対して線形である。

```text
fact/action/StockReservation/commit/precondition/withinRequestLimit/unsatisfied/from/Pending

setup:
  source state Pending
  all Precondition and Changes relations resolved
input:
  action dispatches = 1
  precondition withinRequestLimit result = false
expected:
  outcome = rejected
  reason = precondition-unsatisfied
  atomicity = no-changes-committed
  appliedMutations = 0
  state and every resolved subject = unchanged
  enforcement = authoritative
  page counterpart feedback = [invalid]
```

kindはdomainで`precondition-unsatisfied`、page action counterpartで`action-precondition-unsatisfied`とする。surface Factは
実action invocationを通る。`outcome=rejected`はsuccess navigationが発生しないことを含み、別の架空のstay destinationは
作らない。具体的なHTTP status、exception type、文言は規定しない。

### unavailableはpredicate resultを持たない

relationが欠落して式を評価できないFactでは、`preconditions[].result=false`を書かない。setupのrelation conditionと
`reason=value-unavailable`だけを持ち、predicateが未評価であることを表す。falseとunavailableを同じinvalid caseへ潰すと、
dangling relationやconcurrent deleteをbusiness rejectionとして隠すためである。

### source rejectionにはpredicateを載せない

既存`transition-source-rejected` FactはPrecondition inputとrelation setupを持たない。source state gateが先に失敗し、
predicateを評価しないことを固定する。accepted Factだけへtrueを足すため、現在の「source stateだけで必ずaccepted」という
暗黙の意味は残らない。

### Fact completeness

validatorはResolved Intentから次を再導出する。

- accepted transition／Changes Factが全Preconditionをresult=trueで過不足なく持つ。
- Preconditionごとにfalseのdomain／surface Factが存在する。
- false Factが該当predicate ID、complete tree、全leaf binding、pre-state、no-commitを持つ。
- source mismatch、target/value unavailable Factが到達不能なpredicate resultを持たない。
- Precondition-only relationのavailability Factが欠落せず、target／Changes value relationと重複しない。
- operator、leaf binding、result、reason、feedbackの捏造を拒否する。
- 既存transition source rejectionが`reason=source-state-mismatch`を持ち、Precondition inputを持たない。

上位Generation Request全体のcanonical比較だけに依存せず、exported `ValidateAcceptanceFacts`のaction contract検査でも
欠落・捏造・result反転を落とす。

## Outcome table

| 条件 | outcome/reason | page feedback | predicate evaluation | commit |
| --- | --- | --- | --- | ---: |
| access denied | existing access-denied | access outcome | none | 0 |
| confirmation declined | cancelled / dispatch none | confirmation surface | none | 0 |
| source state不一致 | rejected / source-state-mismatch | `invalid` | none | 0 |
| required target relation欠落 | rejected / target-unavailable | `failure` | none | 0 |
| required value relation欠落 | rejected / value-unavailable | `failure` | none | 0 |
| Precondition false | rejected / precondition-unsatisfied | `invalid` | exact false | 0 |
| Precondition true、post-state Invariant違反 | rejected / invariant-violated | `invalid` | exact true | 0 |
| Precondition true、repositoryがChanges resultを保存不能 | rejected / implementation failure | `failure` | exact true | 0 |
| 全条件成立 | accepted | success navigation | exact true | 1 atomic outcome |

`IRActionRef.interactionStates`は明示domain actionについて既に`[invalid, failure]`であり、schema追加は不要である。
list/detailのview自身の`observable-feedback`へaction feedbackを混ぜない既存ownershipも維持する。

## Human Review Requirements

### concurrent-action-precondition-enforcement

Preconditionを持つactionごとに次を1件導出する。

```text
review/action/StockReservation/commit/concurrent-action-precondition-enforcement
```

人間は次を確認する。

- predicate fieldとrelation identityがauthoritative boundary内の同じconsistent pre-stateから読まれる。
- source state検査、predicate評価、Changes／transition commitが同じtransaction、lock、またはconflict-retry境界にある。
- concurrent operationがpredicateをfalseにした後、staleなtrue判定でcommitしない。
- enforcementがUIのdisabled stateやsingle-threaded unit testだけに限定されない。

既存`atomic-changes-enforcement`とは統合しない。Changesの一部commit防止と、predicate readからcommitまでのstaleness防止は
別のreview questionであり、Preconditionだけを持つactionにも後者は必要だからである。

### 既存requirementの拡張

- `cross-entity-value-read-authorization`: Preconditionのrelation leafと提示surface/accessをsourceNodesへ含める。
- `exact-numeric-expression-enforcement`: Changesの保存結果だけでなく、Preconditionの中間演算と比較結果がwrap、rounding、
  saturationで反転しないことを含める。pure predicateの数学的resultはrepository保存表現へ収まらないだけでfailureにしない。
- `atomic-changes-enforcement`: actionがPreconditionを持つ場合、そのpredicate bindingsとevaluation nodeをsourceNodesへ含める。

`exact-numeric-expression-enforcement`の導出条件は、actionのChanges valueまたはいずれかのPrecondition predicateに`add`が
あることとする。Changesを持たないPrecondition-only actionでも必ず導出し、sourceNodesはpredicate root、全operand leaf、
operand typeとconstraintを含む。同じactionのChangesとPreconditionの両方に`add`があってもrequirementは1件へdedupeする。

E2E fixtureでは既存5件に`concurrent-action-precondition-enforcement`が1件加わり、Review Requirementsは6件になる。

## Outcome Projection

Outcome ProjectionはPrecondition falseをaction group内の独立rowとして表示し、accepted／Invariant rejected rowにも
predicate resultを表示する。少なくとも次を人間が区別できるようにする。

```text
source state mismatch        predicate not evaluated
relation unavailable         predicate not evaluated
withinRequestLimit false      invalid / no commit
withinRequestLimit true       continue to post-state validation
stockAvailable false         invariant rejection / no commit
```

projectionはsourceにないoutcomeを追加せず、Acceptance Factsのpredicate treeとruntime bindingsをread-onlyに表示する。

## Schema versioning

compiler sliceで予定するversionは次である。

- Resolved Intent: `v0.11` → `v0.12`（`IRAction.preconditions`）
- Acceptance Facts: `v0alpha9` → `v0alpha10`（Precondition input、unsatisfied／availability family、source rejection reason）
- Source Map: schema shapeは維持し、`intentVersion`だけ`v0.12`へ更新
- Outcome Projection: `v0alpha4` → `v0alpha5`（predicate evaluationとPrecondition outcome row）
- Review Requirements: `v0alpha5` → `v0alpha6`（concurrent Precondition enforcementと既存source拡張）
- Generation Request: envelope schemaは維持し、canonical componentとrequest digestを更新

Flow、Navigation、Domain State Projectionはown schema shapeを変えず、埋め込む`intentVersion`だけ更新する。
version bumpはmembership golden、両Generation Request、retry baseline、integrity artifactを同じrunで再検証する。

## 実装したcompiler slice

設計review後の実装範囲を次に固定する。

1. Parser／ASTがaction bodyのnamed `precondition`を0件または1件受理し、Precondition-only bodyも許し、member宣言順を意味にしない。
2. checkerがroot `<=`、required self／1-hop relation scalar、predicate全体でadd最大1個、nominal typeを検査する。
3. existing numeric addition helperをconsumer-neutralに再利用しつつ、Invariant／Changes／Precondition validatorのclosed shapeを維持する。
4. `IRActionPrecondition`、canonical expression ID、Source Map coverage、deep-copy／semantic ID traversalを実装する。
5. Resolved Intent validatorがpredicate shape、type、binding、evaluationをIRから独立に再導出する。
6. action-wide binding planをtarget／Changes value／Precondition-only relationでdedupeする。Changesを持たずpredicateだけが
   required relationとnumeric `add`を使うcompiler fixtureで、Precondition-only handle、availability Fact、Review Requirementを固定する。
7. accepted transition／Changes Factへtrue predicateを追加し、falseのdomain／surface FactとPrecondition-only availability Factを導出し、
   既存source rejectionへ`reason=source-state-mismatch`を加える。
8. Fact validatorがpair completeness、tree／leaf binding、priority、feedback、no-commitを再導出する。
9. concurrent Precondition Review Requirementを追加し、cross-value／exact numeric／atomic requirementの導出条件とsourceNodesを広げる。
10. Outcome Projection、schema、golden、Generation Request、repair／integrity artifactを更新する。
11. order E2Eへ`requestCeiling`とPreconditionを追加し、current 278 Factsにfalseのdomain／surface 2 Factsを加えた
    280 Facts、Review Requirements 6件を再測定する。
12. page subject Factが引き続き全件HTTP testを持つ規則とaccess Factの分散上限を維持する。

## 実装結果（2026-08-21）

上のsliceをParser、checker、Resolved Intent `v0.12`、Acceptance Facts `v0alpha10`、Outcome Projection `v0alpha5`、
Review Requirements `v0alpha6`へ実装した。Source Mapはshapeを変えず`v0.6`を維持する。compiler testでは
Precondition-only relation、binding ownerのdedupe、Fact pair completeness、closed IR validation、`F2810`–`F2814`を固定した。

[`order-invariant-agent-e2e`](../experiments/order-invariant-agent-e2e/)ではReservationPlanへpage/viewを追加せず、
52 mapped repository testsから280/280 Factsを再測定した。page subject 244件はすべてHTTP testを持つ。
accepted、`<=`等号境界、Precondition-only false、Invariant-only false、両方false、predicate overflow、Changes representation failure、
relation欠落、source mismatch、snapshot後のbacking value変更、concurrent commitを独立に観測する。Review Requirementsは
`concurrent-action-precondition-enforcement`を加えた6件で、機械検査へ吸収せず人間確認待ちである。

## E2Eとmutation判定

repository targetでは少なくとも次を観測する。

- accepted caseは2+3<=6をpre-stateで評価し、別のChanges expression 2+6からreserved=8とCommittedを同時commitする。
- requestCeiling=5の等号境界もacceptedになり、`<`への取り違えを検出する。
- requestCeiling=4、onHand=10はPreconditionだけがfalseになり、typed domain rejection、HTTP `invalid`、全subject不変になる。
- requestCeiling=6、onHand=7はPrecondition=true、Invariant=falseになり、別のtyped rejectionと全subject不変になる。
- requestCeiling=4、onHand=7は両方falseでもPrecondition rejectionになり、優先順位をrepository testで観測する。
- predicateの`MaxInt + 1 <= MaxInt`はwrapせずfalse／`invalid`になり、後段は保存可能な値にして誤りを隠さない。
- 別caseではpredicateをtrueに保ったままChanges resultを保存不能にし、既存representation `failure`を維持する。
- plan／stock欠落はpredicate falseでなく既存`value-unavailable`／`target-unavailable`と`failure`になる。
- source state不一致ではpredicate relationを読まず、既存`source-state-mismatch`を維持する。
- snapshot後にbacking stock／reservation／plan／requestCeilingを変えてもcapture済みpre-stateだけでpredicateとChangesを評価する。
- concurrent commitはonHandに余裕があってもrequest ceilingを越えず、成功1件・Precondition rejection 1件になる。
- page action rejectionは実HTTP surfaceを通り、success redirectを行わない。

少なくとも次のmutationで対応testが落ちなければならない。

1. Precondition評価を削除して常にChangesへ進む。
2. predicateをcandidate post-stateのreservedへ置換し、accepted caseを誤って拒否する。
3. falseをInvariant rejectionまたはgeneric failureへ分類する。
4. PreconditionをUIだけで検査し、store／domain boundaryでは無視する。
5. predicate判定後にlockを外し、concurrent update後もstale trueでcommitする。
6. relation欠落をpredicate false／`invalid`へ変換する。
7. source state不一致より先にpredicateを評価する。
8. predicateの`MaxInt + 1`をnative integerでwrapし、predicate resultを反転する。
9. predicateのstock／requestedReserved／plan bindingを別leafまたは別runtime entityへ誤配線する。
10. accepted FactからPreconditionを落とす、false Factを落とす、またはresultを反転する。
11. Precondition validatorを無効化する。
12. Invariantへrelation/addを許す、またはChangesへcomparisonを許す。

## 必須diagnostic

実装時に少なくとも次をcompile errorへする。codeは既存`F2801`–`F2809`の後ろで衝突しない範囲を使う。

- actionに2件以上のPreconditionがある。
- Precondition名が同じaction内で重複する。
- rootが`<=`でない、またはpredicate全体に2個以上の`+`がある。
- unknown field、state、relation terminal、optional field／relation、collection、2 hop以上を参照する。
- comparison operandのnominal typeが異なる、またはordered scalarでない。
- addition operandがnumeric addition sliceのtype／direct base／closure規則を満たさない。
- parserを経ずに渡されたIRのID、evaluation、tree、binding、typeがcanonicalでない。

parser errorのhintはaction bodyが`precondition`と`changes`を支持することを表示し、既存の
「first action body slice supports only changes」というstale messageを残さない。

## このsliceへ入れないもの

- multiple PreconditionとAND／OR／NOT、short-circuit、failure message selection
- equality、不等号の追加、literal、parentheses、unary operator
- optional traversal、collection、aggregate、count、quantifier
- multiple assignment、collection fan-out、record creation
- Derived Value、relationを読むInvariant
- comparison専用のwidened intermediate numeric type
- time／clock、external lookup、nondeterministic predicate
- Preconditionに応じたconditional Changesやbranch
- button visibility／disabled stateのsource syntax
- Occurrence、Effect

これらをerrorにすることは不要という判断ではなく、Action Precondition固有の評価時点、failure semantics、surface observation、
concurrent enforcementを他の未知から分離する境界である。

## Review判定基準

- source state、Precondition、Invariantのowner、評価時点、failure reasonが混ざっていないか。
- false predicateがaction transition／Changesを実際に止め、孤立predicate unit testだけでFactを満たせないか。
- PreconditionとInvariantを独立にfalseにできるfixtureになっているか。
- access denied、confirmation decline、source mismatchでpredicateを不要に評価・開示していないか。
- relation unavailableをfalseとみなさず、target／Changes valueをE2Eで、Precondition-onlyをcompiler testで固定し、
  3系統のFactが重複しないか。
- predicateとChangesで同じrelation identity、snapshot、numeric semanticsを共有しているか。
- exact mathematical predicateをrepository storage representation failureへ誤分類していないか。
- concurrent operationがpredicateをfalseにした後のstale commitをReview Requirementが見落としていないか。
- cross-entity read authorization requirementがPrecondition leafと提示surfaceを含むか。
- Resolved Intent／Fact validatorがtree、type、binding、evaluation、fact pairを独立に再導出するか。
- Invariant／Changes consumerがPrecondition導入によって暗黙に広がっていないか。
- multiple Precondition、collection、conditional Changesを先回りでstatement languageとして発明していないか。
