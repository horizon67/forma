# Forma v0 — プリミティブと言語仕様

Status: design draft 0.4 (reference implementation: partial; core Semantic IR: `forma/v0.4`)

> Forma is a high-level application programming language for expressing what
> software should be, not how it should be implemented.

```text
自然言語
  ↓ AI（任意の翻訳器）
Forma source
  ↓ Lexer → Parser → AST → Semantic Checker
Semantic IR + Conformance Contract
  ↓ target profile + generator（AI可）
Generated Artifact
  ↓ build + deterministic conformance
Accepted Artifact
```

AIは自然言語からFormaへの翻訳、target profileの作成、Semantic IRからtarget artifactへの生成に
利用できる。ただし、Forma sourceのparse、意味解決、Semantic IR、conformance contract、合否判定に
LLMの判断を用いてはならない。同じForma sourceとfront-end versionからは、byte-identicalな
Semantic IRとconformance contractを得なければならない。生成artifact自体は実行ごとに異なって
よいが、同じ観測可能な意味を持ち、同じconformance contractを通過しなければならない。

生成されたtarget codeは人間が編集するsourceではなく、破棄・再生成可能なartifactとする。
target固有の恒久的な判断はversioned target profileに置き、生成物へ手修正してはならない。
人間がreviewし保守するアプリケーションsourceはFormaだけである。diagramはFormaから生成する
viewであって、別のsource of truthにはしない。

本書をForma v0の規範仕様とする。

## 1. v0が検証する仮説

Formaは、AI時代には人間がtarget codeの大部分を書かず、すべてを読むことも前提にできない、という
認識から出発する。一方で、自然言語の設計書やpromptだけをsource of truthにすると、parse、静的検査、
参照解決、差分の意味、実装との同期を機械的に保証できない。人間が保守する設計図を、散文documentでは
なく実行可能な高水準言語にすることがFormaの中心的な選択である。

Forma v0が検証するのは、アプリケーションの「何が存在し、誰が、何を見て、どう変えられるか」
を短く記述し、実装詳細を含まないSemantic IRと、実装の合否を判定できるconformance contractへ
決定的に変換できるか、という一点である。target generatorの実装や生成コードが異なっても、
同じcontractによって観測可能な意味を検証できることを含む。

最初の対象は、次を備えたデータ管理アプリケーションとする。

- entityの一覧・詳細・作成・編集・削除
- 検索・絞り込み・ソート・ページング
- 状態遷移と、状態に応じたactionの表示・拒否
- roleによるpageとactionの認可
- loading、empty、pending、validation error、failureの各状態

`list User`はHTML tableやReact componentを意味しない。「Userの集合を利用者に提示する」
という意図である。Webではtable、mobileではlistとinfinite scrollなど、具体的な実現方法は
target profileが決める。

### 1.1 人間可読性の設計contract

人間が継続的に読むsourceはFormaだけであるため、可読性を言語の付加的な美観ではなく設計要件とする。
想定読者はdomainとapplicationの構造を考えられるsoftware builderであり、targetのframework、
transport、database schemaを知らなくても、sourceから次を説明できなければならない。

1. 何が存在するか。
2. 誰が何を見て、何を変更できるか。
3. どのconstraint、state、transitionが適用されるか。
4. 要求を変えるにはどの宣言を変更するか。
5. 変更によって観測可能な意味がどう変わるか。

短さや自然言語らしさだけを理由にsyntaxを追加しない。同じconceptには一つのcanonical formを与え、
実装詳細は省略してもapplication semanticsを決める事実は隠さない。defaultや参照解決による暗黙性は、
閉じた決定的規則から導出し、Semantic IRへ記録し、人間向けに展開できる場合だけ認める。宣言間の依存と
diagnosticはSource MapによってForma declarationへ追跡可能にする。

詳細なproposal checklistと完全例の監査結果は
[`language-design-principles.md`](language-design-principles.md)に記録する。この文書は設計guideであり、
規範的なsyntaxとsemanticsは本書にのみ定める。

## 2. プリミティブの選別基準

プリミティブは、target-neutralなSemantic IRに独立した意味ノードを導入し、他の構文の
パラメータだけでは表現できない概念とする。

修飾子は、既存のプリミティブを限定または調整するだけで、単独のidentityやlifecycleを
持たない。生成されるroute、table、component、controller、functionなどの実装構造は、
プリミティブの選別基準にしない。Formaが実装構造から独立した言語であるためである。

例:

| 語 | 判定 | 理由 |
| --- | --- | --- |
| `list` | プリミティブ | entity集合を提示する独立した意味ノード |
| `search` `filter` `sort` `paginate` | 修飾子 | 一つの`list`の能力を限定する |
| `create` `view` `edit` `delete` | 標準action | `action`の標準インスタンス |
| `confirm` | 修飾子 | 一つの`action`の実行条件を追加する |
| `login` `logout` | v0対象外 | roleは認可を記述するが、認証方式は決めない |
| `upload` `retry` `cache` | v0対象外 | インフラとeffectの設計を必要とする |

### 2.1 compilation unitとapplication境界

1回のfront-end compile operationへ明示的に渡されたForma source fileの集合を、1つの
**compilation unit**とする。1 compilation unitは1つのapplication namespaceを持ち、すべての
top-level declaration、参照解決、重複検査、Semantic IR、Source Map、Conformance Contractは
その集合全体に対して生成する。

source pathとdirectory階層はnamespaceやapplication境界を作らない。複数fileまたはdirectoryを
1回の`forma check`へ渡した場合、再帰的に得たすべての`.forma`を1つのcompilation unitへunionする。
同じdirectoryにあることだけを理由に複数のapplicationへ分割したり、subdirectoryごとに暗黙の
moduleを作ったりしてはならない。file間の参照に`import`は不要であり、v0はpackage/module構文を
持たない。

