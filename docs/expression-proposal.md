# Minimal Expression Layer Proposal

Status: exploratory proposal — first self-only `<=` compiler slice implemented outside normative v0

この文書は、Forma sourceに書かれた計算や条件を、未検査のpromptではなく、compilerがparse・名前解決・
型検査できるapplication semanticsとして扱うための最小expression layerを提案する。

規範仕様は[`v0-primitives.md`](v0-primitives.md)である。本書はまだlanguage decisionではなく、v0の
10 primitivesも変更しない。reference compilerは検証用の最初の縦切りとして、名前付きInvariant、
selfのfield参照、`<=`、名前解決・型検査、Resolved Expression IR、Source Mapまでを実装している。
numeric literal、他のoperator、式評価器、`=`によるDerived Value、`require`は未実装である。

本書は[`order-approval-proposal.md`](order-approval-proposal.md)で実測した不足を受けている。Expressionを
先に一般化するのではなく、注文・在庫で必要になった具体例から、EffectやChangesより前に共有すべき
最小の意味モデルを抽出する。

## 目的

次のような直感的な記述を、そのままFormaの正式な意味として扱えるようにする。

```forma
invariant stockAvailable: reserved <= onHand
```

AIは`reserved <= onHand`を理解してtarget codeへ変換できる。しかし、式をopaqueなpromptとして渡すだけでは、
Forma compilerはfieldのtypo、型の不一致、依存するdeclaration、期待する拒否条件を判断できない。
そこで責務を次のように分ける。

```text
Forma source
  ↓ Go front-end: parse、名前解決、型検査
Resolved Expression IR
  ├─ reference evaluator: fixtureに対する期待値を決定
  └─ AI generator: targetに適した実装を選択
       ↓
     Generated Artifact
       ↓ deterministic conformance: evaluatorの期待値と照合
     Accepted Artifact
```

Go front-endは「何を意味するか」を確定し、AI generatorは「どう実装するか」を決める。reference
toolchainはfront-endに加えて、Resolved Expression IRを実行する決定的な式評価器を持つ。式評価器は
target artifactを実装するものではなく、Conformanceのoracleを生成する。

## 実例から必要になった式

| 利用場所 | 式 | 意味 |
| --- | --- | --- |
| entity invariant | `reserved <= onHand` | 予約数が在庫数を超えない |
| derived value | `quantity * product.price` | 明細金額を計算する |
| action precondition | `lines.count > 0` | 空の注文は提出できない |
| effect binding | `customer.email` | 通知先をrelationから得る |
| occurrence predicate候補 | `onHand < threshold` | 在庫が閾値を下回ったことを判定する |

これらは同じexpression treeを共有できるが、評価する時点と失敗時の意味は異なる。したがって
Expression自体と、それを保持するInvariant、Derived Value、Precondition、Effect Binding、
Occurrence Predicateを同一概念にはしない。

## 提案する最小境界

最初のexpression layerは、値を読むだけの**pure expression**とする。

- field参照
- requiredなto-one relationのtraversal。ただし最初のInvariantでは使わない
- literal
- 二項算術`+`、`-`
- 比較`<`、`<=`、`>`、`>=`
- 等価`==`、`!=`
- boolean演算`and`、`or`、`not`
- 括弧

次は最小境界へ含めない。

- assignmentとmutation
- `if`、loop、statement block
- 任意のfunction call
- network、database、file、clockへのaccess
- to-many relationの集計
- 単項`-`、乗算、division
- optional valueの暗黙なunwrap
- target固有のcodeまたはembedded language

Expressionは副作用を起こさず、同じbindingと値から常に同じ結果を返す。`clock.now`のようなruntime値は、
Expressionが直接取得せず、将来のevaluation contextが明示的なbindingとして与える。

## 最初の縦切り: entity invariant

compiler実装へ進む場合、最初はentity invariantだけを追加する。

```forma
entity StockItem {
    onHand   Quantity required
    reserved Quantity required

    invariant stockAvailable: reserved <= onHand
}
```

