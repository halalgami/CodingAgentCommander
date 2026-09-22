package delegate

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestResultHasNoCostField(t *testing.T) {
	// total_cost_usd is fabricated for Ollama models (Claude pricing applied to
	// a free model). Never carry it, so it can never be surfaced.
	rt := reflect.TypeOf(Result{})
	for i := 0; i < rt.NumField(); i++ {
		if strings.Contains(strings.ToLower(rt.Field(i).Name), "cost") {
			t.Errorf("Result must not carry a cost field, found %s", rt.Field(i).Name)
		}
	}
}

func TestEnvelopeTokensFallsBackToModelUsage(t *testing.T) {
	// Measured on a live run: a successful 3-turn answer reported top-level
	// usage of 0/0 while modelUsage held the real counts. Reading only the
	// aggregate logs zeros and quietly destroys the cost record.
	var e envelope
	if err := json.Unmarshal([]byte(`{"usage":{"input_tokens":0,"output_tokens":0},
		"modelUsage":{"ollama-glm-5.3-oai":{"inputTokens":11250,"outputTokens":775,"costUSD":0.075625}}}`), &e); err != nil {
		t.Fatal(err)
	}
	in, out := e.tokens()
	if in != 11250 || out != 775 {
		t.Errorf("tokens() = %d/%d, want 11250/775 from modelUsage", in, out)
	}
}

func TestEnvelopeTokensPrefersTheAggregate(t *testing.T) {
	// When the aggregate is populated it wins: modelUsage can fold in an extra
	// title-generation call and so exceed the run's real total.
	var e envelope
	if err := json.Unmarshal([]byte(`{"usage":{"input_tokens":5629,"output_tokens":285},
		"modelUsage":{"m":{"inputTokens":99999,"outputTokens":99999}}}`), &e); err != nil {
		t.Fatal(err)
	}
	if in, out := e.tokens(); in != 5629 || out != 285 {
		t.Errorf("tokens() = %d/%d, want the top-level 5629/285", in, out)
	}
}

func TestEnvelopeTokensZeroWhenNeitherPresent(t *testing.T) {
	var e envelope
	if in, out := e.tokens(); in != 0 || out != 0 {
		t.Errorf("tokens() = %d/%d, want 0/0", in, out)
	}
}