1 repositoryに複数のForma applicationを置いてよい。その場合は、それぞれのsource集合を別の
compile operationとして明示する。したがって独立した`examples/users.forma`と
`examples/orders.forma`は個別にはvalidでも、両方を同じoperationへ渡せば、同名の`role admin`などは
通常の重複宣言としてerrorになる。

`forma check`は1個以上のfileまたはdirectory引数を必須とし、引数なしでcurrent directoryを暗黙に
探索しない。将来のproject/profile manifestは1つのbuildについてこのsource集合を明示する責務を持つが、
manifestの所在やdirectory layoutによってlanguage semanticsを変えてはならない。

## 3. プリミティブ10個

### 3.1 何が存在するか

#### 1. `type`

domain固有のscalarまたは閉じた値集合を宣言する。

```forma
type Email = String matches /.+@.+/
type Score = Int min 0 max 100
type Plan = Free | Pro | Enterprise
```

#### 2. `entity`

identityとlifecycleを持つdomain objectを宣言する。各entityは、targetが生成・管理する
opaqueなidentityを暗黙に持つ。ID fieldやdatabase keyをFormaソースに書く必要はない。

#### 3. `field`

entityが持つ値または関連を宣言する。`field`というkeywordは使わず、entity本体に
`name Type`と書く。

```forma
entity Team {
    name String required label
}

entity User {
    name  String required
    email Email required unique
    team  Team
    posts [Post]
}
```

entity型のfieldはto-one関連、`[Entity]`はto-many関連を意味する。foreign key、join table、
埋め込みなどの永続化方式はtarget profileが決める。

entity型のfieldを人間に提示するときは、参照先entityの`label` fieldを使う。上の例では
`User.team`を表示する値は`Team.name`である。`label`はHTML labelやfield captionではなく、
entity instanceの人間向け表現を指定する。

### 3.2 どう変わるか

#### 4. `state`

entityのlifecycleを、名前付きの閉じた値集合として宣言する。

```forma
entity User {
    state status Pending | Confirmed | Active | Suspended initial Pending
}
```

`initial`に指定した値が初期状態である。valueのsource orderはpresentation orderだけを表し、並べ替えで
初期状態を変えない。1 entityにつきstateは0個または1個。state名はfieldと同じように
`columns`、`filter`、`detail`から参照できるが、`form`では直接編集できない。

`state`を宣言しながら参照時だけ暗黙に`status`へ改名する方式は採用しない。宣言名と参照名は
常に同じであり、`state status ...`と明示する。

#### 5. `action`

entityに対する許可された状態遷移を宣言する。

```forma
action User.activate: Confirmed -> Active
action User.suspend: Active -> Suspended confirm allow admin
```

v0のdomain actionは引数と処理本体を持たない。宣言された遷移、確認要否、role制約、成功後の
遷移先pageだけを持つ。source stateが一致しない実行は、authoritativeなmutation境界でatomicに
拒否しなければならない。

`create`、`view`、`edit`、`delete`は全entityに暗黙に存在する標準actionである。`view`も含む
4個を閉じた集合とし、UI側から宣言なしで参照できる。

### 3.3 どう見えるか

#### 6. `page`

利用者が到達できるapplication surfaceを宣言する。

```forma
page Users {
    list User
}

page UserDetail(user User) {
    detail user
}
```

pageは0個または1個のentity parameterを取る。routeはpage名、parameter、target profileから
決定的に導出する。route文字列はv0ソースに書かない。

#### 7. `list`

entity集合の提示意図と、利用者に許すquery能力を宣言する。

```forma
list User {
    columns name, email, status
    search name, email
    filter status
    sort name asc
    paginate 20
    actions create, view, edit, delete, suspend
}
```

`create`はcollection action、それ以外は各itemのcontextual actionとして配置される。

#### 8. `detail`

一つのentityのread-onlyな提示意図を宣言する。

```forma
detail user {
    fields name, email, status
    actions edit, suspend
}
```

#### 9. `form`

entityの作成または編集入力を宣言する。entity名を渡すと作成、page parameterを渡すと編集に
解決される。

```forma
form User {
    fields name, email
    submit create
}

form user {
    fields name, email
    submit edit
}
```

`fields`を省略すると、state以外の`readonly`でないfieldをsource orderで選ぶ。

### 3.4 誰が実行できるか

#### 10. `role`

認可規則の主語を宣言する。

```forma
role admin

page Users {
    allow admin
    list User
}
```

`role`と`allow`からpage guardとaction guardを生成する。ただし、roleは認証方式を定義しない。
OAuth、password、session、token、login/logout UIなどはtarget profileまたはv1以降のidentity設計が
担当する。

## 4. 組み込み型

v0の組み込み型は次の6個だけとする。

| 型 | 意味 |
| --- | --- |
| `String` | Unicode text |
| `Int` | 符号付き整数 |
| `Decimal` | exact decimal number |
| `Bool` | boolean |
| `Date` | timezoneを持たない暦日 |
| `DateTime` | 時点 |

`Money`、`Email`、`URL`、`File`、`UUID`は組み込まない。`Email`や`URL`相当は`type`で
宣言できる。`File`はupload/storage/effectの設計を伴うためv0では表現できない。

`type`はnominalである。たとえば`type Email = String ...`で作った`Email`と`String`は、
意味解析上は別の型として扱う。

union typeのvariantとstate valueはuppercase-leading identifierを使う。値のsource orderは安定した
presentation orderとして保持するが、大小比較やbusiness priorityを意味しない。

### 4.1 値の等価性とtext matching

異なるtargetで`unique`、`filter`、`search`の結果が変わらないよう、v0の値比較を次のように定める。

- `String`はNFCへ正規化してから、case-sensitiveなUnicode scalar value列として比較する。
- `Int`は数学的な符号付き整数、`Decimal`は有限桁の正確な10進数として比較する。表記上の末尾の
  0は等価性に影響しない。binary floating-pointへ意味を落としてはならない。
