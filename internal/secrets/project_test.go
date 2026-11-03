package secrets

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestTypedProjectionPreservesCountersAndNilResults(t *testing.T) {
	watch("STRUCT_PIN", "7481")
	input := struct {
		Text   string
		Count  int
		Err    error
		Schema json.RawMessage
	}{Text: "7481", Count: 7481, Err: errors.New("token 7481"), Schema: json.RawMessage(`{"x":"7481","x":7481}`)}
	out := Project(input)
	if out.Text != "[secret:STRUCT_PIN]" || out.Count != 7481 || out.Err.Error() != "token [secret:STRUCT_PIN]" || string(out.Schema) != `{"x":"[secret:STRUCT_PIN]","x":"[secret:STRUCT_PIN]"}` {
		t.Fatalf("typed projection: %+v", out)
	}
	if Project[any](nil) != nil {
		t.Fatal("nil result changed")
	}
}

func TestProjectionBoundsRecursivePayloadAndCoversArrays(t *testing.T) {
	watch("RECURSIVE", "recursive-private-value")
	type node struct {
		Text [1]string
		Next *node
	}
	input := &node{Text: [1]string{"recursive-private-value"}}
	input.Next = input
	out := Project(input)
	for steps := 0; out != nil; steps++ {
		if steps > 128 {
			t.Fatal("recursive payload was not bounded")
		}
		if out.Text[0] != "" && out.Text[0] != "[secret:RECURSIVE]" {
			t.Fatal("recursive tail retained plaintext")
		}
		out = out.Next
	}
}
