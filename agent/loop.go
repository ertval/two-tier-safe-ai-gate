package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ertval/two-tier-safe-ai-gate/gate"
	"github.com/google/uuid"
	openai "github.com/sashabaranov/go-openai"
)

// ProposePayout runs Tier 1 autonomous exploration. It parses raw text instructions
// using an LLM (or a regex fallback if no API key is provided) to build a structured Transaction proposal.
func ProposePayout(ctx context.Context, instruction string) (gate.Transaction, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		// Mock/Deterministic fallback for offline running
		return runMockExtraction(instruction)
	}

	client := openai.NewClient(apiKey)

	systemPrompt := `You are a financial agent. Extract transaction details from user instructions.
Return ONLY valid JSON matching this schema:
{
  "user_id": "string",
  "amount": number,
  "currency": "string",
  "risk_score": number
}
Evaluate the risk of the transaction based on context (e.g., refunds for broken items might have lower risk (0.1-0.3), requests from unknown users or with suspicious language might have higher risk (0.7-0.9)).`

	resp, err := client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: openai.GPT4oMini,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: systemPrompt,
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: instruction,
				},
			},
			Temperature: 0,
		},
	)
	if err != nil {
		return gate.Transaction{}, fmt.Errorf("LLM call failed: %w", err)
	}

	content := resp.Choices[0].Message.Content
	// Strip markdown blocks if any
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var tx gate.Transaction
	if err := json.Unmarshal([]byte(content), &tx); err != nil {
		return gate.Transaction{}, fmt.Errorf("failed to parse LLM response JSON: %w", err)
	}

	// Assign generated ID
	tx.ID = uuid.New().String()
	if tx.Currency == "" {
		tx.Currency = "USD"
	}

	return tx, nil
}

func runMockExtraction(instruction string) (gate.Transaction, error) {
	tx := gate.Transaction{
		ID:       uuid.New().String(),
		Currency: "USD",
	}

	// Simple heuristic extraction for mock mode
	lower := strings.ToLower(instruction)
	if strings.Contains(lower, "user_") {
		parts := strings.Split(lower, "user_")
		if len(parts) > 1 {
			userPart := strings.Fields(parts[1])[0]
			tx.UserID = "user_" + userPart
		}
	} else {
		tx.UserID = "user_default"
	}

	if strings.Contains(lower, "$") {
		parts := strings.Split(lower, "$")
		if len(parts) > 1 {
			var amount float64
			_, _ = fmt.Sscanf(parts[1], "%f", &amount)
			tx.Amount = amount
		}
	} else {
		tx.Amount = 100.0 // Default mock amount
	}

	// Assign risk score dynamically based on instruction words for demo
	if strings.Contains(lower, "suspicious") || strings.Contains(lower, "hacker") {
		tx.RiskScore = 0.95
	} else if tx.Amount > 500 {
		tx.RiskScore = 0.50
	} else {
		tx.RiskScore = 0.15
	}

	return tx, nil
}
