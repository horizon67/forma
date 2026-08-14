# Forma v0 — プリミティブと言語仕様

Status: design draft 0.3

> Forma is a high-level application programming language for expressing what
> software should be, not how it should be implemented.

```text
自然言語
  ↓ AI（任意の翻訳器）
Forma source
  ↓ deterministic compiler
Semantic IR
  ├─ Web
  ├─ Server
  ├─ Native
  └─ Test
```

AIは自然言語からFormaへの翻訳に利用できるが、規範的なコンパイル経路には含めない。
同じFormaソース、target profile、コンパイラバージョンからは、常に同じ生成結果を得なければ
ならない。

本書をForma v0の規範仕様とする。

## 1. v0が検証する仮説

Forma v0が検証するのは、アプリケーションの「何が存在し、誰が、何を見て、どう変えられるか」
を短く記述し、実装詳細を含まないSemantic IRへ決定的に変換できるか、という一点である。

最初の対象は、次を備えたデータ管理アプリケーションとする。

- entityの一覧・詳細・作成・編集・削除
- 検索・絞り込み・ソート・ページング
- 状態遷移と、状態に応じたactionの表示・拒否
- roleによるpageとactionの認可
- loading、empty、pending、validation error、failureの各状態

`list User`はHTML tableやReact componentを意味しない。「Userの集合を利用者に提示する」
という意図である。Webではtable、mobileではlistとinfinite scrollなど、具体的な実現方法は
target profileが決める。

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
    state status Pending | Confirmed | Active | Suspended
}
```

最初の値が初期状態である。1 entityにつきstateは0個または1個。state名はfieldと同じように
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

union typeのvariantとstate valueは`UpperCamelCase`を使う。値のsource orderは安定した
presentation orderとして保持するが、大小比較やbusiness priorityを意味しない。

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

### 5.1 type修飾子

- `matches /.../`は`String`を基底とする型にだけ使える。
- `min N`と`max N`は`Int`または`Decimal`を基底とする型にだけ使える。
- 同じ修飾子は1宣言に1回まで。`min`は`max`以下でなければならない。

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

stateは`detail fields`には指定できるが、`form fields`には指定できない。状態変更は必ずdomain
actionを通す。

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

## 7. 実行時に必須の意味

簡潔なFormaソースから省略されても、次は全targetに必須である。

- list/detailのloading、empty、failure状態
- formの初期値、client feedback、authoritative validation error、pending状態
- actionのpending中のduplicate dispatch防止、成功後の表示整合、failure feedback
- state preconditionとrole ruleのserver-side相当境界での再検査
- field constraintとunique constraintのauthoritativeな検証
- query traversalの安定性

具体的なcomponent、HTTP、cache invalidation、再取得、transaction、database constraint、test
frameworkはtarget profileが選ぶ。これらをForma sourceやSemantic IRのUI/transport固有ノードに
漏らしてはならない。

## 8. target profileとSemantic IR

コンパイル入力は次の3つである。

```text
Forma source + target profile + compiler version -> generated application
```

target profileは、component、route形状、transport、persistence、pagination方式、concurrency、
cache更新、authentication integration、test frameworkを所有する。

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

Semantic IRは、解決済みsymbol、型、constraint、source span、認可、状態precondition、必須の
interaction stateを保持する。React component、Rails controller、HTTP verb、SQLなどは保持しない。

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

重要でないからではなく、identity、effect、infrastructure、queryという次の設計軸を必要とし、
v0の仮説検証範囲を広げるためである。

## 10. 決定事項

1. v0のプリミティブは`type entity field state action page list detail form role`の10個。
2. 組み込み型は`String Int Decimal Bool Date DateTime`の6個。
3. field関連はfield typeで表し、`relation`などの別プリミティブを作らない。
4. stateは名前を明示し、宣言名と参照名を一致させる。例: `state status ...`。
5. 1 entityにつきstateは最大1個。最初のvalueを初期状態とする。
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
                 { "|", value_name }, line_end ;

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

type_name      = upper, { letter | digit } ;
value_name     = upper, { letter | digit } ;
name           = lower, { letter | digit | "_" } ;
regex          = "/", { regex_char }, "/" ;
string         = '"', { string_char }, '"' ;
comment        = "//", { non_newline_char } ;
blank          = [ comment ], newline ;
line_end       = [ comment ], newline ;
```

