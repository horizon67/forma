# Admin E2E Generation Experiment — Frozen Prototype

Status: frozen historical prototype — non-normative, not the planned generation architecture

このprototypeは、Forma sourceから管理画面flowに必要な意味を抽出できるか検証するために作った。
決定的なGo generator、target profile、artifact protocol、byte-identical regenerationをForma coreへ
採用する提案ではない。新しいcapabilityや二つ目のframeworkを追加せず、実測結果を再現するcodeとして
凍結する。

現在の正式な方向は、Resolved IntentとAcceptance Factsを
[AI coding agentへ渡して通常のrepositoryを変更するmodel](../../docs/agent-generation.md)である。

このexperimentは、[`app.forma`](app.forma)だけをapplication intentとして、
管理画面のユーザー一覧・詳細・編集flowを持つstandalone artifactを生成できるか検証する。

完全例の[`examples/users.forma`](../../examples/users.forma)は、`unique`、`matches`、create、delete、
state action、search、filter、sort、paginationまで宣言する。このprofileはそれらをまだ実現しないため、完全例を入力すると
生成errorにする。未実現intentを無視して部分的なartifactを成功扱いにはしない。

## 入力の分担

| input | 所有するもの |
| --- | --- |
| `app.forma` | entity、field、union、relation、page、list/detail/form、view/edit action、role access。Conformance Contractの唯一の意味入力 |
| `profile.json` | Go標準library、memory persistence、test principal adapterという実験architecture |
| `fixtures.json` | E2Eで操作するtest data。application semanticsではない |

生成先は`.forma-build/admin-e2e`で、repositoryのsource of truthには含めない。生成artifactは手編集せず、
必要なら削除して同じ入力から作り直す。

## 実行

```bash
go run ./experiments/admin-e2e/cmd/generate \
  -source experiments/admin-e2e/app.forma \
  -profile experiments/admin-e2e/profile.json \
  -fixtures experiments/admin-e2e/fixtures.json \
  -out .forma-build/admin-e2e \
  -force

cd .forma-build/admin-e2e
go test ./...
go run .
```

default URLは`http://127.0.0.1:4317`。`FORMA_ADDRESS`で変更できる。

## Acceptance flow

1. test principal adapterで`admin`を選ぶ。
2. User一覧にfixture 3件と、Formaの`columns name, email, team, plan, status`が表示される。
3. Aliceの詳細で、Formaの`fields name, email, team, plan, status`が表示される。
4. 編集formにFormaの`fields name, email, team, plan`だけが表示される。
5. nameを変更すると詳細・一覧へ反映される。
6. required fieldのvalidation failureを`422`にし、入力済みの他fieldを失わない。
7. admin以外のprincipalではUsers pageを`403`にする。
8. artifactを削除・再生成してもbyte-identicalな結果になる。
9. Formaのedit fieldsを変更すると、生成artifactだけを再生成してformへ反映される。
10. union fieldを閉じたselectとして表示し、fixture投入時と保存時にmembershipを検査する。
11. 0件のlistで`empty` interaction stateを表示する。
12. profileが実現しないfield constraint、to-many、default、view、search/filter/sort/paginate/actionを生成前に拒否する。
13. action/submitのtarget、success navigation、access、failure feedbackをIRからartifactへ保持する。
14. edit submitをpending表示と一回限りtokenで保護する。保持中の成功済みsubmissionの再送は同じsuccess redirectへ着地し、
    処理中tokenは`409`で拒否する。未知・期限切れtokenは入力を保持した新token付きの`409` formへ戻す。
15. state値を閉じた集合としてfixture投入時に検査する。
16. role-only adapterでsubmission tokenのscopeを有限にできない、`allow`のないedit pageは生成時に拒否する。
17. Resolved Intentからbyte-identicalな`conformance.json`を生成し、target非依存な7 caseをadmin adapterで通す。

## このexperimentで検証しないもの

- Signup、Signin、sessionなどのIdentity semantics
- database durability、concurrency、migration
- production authentication、CSRF、deployment
- Effect、background job、email
- AI modelをAPIから呼ぶgeneration protocol

このprototypeは、現在Resolved Intentと呼ぶ解決済み出力が管理画面flowを十分に伝えられるか検証した
deterministic loweringである。このloweringを拡張せず、今後はcoding agentが通常のrepositoryへ実装する。

## 実測結果

2026-08-16に、reference front-endと`go-stdlib-admin/v0` profileで次を確認した。

- repository全体と生成artifactの`go test ./...`が成功する
- 生成artifactはGo標準libraryだけでbuild・起動できる
- browserでadminを選び、User一覧からAliceの詳細・編集へ遷移できる
- edit viewにはFormaが宣言した`name`、`email`、`team`、`plan`だけが現れる
- name、Team、Planを保存すると、詳細と一覧の両方へ反映される
- anonymous principalでUsersを開くと`403 Access denied`になる
- browser consoleのwarningとerrorは0件
- 同じversioned inputsから`-force`で再生成した7 fileをtest内でbyte比較し、すべて一致する
- generator testでUserEditの`fields`を変えると、lowering後のedit fieldsも追随する
- validation failureは`422`になり、後続fieldの入力も再表示される
- dangling relation fixtureと、field constraint・未消費viewを含む未実現intentはartifact生成前に拒否される
- unionはselectと保存境界のmembership検査へloweringされ、list 0件時はempty stateが表示される
- action/submitのnavigationとaccessは名前から再推測せず、Resolved Intentのcontractからloweringされる
- edit submitはpending中にbuttonを無効化する。tokenは予測不能なruntime値でrecordとtest principal roleへ束縛し、
  scopeごとの発行済み・完了済みtokenをそれぞれ最大5件に制限する