invariant名を必須にする。匿名の式からidentityを作ると、式の変更だけでnode identityが変わり、複数の
invariantを安定して区別できないためである。

```text
entity/StockItem/invariant/stockAvailable
```

この名前はcompiler diagnostic、Source Map、Conformance Contractの参照に使う。利用者向けのerror copyを
この名前から自動生成するかは、本proposalでは決めない。

InvariantはBool expressionを持ち、createまたはmutationのpost-stateに対して、authoritativeなatomic
boundaryのcommit前に評価する。`false`なら操作全体を拒否し、部分的な変更を残さない。中間状態は
観測せず、将来複数fieldを同時変更する場合も完成したpost-stateだけを評価する。

最初の実装では単一entityのcreate/editを対象とし、Invariantから参照できる値も、そのentity自身の
fieldとstateに限定する。relation traversalを含むInvariantはcompile errorとする。

```forma
entity OrderLine {
    product  Product required
    quantity Quantity required

    // first sliceではerror
    invariant withinStock: quantity <= product.onHand
}
```

`Product.onHand`だけを変更した場合、`OrderLine`のmutation境界ではこのInvariantを再評価できないためである。
複数entityのatomic post-stateはChanges proposalで定める。relationを読むInvariantを将来認める場合は、
式が依存するentity集合をIRに持たせ、逆方向の再検査対象とatomic boundaryを決定できることを前提とする。

## Surface syntax sketch

次は議論用のsyntaxであり、まだEBNFへの追加ではない。

```forma
entity StockItem {
    onHand   Quantity required
    reserved Quantity required

    invariant stockAvailable: reserved <= onHand
}
```

entity-bound expression内のunqualified nameは、そのentityのfieldまたはstateへ解決する。Semantic IRでは
暗黙性を残さず、`self` bindingへ解決する。

```text
reserved
  → FieldRef(binding: self, field: entity/StockItem/field/reserved)
```

surface syntaxに`self.reserved`という別表記も同時に入れるとcanonical formが二つになるため、最初は
unqualified formだけを候補とする。Action argumentなど複数bindingが必要になった時点で、explicit binding
syntaxを別途検討する。

Expression layer全体では、to-one relationを`.`でtraverseする候補を持つ。

```forma
product.price
customer.email
```

ただし中間relationは`required`でなければならない。例えば`product Product`がoptionalなら、
`product.price`はcompile errorとする。値が存在しない場合の意味をgeneratorへ推測させないためである。

```forma
product Product required
```

optional traversal、absence test、fallbackは、必要な実例とともに後続proposalで設計する。

ただし、これは最初のInvariantでは許可しない。まずDerived Valueのように、参照先の変更によってdomain
constraintを破らない利用場所で導入する。Invariantへ広げるのは、前節の依存追跡と再検査contractを
定めた後とする。

## Candidate expression grammar

最小構文は通常のprecedenceを持つ。

```ebnf
expression      = or_expr ;
or_expr         = and_expr, { "or", and_expr } ;
and_expr        = not_expr, { "and", not_expr } ;
not_expr        = { "not" }, equality_expr ;
equality_expr   = comparison_expr, [ ( "==" | "!=" ), comparison_expr ] ;
comparison_expr = additive_expr, [ ( "<" | "<=" | ">" | ">=" ), additive_expr ] ;
additive_expr   = primary_expr, { ( "+" | "-" ), primary_expr } ;
primary_expr    = literal | field_path | "(", expression, ")" ;
field_path      = name, { ".", name } ;
```

このprecedenceでは`not isActive == isPending`を`not (isActive == isPending)`と解釈する。`not`は0回以上
繰り返せるため、`not not value`も書ける。

`or`、`and`、二項`+`、二項`-`は左結合のbinary treeへ正規化する。

```text
a and b and c
  → Binary(and, Binary(and, a, b), c)
```

この規則により、n-aryに見えるsurface syntaxにも`left` / `right`による一意なIRとchild identityを
与えられる。比較と等価は文法どおり1回だけで、chainを作らない。

