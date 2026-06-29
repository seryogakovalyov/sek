package distill

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/seryogakovalyov/sek/internal/llm"
	"github.com/seryogakovalyov/sek/internal/models"
)

const minModuleRouteConfidence = 0.60

func (p *Pipeline) routeModule(ctx context.Context, observation string) (*models.ModuleRoute, error) {
	resp, err := p.llm.Chat(ctx, llm.ChatRequest{
		Model: p.model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: `Classify this distilled observation into one SEK memory module.

Choose exactly one module:
- engineering: code, architecture, bugs, tests, build systems, APIs, repository conventions
- local-ai: local model serving, llama.cpp/vLLM, embeddings endpoints, GPU/runtime setup, model quirks
- agent-behavior: how Codex, Claude Code, Cursor, Opencode, or other agents use instructions and tools
- personal: durable user preferences, working style, recurring constraints
- company: team/company process, release policy, ownership, communication norms

Classify by the meaning of the observation, not by source/channel. A web UI, MCP client, CLI command, or agent name is source/channel metadata, not the module by itself.

If confidence is low, choose engineering. Do not invent modules.

Return only compact JSON:
{"module":"engineering","confidence":0.0,"reason":"short reason"}`},
			{Role: llm.RoleUser, Content: observation},
		},
		Temperature: 0,
	})
	if err != nil {
		return nil, err
	}

	var route models.ModuleRoute
	if err := json.Unmarshal([]byte(stripJSONFence(resp.Content)), &route); err != nil {
		return nil, fmt.Errorf("parse module route: %w", err)
	}
	if route.Module == "" {
		route.Module = models.ModuleEngineering
	}
	if !knownModule(route.Module) {
		route.Module = models.ModuleEngineering
		route.Confidence = 0
		route.Reason = "unknown module from classifier; fell back to engineering"
	}
	if route.Module != models.ModuleEngineering && route.Confidence < minModuleRouteConfidence {
		route.Module = models.ModuleEngineering
		route.Reason = "low-confidence module route; fell back to engineering"
	}
	return &route, nil
}

func knownModule(module string) bool {
	switch module {
	case models.ModuleEngineering,
		models.ModuleLocalAI,
		models.ModuleAgentBehavior,
		models.ModulePersonal,
		models.ModuleCompany:
		return true
	default:
		return false
	}
}

func stripJSONFence(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