- `Bool`は真偽値、`Date`は同じ暦日、`DateTime`は同じ時点なら等しい。
- unionとstateは宣言されたvariant identity、relationはopaqueなentity identityで比較する。
- absenceはどのliteralとも等しくない。absenceだけを選ぶfilterはv0では提供しない。

`unique`とscalar/state/relationのexact `filter`は、この等価性を使う。

`search a, b`へquery `q`を与えた場合、`q`が空ならcollectionを制限しない。空でなければ、NFCへ
正規化した`q`が、指定fieldの少なくとも一つのNFC正規化済み値にcase-sensitiveな連続substringとして
含まれるrecordだけを返す。tokenization、stemming、locale依存collation、fuzzy searchはv0に含めない。

`matches /.../`はNFC正規化済みの値に対し、RE2-compatibleな正規表現のsearch semanticsを使う。
文字列全体を検査する場合はauthorが`^`と`$`を明記する。profileは同じdialectを実装し、host
language固有のregexへ意味を変えてはならない。

## 5. 修飾子の閉じたセット

v0で使用できる修飾子は次のものだけである。

| 対象 | 修飾子 |
| --- | --- |
| `type` | `matches` `min` `max` |
| `field` | `required` `unique` `readonly` `default` `label` |
| `action` | `confirm` `allow` `goto` |
| `page` | `allow` |
| `list` | `columns` `search` `filter` `sort` `paginate` `actions` |
| `detail` | `fields` `actions` |
| `form` | `fields` `submit` |

`state`の`initial`は省略可能な修飾子ではなく、state declarationに必須の句である。

### 5.1 type修飾子

- `matches /.../`は`String`を基底とする型にだけ使える。
- `min N`と`max N`は`Int`または`Decimal`を基底とする型にだけ使える。
- 同じ修飾子は1宣言に1回まで。`min`は`max`以下でなければならない。

named scalarが別のnamed scalarをbaseにする場合、baseのconstraintをtransitiveに継承する。
derived typeの`matches`は継承したpatternとのAND、`min`は全lower boundの最大値、`max`は全upper
boundの最小値として合成する。合成結果が充足不能ならcompile errorとする。Semantic IRには宣言ごとの
差分ではなく、targetが再解決を必要としないeffective constraintを記録する。

### 5.2 field修飾子

- `required`: 作成時と保存時に値が必要。指定がなければto-one/scalar fieldはabsenceを許す。
- `unique`: absence以外の値がentity内で一意。authoritativeな保存境界で検証する。
- `readonly`: `form`から変更できない。
- `default value`: 作成時に省略された場合の値。型検査されるliteralのみを許す。
- `label`: このentityのinstanceを人間に提示するときに使うfield。

`label` fieldは1 entityに最大1個で、`required`なString基底のscalar fieldでなければならない。
relation fieldが`columns`、`filter`、`detail fields`、`form fields`のいずれかに現れる場合、
参照先entityには`label`がちょうど1個必要である。

relationの表示は参照先の`label`値を使う。relationを`filter`で使う場合は参照先identityによる
exact filterとlabelによる選択肢を提供する。relationを`form`で使う場合は、現在のpage/actionの
認可contextで選択可能な参照先entityへ到達できるrelation pickerを提供する。dropdown、検索型
picker、lazy queryなどの実現方法はprofileが決めるが、選択肢の意味は
`identity + label value`であり、別のfieldを推測してはならない。この能力はSemantic IRに
`RelationChoiceIntent`として明示する。

to-many relationを`columns`または`detail fields`へ明示した場合は、各要素を
`identity + label value`として持つcollectionを提示する。並び順をsourceが宣言していないため、
v0はそのcollectionの表示順を保証しない。

to-many fieldは空collectionを既定値とし、`required`、`unique`、`default`を指定できない。
entity identityは暗黙であるため、target生成IDや`createdAt`のようなruntime由来fieldをv0の
完全例では扱わない。runtime由来値の一般構文はv1以降で設計する。

v0の`literal`には`Date`と`DateTime`の表記がない。そのため、この2型およびそれらを基底とする
型には`default`を指定できない。日付をfieldやformで扱うこと自体はできる。

### 5.3 action修飾子

- `confirm`: dispatch前に肯定的な確認を要求する。具体的な文言とUIはtarget profileが提供する。
- `allow roles`: 記載roleのいずれかを要求する。
- `goto Page`: 成功後にpageへ遷移する。

`delete`は`confirm`を明記できない標準actionだが、常にtarget profileの標準確認を要求する。
破壊的操作を無確認で生成してはならない。

### 5.4 page修飾子

`allow roles`は記載roleのいずれかを要求する。pageとdomain actionの両方に`allow`がある場合、
両方を満たさなければならない。`allow`がない宣言はFormaの認可層では制限しない。

### 5.5 list修飾子

- `columns`: 主に提示するfield/stateとその順序。
- `search`: 一つのtext queryで検索する`String`基底field。
- `filter`: exact-value filterを提供するfield/state。
- `sort`: scalar fieldによる初期順序。方向を省略した場合は`asc`。
- `paginate N`: 1 logical windowの最大件数。`N`は正の整数。
- `actions`: 利用者に提示する標準actionまたはdomain action。

`paginate`はoffset、cursor、infinite scrollを指定しない。観測可能な契約は、安定した順序、
一巡中に重複しないこと、残りの結果へ到達できること、の3点である。指定sortが一意でない場合、
targetは暗黙のentity identityをtie-breakerとして追加する。

`sort`には、組み込みscalarまたはそれを基底とするnamed scalar fieldだけを指定できる。
relation、collection、union、stateはv0ではsortできない。比較はStringがNFC正規化後のUnicode
scalar valueによるcase-sensitive辞書順、Int/Decimalが数値順、Boolが`false < true`、
Date/DateTimeが時系列順とする。

各list修飾子は1回まで。targetが意図を実現できない場合はcompile errorとし、黙って無視しない。

### 5.6 detailとform修飾子

