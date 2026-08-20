# Membership Repair Loop Experiment

Status: first probe measured — a controlled failure → repair → success on the
membership target. Not a claim of automatic agent orchestration or general repair.

このexperimentは、[`../membership-agent-e2e`](../membership-agent-e2e/README.md)で
81/81まで通ったmembership targetへ、duplicate registration時に既存credentialを上書きする
実装bugを一時的に入れ、実際のtest failureから`status: failed`のGeneration Feedbackを作り、
Formaの意味を弱めずに実装だけを直して再び成功するかを実測する。

Forma coreはrepair agent APIや汎用orchestrationを持たない。今回必要な最小sliceは、
失敗したrepository testからFactとSource Map上のintent nodeを結ぶこと、failed feedbackを
atomicに公開すること、そしてその記録を使った1回のcontrolled repairである。

## 入力の分担

| input | 所有するもの |
| --- | --- |
| [`../membership-agent-e2e/app.forma`](../membership-agent-e2e/app.forma) | 変更しない。何を成立させるか |
| [`../membership-agent-e2e/generation-request.json`](../membership-agent-e2e/generation-request.json) | 変更しない。81 Facts、3 Review Requirements |
| [`../membership-agent-e2e/target/forma.implementation.yaml`](../membership-agent-e2e/target/forma.implementation.yaml) | 変更しない。implementation policy |
| coverage map in [`../membership-agent-e2e/cmd/feedback/coverage.go`](../membership-agent-e2e/cmd/feedback/coverage.go) | 変更しない。fact → test reference |
| target tests | 変更・弱化しない |
| [`fault.patch`](fault.patch) | duplicate時に既存credentialを上書きするcontrolled fault |
| target `store.Register` | repair対象。拒否してもbindingを書き換えてはならない |

## 実行

```bash
# 最終treeへfaultを載せ、失敗を再現する
git apply experiments/membership-repair-loop/fault.patch
go run ./experiments/membership-agent-e2e/cmd/feedback
# 旧succeeded feedbackは撤回され、failed feedbackがatomicに公開される
go run ./cmd/forma verify \
  --repository experiments/membership-agent-e2e/target \
  --baseline internal/agentrequest/testdata/admin.incremental.request.json \
  experiments/membership-agent-e2e/generation-request.json \
  experiments/membership-agent-e2e/target/generation-feedback.json
# → exit 1: Generation Feedback status is failed

# 実装だけを戻し、成功feedbackを公開する
git apply -R experiments/membership-repair-loop/fault.patch
go run ./experiments/membership-agent-e2e/cmd/feedback
go run ./cmd/forma verify \
  --repository experiments/membership-agent-e2e/target \
  --baseline internal/agentrequest/testdata/admin.incremental.request.json \
  experiments/membership-agent-e2e/generation-request.json \
  experiments/membership-agent-e2e/target/generation-feedback.json
```

broken codeは最終成果へ残さない。[`fault.patch`](fault.patch)と
[`generation-feedback.failed.json`](generation-feedback.failed.json)が再現用artifactである。

## 実測したloop

```text
fault.patch
  → go test -count=1 -json ./... が TestDuplicateIdentifierCoversExactAndCanonicalForms で失敗
  → fact/identity/UserAccount/operation/register/identifier/duplicate が failed
  → 実行された他testに紐づく80 Factsは passed。未実行なら not-run
  → sourceNodes ∩ Source Map
      identity/UserAccount/credential/password
      identity/UserAccount/identifier/email
      identity/UserAccount/operation/register
      identity/UserAccount/verification/email
      identity/UserAccount/verification/email/notice
  → forma verify は status: failed を成功にしない
  → Register の duplicate 分岐から credential 上書きを除く
  → 元の secret だけが通り、duplicate 時の secret は拒否される
  → forma verify --baseline は 81/81、40 distinct tests
```

失敗時feedbackの要点:

```text
schema  forma/generation-feedback/v0alpha2
stage   test
status  failed
command cd experiments/membership-agent-e2e/target && go test -count=1 -json ./...
failed  fact/identity/UserAccount/operation/register/identifier/duplicate
        internal/web/membership_e2e_test.go#TestDuplicateIdentifierCoversExactAndCanonicalForms
counts  80 passed, 1 failed, 0 not-run
policyCoverage  なし
diagnostics（実行時間なし）
        --- FAIL: TestDuplicateIdentifierCoversExactAndCanonicalForms
        membership_e2e_test.go:739: the duplicate attempt's secret signed in: 303
```

