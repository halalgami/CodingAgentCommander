package bedrock

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrock/types"
)

func TestBaseModelID(t *testing.T) {
	cases := map[string]string{
		"us.anthropic.claude-sonnet-4-20250514-v1:0":   "anthropic.claude-sonnet-4-20250514-v1:0",
		"eu.anthropic.claude-3-7-sonnet-20250219-v1:0": "anthropic.claude-3-7-sonnet-20250219-v1:0",
		"apac.amazon.nova-pro-v1:0":                    "amazon.nova-pro-v1:0",
		"us-gov.anthropic.claude-v2":                   "anthropic.claude-v2",
		"anthropic.claude-v2":                          "anthropic.claude-v2", // no geo prefix
	}
	for in, want := range cases {
		if got := baseModelID(in); got != want {
			t.Errorf("baseModelID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSlug(t *testing.T) {
	if got := slug("us.anthropic.claude-sonnet-4-20250514-v1:0"); got != "bedrock-us-anthropic-claude-sonnet-4-20250514-v1-0" {
		t.Errorf("slug = %q", got)
	}
}

func TestOnDemandAndAnthropic(t *testing.T) {
	claude := types.FoundationModelSummary{
		ModelId:                 aws.String("anthropic.claude-v2"),
		ProviderName:            aws.String("Anthropic"),
		InferenceTypesSupported: []types.InferenceType{types.InferenceTypeOnDemand},
	}
	if !onDemand(claude) || !isAnthropic(claude) {
		t.Error("claude should be on-demand and anthropic")
	}
	provisioned := types.FoundationModelSummary{
		ModelId:                 aws.String("amazon.titan"),
		ProviderName:            aws.String("Amazon"),
		InferenceTypesSupported: []types.InferenceType{types.InferenceTypeProvisioned},
	}
	if onDemand(provisioned) {
		t.Error("provisioned-only model must not report on-demand")
	}
	if isAnthropic(provisioned) {
		t.Error("amazon model is not anthropic")
	}
}

func TestListModelsRequiresCreds(t *testing.T) {
	// No network call should happen without creds.
	if _, err := ListModels(context.Background(), "", "", "", "us-east-1"); err == nil {
		t.Error("expected error when access key/secret are empty")
	}
}

func TestAgentCapable(t *testing.T) {
	yes := []string{
		"anthropic.claude-opus-4-8-v1:0",
		"amazon.nova-pro-v1:0",
		"meta.llama3-3-70b-instruct-v1:0",
		"mistral.mistral-large-2402-v1:0",
		"cohere.command-r-plus-v1:0",
		"qwen.qwen3-coder-480b-v1:0",
		"openai.gpt-oss-120b-1:0",
	}
	no := []string{
		"amazon.titan-text-express-v1",
		"deepseek.r1-v1:0",
		"meta.llama3-70b-instruct-v1:0", // 3.0 — no converse tool use
		"mistral.mixtral-8x7b-instruct-v0:1",
	}
	for _, id := range yes {
		if !agentCapable(id) {
			t.Errorf("agentCapable(%q) = false, want true", id)
		}
	}
	for _, id := range no {
		if agentCapable(id) {
			t.Errorf("agentCapable(%q) = true, want false", id)
		}
	}
}
