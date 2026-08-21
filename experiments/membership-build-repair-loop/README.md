# Membership Build Repair Loop Experiment

Status: measured — a controlled Go **build** failure on the membership target, a
`stage: build` / `status: failed` Generation Feedback, and a repair back to
85/85. Not a claim of automatic agent orchestration or general repair.

[`../membership-repair-loop`](../membership-repair-loop/README.md) measured a
**test** failure: an assertion ran and rejected the implementation, so exactly one
Acceptance Fact went to `failed` and Source Map gave it five related intent nodes.
This experiment measures the other half of Milestone 5's
"compiler errorとrepository build/test failureの明確な分離": the target does not
compile, **no assertion that observes an Acceptance Fact runs**, and the feedback
has to say so without inventing a rejected Fact. Tests elsewhere in the module do
still run — 25 top-level tests and 12 subtests passed in the four packages that
compiled — and two of them are references of a Fact. A Fact needs **all** of its
references to complete, so a Fact split across a package that built and one that
did not is still `not-run`, and no Fact may be called `passed`.

The distinction matters because the two failures license different next steps. A
failed assertion is evidence about behaviour and points at intent. A build failure
is evidence about nothing except the compiler; treating it as a failed Fact would
tell an agent that a requirement was violated when no requirement was ever
evaluated.

## 入力の分担

| input | 所有するもの |
| --- | --- |
| [`../membership-agent-e2e/app.forma`](../membership-agent-e2e/app.forma) | 変更しない。何を成立させるか |
| [`../membership-agent-e2e/generation-request.json`](../membership-agent-e2e/generation-request.json) | 変更しない。85 Facts、3 Review Requirements |
| [`../membership-agent-e2e/target/forma.implementation.yaml`](../membership-agent-e2e/target/forma.implementation.yaml) | 変更しない。implementation policy |
| coverage map in [`../membership-agent-e2e/cmd/feedback/coverage.go`](../membership-agent-e2e/cmd/feedback/coverage.go) | 変更しない。fact → test reference |
| target tests | 変更・弱化しない |
| [`fault.patch`](fault.patch) | `signIn`のcredential照合をarity違いにするcontrolled compile error |
| target `web.(*Server).signIn` | repair対象。compiler diagnosticだけを根拠に直す |

## 実行

```bash
git apply experiments/membership-build-repair-loop/fault.patch
go run ./experiments/membership-agent-e2e/cmd/feedback
# 旧succeeded feedbackは撤回され、failed feedbackがatomicに公開される
go run ./cmd/forma verify \
  --repository experiments/membership-agent-e2e/target \
  --baseline internal/agentrequest/testdata/admin.incremental.request.json \
  experiments/membership-agent-e2e/generation-request.json \
  experiments/membership-agent-e2e/target/generation-feedback.json
# → exit 1: Generation Feedback status is failed

git apply -R experiments/membership-build-repair-loop/fault.patch
go run ./experiments/membership-agent-e2e/cmd/feedback
go run ./cmd/forma verify \
  --repository experiments/membership-agent-e2e/target \
  --baseline internal/agentrequest/testdata/admin.incremental.request.json \
  experiments/membership-agent-e2e/generation-request.json \
  experiments/membership-agent-e2e/target/generation-feedback.json
```

broken codeは最終成果へ残さない。[`fault.patch`](fault.patch)と
[`generation-feedback.failed.json`](generation-feedback.failed.json)が再現用artifactである。

## 入れたfault

```go
-			authenticated = stored.Matches(credential)
+			authenticated = stored.Matches(credential, user.ID)
```

`identity.Credential.Matches`は`func (c Credential) Matches(value string) bool`である。
1行、1 packageのarity違いであり、意味の書き換えではない。実装編集を途中で失敗した状態を
最小に再現する。

## この実験が最初に見つけたこと

fault適用後の最初の実行で、feedback generatorは`stage: build`と81件の`not-run`までは
正しく出したが、**diagnosticsからGo compiler errorが完全に落ちていた**。

```text
FAIL	example.com/forma-admin-target/cmd/server [build failed]
FAIL	example.com/forma-admin-target/cmd/server
FAIL	example.com/forma-admin-target/internal/web [build failed]
FAIL	example.com/forma-admin-target/internal/web
```

