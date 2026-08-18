# Experiments

Formaの実験は、language semanticsとagent generation boundaryを実例から確認するために置く。

## Active experiments

- [`admin-agent-e2e`](admin-agent-e2e/README.md) — Resolved IntentとAcceptance Factsをcoding agentへ渡し、standalone targetで43 factsを検証
- [`membership-agent-e2e`](membership-agent-e2e/README.md) — 既存admin targetへメール認証付きsignup/signinを追加し、81 factsを検証
- [`membership-repair-loop`](membership-repair-loop/README.md) — 同じmembership targetでtest failure → failed feedback → repair → successを1回実測
- [`membership-build-repair-loop`](membership-build-repair-loop/README.md) — 同じtargetでcompile failure → `stage: build`のfailed feedback → repair → successを1回実測。81 factsすべてが`not-run`
- [`membership-repair-integrity`](membership-repair-integrity/README.md) — test・coverage map・要求を弱めてgreenにするrepairを、retry baselineとの比較で拒否できるかを実測。implementation-only repairだけ通す

## Frozen prototypes

- [`admin-e2e`](admin-e2e/README.md)
- [`conformance`](conformance/README.md)

この2つは、FormaがGo code generator、target profile system、共通conformance adapterを所有する方向を
試した過去のprototypeである。view、action、validation、accessなど、Resolved Intentへ必要な情報を
発見する役割は果たしたが、Forma coreをframework generator projectへ近づけることも分かったため凍結した。

codeとtestは実測結果を再現するために残す。新しいframework対応、profile capability、runtime adapter、
artifact protocol、byte-identical generationをここへ追加しない。

今後の実験は[`../docs/agent-generation.md`](../docs/agent-generation.md)に従い、Resolved Intentと
Acceptance FactsをAI coding agentへ渡し、通常のtarget repositoryをincrementalに変更する形で行う。
