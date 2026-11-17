package claude

import (
	"strings"

	"github.com/MoeForge/moego/base64"
	"github.com/MoeForge/moego/ojson"
)

const reasoningDetailsSignaturePrefix = "cliproxy-reasoning-details:"

// Claude clients replay thinking signatures verbatim; keep the provider JSON opaque.
func reasoningDetailsBlock(details ojson.Result) []byte {
	if !details.IsArray() || len(details.Array()) == 0 {
		return nil
	}
	block := ojson.NewDocString(`{"type":"thinking","thinking":".","signature":""}`)
	_ = block.Set("signature", reasoningDetailsSignaturePrefix+base64.StdEncoding.EncodeToString([]byte(details.Raw)))
	return block.Bytes()
}

func decodeReasoningDetails(signature string) []ojson.Result {
	encoded, ok := strings.CutPrefix(signature, reasoningDetailsSignaturePrefix)
	if !ok {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || !ojson.ValidBytes(raw) {
		return nil
	}
	details := ojson.ParseBytes(raw)
	if !details.IsArray() {
		return nil
	}
	return details.Array()
}