原因はschemaではなくgeneratorのparserにある。`go test -json`はcompiler errorを
package eventではなく`ImportPath`付きの別recordとして出す。

```json
{"ImportPath":"…/internal/web","Action":"build-output","Output":"# …/internal/web\n"}
{"ImportPath":"…/internal/web","Action":"build-output","Output":"internal/web/membership.go:309:47: too many arguments in call to stored.Matches\n"}
{"ImportPath":"…/internal/web","Action":"build-output","Output":"\thave (string, string)\n"}
{"ImportPath":"…/internal/web","Action":"build-output","Output":"\twant (string)\n"}
{"ImportPath":"…/internal/web","Action":"build-fail"}
```

`Package`も`Test`も空なので、既存parserはこれをどのpackageの出力にも束ねられず捨てていた。
残るのは`[build failed]`という症状だけで、agentが修復に使える情報が1つもない。
`stage: build`が当たっていたのは、package levelの`fail` eventが持つ`FailedBuild`を見ていたためで、
compiler errorを読めていたからではない。

既存testがこれを捕まえられなかったのは、fixtureがcompiler errorをpackage `output` eventへ
畳んだ手書きの形だったからである。実toolchainが出さない形を検査していた。
`buildFailureJSON`は実行時のeventをそのまま写したものへ置き換えた。

修正は`ImportPath`を持つeventを`build-output` / `build-fail`として取り込み、compiler errorを
diagnosticsの先頭へ置くだけで、schemaにもForma semanticsにも触れていない。

## 実測したloop

```text
fault.patch
  → internal/web が compile 失敗、cmd/server も build failed
  → stage: build / status: failed
  → webに依存する81 Factsはnot-run、storeだけで観測する新しい4 transition Factsはpassed
  → relatedIntentNodes は空。failed Fact がないので Source Map を引く根拠がない
  → policyCoverage も空。この run は policy を1件も検証していない
  → diagnostics に compiler error 4行 + 症状 4行
  → forma verify --baseline → exit 1
  → diagnostic の "want (string)" と identity.Credential.Matches の signature だけを根拠に
     引数を1つ戻す
  → forma verify --baseline は 85/85、41 distinct tests
```

失敗時feedbackの要点:

```text
schema  forma/generation-feedback/v0alpha2
stage   build
status  failed
command cd experiments/membership-agent-e2e/target && go test -count=1 -json ./...
summary The target did not compile, so any fact whose required test
        references did not all complete is not-run: 0 passed, 0 failed,
        4 passed, 0 failed, 81 not-run. The compiler diagnostic is in diagnostics.
        No implementation policy was verified in this run, so this feedback
        reports no policy coverage.
policyCoverage  なし
diagnostics
        # example.com/forma-admin-target/internal/web
        internal/web/membership.go:309:47: too many arguments in call to stored.Matches
        have (string, string)
        want (string)
        FAIL	example.com/forma-admin-target/cmd/server [build failed]
        FAIL	example.com/forma-admin-target/cmd/server
        FAIL	example.com/forma-admin-target/internal/web [build failed]
        FAIL	example.com/forma-admin-target/internal/web
```

## 失敗時のcoverage集計

```text
passed    4
failed    0
not-run  81
```

`internal/store`、`internal/identity`、`internal/clock`、`internal/mail`の4 packageはbuildも
実行も成功しており、25 top-level testsと12 subtestsがpassしている。それでも`passed`が0件なのは、
残る81 Factsすべてが少なくとも1件のtest referenceを`internal/web`へ持つためである。新しい4件の
`User.activate` transition Factは`internal/store`だけを参照するため、このbuild failureでもpassedになる。

| Factが参照するpackage | Facts |
| --- | --- |
| `internal/store` のみ | 4 |
| `internal/web` のみ | 77 |
| `internal/web` + `cmd/server` | 2 |
| `internal/web` + `internal/store` | 1 |
| `internal/web` + `internal/identity` | 1 |

これはcoverage mapの弱点ではなく観測構造の事実である。Acceptance Factsはすべて
HTTP surface越しに観測される設計なので、web packageのbuild failureはcoverage全体を
盲目にする。1つのpackageのcompile errorがwebに依存する81件すべての観測可能性を落とす、というのが
この構造から出る帰結である。

