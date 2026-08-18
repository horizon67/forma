# Admin Agent Generation Experiment

Status: active experiment — first full run and first incremental update completed

このexperimentは、Forma自身が管理画面generatorを持たず、structuredな要求をAI coding agentへ渡して
通常のrepositoryへ実装できるかを検証する。

```text
app.forma
  ↓ forma request
Resolved Intent + Acceptance Facts + Source Map
  ↓ Generation Request
AI coding agent + ordinary target repository
  ↓ repository-native code and tests
build / test / fact coverage
```

[`../admin-e2e`](../admin-e2e/README.md)はGo generatorで意味の不足を発見した凍結prototypeである。この
experimentでは、そのgenerator、template、profile capability、runtime adapterを使用しない。

## 対象flow

- adminだけが利用できるUser一覧・詳細・編集
- name/nickname/email検索、team/plan/status filter、name sort、page size 10
- Team relationと`Team.name`による人間向けlabel
- Planのclosed set
- emailのrequired、unique、format constraint
- validation拒否時に他の入力を保持すること
- edit成功後のdetail navigation
- 1回の論理mutationを複数回適用しないこと

## Generation Request

次のcommandはtarget-neutralなrequestをstdoutへ出力する。

```bash
go run ./cmd/forma request experiments/admin-agent-e2e/app.forma
```

requestには次を含む。

- canonicalなResolved Intent
- stable ID付きAcceptance Facts
- Source Map
- `requestedChange.kind = full | incremental`
- incremental時のbaseline digest、intent/fact change、unchanged件数
- optionalな正規化済みImplementation Policy Manifest
- 全fact IDを列挙したverification policy

初回のimmutable `v0alpha1` requestは
[`admin.request.json`](../../internal/agentrequest/testdata/admin.request.json)、incremental `v0alpha2` requestは
[`admin.incremental.request.json`](../../internal/agentrequest/testdata/admin.incremental.request.json)に固定した。

```bash
go run ./cmd/forma request \
  --previous internal/agentrequest/testdata/admin.request.json \
  --manifest experiments/admin-agent-e2e/target/forma.implementation.yaml \
  experiments/admin-agent-e2e/app.forma
```

repository固有testは対応するfact IDをtest名、metadata、またはsidecar manifestから参照できなければ
ならない。`testReferences`は`repository/relative/path#test-identifier`で返す。1つのintegration/E2E testが
複数factを覆う場合は、同じreferenceを異なるfactから共有してよい。成功時はGeneration Feedbackのfact
coverageと、orchestration layerがimmutableに保持するrequestのcanonical fact ID集合を完全一致させる。
agentが編集したrequestのcopyや`requiredFactIds`だけを判定根拠にはしない。

## このsliceで確認すること

1. 同じsourceからbyte-identicalなGeneration Requestを得られる。
2. 正常系と拒否系のfactが両方ある。
3. route、HTTP、HTML、DOM、framework、submission tokenをrequestへ含めない。
4. agentがFormaにないapplication requirementを推測せず実装できる。
5. agentがrepository固有testへ全fact IDを対応付けられる。
6. build/test failureから修正し、全factをpassedにできる。

1〜3はreference front-endのgolden testで、4〜5と全factの初回成功は
[`target`](target/README.md)のstandalone Go repositoryで検査した。6のfailureからの自動repairは
このadmin targetでは未実施である。membership targetでの最初のcontrolled probeは
[`../membership-repair-loop`](../membership-repair-loop/README.md)に記録する。

## 初回runの結果

coding agentは、[`target/ARCHITECTURE.md`](target/ARCHITECTURE.md)にある通常のrepository規約を保ちながら、
server-renderedなUser一覧・詳細・編集とrepository固有testを実装した。Forma coreにはGo/HTTP/HTML向けの
lowerer、template、runtime adapterを追加していない。

```bash
cd experiments/admin-agent-e2e/target
go test ./...
go vet ./...

cd ../../..
go run ./cmd/forma verify \
  internal/agentrequest/testdata/admin.request.json \
  experiments/admin-agent-e2e/baseline/generation-feedback.json
```

結果は43/43 factsが`passed`である。coverageは12本のdistinct testに分かれ、最大集中は1 testあたり
8 factsだった。主な対応は次のとおり。

- admin accessの正常系とanonymous拒否
- listの全field、search、3 filter、stable sort、20件page boundary、empty/failure
- detailのfield、Team label、edit navigation、empty/failure
- editの正常保存、detailへのnavigation、required/unique/matches/closed-set拒否、入力保持
- 同じ論理submitを2回dispatchしてもmutation適用は1回