- `fields`は提示または編集するfield/stateと順序を指定する。
- `detail actions`はcontextual actionを提示する。
- `form submit`は、作成formでは`create`、編集formでは`edit`でなければならない。

`form submit`を省略した場合も、form subjectから解決されたmodeと同じ標準actionを自動的に使う。
したがって`submit`はactionを選択するescape hatchではなく、意図を明示して検査するための修飾子である。

stateは`detail fields`には指定できるが、`form fields`には指定できない。状態変更は必ずdomain
actionを通す。

### 5.7 `columns`と`detail fields`を省略した場合

省略をprofileの推測にしないため、次のprojectionへ決定的に展開する。

- `list`の`columns`省略: collection以外のfieldをsource orderで並べ、最後にstateを置く。
- `detail`の`fields`省略: collection以外のfieldをsource orderで並べ、最後にstateを置く。
- to-many relationは、データ量を暗黙に展開しないため、どちらも明示した場合だけ提示する。

暗黙に選ばれたto-one relationにも通常の`label`検査を適用する。展開後のprojectionをSemantic IRへ
記録し、target generatorに省略の再解釈をさせない。展開結果が空になるlist/detailは、人間に
提示できる値がないためcompile errorとする。

## 6. actionの宣言・参照・解決

domain actionを宣言できるのはtop levelだけである。

```forma
action User.suspend: Active -> Suspended confirm allow admin
```

`list`と`detail`の`actions`は、既存actionの参照リストであり、別のactionを宣言しない。
これにより状態遷移と認可のsingle source of truthを保つ。

action参照は、viewのcontext entityに対して次の順で解決する。

1. `create`、`view`、`edit`、`delete`なら標準action。
2. それ以外なら同名の`action Entity.name`。
3. 見つからない場合は`forma check` error。

標準actionの解決規則:

- `create`: 同じentityを引数に取るcreate form (`form Entity`) が一つだけ必要。
- `view`: 同じentity bindingを表示するdetailが一つだけ必要。
- `edit`: 同じentity bindingを編集するform (`form value`) が一つだけ必要。
- `delete`: entityを削除する。

候補が0個または複数なら曖昧さを推測せずerrorにする。domain actionはsource stateでのみ提示し、
authoritativeな境界でも同じpreconditionを検査する。

### 6.1 成功後の遷移

標準actionは宣言を持たず`goto`を付けられないため、成功後のnavigationを次のように規定する。

- `create`: 作成したentityの一意なdetailへ遷移する。detailがなければ呼び出し元listへ戻る。
  direct navigationされたcreate formで、どちらもなければ同じformに留まり作成済み値を示す。
- `edit`: 同じentityの一意なdetailへ遷移する。detailがなければ呼び出し元listへ戻る。
  direct navigationされ、どちらもなければ同じformに留まり保存済み値を示す。
- `delete`: list上では同じlistに留まり再評価する。list外では同じentityのlistを含むpageへ
  遷移して再評価する。このpageが0個または複数なら、そのcontextで`delete`を公開できない。
- `view`はread-onlyなnavigationなので成功後遷移を持たない。

domain actionは`goto`があればそのpageへ遷移し、なければ現在のcontextに留まってentityを
再評価する。parameterを持つ遷移先には、作成・編集・遷移後のcontext entityを渡す。

「呼び出し元list」はaction dispatch時のnavigation contextとして保持する。以上の規則は
Semantic IRに解決済みdestinationとして記録し、target profileが別の遷移を選んではならない。
ここでdestinationは固定page名だけでなく、`caller-list`または`same-context`という閉じたruntime
policyを取り得る。各form viewのIRにはmodeだけでなく、`submit create/edit`、成功destination、
duplicate dispatch防止、failure feedbackを持つ`SubmitIntent`を必ず含める。

`SubmitIntent`は少なくとも、standard action名、成功navigation、合成済みaccess、pending中の
duplicate dispatch防止、failure feedbackを保持する。成功navigationは固定`page`、
`caller-list`、`same-context`の閉じた3種類とし、`caller-list`にはdirect navigation時の
`same-context` fallback pageを記録する。遷移時にはdestination pageのaccessを再検査する。

accessは単一role listへ平坦化せず、source page、action、destination pageそれぞれの`allow`を
`allOf`で合成し、各`allow`内のrole listを`anyOf`として保持する。例えば
`allow admin, editor`のsource pageから`allow member`のpageへ遷移する場合は、
`(admin OR editor) AND member`である。固定destinationはcompile時に合成し、`caller-list`の
destination accessはdispatch時に保持したnavigation contextから再検査する。

### 6.2 navigationと認可の合成

actionを提示するには、現在pageの`allow`、domain action自身の`allow`、遷移先pageの`allow`を
すべて満たす必要がある。標準actionは自身の`allow`を持たないが、現在pageと遷移先pageの両方を
満たした場合だけ提示する。mutationのauthoritativeな境界ではdomain action自身の認可を、pageの
境界では遷移先pageの認可をそれぞれ再検査する。認可されないpageへ遷移するactionを表示してから
失敗させる実装を標準動作にしてはならない。

## 7. 実行時に必須の意味

簡潔なFormaソースから省略されても、次は全targetに必須である。

- list/detailのloading、empty、failure状態
- formの初期値、client feedback、authoritative validation error、pending状態
- actionのpending中のduplicate dispatch防止、成功後の表示整合、failure feedback
- state preconditionとrole ruleのserver-side相当境界での再検査
- field constraintとunique constraintのauthoritativeな検証
- query traversalの安定性

これらのinteraction stateは特定widgetの表示を要求するものではなく、利用者から観測できるruntime
conditionである。たとえばserver-rendered targetはplatformのnavigation中状態で`loading`を満たして
よいが、処理中を`ready`や`empty`として誤表示してはならない。conformance adapterは各target固有の
表現を、共通のconditionへ対応付けて観測する。