`not-run`になる条件は「失敗packageだけで観測されるFact」より広い。`factResult`はFactの
test referenceが**すべて**完了することを要求するので、成功packageと失敗packageの両方へ
referenceを持つ次の4件も`not-run`である。片方は実際に走ってpassしている。

```text
identity/UserAccount/operation/verify/expiry/boundary
  internal/web/…#TestExpiryBoundaryIsCheckedOnBothSides          not run
  internal/identity/…#TestEvidenceExpiryIsAClockRelation         passed
page/UserEdit/view/form/edit/User/submit/validation/unique/email
  internal/web/…#TestEditValidationAndFailure                    not run
  internal/store/…#TestUpdateUserEnforcesUniqueEmail             passed
page/SignUp/identity/register/UserAccount/access/allowed/anonymous
page/Users/view/list/User/access/denied/anonymous
  いずれも cmd/server 側 reference も build failed
```

部分的に観測できたFactを`passed`へ繰り上げないのは、Factが要求する観測の一部しか
成立していないためである。

failed feedbackは`policyCoverage`を1件も持たない。最初の測定では成功時と同じ
`satisfied` 2件・`deviated` 1件を出していたが、これは検証されない主張だった。
`ValidateCompletion`は`feedback.Status != "succeeded"`の時点でreturnし、
`implementationpolicy.ValidateCoverage`を呼ばない（[`../../internal/agentrequest/request.go`](../../internal/agentrequest/request.go)）。
つまりfailed feedbackへ書いたpolicy statusは誰も検査しないまま artifact に残る。

`factCoverage`には`not-run`があるので未観測を未観測として書けるが、policy coverageの
statusは`satisfied | deviated | flagged`だけで「検証していない」を表す語がない。
schemaを増やさない方針なので、このprobeでは**何も主張しない**（fieldを出さない）方を選んだ。
語彙が足りないという発見自体は未解決事項として残す。

feedback generatorは[`../membership-repair-loop`](../membership-repair-loop/README.md)と共有なので、
この変更はtest failure側にも効く。あちらの`generation-feedback.failed.json`も
`133b3648…`から`347e8c85…`へ再生成した。stage `test`の実測結果（failed Fact 1件、
relatedIntentNodes 5件、diagnostics）は変わっていない。

summaryはstage固有部分とpolicy未検証の共通suffixから組み立てる。stage固有部分は
passed / failed / not-runの集計を必ず持つ。build stageでも、compileできたpackageが
Factを観測することも拒否することもあり得るため、「何も観測されなかった」と決め打ちしない。
今回の実測ではたまたま0 passedだった。

## 保護したartifactのhash

fault適用中とrepairの前後で、次はbyte-identicalだった。coverage mapのfingerprintも
`5413385c879192430e4fce3f4ff0e3763afbf7fd357a3b57257394eed4b56e07`のまま変わらない。

```text
app.forma                          4a74e51d3c433ae3f15c6852925b584f944759dccd7621d8e076ebcca927250a
generation-request.json            12fe5c8bfd36d161af462d8ef67065084ff2d3ef72fb3124b41cf7ee5f77d544
forma.implementation.yaml          6b2712b999bbc26a10477f8fb6ce0a0c0d903c8b712b608bb46359f74ddc7d8c
membership_e2e_test.go             4831e672962c450bceb81652bbaf55f7c750596a56252b776dcc02509dbe066a
server_test.go                     b8d324560d52558577c4d6e2c0d6440b13380a898770d8fee69e28f3aa87be9f
submission_test.go                 ae83bb8ce513e34f2113cc4da4f2e59c401344e02d69b0619d6b0d25ccaea238
store/identity_test.go             730ce623fa5c835e455e338c094a918ce3f4a02ec50250a8e6ad7a0195ca77cc
store/store_test.go                72497725856c389224e1bef739e09c6030424031d173d3564ca0151be3e7d430
identity/identity_test.go          ebeae4189689b4ade715ec52d8935f8cdc78f9aaaf5290ba55e577bb176bf20c
cmd/server/main_test.go            4fdfd2fc28967e77e50891fbb91916f261b5bd260f7177093238a58efda74283
cmd/feedback/coverage.go           c009dfe154e5ffbe4f1a2c572c2f2fe78dfa9c5f2162a0a7b3016cd8b693f675
```

`membership.go`はrepair後にfault適用前とbyte-identicalへ戻った。

測定artifact:

