# Experiments

Formaの実験は、language semanticsとagent generation boundaryを実例から確認するために置く。

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