具体的なcomponent、HTTP、cache invalidation、再取得、transaction、database constraint、test
frameworkはtarget profileが選ぶ。これらをForma sourceやSemantic IRのUI/transport固有ノードに
漏らしてはならない。

## 8. target profile、Semantic IR、conformance

### 8.1 deterministic front-end

reference compilerのfront-endは次の責務に分ける。

```text
Forma source
  ↓ Lexer
Token stream
  ↓ Parser
Syntax AST
  ↓ Semantic Checker
Semantic IR
  ↓ Conformance Builder
Conformance Contract
```

Parserまたは同等の決定的な構文解析は、textual languageであるFormaに必須である。独立したLexerは
実装方式として必須ではないが、reference compilerでは改行、comment、string、regex、記号を
tokenizeするために使用する。ASTはsource syntaxとsource spanを保持するが、未解決参照を含み得る。
名前、型、action、navigationを解決するのはSemantic Checkerであり、AIにparseや意味解決を
代行させない。

### 8.2 front-end output

front-endの規範的な出力は次のように決まる。

```text
Forma source + front-end version -> Semantic IR + Conformance Contract
```

target profileは、component、route形状、transport、persistence、pagination方式、concurrency、
cache更新、authentication integration、test frameworkを所有する。AI generatorを利用するprofileは、
model設定、prompt template、利用可能なtool、出力schemaもversion管理する。これらは実装方針であり、
Semantic IRの意味を変更してはならない。

コンパイラは最初にsourceをtarget-neutralなSemantic IRへ正規化する。たとえばlistは最低でも
次を保持する。

```text
CollectionIntent(entity: User)
  projection: [name, email, team, status]
  relations: [team -> RelationPresentation(Team.name)]
  search: [name, email]
  filters: [status, team]
  ordering: [name ascending, identity stable-tiebreaker]
  window: 20
  actions: [create, view, edit, delete, transition(User.suspend)]
  access: anyOf(admin)
```

Semantic IRは、解決済みsymbol、型、constraint、認可、状態precondition、必須のinteraction stateを
保持する。各semantic nodeには安定したidentityを与え、compilerはそのidentityからsource spanへの
Source Mapを別に出力する。pathやlineの変更をsemantic changeとして扱わないため、Source Mapは
Semantic IRの意味的等価性やbuild keyには含めない。React component、Rails controller、HTTP verb、
SQLなどはSemantic IRに保持しない。

semantic identityはsource path、line、column、declarationのglobalなsource orderを含まないcanonical
pathとする。named declarationは次の形を取る。

```text
role/admin
type/Email
entity/User
entity/User/field/email
entity/User/state/status
action/User/activate
page/UserDetail
```

page内のviewは`page/{Page}/view/{kind}/{Entity}`、formはmodeも含む
`page/{Page}/view/form/{create|edit}/{Entity}`とする。view内の解決済みaction参照、relation choice、
sort、SubmitIntent、navigation、accessなどの匿名nodeは、親identityへkindと局所名を追加する。
同じpage内に同一identityとなるviewを複数宣言することはできない。declarationまたはviewの意味的な
主語をrenameすればidentityは変わるが、file移動、空行、comment、source positionだけの変更では
identityは変わらない。top-level IR collectionはidentity順へcanonicalizeする。field順、state value順、
view内の明示的なprojection/action順のようにpresentation semanticsを持つ順序は保持する。

Source Mapのformat versionは`forma/source-map/v0.1`とし、対象IR versionと、semantic nodeごとの
`nodeId`、`kind`、half-open source spanを保持する。generator/build/runtime diagnosticは`nodeId`で
Forma declarationを参照し、表示時にSource Mapを使って現在のfileと位置へ戻す。

conformance contractはSemantic IRから決定的に導出する。最低でも、正常系と次の否定的な性質を
target-neutralなcaseとして保持する。

- field constraintとunique constraintの受理・拒否
- state transitionのsource preconditionと遷移後state
- page/actionのrole認可と権限拒否
- relation choiceのentityとlabel
- search、filter、stable sort、page boundary
- 標準actionの成功後navigation
- loading、empty、invalid、pending、failureを含む必須interaction state

conformance contractは言語仕様を置き換えるものではなく、生成artifactを検査するための決定的な
projectionである。contractに未収録の振る舞いをgeneratorが自由に決めてよいことにはならず、
規範的な意味は本書とSemantic IRに残る。

profileは、version管理された決定的なadapter/harnessによってcontractをtargetへ適用する。AIにtestの
期待値や合否を決めさせてはならない。生成artifactはbuildに成功し、contractを全て通過した場合だけ
採用する。

生成コストと不要な差分を抑えるため、実装は次の入力からbuild keyを作り、成功済みartifactを
content-addressed cacheで再利用してよい。IRだけでなくprofileやgenerator設定の変更もkeyへ含める。

```text
build key = hash(Semantic IR, Conformance Contract,
                 target profile version, generator configuration)
```

cacheは最適化であり、正しさの根拠ではない。cache missで再生成したartifactも同じconformance
contractを通過しなければならない。

### 8.3 profile compatibility

`forma check`はtarget-neutralなsyntaxとsemanticsだけを検査する。特定profileがintentを実装できるかは、
`forma build --profile P`がgeneration前に検査する。profileは対応するIR versionとcapabilityを機械可読に
宣言し、未対応のsemantic nodeまたはcapabilityがあれば、AI generatorを呼ぶ前にsource-addressedな
compile errorとする。profile非対応をgeneratorの推測やconformance failureまで遅延させない。

## 9. v0で意図的に扱わないもの

- login/logoutと認証方式
- upload/download、`File`
- import/export
- schedule/retry/background job
- cache policy
- notification effect
- realtime collaboration
- i18n copy
- runtime由来fieldの一般構文
- domain actionの引数と処理本体
- 集計、joinを伴うderived list
- inverse relationとcascade rule
- 既存deploymentのschema/data migrationとzero-downtime evolution
- deployment、secret管理、monitoringなどのoperations
- 生成target codeへ手書き機能を混在させるcustomization