比較のchainは許さない。

```forma
// error: 意味を暗黙に展開しない
minimum <= value <= maximum

// canonical candidate
minimum <= value and value <= maximum
```

`/`はdivision by zero、Int division、Decimal precisionとroundingの意味を先に決める必要があるため、最小形から
外す。`*`もnamed typeのresult typeが未決定なので含めない。string concatenationにも`+`を流用しない。

### `-`に必要なv0 lexer migration

現行lexerは、`-`の直後がdigitなら符号付きnumber tokenへ取り込み、それ以外をlex errorにする。現行v0
EBNFも`number`、`integer`、`decimal`の内側に任意の`-`を含める。そのため、現在のままでは二項減算の
operatorとsigned numeric constantを別々に字句化できない。

```forma
reserved - 1 <= onHand // 現在はunexpected `-`
reserved -1 <= onHand  // `-1`が1個のnumber tokenになり、operatorが存在しない
```

Expressionを実装するときは、`-`を`->`とは独立したoperator tokenにし、number token自体はunsignedへ
変更する。既存surface syntaxを維持するため、移行対象は正確には次の2箇所である。

1. type modifierの`min` / `max`。
2. field modifierの`default literal`。

両方のparserが`"-", unsigned_number`をsigned numeric constantとして受理し、`min -1`と`default -1`を
引き続き書けるようにする。これはexpression parserだけの追加ではなく、`v0-primitives.md`の`number` /
`integer` / `decimal`規則、lexer、既存のtype・field modifier parserを同時に移行する変更である。

## 名前解決

field pathはcompilerがdeclaration identityへ解決する。

```forma
product.price
```

```text
binding: self (OrderLine)
segments:
  - entity/OrderLine/field/product -> Product
  - entity/Product/field/price    -> Decimal
result type: Decimal
```

次はAI generatorを呼ぶ前にcompile errorとする。

- 存在しないfield
- scalar値に対する`.` traversal
- to-many relationをscalar pathとしてtraverseすること
- optional relationを暗黙にtraverseすること
- relation traversalの途中または結果がprofile固有値であること

## 型規則

すべてのExpression IR nodeは、解決済みresult typeを持つ。

| 構文 | operand | result |
| --- | --- | --- |
| `==` `!=` | 同じsemantic type | `Bool` |
| `<` `<=` `>` `>=` | 同じordered scalar type | `Bool` |
| 二項`+` `-` | 同じnumeric semantic type | operandと同じtype |
| `and` `or` | `Bool`と`Bool` | `Bool` |
| `not` | `Bool` | `Bool` |

`type Quantity = Int min 0`のようなnamed scalarはnominal typeのまま扱う。`Quantity + Quantity`は
`Quantity`だが、`Quantity + Price`のような異なるnamed typeの加算・減算・大小比較はerrorとする。

numeric literalは周囲の期待typeへ適合できる。

```forma
price >= 0
```

この`0`は`price`と比較可能なnumeric literalとして解決する。literalを理由にnamed typeを`Int`へ落とさない。
両operandがnumeric literalで期待typeもない場合、整数literalは組み込み`Int`、小数literalは組み込み
`Decimal`をanchorとする。したがって`1 <= 2`は`Int`同士の比較として一意に解決する。

ordered scalar typeは、同じnominal typeを持つInt/Decimal基底のnumeric type、`Date`、`DateTime`とする。
`String`と`Bool`はorderedではなく、等価比較だけを許す。DateとDateTimeは相互比較せず、同じtypeのfield同士に
限る。Date/DateTime literalは未定義なので追加しない。

### 単項マイナスと乗算の型規則は未決定

単項`-`はnumeric operandへ限定するだけではresult typeが決まらない。named typeをそのまま継承すると、
宣言された値域を外れる場合がある。

```forma
type Quantity = Int min 0
invariant invalidNegation: -reserved <= onHand
```

