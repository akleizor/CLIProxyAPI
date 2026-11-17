package claude

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/MoeForge/moego/base64"
	"github.com/MoeForge/moego/ojson"
)

// The gateway uses index 0 for both calls, then omits the ID on argument continuations.
var geminiReasoningDetailsChunks = []string{
	`{"choices":[{"delta":{"content":"Checking.","reasoning_details":[{"type":"content","text":"Checking.","signature":"content-signature","extra":{"n":9007199254740993}}]}}]}`,
	`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_bash","type":"function","function":{"name":"Bash","arguments":"{\"command\":"}}],"reasoning_details":[{"type":"tool","text":"Bash","signature":"AY89..."}]}}]}`,
	`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"echo hi\"}"}}]}}]}`,
	`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read","type":"function","function":{"name":"Read","arguments":"{\"file_path\":"}}],"reasoning_details":[{"type":"tool","text":"Read","signature":"read-signature"}]}}]}`,
	`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read","function":{"arguments":"\"/etc/hosts\"}"}}]}}]}`,
	`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
}

const geminiReasoningDetails = `[{"type":"content","text":"Checking.","signature":"content-signature","extra":{"n":9007199254740993}},{"type":"tool","text":"Bash","signature":"AY89..."},{"type":"tool","text":"Read","signature":"read-signature"}]`

func TestStreaming_ReusedToolIndexIssue646(t *testing.T) {
	events := runStream(t, streamReq, geminiReasoningDetailsChunks...)
	assertSequentialContentBlocks(t, events)
	starts := toolUseStarts(events)
	if len(starts) != 2 {
		t.Fatalf("tool blocks = %d, want 2", len(starts))
	}
	for i, want := range []struct{ id, name string }{{"call_bash", "Bash"}, {"call_read", "Read"}} {
		block := ojson.Get(starts[i].Payload, "content_block")
		if block.Get("id").String() != want.id || block.Get("name").String() != want.name {
			t.Fatalf("tool %d = %s", i, block.Raw)
		}
	}
	var arguments []string
	for _, event := range events {
		if ojson.Get(event.Payload, "delta.type").String() == "input_json_delta" {
			arguments = append(arguments, ojson.Get(event.Payload, "delta.partial_json").String())
		}
	}
	if want := []string{`{"command":"echo hi"}`, `{"file_path":"/etc/hosts"}`}; !reflect.DeepEqual(arguments, want) {
		t.Fatalf("arguments = %q, want %q", arguments, want)
	}
	if got := lastStopReason(events); got != "tool_use" {
		t.Fatalf("stop reason = %q", got)
	}
}

func TestReasoningDetails_StreamReplayIssue646(t *testing.T) {
	events := runStream(t, streamReq, geminiReasoningDetailsChunks...)
	assertSequentialContentBlocks(t, events)
	replay := ojson.NewDocString(`{"messages":[{"role":"assistant","content":[]}]}`)
	var block *ojson.Doc
	for _, event := range events {
		payload := ojson.Parse(event.Payload)
		switch event.Type {
		case "content_block_start":
			block = ojson.NewDocString(payload.Get("content_block").Raw)
		case "content_block_delta":
			switch payload.Get("delta.type").String() {
			case "thinking_delta":
				_ = block.Set("thinking", block.Get("thinking").String()+payload.Get("delta.thinking").String())
			case "signature_delta":
				_ = block.Set("signature", block.Get("signature").String()+payload.Get("delta.signature").String())
			case "text_delta":
				_ = block.Set("text", block.Get("text").String()+payload.Get("delta.text").String())
			case "input_json_delta":
				_ = block.SetRaw("input", payload.Get("delta.partial_json").String())
			}
		case "content_block_stop":
			_ = replay.SetRawBytes("messages.0.content.-1", block.Bytes())
		}
	}
	out, err := ConvertClaudeRequestToOpenAI("gemini", replay.Bytes(), true)
	if err != nil {
		t.Fatal(err)
	}
	if got := ojson.GetBytes(out, "messages.0.reasoning_details").Raw; got != geminiReasoningDetails {
		t.Fatalf("reasoning_details = %s, want %s", got, geminiReasoningDetails)
	}
	if got := ojson.GetBytes(out, "messages.0.tool_calls.#").Int(); got != 2 {
		t.Fatalf("replayed tool calls = %d, want 2", got)
	}
}

func TestReasoningDetails_NonStreamReplay(t *testing.T) {
	for _, convert := range []struct {
		name string
		fn   func([]byte) []byte
	}{
		{"nonstream", func(raw []byte) []byte {
			return ConvertOpenAIResponseToClaudeNonStream(context.Background(), "", nil, nil, raw, nil)
		}},
		{"stream-disabled", func(raw []byte) []byte {
			var state any
			return ConvertOpenAIResponseToClaude(context.Background(), "", []byte(`{}`), nil, append([]byte("data: "), raw...), &state)[0]
		}},
	} {
		t.Run(convert.name, func(t *testing.T) {
			raw := ojson.NewDocString(`{"choices":[{"message":{"content":"done"},"finish_reason":"stop"}]}`)
			_ = raw.SetRaw("choices.0.message.reasoning_details", geminiReasoningDetails)
			response := convert.fn(raw.Bytes())
			replay := ojson.NewDocString(`{"messages":[{"role":"assistant"}]}`)
			_ = replay.SetRaw("messages.0.content", ojson.GetBytes(response, "content").Raw)
			out, err := ConvertClaudeRequestToOpenAIWithCompat("gemini", replay.Bytes(), true)
			if err != nil {
				t.Fatal(err)
			}
			if got := ojson.GetBytes(out, "messages.0.reasoning_details").Raw; got != geminiReasoningDetails {
				t.Fatalf("reasoning_details = %s, want %s", got, geminiReasoningDetails)
			}
		})
	}
}

func TestReasoningDetails_ReplayValidation(t *testing.T) {
	for _, tc := range []struct{ name, role, signature, want string }{
		{"assistant", "assistant", "cliproxy-reasoning-details:" + base64.StdEncoding.EncodeToString([]byte(geminiReasoningDetails)), geminiReasoningDetails},
		{"user", "user", "cliproxy-reasoning-details:" + base64.StdEncoding.EncodeToString([]byte(geminiReasoningDetails)), ""},
		{"ordinary-signature", "assistant", "ordinary-signature", ""},
		{"bad-base64", "assistant", "cliproxy-reasoning-details:!", ""},
		{"bad-json", "assistant", "cliproxy-reasoning-details:" + base64.StdEncoding.EncodeToString([]byte(`[{`)), ""},
		{"not-array", "assistant", "cliproxy-reasoning-details:" + base64.StdEncoding.EncodeToString([]byte(`{"signature":"x"}`)), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			replay := ojson.NewDocString(`{"messages":[{"content":[{"type":"thinking","thinking":"carrier"},{"type":"text","text":"hello"}]}]}`)
			_ = replay.Set("messages.0.role", tc.role)
			_ = replay.Set("messages.0.content.0.signature", tc.signature)
			out, err := ConvertClaudeRequestToOpenAI("gemini", replay.Bytes(), true)
			if err != nil {
				t.Fatal(err)
			}
			if got := ojson.GetBytes(out, "messages.0.reasoning_details").Raw; got != tc.want {
				t.Fatalf("reasoning_details = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestStreaming_NoReasoningDetailsAddsNoSignature(t *testing.T) {
	events := runStream(t, streamReq, `{"choices":[{"delta":{"content":"hello"},"finish_reason":"stop"}]}`)
	for _, event := range events {
		if strings.Contains(event.Payload, "signature") || ojson.Get(event.Payload, "content_block.type").String() == "thinking" {
			t.Fatalf("unexpected reasoning block: %s", event.Payload)
		}
	}
}