重要でないからではなく、identity、effect、infrastructure、queryという次の設計軸を必要とし、
v0の仮説検証範囲を広げるためである。

## 10. 決定事項

1. v0のプリミティブは`type entity field state action page list detail form role`の10個。
2. 組み込み型は`String Int Decimal Bool Date DateTime`の6個。
3. field関連はfield typeで表し、`relation`などの別プリミティブを作らない。
4. stateは名前を明示し、宣言名と参照名を一致させる。例: `state status ...`。
5. 1 entityにつきstateは最大1個。初期状態は`initial Value`で明示する。
6. 標準actionは`create view edit delete`の4個。
7. domain actionはtop levelで一度だけ宣言し、view側は参照だけを行う。
8. formは大文字のentityならcreate、小文字のbindingならeditに解決する。
9. roleは認可だけを表し、認証方式を暗黙に発明しない。
10. routeは自動導出し、明示route構文をv0に入れない。
11. 改行をseparatorとし、semicolonは持たない。
12. modifiersは§5の閉じた集合とする。
13. 人間に提示するrelation先entityは、String基底の`label` fieldを明示する。
14. keywordはcontextualとし、globalな予約語表を持たない。
15. 標準actionの成功後遷移は§6.1の規則で決定し、profileの裁量にしない。
16. 決定的なcompile境界はSemantic IRとconformance contractまでとする。
17. target generatorはAIを利用できるが、生成artifactは決定的なconformanceで検証する。
18. 生成されたtarget codeは編集対象のsourceではなく、破棄・再生成可能なartifactとする。
19. reference front-endはLexer、Parser、syntax AST、Semantic Checkerを明示的に分離する。
20. `columns`と`detail fields`の省略は§5.7のprojectionへ展開し、profileに推測させない。
21. value equality、`search`、regex semanticsは§4.1に固定し、profile固有のcollationへ委ねない。
22. `forma check`はtarget-neutralとし、profile compatibilityは`forma build`でgeneration前に検査する。
23. diagramはFormaから生成するviewとし、別のsource of truthにしない。
24. named scalarのconstraintはtransitiveに合成し、Semantic IRへeffective constraintを記録する。
25. form submissionと成功後navigationは`SubmitIntent`へ解決し、generatorに再導出させない。
26. 可読性は言語設計要件とし、短さよりintentの直接性、canonical form、説明可能な暗黙性を優先する。
27. semantic identityはcanonical pathから導出し、Source MapをIRと分離する。同じpage内で同一identityに
    なるviewはcompile errorとする。
28. 1回のcompile operationへ明示的に渡したsource集合を1 compilation unit、1 application namespaceと
    する。pathやdirectoryから暗黙のapplication/module境界を導出しない。

### 10.1 統合レビューで埋めた仕様の穴

| 発見事項 | v0での決定 |
| --- | --- |
| 組み込み型が未定義 | `String Int Decimal Bool Date DateTime`の6個に固定 |
| `state`の宣言名と`status`参照が接続されていない | `state status ...`のように名前を明示し、宣言名と参照名を一致させる |
| `view`が未宣言のまま参照される | 標準actionを`create view edit delete`の4個に固定 |
| `createdAt Date readonly`の値を誰が作るか未定 | runtime由来fieldをv0から外し、完全例のsortを`name`へ変更。entity identityだけは暗黙 |
| action引数が未決なのにEBNFでは許可される | domain actionのparameterをv0 EBNFから削除 |
| 「完全例」にcreate/deleteの到達経路がない | create formを追加し、list/detailから標準actionを参照 |
| roleからlogin/logoutまで導出できるように読める | roleは認可だけと明記し、認証integrationはprofileの責務に分離 |
| relationの人間向け表現とpicker queryが未定義 | `label` fieldと`RelationChoiceIntent`を追加 |
| 標準actionの成功後遷移が未定義 | create/edit/deleteの遷移規則を§6.1に固定 |
| keywordの予約方式と標準action名衝突が未定義 | keywordをcontextualとし、domain actionと標準actionの同名宣言をerrorにする |
| Date/DateTimeのliteralがない | v0では両型への`default`を許さないと明記 |
| stateをsortできるか不明 | v0のsort対象からstate、union、relation、collectionを除外 |
| stateの先頭valueが暗黙に初期状態を兼ねていた | `initial Value`を必須にし、presentation orderと初期状態を分離 |

## 11. EBNF

以下はv0のsurface syntaxを定義する。spaceとtabはtoken間で無視する。改行はseparatorとして
有意であり、commentは改行の直前までを占める。