この`-reserved`を`Quantity`とすると`min 0`を必ず破り、組み込み`Int`へ落とすとnominal comparisonが
成立しない。したがって単項`-`はCandidate grammarへまだ入れず、result type、constraint propagation、
literal negationとの区別を決めるまで保留する。

乗算の結果を常に組み込み`Int` / `Decimal`へ落とす規則は採用しない。その規則では、nominal typeを持つ
fieldと結果を比較できない。

```forma
quantity * product.price == lineTotal // DecimalとMoneyになり得る
reserved * 2 <= onHand                // IntとQuantityになり得る
```

`Quantity * Int -> Quantity`のように片側のnamed typeを継承できる場合と、`Quantity * Price`のように
新しいdomain typeを合成する場合を同じ規則で扱えるとは限らない。result type annotation、Moneyや単位を持つ
型を含め、Derived Value proposalの比較例で決める。それまでは単項`-`と`*`をgrammar、type checker、
実装順序へ入れない。採用する場合は、単項`-`をnon-numeric operandへ適用したときの専用diagnosticも
同時に定める。

union、state、relation identityの等価性は、現行v0の値等価性を再利用する。

## absenceとoptional value

v0では`required`でないfieldはabsenceを取り得る。最小expression layerは、absenceに対する暗黙の
truthiness、zero、empty string、defaultを導入しない。

- arithmetic、ordered comparison、boolean operatorのoperandはdefinitely presentでなければならない。
- optional fieldをそのまま使う式はcompile errorとする。
- `absence` literal、`exists`、optional refinement、fallback operatorは最小形へ含めない。

to-many collectionはv0で常にpresentであり、空collectionが既定値なのでoptionalとして扱わない。ただし
scalar operandではないため、最初のInvariantで使えばpresence errorの`F2604`ではなくtype errorの`F2605`とする。
to-one relationとunion fieldも、requiredかどうかを検査する前にnon-scalarとして`F2605`にする。

これは制限だが、Effect bindingに必須のrelationをoptionalのまま残すといったdomainの曖昧さを発見できる。
例えばすべてのOrderがCustomerを持つなら、次を明示する。

```forma
customer Customer required
```

optionalを必要とする実例が出た時点で、target非依存なabsence semanticsを追加する。

## Collectionは次段階に分ける

注文の`lines.count > 0`や`sum(lines.lineTotal)`には、to-many relationのcollection expressionが必要である。
しかしcollectionを入れると、少なくとも`count`、`sum`、`any`、`all`、element binding、empty collectionの
意味を同時に決める必要がある。

最小scalar expressionへ一般function callやlambdaを持ち込まないため、collectionは別段階に分ける。
Semantic IRとして次が本当に共通になるかを比較例で確認してからsurface syntaxを決める。

```text
Count(collection)
Sum(collection, projection)
Any(collection, predicate)
All(collection, predicate)
```

したがって最初の縦切りで表現できるのはself fieldだけを使う`reserved <= onHand`である。
`quantity * product.price`、`lines.count > 0`、`Order.total`はまだ表現できない。この不足をgeneratorに
推測させない。

## Expressionの利用場所ごとの評価context

同じExpression IRを使っても、評価時点は利用場所ごとに異なる。

| 利用場所 | binding | 評価時点 | 必須result type | false/failureの意味 |
| --- | --- | --- | --- | --- |
| Invariant | entity `self`。最初はlocal field/stateのみ | commit前のpost-state | `Bool` | atomic operationを拒否 |
| Derived Value | entity `self` | 値の観測時 | 宣言field type | 値を生成できないartifactは不適合 |
| Action Precondition | action targetと将来のinput | mutation前のpre-state | `Bool` | actionを提示せず、authoritative境界でも拒否 |
| Effect Binding | occurrence snapshot | emission作成時 | effect parameter type | bindingできないoccurrenceを成功扱いしない |
| Occurrence Predicate | pre/post snapshot | occurrence判定時 | `Bool` | predicateの成立変化規則に従う |

