package agentrequest

import (
	"reflect"
	"strings"
	"testing"

	"github.com/horizon67/forma/internal/compiler"
)

func TestIntentNodesForFactsUsesSourceMap(t *testing.T) {
	request, err := BuildFull(membershipRequestResult(t))
	if err != nil {
		t.Fatal(err)
	}
	const duplicate compiler.SemanticID = "fact/identity/UserAccount/operation/register/identifier/duplicate"
	nodes, err := IntentNodesForFacts(request, []compiler.SemanticID{duplicate})
	if err != nil {
		t.Fatal(err)
	}
	want := []compiler.SemanticID{
		"identity/UserAccount/credential/password",
		"identity/UserAccount/identifier/email",
		"identity/UserAccount/operation/register",
		"identity/UserAccount/verification/email",
		"identity/UserAccount/verification/email/notice",
	}
	if !reflect.DeepEqual(nodes, want) {
		t.Fatalf("related intent nodes = %#v, want %#v", nodes, want)
	}
}

func TestIntentNodesForFactsRejectsUnknownFactAndUnmappedSource(t *testing.T) {
	request, err := BuildFull(membershipRequestResult(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := IntentNodesForFacts(request, []compiler.SemanticID{"fact/unknown"}); err == nil || !strings.Contains(err.Error(), "unknown fact") {
		t.Fatalf("unknown fact error = %v", err)
	}

	var duplicate *compiler.AcceptanceFact
	for index := range request.AcceptanceFacts.Facts {
		if request.AcceptanceFacts.Facts[index].ID == "fact/identity/UserAccount/operation/register/identifier/duplicate" {
			duplicate = &request.AcceptanceFacts.Facts[index]
			break
		}
	}
	if duplicate == nil {
		t.Fatal("membership fixture is missing the duplicate-identifier fact")
	}
	duplicate.SourceNodes = append(duplicate.SourceNodes, "identity/UserAccount/missing")
	if _, err := IntentNodesForFacts(request, []compiler.SemanticID{duplicate.ID}); err == nil || !strings.Contains(err.Error(), "absent from the Source Map") {
		t.Fatalf("unmapped sourceNode error = %v", err)
	}
}
