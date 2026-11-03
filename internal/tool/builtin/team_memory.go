package builtin

import (
	"context"
	"encoding/json"

	"deepzora/internal/tool"
)

func teamMemoryTools(link *tool.TeamLink) []tool.Tool {
	return []tool.Tool{
		&teamTool{link: link, name: "read_shared_memory", readOnly: true, description: "List, search or read the team's shared memories, or a group's memories if you are a member. Use stable memory IDs and revisions when proposing a change.", schema: `{"type":"object","properties":{"tier":{"type":"string","enum":["team","group"]},"group":{"type":"string"},"operation":{"type":"string","enum":["list","search","read"]},"query":{"type":"string"},"name":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":20}},"required":["tier","operation"],"additionalProperties":false}`, run: func(ctx context.Context, dir tool.TeamDirectory, args json.RawMessage) (string, error) {
			var input struct {
				Tier  string `json:"tier"`
				Group string `json:"group"`
			}
			if err := decodeTeamArgs("read_shared_memory", args, &input); err != nil {
				return "", err
			}
			memory, ok := dir.(tool.TeamMemoryDirectory)
			if !ok {
				return "", tool.Refusal{Code: "team.memory_unavailable", Message: "Shared memory is unavailable on this host."}
			}
			return memory.ReadSharedMemory(ctx, input.Tier, input.Group, args)
		}},
		&teamTool{link: link, name: "remember_shared", readOnly: true, description: "Propose saving, updating or archiving a shared memory. Each exact change requires a new human decision, including under Full access. For updates or archive, use the stable id and expected_revision from read_shared_memory. Never save credentials.", schema: `{"type":"object","properties":{"tier":{"type":"string","enum":["team","group"]},"group":{"type":"string"},"operation":{"type":"string","enum":["save","archive"]},"id":{"type":"string"},"expected_revision":{"type":"integer","minimum":1},"name":{"type":"string"},"description":{"type":"string"},"type":{"type":"string","enum":["user","feedback","project","reference"]},"body":{"type":"string"}},"required":["tier","operation"],"additionalProperties":false}`, run: func(ctx context.Context, dir tool.TeamDirectory, args json.RawMessage) (string, error) {
			var input tool.TeamSharedMemoryRequest
			if err := decodeTeamArgs("remember_shared", args, &input); err != nil {
				return "", err
			}
			memory, ok := dir.(tool.TeamMemoryDirectory)
			if !ok {
				return "", tool.Refusal{Code: "team.memory_unavailable", Message: "Shared memory is unavailable on this host."}
			}
			return memory.RememberShared(ctx, input)
		}},
	}
}