本proposalで固定候補にするのはInvariantのcontextだけである。残りはExpressionの利用者側proposalで決める。

## Semantic IR sketch

名称は候補だが、target generatorへunresolved sourceを渡さないため、概ね次のnodeが必要になる。

```text
InvariantIntent
  id: entity/StockItem/invariant/stockAvailable
  entity: entity/StockItem
  predicate:
    BinaryExpression
      operator: less-than-or-equal
      resultType: Bool
      left:
        FieldReference
          binding: self
          field: entity/StockItem/field/reserved
          resultType: Quantity
      right:
        FieldReference
          binding: self
          field: entity/StockItem/field/onHand
          resultType: Quantity
```

Expression nodeはsource textではなく、解決済みfield identity、operator、result typeを保持する。元の位置は
既存Source Mapへ分離する。root expressionのidentityは親nodeから導出し、childは`left`、`right`、
`operand`などtree上の役割から導出する。

```text
entity/StockItem/invariant/stockAvailable/expression
entity/StockItem/invariant/stockAvailable/expression/left
entity/StockItem/invariant/stockAvailable/expression/right
```

左結合で正規化したtreeでも、各nodeの役割をpathへ再帰的に追加する。

```text
a and b and c
  root:       .../expression
  inner a&&b: .../expression/left
  a:          .../expression/left/left
  b:          .../expression/left/right
  c:          .../expression/right
```

## Conformanceへの要求

AIに式の期待値を決めさせない。reference toolchainは、front-endのparse・名前解決・型検査とは別に、
同じResolved Expression IRを決定的に実行する式評価器を持たなければならない。

```text
onHand = 10, reserved = 8
  → predicate true、保存成功

onHand = 10, reserved = 12
  → predicate false、保存拒否
```

任意expressionに対するfixtureの自動合成は別問題である。本proposalは、少なくともfixtureを与えたときの
oracleがAIやtarget implementationに依存せず、同じ式評価結果を得ることを要求する。negative fixtureを
どこまでcompilerが自動生成するかはConformance Schemaで決める。

InvariantはUIだけで検査してはならない。authoritativeなmutation境界で、concurrent operationを含めて
post-stateがInvariantを満たす必要がある。database constraint、transaction、server validationのどれを
使うかはprofileが決める。

## 必須diagnostic

最初の実装は次をsource-addressed errorにする。括弧内はreference compilerで割り当てたcodeである。

- duplicate invariant name（`F2601`）
- unknown fieldまたはstate（`F2602`）
- 最初のInvariantでrelation traversalを使うこと（`F2603`）
- optional valueの未処理利用（`F2604`）
- operatorへ適用できないoperand type（`F2605`）
- Invariant rootが`Bool`でないこと（`F2605`）
- chained comparison（`F1003`）
- 同じ式に対するambiguous name resolution

traversalを導入する後続の利用場所では、invalid relation traversalもsource-addressed errorにする。

例:

```forma
invariant stockAvailable: reserveed <= onHand
```

```text
error: unknown field `StockItem.reserveed`
help: use a field declared by `StockItem`
```

```forma
invariant stockAvailable: location <= onHand
```

```text
error: operator `<=` cannot compare `String` and `Quantity`
```

```forma
invariant bounded: minimum <= value <= maximum
```

```text
error: comparison operators cannot be chained
help: declare two named invariants, one for each comparison; boolean `and` is not implemented yet
```

boolean `and`を実装した後は、helpをcanonical candidateの
`minimum <= value and value <= maximum`へ切り替える。

## 採らない案

### Opaqueなprompt文字列

```forma
invariant "reserved must not exceed onHand"
```

AIは理解できるが、compilerが名前解決、型検査、dependency追跡、決定的なoracle生成を行えないため採らない。

### target言語を埋め込む

```forma
invariant js { reserved <= onHand }
```

target-neutralな意味ではなく実装codeになるため採らない。

### general-purposeなstatement block

```forma
invariant stockAvailable {
    if reserved > onHand {
        return false
    }
    return true
}
```