```ebnf
program        = { blank | declaration } ;
declaration    = type_decl | entity_decl | action_decl | page_decl | role_decl ;

(* type *)
type_decl      = "type", type_name, "=", type_expr, { type_mod }, line_end ;
type_expr      = type_ref | union ;
union          = value_name, "|", value_name, { "|", value_name } ;
type_mod       = "matches", regex | "min", number | "max", number ;

(* entity and field *)
entity_decl    = "entity", type_name, "{", line_end,
                 { blank | entity_member }, "}", [ line_end ] ;
entity_member  = field_decl | state_decl ;
field_decl     = name, field_type, { field_mod }, line_end ;
field_type     = type_ref | "[", type_ref, "]" ;
field_mod      = "required" | "unique" | "readonly" | "label" |
                 "default", literal ;
state_decl     = "state", name, value_name, "|", value_name,
                 { "|", value_name }, "initial", value_name, line_end ;

(* domain action; parameters and body are intentionally absent in v0 *)
action_decl    = "action", type_name, ".", name, ":",
                 state_set, "->", value_name,
                 { action_mod }, line_end ;
state_set      = value_name, { "|", value_name } ;
action_mod     = "confirm" | "allow", name_list | "goto", type_name ;

(* page and views *)
page_decl      = "page", type_name, [ "(", parameter, ")" ], "{", line_end,
                 { blank | page_member }, "}", [ line_end ] ;
parameter      = name, type_name ;
page_member    = allow_clause | list_view | detail_view | form_view ;
allow_clause   = "allow", name_list, line_end ;

list_view      = "list", type_name,
                 ( line_end | "{", line_end,
                   { blank | list_mod }, "}", [ line_end ] ) ;
detail_view    = "detail", name,
                 ( line_end | "{", line_end,
                   { blank | detail_mod }, "}", [ line_end ] ) ;
form_view      = "form", ( type_name | name ),
                 ( line_end | "{", line_end,
                   { blank | form_mod }, "}", [ line_end ] ) ;

list_mod       = ( "columns" | "search" | "filter" | "actions" ),
                 name_list, line_end
               | "sort", name, [ "asc" | "desc" ], line_end
               | "paginate", positive_int, line_end ;
detail_mod     = ( "fields" | "actions" ), name_list, line_end ;
form_mod       = "fields", name_list, line_end
               | "submit", ( "create" | "edit" ), line_end ;

(* role *)
role_decl      = "role", name, line_end ;

(* common *)
type_ref       = builtin_type | type_name ;
builtin_type   = "String" | "Int" | "Decimal" | "Bool" | "Date" | "DateTime" ;
name_list      = name, { ",", name } ;
literal        = string | decimal | integer | bool | value_name ;
bool           = "true" | "false" ;
number         = [ "-" ], digit, { digit }, [ ".", digit, { digit } ] ;
integer        = [ "-" ], digit, { digit } ;
decimal        = [ "-" ], digit, { digit }, ".", digit, { digit } ;
positive_int   = nonzero_digit, { digit } ;

type_name      = upper, { letter | digit | "_" } ;
value_name     = upper, { letter | digit | "_" } ;
name           = lower, { letter | digit | "_" } ;
regex          = "/", { regex_char }, "/" ;
string         = '"', { string_char }, '"' ;
comment        = "//", { non_newline_char } ;
blank          = [ comment ], newline ;
line_end       = [ comment ], ( newline | end_of_file ) ;
```

`type_name`と`value_name`は字句規則が同じだが、意味解析上のnamespaceが異なる。parser実装時に
Unicode identifierやregex flagを追加する場合は、この仕様と同じ変更で更新する。

v0 identifierの`upper`、`lower`、`letter`、`digit`はASCIIだけとする。`name`はlowercase-leading、
`type_name`と`value_name`はuppercase-leadingであり、残りにASCII letter、digit、`_`を使える。
したがって厳密なcamelCaseを字句的には強制しない。

string literalは改行を含めず、`\"`、`\\`、`\n`、`\r`、`\t`、`\b`、`\f`、`\uXXXX`、
`\UXXXXXXXX`だけをescapeとして認める。regex literal内では`\/`でdelimiterの`/`を表す。

lowercase keywordはすべてcontextual keywordであり、globalな予約語表は持たない。keywordは
該当productionの位置にあるときだけkeywordとして解釈され、それ以外では`name`として使える。
複数productionが始まり得る位置では、declarationを導入するkeywordを優先する。たとえばentity
member先頭の`state`は常にstate declarationを開始するため、field名には使えない。
したがって`action User.confirm: ...`の`confirm`は正当なaction名である。一方、組み込み型名は
type namespaceで予約され、同名の`type`を再宣言できない。

## 12. 静的検査

`forma check`は少なくとも次を検査する。

- 未宣言type、entity、role、field、state、action、pageの参照
- type mismatch、不正なmodifier対象、modifierの重複
- duplicate declarationと同一scope内のduplicate name
- 組み込み型名の再宣言
- state value以外を使う遷移、sourceとdestinationが同じ遷移
- `initial`が同じstate declarationのvalueを参照しないこと
- domain action名と標準action `create view edit delete`の衝突
- `form`からstateまたは`readonly` fieldを編集すること
- `search`にString基底でないfieldを指定すること
- 省略された`columns`/`detail fields`の展開結果が空になること
- relationを人間向けに提示するのに、参照先の有効な`label`がないこと
- 1 entityに複数の`label`がある、または`label`がrequiredなString基底scalarでないこと
- relation、collection、union、stateを`sort`に指定すること
- `Date`/`DateTime`またはそれらのnamed typeに`default`を指定すること
- `default` literalがnamed typeの有効な`matches`/`min`/`max` constraintを満たさないこと
- `required readonly` fieldに値のproducerとなる`default`がないこと
- 0以下の`paginate`、不正な`min`/`max`
- 標準actionのdestinationが存在しない、または複数存在すること
- list外の`delete`成功後に戻るlist pageが一意に解決できないこと
- parameterized pageへの`goto`に必要なentity contextがないこと

diagnosticはfile、source span、stable code、説明、可能なら修正案を含む。compilerはdomain field、
遷移、role、破壊的actionを推測で追加してはならない。

target profileの非対応は`forma check`ではなく§8.3のprofile compatibility checkで検出する。

## 13. 完全例

[`examples/users.forma`](../examples/users.forma)は、ここで定めた10個のプリミティブと閉じた
modifierだけでユーザー管理を記述する。

同例は[`Forma Language Design Principles`](language-design-principles.md)の可読性contractでも
監査している。監査の結果、プリミティブは追加せず、初期stateだけを`initial Pending`として明示した。

例には次が含まれる。

- Userのcreate/list/view/edit/delete
- name/email検索、status/team絞り込み、安定sort、20件window
- `Team.name label`によるrelation表示・filter・form picker
- detailとform
- Pending → Confirmed → Active → Suspendedのstate machine
- adminによるpage/action認可
- 標準action 4個とdomain action 4個の参照解決

## 14. v0の完了条件

### 14.1 target実装前に固定する境界contract

language semanticsはtarget実装へ進める状態にあるが、`forma build`と`forma conformance`を実装する前に
次の機械可読contractを固定する必要がある。これらはv1へ送らず、v0のblockerとして扱う。