exact と canonical-equivalent の両subtestが同じ観測で落ちた。countが変わらないだけでは
足りず、duplicateが持ち込んだsecretでsigninできてしまうことが失敗理由である。

## 保護したartifactのhash

repairの前後で次はbyte-identicalだった。coverage mapのfingerprintも同一である。

```text
app.forma                          4a74e51d3c433ae3f15c6852925b584f944759dccd7621d8e076ebcca927250a
generation-request.json            19b10ca6fa7515325fbc3b8a23a3a084b0989023ca33c0f8fce4d108eb343dcc
forma.implementation.yaml          6b2712b999bbc26a10477f8fb6ce0a0c0d903c8b712b608bb46359f74ddc7d8c
membership_e2e_test.go             4831e672962c450bceb81652bbaf55f7c750596a56252b776dcc02509dbe066a
server_test.go                     b8d324560d52558577c4d6e2c0d6440b13380a898770d8fee69e28f3aa87be9f
submission_test.go                 ae83bb8ce513e34f2113cc4da4f2e59c401344e02d69b0619d6b0d25ccaea238
store/identity_test.go             83a89cc3c42306b61bcd2b0443ce0bdb2f237f2ef8a8efe2d649d83ab5956dbf
store/store_test.go                72497725856c389224e1bef739e09c6030424031d173d3564ca0151be3e7d430
identity/identity_test.go          ebeae4189689b4ade715ec52d8935f8cdc78f9aaaf5290ba55e577bb176bf20c
cmd/server/main_test.go            4fdfd2fc28967e77e50891fbb91916f261b5bd260f7177093238a58efda74283
coverage fingerprint               341f1f8042fa16b528704c494ca17e126a7fc2f6ee08059b622f24b23392c7f9
```

成功時の`generation-feedback.json`はcommandを実際の`go test -count=1 -json ./...`へ合わせたため、
Stage D記録とはbyte-identicalではない。Acceptance Factsはrequestに含まれ、そのhashが不変であることで
弱化していない。

failed feedbackとfault patch自体は測定artifactであり、repairで消さない。diagnosticsから実行時間を
除いているので、再実行してもこのfailed JSONのSHA-256はdurationで動かない。

`generation-feedback.failed.json`は`133b3648…`から`347e8c85…`へ再生成した。
[`../membership-build-repair-loop`](../membership-build-repair-loop/README.md)がfeedback generatorを
共有しており、そこで2つの修正が入ったためである。1つはfailed feedbackから`policyCoverage`を落とすこと
（`ValidateCompletion`は`succeeded`でないfeedbackのpolicy coverageを検証しないので、書いても誰も
検査しない主張になる）、もう1つはsummaryをpassed / failed / not-runの集計から組み立てることである。
このexperimentの実測結果自体は変わっていない。stageは`test`、failed Factは同じ1件、
relatedIntentNodesは同じ5件、diagnosticsも同じである。

```text
generation-feedback.json           2c82dbb96360ec598b5f3b0f31de91e36ba79cb067f214727fb8f793e08e8fdb
generation-feedback.failed.json    347e8c85bbf68c1711ed433a3597a92debc1a2ff29054515068845fd03b5de31
fault.patch                        36cc66aa3e6ddf8684e63c09a84ba35f8f65fd90e6c93fb45892e972622647cf
```

## 検証結果

```text
verified 81 acceptance facts: all passed
  40 distinct tests, max 8 facts per test
verified 3 implementation policies
  2 satisfied, 1 deviated, 0 flagged
human review required: 3 requirements are not machine-verified
```

- root と target の `go test ./...`、`go vet ./...`、`git diff --check` は成功した。
- 3 Review RequirementsのIDと文言はStage Dと同じである。機械検査はこれらを再計算しない。
- schemaは`forma/generation-feedback/v0alpha2`のままである。新しいfeedback fieldは足していない。

## このexperimentで検証していないもの

- 独立agentによる再現性。今回は同一workspace内のcontrolled runである
- 完全な自動agent orchestration、retry scheduler、repair API
- build失敗やForma compiler diagnosticからのrepair
- failureが実装bugかFormaのintent gapかの自動分類。今回のfaultは実装bugとして置いた
- 複数種類のfailureや、request/testを弱めて通す経路の機械的拒否
- 実在する大規模repositoryへの適用