同じ意味をpure expressionで直接書け、statement順序とcontrol flowを導入するため採らない。

## Primitiveとしての位置づけ

v0のprimitiveは引き続き10個であり、本proposalはv0の規範一覧を変更しない。ただし、このproposalを将来の
language versionで採用する場合、`invariant`はentityに属する**追加primitive**とする。固有のidentity、
predicate、評価時点、違反時の意味を持ち、他のdeclarationのmodifierだけでは表現できないため、§2の
選別基準を満たす。

entity-ownedであることはprimitiveでない理由にはならない。既存の`field`と`state`もentityに属する
primitiveである。一方、個々のBinary ExpressionやField ReferenceはInvariantなどに所有される内部treeで
あり、独立したapplication conceptではないためprimitiveへ数えない。

## 未決定事項

- named numeric scalarに対する単項`-`と`*`のresult type、constraint propagationをどう宣言・導出するか。
- optional valueのtest、refinement、fallbackをどのsyntaxで表すか。
- Date、DateTime literalをexpression layerと同時に設計するか。
- collection operatorのsurface syntaxをproperty、closed builtin、別primitiveのどれにするか。
- Derived Valueを`name Type = expression`と書くか、`derived`を明示するか。
- Action Preconditionをmodifier、named clause、action bodyのどこへ置くか。
- Invariant違反の利用者向けcopyとfield feedbackをどこで宣言するか。
- invariantを満たす／破るfixtureをcompilerがどこまで決定的に合成するか。

## 決定前に通す比較例

1. 在庫上限 — `reserved <= onHand`。
2. 非負の金額 — `price >= 0`。既存`min 0`との重複を確認する。
3. 明細金額 — `quantity * product.price`。named numeric typeとrelation traversalを確認する。
4. optional relation — CustomerがいないOrder。compile errorだけで十分か確認する。
5. boolean合成 — 下限と上限を`and`で組み合わせる。
6. identity/effect binding — requiredなUserのemail参照に同じFieldReference IRを再利用できるか。

`price >= 0`は既存の`type Price = Decimal min 0`でも表現できる。既存constraintで十分なものをInvariantの
別表記にしないよう、formatterやdiagnosticがより直接的な既存表現を案内すべきか確認する。

## 推奨する実装順序

1. `invariant name: expression`のentity memberとstable identity。**実装済み**。
2. field参照と`<=`の最小parser・型検査・IR。**実装済み**。numeric literalは未実装。
3. self-only Invariant向けのdeterministic evaluatorとConformance oracleを追加する。
4. 比較、等価、boolean、括弧と、左結合binary normalizationを追加する。
5. v0 lexerの符号付きnumberを移行して二項`+`、二項`-`を追加する。
6. 同じExpression IRをDerived Value proposalで再利用し、そこでrequiredなto-one relation traversalを追加する。
7. named numeric typeの単項マイナス・乗算規則を決め、比較例を通してから単項`-`と`*`を追加する。
8. relationを読むInvariantのdependency/revalidation contractを設計する。
9. collection、Action Precondition、Effect Binding、Occurrence Predicateをそれぞれの実例で拡張する。

最初のacceptance caseは次である。

```forma
entity StockItem {
    onHand   Quantity required
    reserved Quantity required

    invariant stockAvailable: reserved <= onHand
}
```

- `forma check`が成功する。
- Invariantのfield参照が`StockItem`自身へ限定され、relation traversalは拒否される。
- typoと型不一致をGo front-endが拒否する。
- byte-identicalなResolved Expression IRとSource Mapを生成する。
- AI generatorを使わず、fixtureに対するpredicateの期待値を決定できる。
- target artifactがinvalid post-stateをauthoritativeな境界で拒否することをconformanceで検査できる。

現在は最初の4項目までをreference compilerとtestで確認済みである。式評価器とConformanceが必要な最後の
2項目は未実装である。これらが成立するまでは、general function、collection、Effect syntaxをcompilerへ
追加しない。
