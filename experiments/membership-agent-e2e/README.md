# Membership Agent E2E Experiment

Status: complete — measured, and the three Review Requirements passed human review.
A controlled first run, not a reproducibility claim.

このexperimentは、[`../admin-agent-e2e`](../admin-agent-e2e/README.md)で生成済みのadmin application
へ、メール認証付きのsignup/signinをapplication codeとして追加できるかを実測する。Forma coreは
Go/HTTP/HTMLのgeneratorやadapterを持たず、target repositoryは通常のGo applicationとして書かれている。

## 入力の分担

| input | 所有するもの |
| --- | --- |
| [`app.forma`](app.forma) | entity、field、relation、page、list/detail/form、Identity、Credential、Verification、Authentication、Ownership |
| [`target/forma.implementation.yaml`](target/forma.implementation.yaml) | 使用ライブラリのpolicy |
| target repository | route、HTTP、HTML、storage、session transport、KDF、fixture |

## 実行

```bash
# Generation Request を applied historical baseline から決定的に作る
go run ./experiments/membership-agent-e2e/cmd/generate

# target testを`go test -count=1 -json`で実行し、観測できたcoverageだけをfeedbackへ書き出す
# （旧feedbackは実行開始時に撤回され、成功時も失敗時もrenameで置き換わる。
#  未実行のFactはpassedにせず not-run にする）
go run ./experiments/membership-agent-e2e/cmd/feedback

# target を検証
cd experiments/membership-agent-e2e/target && go test ./... && go vet ./...

# lineage と coverage を検証
go run ./cmd/forma verify \
  --repository experiments/membership-agent-e2e/target \
  --baseline internal/agentrequest/testdata/admin.incremental.request.json \
  experiments/membership-agent-e2e/generation-request.json \
  experiments/membership-agent-e2e/target/generation-feedback.json
```

## 実測結果

```text
facts 81 (changed 38, unchanged 43)
intent nodes changed 43, unchanged 32
review requirements 3 (changed 3, unchanged 0)

verified 81 acceptance facts: all passed
  40 distinct tests, max 8 facts per test
verified 3 implementation policies
  2 satisfied, 1 deviated, 0 flagged
human review required: 3 requirements are not machine-verified
```

- **admin semanticsは不変**。baselineと共通するFact IDはちょうど43件で、38件の変更はすべて`added`。
  既存Factの変更・削除はない。
- **lineageは実artifactに対して検証**した。`cmd/generate`はbaselineのbyte列がGit blob
  `5751ecf85e9b7be2665aa91854ee5b69798e81a3`と一致することを確認してから使い、`ValidateIncrementalBaseline`が
  canonical payloadまで再比較する。
- Generation Requestは実際の`.forma` sourceから毎回構築する。JSONは手編集しない。

## 実装上の判断

| 意味 | 実装 |
| --- | --- |
| credentialを平文で保持しない | `crypto/pbkdf2`、PBKDF2-HMAC-SHA256、600,000 iterations、16-byte salt。KDF名とwork factorを保存し、未知KDFはfail-closed |
| verification tokenを保持しない | storeはSHA-256 digestのみ。平文tokenは発行時にメモリで呼び出し元へ渡し、`mail.Outbox`だけが観測点 |
| 30分・一度限り | `Evidence.Expired(now)`は`now < issuedAt + 30m`の関係判定。`expired` flagは存在しない |
| 4要素のatomic outcome | `store.Register`がUser・Credential・Evidence・Emission recordを1ロックでcommit。外部配送だけが外側 |
| verify成功 | `VerifyEvidence`が解決・検査・consume・Active化を1ロックで実行。2回目は使用済みとして拒否 |
| resend rotation | 新evidenceを先に生成し、成功後に旧evidenceをsupersedeしてcommit。状態変更後に失敗する経路がない |
| enumeration境界 | unknown / Active / Pendingで応答bodyが完全一致。signinのunknown / wrong credentialも同様 |
| 匿名submissionの冪等性 | server生成tokenがscope。全体上限512 + TTL 30分。running tokenはcapacity evictionから保護 |
| session | 不透明なid、`HttpOnly`、`SameSite=Lax`、`Secure`は設定可能 |

## 分かったこと

**Forma側のFactが2件、到達不能な状態をsetupしていた。** どちらもE2Eを実flowで書いたことで発覚した。
両方の修正はAcceptance Facts `v0alpha4`に含まれる（`v0alpha3`からの1回のbump内で完結しており、
`v0alpha4`のartifactが外部へ出たことはない）。

1. `verification-rejected`のconsumed caseが`Pending + consumed evidence`をsetupしていた。consumed
   evidenceは「consumeとPending → Activeをatomicに適用する」成功verificationからしか生まれないため、
   この状態はmodel内で到達不能。setup/expectedを`Active`へ修正し、consumed caseが同じoperationの
   `verification-accepted` Factの到達先とstate ID・valueで一致することを要求する不変則を追加した。
2. `duplicate-identifier-rejected`が「credentialを持つがevidenceが0件のPending subject」をsetupし、
   evidence・notice各0件を期待していた。registrationは4 recordを1 commitで作るため、evidence 0件の
   既存registrationは存在しない。setupを実際のregistrationが残す状態（subject 1・credential 1・
   issued evidence 1）へ直し、期待値を絶対数からgrowth（`added: 0`）へ変えた。setupが結果を
   先取りしないことは、この`added: 0`の要求そのものが担保する。compiler側の自己充足checkも
   「evidenceを持たないこと」から「registrationがcommitする4 recordを持つこと」へ反転させた。