historical [`baseline/generation-feedback.json`](baseline/generation-feedback.json)はrepository相対のtest
referenceを持ち、`forma verify`はcanonical factsとの
完全一致、未知・重複・未参照fact、test reference形式、全resultを検査する。出力にはdistinct test数と
1 testあたりの最大fact数も表示し、異常なcoverage集中を人間が確認できるようにした。

## 凍結prototypeからの独立性

同じworkspaceには過去のGo generator prototypeがあるため、targetがその実装を写した可能性を照合した。
次の恣意的な識別子と構成は一致しなかった。

| 凍結prototype | 今回のtarget |
| --- | --- |
| `_forma_submission` | `submission` |
| `forma_role` | 使用しない。seedの`internal/auth`境界を使用 |
| `data-prevent-duplicate` | 使用しない |
| `Saving…` | 使用しない |
| `/users` | `/admin/users` |
| 単一`main.go` | `cmd/server` + `internal/{auth,domain,store,web}` |

さらにtargetは凍結generatorやtemplateをimport・実行していない。これは完全なblind testではないが、
Generation Requestからrepository規約に沿って独立設計されたことを支持する具体的な証拠である。

## 実測から分かった境界

- 認証方式、URL、HTML、storage、submissionの実現機構はGeneration Requestに不要だった。target repository
  の規約とcoding agentの実装判断で決められた。
- fixture内容と表示文言もFormaにはない。今回のflowでは実装判断として扱えたが、厳密なcopyやdesignが
  product intentなら将来別axisが必要になる。
- coverage gateはfactの変換漏れと異常な集中を可視化するが、test内容の忠実さまでは証明しない。今回の
  43/43は、page boundary、at-most-once、search/filter/sortなど偽装しやすいtestの直接reviewも行った。
- このrunは同一workspace内のcontrolled experimentで、独立agentや実在repositoryでの再現性まではまだ
  検証していない。

管理画面list/detail/editについて、現時点で新しいForma primitiveが必要になる不足は見つからなかった。

## 最初のincremental run

### Baseline

変更前のcommit、target tree、Forma source blob、full request blob、43 Factsと12 testsを
[`baseline.json`](baseline.json)へ固定した。sourceとhistorical feedbackは[`baseline/`](baseline/)に保存し、
target変更前にroot/target双方の`go test ./...`と`go vet ./...`、`forma verify`が成功することを確認した。

適用後のcommit、target tree、source、incremental request、feedbackのGit identityは
[`incremental-baseline.json`](incremental-baseline.json)へ別に固定した。特にrequest blob
`5751ecf85e9b7be2665aa91854ee5b69798e81a3`は、Identity B4 migrationで作り直さずhistorical `v0alpha2`
baselineとしてそのまま使う。

### Requested change

- optionalな`User.nickname`を追加する。
- list、detail、editへnicknameを追加し、search対象にする。
- logical page sizeを20から10へ変更する。
- existing targetをfull regenerationしない。

compilerは8 intent nodesをadded/changed、13 Factsをchanged、30 Factsをunchangedとして導出した。Fact IDは
43件のまま安定し、field projection、search input、page boundary、form mutationとvalidation時の入力保持が
新しいpayloadへ更新された。

### Implementation Policy

target内の[`forma.implementation.yaml`](target/forma.implementation.yaml)をcanonical JSONへ正規化して
incremental requestへ埋め込んだ。結果は次のとおり。

- required server-rendering policy: evidence fileとopaque valueを確認して`satisfied`
- preferred persistence policy: existing in-memory storeを維持するreason付き`deviated`
- forbidden router policy: repository text scanが0 hitsで`satisfied`

Forma coreは技術valueごとの分岐を持たず、schema、ID、mode、evidence、reason、scan結果だけを検査する。

### Result

既存の`cmd/server`と`internal/{auth,domain,store,web}`境界を維持し、domain、store search、server presentation、
template、既存testだけを局所的に更新した。`ARCHITECTURE.md`、entry point、auth、target `go.mod`、無関係な
store testのblobはbaselineと同一だった。

```bash
cd experiments/admin-agent-e2e/target
go test ./...
go vet ./...

cd ../../..
go run ./cmd/forma verify \
  --repository experiments/admin-agent-e2e/target \
  --baseline internal/agentrequest/testdata/admin.request.json \
  internal/agentrequest/testdata/admin.incremental.request.json \
  experiments/admin-agent-e2e/target/generation-feedback.json
```

結果は43/43 Acceptance Facts、12 distinct testsを確認した。implementation policyは2件が`satisfied`、
preferred persistence 1件がreason付き`deviated`で、人間のreview対象としてCLIにも表示される。これにより、
Formaが初回生成用の詳しいpromptに留まらず、既存repositoryを保った更新の前処理としても機能する根拠を
得た。rename、削除、constraint migrationは後続incremental probeへ残し、次はroadmapのIdentity probeへ進む。