- 保持中の成功済みtokenの再送は同じ詳細画面へ`303`で戻し、処理中tokenは`409`にする。
  未知・期限切れtokenでは入力を保持し、新tokenを持つ生成UIの`409` formを再表示する
- submission tokenを使うedit pageに有限な`allow` roleがなければ、artifact生成前に拒否する
- stateのfixture値はunionと同じclosed-set検査を通り、未宣言値を拒否する
- `conformance.json`はroute、HTTP、HTML、submission tokenを含まず、role別のlist許可・拒否、妥当な更新、
  required拒否と入力保持、union閉集合拒否をsemantic IDで表す
- shared runnerとadmin固有adapterを分離し、positive 2件とnegative 5件のtarget-neutral caseが
  生成artifactで成功する

### 分かったこと

このflowに限れば、Forma sourceとResolved Intentには、表示するentity・field、relation label、
list/detail/editの区別、view/edit action、page accessを伝えるだけの情報がある。一方、次はFormaではなくprofileが
決める必要があった。

- URL shapeと画面title・button copy・visual design
- persistence方式とfixture投入方法
- principalをruntimeへ渡すadapter
- HTTP server、form encoding、redirectという実装方式

server-rendered profileでは、`loading`はbrowser navigationそのものへ委ね、専用のloading viewは持たない。
formの`pending`はsubmit開始時のbutton無効化と表示変更、`failure`はHTTP error responseまたはform内feedback、
`ready`・`invalid`・listの`empty`は生成HTMLとして表す。Resolved Intentは観測可能な`empty`・`invalid`・
`failure`だけを持ち、`loading`・`ready`・`pending`はこのprofileのartifact specで追加する。

submission tokenは「1回の論理mutationを複数回適用しない」という共通保証を実現するprofile固有の
idempotency機構であり、CSRF防御ではない。
このtest principal adapterは個人identityを持たずroleだけを表すため、tokenのprincipal bindingもrole単位である。
そのため、token scopeを有限にできる`allow` roleをedit pageに要求する。これはForma一般の制約ではなく、
`go-stdlib-admin/v0` profileのprincipal adapterとtoken実装に由来する制約である。

### Resolved Intent property coverage

| IR | 実現 | 明示的に拒否・委譲 | このprofileでは該当なし |
| --- | --- | --- | --- |
| `IRField` | Name、Type、Required、Label、to-one Relation | Unique、Collection、Defaultは拒否。Readonlyのform使用はfront-endが拒否 | — |
| `IRType` | Base、Variants（閉じたselect。fixture投入・保存・表示でmembershipを検査） | Constraintsは拒否 | — |
| `IRState` | Name、Values（state fixtureを閉集合として検査） | 未宣言のstate値はfixture投入時に拒否 | Initial（create flowを持たず、fixtureはrequiredなstate値を明示する） |
| `IRView` | Kind、Entity、Binding、Mode、Fields、invalid/failure、listのempty | Search、Filters、Sort、PageSize、未対応view/action/state、および`allow`のないedit viewは拒否 | loading/ready/pendingはprofile側。detail対象が無い場合はempty viewではなく404 |
| `IRActionRef` | ID、Name、Kind、TargetPage、SuccessPage、Access。link先と表示可否をintentから決定 | 未対応actionと解決不能なnavigationは拒否 | GET navigationのduplicate dispatchは状態を変更しない |
| `IRSubmitIntent` | ID、Action、Success、Access | 未対応action/navigationは拒否 | — |
| profile-owned interaction | RecheckAccess、PreventDuplicateDispatch、FailureFeedback、有界な一回限りtoken、pending表示、validationの`422` feedback、duplicateの`303`/`409`、success redirect | Forma一般の実現機構としては扱わない | — |

### Conformanceとprofile testの分離

[`../conformance`](../conformance/README.md)のbuilderはResolved Intentだけから`conformance.json`を生成する。
shared runnerは`query-view`、`submit-form`、明示的なprincipal kind、許可・拒否・更新結果、subject集合、
保存値、入力保持というtarget非依存な語彙だけを扱う。admin adapterがそれらをURL、HTTP、HTML、
memory storeへ変換する。

一方、生成される`main_test.go`は`/users/user-alice/edit`、`422`、`303`、`_forma_submission`など、
このprofileが選んだ実装を検査する。Formaの意味の期待値は`main_test.go`からではなく
`conformance.json`から与えられる。

したがって、今回の成功は「管理画面全般を生成できた」という結論ではない。v0完全例に必要なunique、
matches、to-many、default、create、delete、search、filter、pagination、state actionと、それらを覆う
Conformance Contractはまだ残る。今回独立できたcontractはaccess、edit persistence、required、unionの
4 semantic axisだけであり、2つ目のprofileへ同じcontractを適用する比較も未実施である。
