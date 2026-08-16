# Admin Agent Generation Experiment

Status: active experiment — first full target-repository run completed

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
- name/email検索、team/plan/status filter、name sort、page size 20
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
- `requestedChange.kind = full`
- 全fact IDを列挙したverification policy

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
[`target`](target/README.md)のstandalone Go repositoryで検査した。6のfailureからの自動repairは未実施である。

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
  experiments/admin-agent-e2e/target/generation-feedback.json
```

結果は43/43 factsが`passed`である。coverageは12本のdistinct testに分かれ、最大集中は1 testあたり
8 factsだった。主な対応は次のとおり。

- admin accessの正常系とanonymous拒否
- listの全field、search、3 filter、stable sort、20件page boundary、empty/failure
- detailのfield、Team label、edit navigation、empty/failure
- editの正常保存、detailへのnavigation、required/unique/matches/closed-set拒否、入力保持
- 同じ論理submitを2回dispatchしてもmutation適用は1回

`generation-feedback.json`はrepository相対のtest referenceを持ち、`forma verify`はcanonical factsとの
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
次は同じtarget repositoryへのincremental changeで、既存codeを保った更新を測る。

## 次のincremental probe

初回実装後に、Forma sourceへUserの編集fieldまたはconstraintを一つ追加する。full regenerationはせず、
変更されたintent nodeとfactだけをrequested changeとしてagentへ渡し、既存codeとtestを保ったまま
更新できるか確認する。