```text
generation-feedback.json           fa9564bfeaa41a5e225bbcc9dc0ea28f90c7a1693f4d13446f9c5822c1ffee51
generation-feedback.failed.json    e3efc034a397c1a0ba270cd7f66d40f46740b5035ba7a8f702822a16e71431b9
fault.patch                        948146752688b546d68235daaed6d5a94ea62fbd2e3668c76acbfd221c56f339
```

成功時`generation-feedback.json`のhashは
[`../membership-repair-loop`](../membership-repair-loop/README.md)が記録した値と同一である。
build failureを1往復しても成功時artifactは元へ戻った。

failed feedbackは2回独立に生成してbyte-identicalだった。diagnosticsから実行時間を除いており、
compile errorの行・列は固定patchから決まるためである。

## 追加したnegative test

[`../membership-agent-e2e/cmd/feedback/main_test.go`](../membership-agent-e2e/cmd/feedback/main_test.go)

| test | 固定する性質 |
| --- | --- |
| `TestBuildFailureIsStagedAsBuildNotTest` | package build failureを`stage: build`と判定し、compileしていないpackageのtest resultを作らない |
| `TestBuildFailureKeepsCompilerDiagnostic` | diagnosticsが空にならず、compiler errorを含み、`[build failed]`だけで終わらない |
| `TestBuildFailureLeavesUnobservedFactsNotRun` | 未観測Factを`passed`にも`failed`にもしない。実行されたpackageだけ`passed` |
| `TestTestFailureIsNotStagedAsBuild` | 実行されたassertionの失敗をbuild failureへ誤分類しない |
| `TestFailedSummaryDoesNotCallABuildFailureATestFailure` | summaryがbuild failureを"tests failed"と言わない |
| `TestFallbackDiagnosticsAreNeverEmpty` | parseできない出力でもdiagnosticsが空にならない |
| `TestBuildSummaryReportsWhatStillRan` | build stageでもpassed / failed / not-runを集計し、「何も観測されなかった」と決めつけない |
| `TestEveryFailedStageSummaryDeclaresNoPolicyWasVerified` | build / testどちらのfailed summaryもpolicy未検証を明示し、集計を持つ |
| `TestFailedFeedbackPublishesNoPolicyClaim` | build / test両方の記録済みfailed artifactがpolicy statusを1件も主張しない。build stageはFactを`passed`/`failed`にせずrelatedIntentNodesも空、test stageはfailed Factとintent nodesを保つ |

`TestBuildFailureKeepsCompilerDiagnostic`はbuild event取り込みを外すと実際に失敗することを
確認した。

## このprobeが証明したこと

- controlledなGo compile errorから、`stage: build` / `status: failed`のGeneration Feedbackを
  実測で作れる
- build failureとtest failureをstageで分離でき、逆方向の誤分類も固定した
- 観測されなかったFactを`failed`へ落とさず`not-run`に保てる。根拠のない
  `relatedIntentNodes`も作らない
- 検証されないpolicy statusをfailed feedbackへ残さない
- compiler diagnosticだけを根拠に実装1行を直し、85/85へ戻せる
- Forma source、Generation Request、Manifest、coverage map、testを一切変えずに1往復できる
- 成功時artifactがbyte-identicalに戻る
- schemaは`forma/generation-feedback/v0alpha2`のままで足りた。build failureを表すのに
  新しいfieldは要らなかった

## このprobeが証明していないこと

- 独立agentによる再現性。今回も同一workspace内のcontrolled runである
- 自動orchestration、retry scheduler、repair API。fault適用も修復も人手のstepである
- 一般的なrepair能力。faultは1行のarity違いで、compiler diagnosticがそのまま修正箇所を
  指していた。型推論越しの間接的なerrorや複数packageに散るerrorは試していない
- Forma compiler diagnostic起因のrepair。今回のerrorはtarget repository側だけで閉じており、
  Formaのintent gapとの分類判断は発生していない
- failureが実装bugかForma intent gapかの自動分類
- policy coverageに「検証していない」を表すstatusがない件。今回はfieldごと省いて
  未検証の主張を出さないようにしたが、省略は「policyが無い」とも読める。
  `factCoverage`の`not-run`に当たる語をpolicy側にも置くべきかは未決である
- request / testを弱めて通す経路の機械的拒否
- 実在する大規模repositoryへの適用