`type_name`と`value_name`は字句規則が同じだが、意味解析上のnamespaceが異なる。parser実装時に
escape、Unicode identifier、regex flagを追加する場合は、この仕様と同じ変更で更新する。

lowercase keywordはすべてcontextual keywordであり、globalな予約語表は持たない。keywordは
該当productionの位置にあるときだけkeywordとして解釈され、それ以外では`name`として使える。
したがって`action User.confirm: ...`の`confirm`は正当なaction名である。一方、組み込み型名は
type namespaceで予約され、同名の`type`を再宣言できない。

## 12. 静的検査

`forma check`は少なくとも次を検査する。

- 未宣言type、entity、role、field、state、action、pageの参照
- type mismatch、不正なmodifier対象、modifierの重複
- duplicate declarationと同一scope内のduplicate name
- 組み込み型名の再宣言
- state value以外を使う遷移、sourceとdestinationが同じ遷移
- domain action名と標準action `create view edit delete`の衝突
- `form`からstateまたは`readonly` fieldを編集すること
- `search`にString基底でないfieldを指定すること
- relationを人間向けに提示するのに、参照先の有効な`label`がないこと
- 1 entityに複数の`label`がある、または`label`がrequiredなString基底scalarでないこと
- relation、collection、union、stateを`sort`に指定すること
- `Date`/`DateTime`またはそれらのnamed typeに`default`を指定すること
- 0以下の`paginate`、不正な`min`/`max`
- 標準actionのdestinationが存在しない、または複数存在すること
- list外の`delete`成功後に戻るlist pageが一意に解決できないこと
- parameterized pageへの`goto`に必要なentity contextがないこと
- target profileが記述された意図を実現できないこと

diagnosticはfile、source span、stable code、説明、可能なら修正案を含む。compilerはdomain field、
遷移、role、破壊的actionを推測で追加してはならない。

## 13. 完全例

[`examples/users.forma`](../examples/users.forma)は、ここで定めた10個のプリミティブと閉じた
modifierだけでユーザー管理を記述する。

例には次が含まれる。

- Userのcreate/list/view/edit/delete
- name/email検索、status/team絞り込み、安定sort、20件window
- `Team.name label`によるrelation表示・filter・form picker
- detailとform
- Pending → Confirmed → Active → Suspendedのstate machine
- adminによるpage/action認可
- 標準action 4個とdomain action 4個の参照解決

## 14. v0の完了条件

### Language/compiler v0

次をすべて満たしたとき、Forma language/compiler v0を完了とする。

1. 本書のEBNFをparseし、§12の検査を行う`forma check`がある。
2. 完全例をtarget-neutralなSemantic IRへ正規化できる。
3. 同じsource、profile、compiler versionからbyte-identicalなartifactを生成できる。
4. 一つのreference profileが完全例から、list/detail/create/edit/delete、query能力、状態遷移、
   relation picker、成功後navigation、認可、必須interaction stateを持つ実行可能なapplicationを
   生成する。
5. 正常系、不正遷移、権限拒否、validation failure、relation label/selection、成功後navigation、
   stable paginationを生成testまたはconformance testで検証する。
6. 規範的なcompile/build/test経路がAIまたはnetwork推論に依存しない。

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

### Formaの中心仮説の検証

同じSemantic IRを二つの意味の異なるtarget profileへ生成し、同じtarget-neutral conformance suiteを
通過したとき、「実装方法ではなくapplication intentを記述する」という中心仮説が検証されたと
みなす。生成コードの類似性ではなく、観測可能な意味の同等性を評価する。

## 15. v0以降へ残す問い

- inverse relationとcascade deleteをどう記述するか
- runtime由来fieldをどのeffect modelで表すか
- join、aggregate、derived listをどう表すか
- domain actionの引数とformをどう統合するか
- 明示routeが本当に必要か
- identity providerとlogin/logoutを言語へ入れるか、profileへ残すか
- notification、background job、fileを共通effectとして扱えるか
