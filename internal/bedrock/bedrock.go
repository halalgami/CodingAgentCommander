// Package bedrock discovers the text-generation models an AWS account can invoke
// on Amazon Bedrock, so the catalog can be populated without hand-typing model
// IDs. It is read-only: it calls ListFoundationModels + ListInferenceProfiles.
package bedrock

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awsbedrock "github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrock/types"
)

// Model is one discovered, invokable Bedrock model ready to add to the catalog.
type Model struct {
	ID           string `json:"id"`           // catalog id (slug)
	Label        string `json:"label"`        // human label
	Upstream     string `json:"upstream"`     // "bedrock/<model-or-profile-id>"
	Region       string `json:"region"`       // region discovery ran against
	Anthropic    bool   `json:"anthropic"`    // Claude family — the strong coding agents
	AgentCapable bool   `json:"agentCapable"` // supports tool use — required by Claude Code
}

// agentFamilies are model-id fragments of families that support tool use via
// the Bedrock Converse API (per AWS "supported models and features"). Claude
// Code is agentic — models outside this list launch but fail on the first tool
// call. The AWS API doesn't expose tool-use capability, hence a curated list;
// unknown/new families default to false (badged, still addable).
var agentFamilies = []string{
	"anthropic.claude", "amazon.nova", "cohere.command-r",
	"meta.llama3-1", "meta.llama3-2", "meta.llama3-3", "meta.llama4",
	"mistral.mistral-large", "mistral.mistral-small", "mistral.pixtral",
	"ai21.jamba-1-5", "writer.palmyra", "qwen.qwen3", "openai.gpt-oss",
}

func agentCapable(baseID string) bool {
	id := strings.ToLower(baseID)
	for _, f := range agentFamilies {
		if strings.Contains(id, f) {
			return true
		}
	}
	return false
}

// knownGeos are the cross-region inference-profile prefixes (e.g. "us." in
// "us.anthropic.claude-..."); stripping one yields the base foundation-model id.
var knownGeos = []string{"us-gov", "us", "eu", "apac"}

// ListModels returns the text-generation models the given credentials can invoke
// in region. Cross-region inference profiles (the ids you actually want as
// upstream) are preferred; on-demand foundation models not covered by a profile
// are included too. Claude-family models are flagged and sorted first.
func ListModels(ctx context.Context, accessKey, secret, sessionToken, region string) ([]Model, error) {
	if accessKey == "" || secret == "" {
		return nil, fmt.Errorf("AWS access key and secret must be set in Providers first")
	}
	if region == "" {
		region = "us-east-1"
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cfg := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(accessKey, secret, sessionToken),
	}
	c := awsbedrock.NewFromConfig(cfg)

	fm, err := c.ListFoundationModels(ctx, &awsbedrock.ListFoundationModelsInput{
		ByOutputModality: types.ModelModalityText,
	})
	if err != nil {
		return nil, fmt.Errorf("list foundation models (check creds/region and bedrock:ListFoundationModels perm): %w", err)
	}
	idToModel := map[string]types.FoundationModelSummary{}
	for _, s := range fm.ModelSummaries {
		idToModel[aws.ToString(s.ModelId)] = s
	}

	var out []Model
	covered := map[string]bool{} // base model ids exposed via a profile

	// Prefer system-defined inference profiles (cross-region).
	if pf, perr := c.ListInferenceProfiles(ctx, &awsbedrock.ListInferenceProfilesInput{
		TypeEquals: types.InferenceProfileTypeSystemDefined,
	}); perr == nil {
		for _, p := range pf.InferenceProfileSummaries {
			pid := aws.ToString(p.InferenceProfileId)
			base, ok := idToModel[baseModelID(pid)]
			if !ok {
				continue // profile's base model isn't a text model in this region
			}
			covered[aws.ToString(base.ModelId)] = true
			out = append(out, Model{
				ID:           slug(pid),
				Label:        label(base) + " · cross-region",
				Upstream:     "bedrock/" + pid,
				Region:       region,
				Anthropic:    isAnthropic(base),
				AgentCapable: agentCapable(aws.ToString(base.ModelId)),
			})
		}
	}

	// On-demand foundation models not already covered by a profile.
	for _, s := range fm.ModelSummaries {
		if covered[aws.ToString(s.ModelId)] || !onDemand(s) {
			continue
		}
		id := aws.ToString(s.ModelId)
		out = append(out, Model{
			ID:           slug(id),
			Label:        label(s),
			Upstream:     "bedrock/" + id,
			Region:       region,
			Anthropic:    isAnthropic(s),
			AgentCapable: agentCapable(id),
		})
	}

	// Claude first (strongest coding agents), then other tool-capable models,
	// then the rest (can't run Claude Code's tool calls), each alphabetical.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Anthropic != out[j].Anthropic {
			return out[i].Anthropic
		}
		if out[i].AgentCapable != out[j].AgentCapable {
			return out[i].AgentCapable
		}
		return out[i].Label < out[j].Label
	})
	return out, nil
}

// baseModelID strips a cross-region geo prefix ("us.", "eu.", …) from a profile
// id to recover the underlying foundation-model id.
func baseModelID(profileID string) string {
	for _, g := range knownGeos {
		if rest, ok := strings.CutPrefix(profileID, g+"."); ok {
			return rest
		}
	}
	return profileID
}

func onDemand(s types.FoundationModelSummary) bool {
	for _, it := range s.InferenceTypesSupported {
		if it == types.InferenceTypeOnDemand {
			return true
		}
	}
	return false
}

func isAnthropic(s types.FoundationModelSummary) bool {
	return strings.EqualFold(aws.ToString(s.ProviderName), "Anthropic") ||
		strings.Contains(aws.ToString(s.ModelId), "anthropic")
}

func label(s types.FoundationModelSummary) string {
	p, n := aws.ToString(s.ProviderName), aws.ToString(s.ModelName)
	switch {
	case p != "" && n != "":
		return "Bedrock · " + p + " " + n
	case n != "":
		return "Bedrock · " + n
	default:
		return "Bedrock · " + aws.ToString(s.ModelId)
	}
}

// slug turns a model/profile id into a catalog id safe for tmux/env use.
func slug(id string) string {
	r := strings.NewReplacer(".", "-", ":", "-", "/", "-", " ", "-")
	s := "bedrock-" + strings.Trim(r.Replace(id), "-")
	return strings.ToLower(s)
}
