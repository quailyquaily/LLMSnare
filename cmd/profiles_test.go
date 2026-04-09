package cmd

import (
	"bytes"
	"strings"
	"testing"

	"llmsnare/internal/config"
)

func TestRenderProfileListShowsCoreFields(t *testing.T) {
	var out bytes.Buffer
	renderProfileList(&out, []namedProfile{
		{
			Name: "cf_llama",
			Profile: config.Profile{
				Provider: "cloudflare",
				Model:    "@cf/meta/llama-3.1-8b-instruct",
				Endpoint: "https://api.cloudflare.com/client/v4",
			},
		},
		{
			Name: "openai_gpt4o",
			Profile: config.Profile{
				Provider: "openai",
				Model:    "gpt-4o",
				Endpoint: "https://api.openai.com/v1",
			},
		},
	})

	got := out.String()
	for _, want := range []string{
		"Available Profiles",
		"- cf_llama",
		"provider: cloudflare",
		"model: @cf/meta/llama-3.1-8b-instruct",
		"endpoint: https://api.cloudflare.com/client/v4",
		"- openai_gpt4o",
		"provider: openai",
		"model: gpt-4o",
		"endpoint: https://api.openai.com/v1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("profile list missing %q:\n%s", want, got)
		}
	}
}
