package agent

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/prompt"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/config"
)

//go:embed templates/agent_tool.md
var agentToolDescription []byte

type AgentParams struct {
	Prompt        string `json:"prompt" description:"The task for the agent to perform"`
	UseSmallModel bool   `json:"use_small_model,omitempty" description:"If true, use the small model for faster/cheaper execution (good for simple searches or lightweight tasks)"`
}

const (
	AgentToolName = "agent"
)

func (c *coordinator) agentTool(ctx context.Context) (fantasy.AgentTool, error) {
	agentCfg, ok := c.cfg.Agents[config.AgentTask]
	if !ok {
		return nil, errors.New("task agent not configured")
	}
	promptTemplate, err := taskPrompt(prompt.WithWorkingDir(c.cfg.WorkingDir()))
	if err != nil {
		return nil, err
	}

	// Build the large model agent for default use.
	largeAgent, err := c.buildAgent(ctx, promptTemplate, agentCfg, true)
	if err != nil {
		return nil, err
	}

	return fantasy.NewParallelAgentTool(
		AgentToolName,
		string(agentToolDescription),
		func(ctx context.Context, params AgentParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.Prompt == "" {
				return fantasy.NewTextErrorResponse("prompt is required"), nil
			}

			sessionID := tools.GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.ToolResponse{}, errors.New("session id missing from context")
			}

			agentMessageID := tools.GetMessageFromContext(ctx)
			if agentMessageID == "" {
				return fantasy.ToolResponse{}, errors.New("agent message id missing from context")
			}

			agentToolSessionID := c.sessions.CreateAgentToolSessionID(agentMessageID, call.ID)
			session, err := c.sessions.CreateTaskSession(ctx, agentToolSessionID, sessionID, "New Agent Session")
			if err != nil {
				return fantasy.ToolResponse{}, fmt.Errorf("error creating session: %s", err)
			}

			var selectedAgent SessionAgent
			var model Model
			var providerCfg config.ProviderConfig

			if params.UseSmallModel {
				// Build a small model agent for this call.
				_, small, err := c.buildAgentModels(ctx, true)
				if err != nil {
					return fantasy.ToolResponse{}, fmt.Errorf("error building models: %s", err)
				}

				systemPrompt, err := promptTemplate.Build(ctx, small.Model.Provider(), small.Model.Model(), *c.cfg)
				if err != nil {
					return fantasy.ToolResponse{}, fmt.Errorf("error building system prompt: %s", err)
				}

				smallProviderCfg, ok := c.cfg.Providers.Get(small.ModelCfg.Provider)
				if !ok {
					return fantasy.ToolResponse{}, errors.New("small model provider not configured")
				}

				agentTools, err := c.buildTools(ctx, agentCfg)
				if err != nil {
					return fantasy.ToolResponse{}, fmt.Errorf("error building tools: %s", err)
				}

				// Use small model for both slots (like agentic_fetch).
				selectedAgent = NewSessionAgent(SessionAgentOptions{
					LargeModel:           small,
					SmallModel:           small,
					SystemPromptPrefix:   smallProviderCfg.SystemPromptPrefix,
					SystemPrompt:         systemPrompt,
					DisableAutoSummarize: c.cfg.Options.DisableAutoSummarize,
					IsYolo:               c.permissions.SkipRequests(),
					Sessions:             c.sessions,
					Messages:             c.messages,
					Tools:                agentTools,
				})
				model = small
				providerCfg = smallProviderCfg
			} else {
				// Use the pre-built large model agent.
				selectedAgent = largeAgent
				model = largeAgent.Model()
				var ok bool
				providerCfg, ok = c.cfg.Providers.Get(model.ModelCfg.Provider)
				if !ok {
					return fantasy.ToolResponse{}, errors.New("model provider not configured")
				}
			}

			maxTokens := model.CatwalkCfg.DefaultMaxTokens
			if model.ModelCfg.MaxTokens != 0 {
				maxTokens = model.ModelCfg.MaxTokens
			}

			result, err := selectedAgent.Run(ctx, SessionAgentCall{
				SessionID:        session.ID,
				Prompt:           params.Prompt,
				MaxOutputTokens:  maxTokens,
				ProviderOptions:  getProviderOptions(model, providerCfg),
				Temperature:      model.ModelCfg.Temperature,
				TopP:             model.ModelCfg.TopP,
				TopK:             model.ModelCfg.TopK,
				FrequencyPenalty: model.ModelCfg.FrequencyPenalty,
				PresencePenalty:  model.ModelCfg.PresencePenalty,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse("error generating response"), nil
			}
			updatedSession, err := c.sessions.Get(ctx, session.ID)
			if err != nil {
				return fantasy.ToolResponse{}, fmt.Errorf("error getting session: %s", err)
			}
			parentSession, err := c.sessions.Get(ctx, sessionID)
			if err != nil {
				return fantasy.ToolResponse{}, fmt.Errorf("error getting parent session: %s", err)
			}

			parentSession.Cost += updatedSession.Cost

			_, err = c.sessions.Save(ctx, parentSession)
			if err != nil {
				return fantasy.ToolResponse{}, fmt.Errorf("error saving parent session: %s", err)
			}
			return fantasy.NewTextResponse(result.Response.Content.Text()), nil
		}), nil
}