**既存applicationへIdentityを足すと宛先解決が壊れた。** admin用`UserDetail`/`UserEdit`とmember用
`Profile`/`ProfileEdit`が同じentityのdetail/edit formとして両方一致し、標準actionが解決できなくなった。
明示`goto`を導入して解決した（[`../../docs/navigation-destination-proposal.md`](../../docs/navigation-destination-proposal.md)）。

**fact→test mapは「参照が存在すること」しか保証しない。** `forma verify`はFact IDに対応する
test referenceが解決できるかを見るが、そのtestがFactの期待値を観測しているかは見ない。review時に
9件のreferenceが、名前は対応しているのに期待値を確認していないと指摘された（surfaceのinput集合・
field集合、navigationの宛先、3 caseのうち1 caseだけの検証、evidence conditionの直接観測）。
protocol上の81/81と意味上の81/81は別物で、後者は現状human reviewでしか埋められない。
このexperimentではtest側を強化して埋めた（distinct tests 36 → 40）。

**target側がFormaにないruleを補っていた。** email検証に独自の空白禁止と`type="email"`を足していた。
宣言された`matches /.+@.+/`をそのまま適用する形へ直し、admin surfaceも同じ関数へ統一した。

## 人間のreviewが必要だった箇所

`forma verify`のexit 0は機械検査が通ったことだけを意味する。次の3件は再計算できないため、
人間が確認するまでこの実験を完了として扱わなかった。3件とも承認済みで、以下は何を見たかの記録である。
再確認の際も同じ箇所を読めばよい。

### `secret-storage`

- [`target/internal/identity/identity.go`](target/internal/identity/identity.go) の
  `credentialIterations`と`deriveCredential` — PBKDF2-HMAC-SHA256 600,000 iterationsがこのapplicationに
  対して十分か
- 同ファイルの`NewEvidence`と`TokenDigest` — verification tokenがdigestでしか保持されないこと
- [`target/internal/store/identity.go`](target/internal/store/identity.go) の`Register`と`issueLocked`
  — credentialとevidenceが通常のdomain fieldとして保存されていないこと

### `secret-redaction`

- [`target/internal/web/membership.go`](target/internal/web/membership.go) の`membershipForm`
  — password fieldを持たないこと
- 同ファイルの`deliver`と`genericAuthFailure` — 診断とlogにtokenやcredentialが出ないこと
- [`target/generation-feedback.json`](target/generation-feedback.json) — runtime secretを含まないこと

### `fixture-fidelity`

- [`target/internal/web/membership_e2e_test.go`](target/internal/web/membership_e2e_test.go) の
  `newHarness`と`verifyLink` — tokenをfake outboxからのみ取得していること
- 同ファイルの`TestExpiryBoundaryIsCheckedOnBothSides` — `Fixed.Advance`だけで境界を越え、
  expired recordを書いていないこと
- 同ファイルの`TestProfileReportsEmptyWhenTheRecordIsGone` — 結果を注入せず、
  2つのrepositoryの分岐として構成していること
- `signUp` / `activate` / `signIn` helper — setupが実operationを通っており、認証・認可の結果を
  直接注入していないこと
- 同ファイルの`TestSignUpFormOffersExactlyTheDeclaredInputs` /
  `TestProfileShowsExactlyTheDeclaredFields` / `TestProfileEditOffersExactlyTheDeclaredFields`
  — 宣言された集合との一致を見ており、包含で済ませていないこと
- 同ファイルの`TestSuccessfulVerificationLeavesTheEvidenceConsumed` — evidenceのconditionを
  store越しに読んでおり、2回目の拒否で代用していないこと
- [`target/internal/store/identity.go`](target/internal/store/identity.go) の
  `EvidenceForSubject` — 観測専用で、mutation経路を持たないこと

### 承認の記録

3件とも承認された。`fixture-fidelity`は2度差し戻されており、最初の版は次の2点で不十分だった。

- Fact ID とtest名が対応していても、そのtestがFactの期待値を観測しているとは限らない（9件該当）
- 「countが変わらない」という観測は、submitされた値が元の値と同じなら上書き実装も通してしまう

後者はmutation testで確定させた。`store.Register`のduplicate分岐にcredential上書きを注入すると
exact/canonical両caseが失敗し、戻すとpassする。fixtureがFactを観測できているかは、
「testがpassすること」ではなく「壊したときに落ちること」でしか確かめられない。

feedbackの信頼性も同じ方法で確かめた。生成を強制的に失敗させると旧feedbackが消え、直後の
`forma verify`はfeedback欠落で失敗する。正常生成後はSHA-256が元へ戻り、temp fileも残らない。

## このexperimentで検証していないもの

- 独立agentによる再現性。今回は同一workspace内のcontrolled runである
- 実在する大規模repositoryへの適用
- field rename / 削除を含むincremental update
- passwordless、external provider、email変更（[`../../docs/identity-variant-probe.md`](../../docs/identity-variant-probe.md)）
- NFC正規化が結果を変える`matches` pattern。`/.+@.+/`は正規化の影響を受けないため、このapplicationでは
  意味的に同値である

test failureからのcontrolled repairは
[`../membership-repair-loop`](../membership-repair-loop/README.md)で最初のprobeを実測した。
このStage D experiment単体ではbuild失敗、独立agent、汎用orchestrationを検証していない。

後続probeでは、build failureを
[`../membership-build-repair-loop`](../membership-build-repair-loop/README.md)、fresh agent processを含む
trusted orchestrationを
[`../membership-automated-repair-loop`](../membership-automated-repair-loop/README.md)で実測した。このStage D
experiment単体がそれらを証明するものではない。後者ではtest/build repairに加え、implementationで
解決できないForma intent gapをrepository不変のままhuman handoffへ返す経路も実測した。
