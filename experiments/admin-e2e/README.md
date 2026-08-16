# Admin E2E Generation Experiment

Status: executable experiment — not a production profile or a language decision

このexperimentは、[`app.forma`](app.forma)だけをapplication intentとして、
管理画面のユーザー一覧・詳細・編集flowを持つstandalone artifactを生成できるか検証する。

完全例の[`examples/users.forma`](../../examples/users.forma)は、`unique`、`matches`、create、delete、
state action、search、filter、sort、paginationまで宣言する。このprofileはそれらをまだ実現しないため、完全例を入力すると
生成errorにする。未実現intentを無視して部分的なartifactを成功扱いにはしない。

## 入力の分担

| input | 所有するもの |
| --- | --- |
| `app.forma` | entity、field、relation、page、list/detail/form、view/edit action、role access |
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
2. User一覧にfixture 3件と、Formaの`columns name, email, team, status`が表示される。
3. Aliceの詳細で、Formaの`fields name, email, team, status`が表示される。
4. 編集formにFormaの`fields name, email, team`だけが表示される。
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

## このexperimentで検証しないもの

- Signup、Signin、sessionなどのIdentity semantics
- database durability、concurrency、migration
- production authentication、CSRF、deployment
- Effect、background job、email
- AI modelをAPIから呼ぶgeneration protocol

このprofileはSemantic IRが管理画面flowを十分に伝えられるかを先に検証するdeterministic loweringである。
AI generatorの再現性やmodel差は、artifact protocolとprofile boundaryを固定した後の別experimentにする。

## 実測結果

2026-08-16に、reference front-endと`go-stdlib-admin/v0` profileで次を確認した。

- repository全体と生成artifactの`go test ./...`が成功する
- 生成artifactはGo標準libraryだけでbuild・起動できる
- browserでadminを選び、User一覧からAliceの詳細・編集へ遷移できる
- edit viewにはFormaが宣言した`name`、`email`、`team`だけが現れる
- nameとTeamを保存すると、詳細と一覧の両方へ反映される
- anonymous principalでUsersを開くと`403 Access denied`になる
- browser consoleのwarningとerrorは0件
- 同じversioned inputsから`-force`で再生成した4 fileをtest内でbyte比較し、すべて一致する
- generator testでUserEditの`fields`を変えると、lowering後のedit fieldsも追随する
- validation failureは`422`になり、後続fieldの入力も再表示される
- dangling relation fixtureと、field constraint・未消費viewを含む未実現intentはartifact生成前に拒否される
- unionはselectと保存境界のmembership検査へloweringされ、list 0件時はempty stateが表示される
- action/submitのnavigationとaccessは名前から再推測せず、Semantic IRのcontractからloweringされる
- edit submitはpending中にbuttonを無効化する。tokenは予測不能なruntime値でrecordとtest principal roleへ束縛し、
  scopeごとの発行済み・完了済みtokenをそれぞれ最大5件に制限する
- 保持中の成功済みtokenの再送は同じ詳細画面へ`303`で戻し、処理中tokenは`409`にする。
  未知・期限切れtokenでは入力を保持し、新tokenを持つ生成UIの`409` formを再表示する
- submission tokenを使うedit pageに有限な`allow` roleがなければ、artifact生成前に拒否する
- stateのfixture値はunionと同じclosed-set検査を通り、未宣言値を拒否する

### 分かったこと

このflowに限れば、Forma sourceとSemantic IRには、表示するentity・field、relation label、
list/detail/editの区別、view/edit action、page accessを伝えるだけの情報がある。一方、次はFormaではなくprofileが
決める必要があった。

- URL shapeと画面title・button copy・visual design
- persistence方式とfixture投入方法
- principalをruntimeへ渡すadapter
- HTTP server、form encoding、redirectという実装方式

server-rendered profileでは、`loading`はbrowser navigationそのものへ委ね、専用のloading viewは持たない。
formの`pending`はsubmit開始時のbutton無効化と表示変更、`failure`はHTTP error responseまたはform内feedback、
`ready`・`invalid`・listの`empty`は生成HTMLとして表す。IRの`InteractionStates`はartifact specへ保持し、
未知のstateがあれば生成errorにする。

submission tokenは`PreventDuplicateDispatch`を実現するprofile固有のidempotency機構であり、CSRF防御ではない。
このtest principal adapterは個人identityを持たずroleだけを表すため、tokenのprincipal bindingもrole単位である。
そのため、token scopeを有限にできる`allow` roleをedit pageに要求する。これはForma一般の制約ではなく、
`go-stdlib-admin/v0` profileのprincipal adapterとtoken実装に由来する制約である。

### IR property coverage

| IR | 実現 | 明示的に拒否・委譲 | このprofileでは該当なし |
| --- | --- | --- | --- |
| `IRField` | Name、Type、Required、Label、to-one Relation | Unique、Collection、Defaultは拒否。Readonlyのform使用はfront-endが拒否 | — |
| `IRType` | Base、Variants（閉じたselect。fixture投入・保存・表示でmembershipを検査） | Constraintsは拒否 | — |
| `IRState` | Name、Values（state fixtureを閉集合として検査） | 未宣言のstate値はfixture投入時に拒否 | Initial（create flowを持たず、fixtureはrequiredなstate値を明示する） |
| `IRView` | Kind、Entity、Binding、Mode、Fields、Relations、ready/invalid/pending/failure、listのempty | Search、Filters、Sort、PageSize、未対応view/action/state、および`allow`のないedit viewは拒否 | loadingはbrowser navigationへ委譲。detail対象が無い場合はempty viewではなく404 |
| `IRActionRef` | ID、Name、Kind、TargetPage、SuccessPage、Access。link先と表示可否をcontractから決定 | 未対応action、解決不能なnavigation、必要なinteraction保証の欠落は拒否 | GET navigationのduplicate dispatchは状態を変更しない |
| `IRSubmitIntent` | ID、Action、Success、Access、RecheckAccess、PreventDuplicateDispatch、FailureFeedback。有界な一回限りtoken、pending表示、validationの`422` feedback、duplicateの`303`/`409`、success redirectへlowering | 未対応action/navigation/interaction保証は拒否 | — |

したがって、今回の成功は「管理画面全般を生成できた」という結論ではない。v0完全例に必要なunique、
matches、to-many、default、create、delete、search、filter、pagination、state action、negative case、
独立したConformance Contractはまだ残る。
また生成testは同じprofileが出力しているため、target非依存なoracleの代わりにはならない。