| contract | 固定する内容 | 現在の状態 |
| --- | --- | --- |
| target profile manifest | profile ID/version、対応IR version、capability、generator/toolchain設定 | 未決定 |
| artifact protocol | generatorの許可されたwrite範囲、file manifest、dependency lock、build entrypoint、失敗形式 | 未決定 |
| conformance schema | fixture、operation、期待値、否定case、interaction stateの表現 | 未決定 |
| profile conformance adapter | data reset、role principal注入、operation実行、UI/runtime観測の共通interface | 未決定 |
| semantic identity / Source Map | IR nodeの安定identityとsource spanへの対応 | `forma/v0.4` IRと`forma/source-map/v0.1`としてreference front-endに実装済み |

model providerや具体的なframeworkはprofileの選択であり、言語全体のblockerではない。上記contractが
固定されるまでは、特定provider向けのpromptやreference applicationを規範interfaceとして
先行実装しない。

### 14.2 現行reference front-endとの差分

現在のGo front-endはdesign draft v0.4のsurface syntaxを部分実装し、Lexer、Parser、syntax AST、
主要な静的検査、`forma/v0.4` core Semantic IR、golden IR、Source Mapまで実装済みである。ただしdesign draft
v0.4に対して、少なくとも次は未実装である。

- §5.7の省略projectionを展開したlist/detail IR
- inherited constraintの合成、constraintに対するdefault検査、`required readonly`のproducer検査
- v0で閉じたstring/regex escape setの厳密な検査
- conformance contract、profile manifest/capability check、artifact protocol

したがってv0.4のgolden IRとSource Mapは現在の実装回帰には使えるが、v0.4 Semantic IR schemaの
完成形ではない。

### 14.3 Language/front-end v0

次をすべて満たしたとき、Forma language/front-end v0を完了とする。

1. 本書のEBNFをparseし、§12の検査を行う`forma check`がある。
2. 完全例をtarget-neutralなSemantic IR、Source Map、conformance contractへ正規化できる。
3. 同じsourceとfront-end versionからbyte-identicalなSemantic IRとconformance contractを生成できる。
4. parse、意味解決、IR、conformance contract、test oracleがAIまたはnetwork推論に依存しない。
5. default、projection、action解決、navigationなどsourceから導出した意味を、人間向けに展開して
   対応するsource declarationとともに確認できる。

### 14.4 End-to-end v0

§14.1のcontractを固定し、language/front-end v0に加えて次を満たしたとき、Forma end-to-end v0を
完了とする。

1. 一つのreference profileが完全例から、list/detail/create/edit/delete、query能力、状態遷移、
   relation picker、成功後navigation、認可、必須interaction stateを持つ実行可能なapplicationを
   生成する。generatorはAIを利用してよい。
2. 正常系、不正遷移、権限拒否、validation failure、relation label/selection、成功後navigation、
   stable paginationを、Semantic IRから決定的に導出したconformance contractで検証する。
3. conformanceの期待値と合否判定がAIまたはnetwork推論に依存しない。
4. target codeを手編集せず、同じsource of truthから破棄・再生成できる。

reference web profileなら、概ね次のcapabilityを生成する。ただしHTTP shape自体は規範ではない。

```text
list users
create user
view user
edit user
delete user
confirm / activate / suspend / reinstate user
search / filter / sort / paginate users
display / filter / select team by Team.name
navigate deterministically after create / edit / delete
enforce field, state, and role constraints
```

### 14.5 Formaの中心仮説の検証

同じSemantic IRを二つの意味の異なるtarget profileへ生成し、同じtarget-neutral conformance suiteを
通過したとき、「実装方法ではなくapplication intentを記述する」という中心仮説が検証されたと
みなす。生成コードの類似性ではなく、観測可能な意味の同等性を評価する。最初から一つのprofileに
最適化しないよう、たとえばSPAとserver-rendered applicationのように実装モデルが離れた二つを選ぶ。
一つのreference profileでend-to-end v0を実装できても、この二profile比較を通るまでは中心仮説を
検証済みとはみなさない。

## 15. v0以降へ残す問い

- inverse relationとcascade deleteをどう記述するか
- runtime由来fieldをどのeffect modelで表すか
- join、aggregate、derived listをどう表すか
- domain actionの引数とformをどう統合するか
- 明示routeが本当に必要か
- [表側の会員登録とidentity](public-membership-proposal.md)をどのsemantic modelで表し、identity providerと
  login/logoutのどこまでを言語へ入れるか
- notification、background job、fileを共通effectとして扱えるか
- page/view identityを使ったversioned profile extensionを、安全なescape hatchとして設けるか
- derived value、`invariant`、action preconditionを式で表し、statementを持たない境界を保てるか
- 表示文言と設計意図を`title`やdoc commentとしてsourceとSemantic IRに載せるか
- 複数entityをまたぐ副作用を、手続き型へ退行せずどのeffect modelで表すか
- 状態を変えないactionを宣言できるようにするか。現在は§12の遷移検査が拒否する
- observable domain occurrenceをactionから導出するか、明示するか
  （[Order Approval, Inventory, and Effect Proposal](order-approval-proposal.md)）
- `forma diagram`でstate machineやentity graphを生成し、図をsource of truthにせず利用できるか
- Ruby/RailsやAWSなどの人間によるarchitecture選定を、application semanticsから分離した
  [Architecture Manifest](architecture-manifest.md)としてどう表すか

v1の式レイヤはまだ決定事項ではない。まず注文・明細・在庫のような実例を`examples/`へ書き、
導出値、invariant、state以外のaction preconditionだけで何が表現でき、どこからeffectが必要に
なるかを確認してからEBNFを定める。この実例は[`examples/orders.forma`](../examples/orders.forma)と
[Order Approval, Inventory, and Effect Proposal](order-approval-proposal.md)で着手した。
