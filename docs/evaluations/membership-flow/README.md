# Membership Flow Human Evaluation

Status: protocol and stimuli ready; no participant result recorded yet

この評価は、現行Forma sourceだけを読む条件Aと、同じsourceに決定的なprojectionを追加する条件Bを、
実際の人間が同じtaskで比較するためのformative studyである。Candidate Cの`flow`構文は未実装のpseudocodeなので、
定量比較へ混ぜない。

## 検証する仮説

- Bは、正常系、external boundary、state effectを理解するT1と、navigation regressionを見つけるT4を速く正確にする。
- Bは、詳細outcomeを読むT2とIdentity policyを探すT5を悪化させない。
- Bは、現行言語で表せないT3を「図へ線を足せば済む」と誤認させない。
- Bは、admin CRUDを読むT6へ新しいsource boilerplateがあるかのような誤解を増やさない。

## Primary study

最初のstop ruleは8 session、A/B各4 sessionとする。参加者をForma経験の有無で層別し、各層の中でA/Bへ交互に
割り当てる。同じ参加者へ両条件を見せると2回目がapplication semanticsを記憶してしまうため、primary resultでは
一人一条件だけを実施する。8人を集められない場合も結果は残すが、統計的結論ではなくpilotとして報告する。

1. 参加者へこのREADMEとanswer keyを見せず、共通の[`common-reference.md`](common-reference.md)と、割り当てた
   [`condition-a.md`](condition-a.md)または[`condition-b.md`](condition-b.md)だけを渡す。common referenceを読む時間は
   task timeへ含めない。
2. task開始前に、職種、software設計経験年数、宣言型言語経験、Forma経験を
   [`score-sheet.md`](score-sheet.md)へ記録する。
3. T1からT6を順に行い、taskごとの経過秒、回答、確信度1〜5を記録する。上限は全体35分である。
4. facilitatorは誘導せず、質問された回数と内容だけを記録する。
5. 終了後に[`answer-key.md`](answer-key.md)で採点する。部分点の単位をsession後に変えない。

検索は配布されたfile内に限り許可する。Forma commandの実行、repository内の別文書、web検索、answer keyの閲覧は
禁止する。条件Bのprojectionは事前生成済みartifactを読むため、tool操作速度を測る試験ではない。

## Primary metrics

- accuracy: answer keyの31点を百分率化する。
- task time: T1〜T6それぞれと全体の秒数。未完了は35分時点でcensoredと記録する。
- false assertion: source/projectionに無い保証を断言した個数。特にdefault entry、email delivery成功、
  verification失敗時の逆命題を分ける。
- confidence: taskごとの1〜5。誤答かつ4〜5はhigh-confidence errorとして別集計する。
- navigation trace: T1で見落としたpage、operation、external boundary、domain/session effectの個数。

平均だけでなく各sessionのraw score、median time、rangeを残す。小標本なのでp-valueを主判断に使わない。

## 事前に固定する判定

visual projectionを有効なreview viewと判断するのは、次をすべて満たす場合である。

- BのaccuracyがAを15 percentage point以上上回る、またはBのT1+T4 median timeが25%以上短い。
- BのT2+T5 accuracyがAより10 percentage pointを超えて悪化しない。
- BのT6 median timeがAより25%を超えて悪化しない。
- Bでdefault entryまたはexternal deliveryを推測したhigh-confidence errorが2 session以上発生しない。

T3はどちらの条件でも実装不能が正答である。Bの参加者が図を編集可能な正本だと答える場合は、language primitiveを
増やす前にviewの表示と説明を修正する。BがT1/T4を改善してもT3の要求自体は満たせないため、その後にだけCandidate C
または汎用page transitionの最小semanticを別評価する。

## Artifact integrity

条件Bのvisual artifactは次のcommandのstdoutとbyte-identicalであることをcompiler/CLI golden testが固定する。

```bash
go run ./cmd/forma project flow experiments/membership-agent-e2e/app.forma
go run ./cmd/forma project flow examples/users.forma
```

`flow` viewはnavigationを骨格にし、Outcome groupはsemantic source、State elementはtrigger/effect/invocationでのみ結ぶ。
対応しないdetailはunlinked indexへ残る。Mermaid layout、edge label、case countはreview projectionであり、Forma source、
Resolved Intent、Acceptance Factsを上書きしない。
