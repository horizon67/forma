# Minimal Conformance Contract Experiment — Frozen Prototype

Status: frozen historical prototype — not a future Forma runtime-adapter architecture

このexperimentは、target-neutralな期待値とtarget固有観測を分離できるか確認するために作った。
分離自体から得た知見はAcceptance Factsへ引き継ぐが、Forma coreがframework別adapter、shared runner、
fixture protocolを保有する方向は採らない。codeは実測結果の記録として凍結する。

今後はFormaがtarget-neutralなAcceptance Factsを出し、AI coding agentがtarget repository固有のtestへ
変換する。現在の方向は[`../../docs/agent-generation.md`](../../docs/agent-generation.md)を参照する。

このexperimentは、Resolved Intentからtarget非依存な検査caseを決定的に生成し、同じcontractをprofile固有の
adapterへ適用できるかを検証する。

## 境界

```text
Resolved Intent
  ↓ experiments/conformance.Build
conformance.json                 target-neutral
  ↓ shared runner
Conformance operation/observation
  ↓ profile adapter
HTTP、DOM、storeなどのtarget固有操作
```

`conformance.json`とshared runnerは、route、HTTP method/status、HTML、hidden field、Go runtimeを知らない。
profile adapterだけが、たとえば`query-view`をHTTP GETへ、`validation-rejected`を`422`と生成HTMLの観測へ
対応付ける。期待値をadapterから生成したり、adapterがcaseの合否を決めたりはしない。

## 現在のtarget-neutral語彙

| 種類 | 値 |
| --- | --- |
| operation | `query-view`、`submit-form` |
| principal | `anonymous`、`roles` |
| outcome | `access-allowed`、`access-denied`、`mutation-accepted`、`validation-rejected` |
| violation | `required`、`closed-set` |
| state observation | `subjects`、`input`、`unchanged`、`preserveInput` |

caseはsemantic IDでview、entity、fieldを参照する。fixture identityと値もcontractに含め、adapterが
都合のよいtest dataや期待値を選べないようにする。

## 最初の5保証

Admin E2E inputから、現在は次を生成する。

1. `allow admin`を満たすprincipalはlistを参照でき、fixtureの全subjectを得る。
2. roleを持たないprincipalは同じlistを参照できない。
3. 妥当なedit inputは受理され、送信したfield値が保存される。
4. edit formのrequired fieldを空にすると保存されず、入力可能な値は保持される。
5. unionのclosed set外の値は保存されず、他fieldの入力は保持される。

list fixtureは2件を生成するため、positive queryは単に応答が成功するだけでなく、期待するsubject集合を
すべて提示したことまで検査する。shared runnerは未知のoperation、principal kind、保存期待値を拒否し、
未対応語彙を無言で読み飛ばさない。

closed set外の値自体は通常のselectで表現不能なので、違反field自身を`preserveInput`には含めない。
この差は実artifactへcontractを適用したことで判明したtarget間共通の観測境界である。

## 生成artifactでの構成

`go-stdlib-admin/v0`は次を生成する。

- `conformance.json` — Resolved Intentだけから生成したcontract
- `conformance_runner_test.go` — このdirectoryの共有runnerをそのまま複製
- `conformance_adapter_test.go` — HTTP/HTML/memory storeをtarget-neutral observationへ変換するprofile adapter

既存の`main_test.go`はURL、HTTP status、submission tokenなどprofile実装を検査する。Conformance runnerは
Formaの意味だけを検査する。この2 suiteが同じartifact内で別testとして実行される。

## 凍結時点で未実装だった範囲

- unique、type constraint、relation choice
- state transitionと不正遷移
- search、filter、stable sort、pagination
- navigationとinteraction stateのtarget-neutral observation
- adapterのprocess境界とArtifact Protocol
- 2つ目のprofileへの適用

このschemaは`v0alpha1`のhistorical artifactであり、正式なschemaやCLIへ発展させない。
