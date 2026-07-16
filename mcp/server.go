package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/ertval/two-tier-safe-ai-gate/gate"
)

// JSONRPCMessage represents a standard JSON-RPC 2.0 request or response envelope.
type JSONRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
	ID      interface{}     `json:"id"`
}

// JSONRPCError represents a standard JSON-RPC 2.0 error.
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// StartMCPServer runs a standard stdio MCP server for tool registry integrations.
func StartMCPServer(ctx context.Context) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Bytes()
		var msg JSONRPCMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			sendError(msg.ID, -32700, "Parse error")
			continue
		}

		handleMessage(msg)
	}
}

func handleMessage(msg JSONRPCMessage) {
	switch msg.Method {
	case "tools/list":
		// Return list of available gate policy tools
		tools := []map[string]interface{}{
			{
				"name":        "verify_transaction_policy",
				"description": "Deterministic gate policy verification for proposed transactions (amount, risk, user). Evaluates safety boundaries before execution.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"user_id":    map[string]interface{}{"type": "string"},
						"amount":     map[string]interface{}{"type": "number"},
						"risk_score": map[string]interface{}{"type": "number"},
					},
					"required": []string{"user_id", "amount", "risk_score"},
				},
			},
		}
		resultBytes, _ := json.Marshal(map[string]interface{}{"tools": tools})
		sendResult(msg.ID, resultBytes)

	case "tools/call":
		var params struct {
			Name      string `json:"name"`
			Arguments struct {
				UserID    string  `json:"user_id"`
				Amount    float64 `json:"amount"`
				RiskScore float64 `json:"risk_score"`
			} `json:"arguments"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			sendError(msg.ID, -32602, "Invalid params")
			return
		}

		if params.Name != "verify_transaction_policy" {
			sendError(msg.ID, -32601, "Method not found")
			return
		}

		tx := gate.Transaction{
			UserID:    params.Arguments.UserID,
			Amount:    params.Arguments.Amount,
			RiskScore: params.Arguments.RiskScore,
		}

		verdict, err := gate.VerifyPolicy(tx)
		var content []map[string]interface{}
		if err != nil {
			content = []map[string]interface{}{
				{"type": "text", "text": fmt.Sprintf("Error running policy verification: %s", err)},
			}
		} else {
			verdictJSON, _ := json.Marshal(verdict)
			content = []map[string]interface{}{
				{"type": "text", "text": string(verdictJSON)},
			}
		}

		resultBytes, _ := json.Marshal(map[string]interface{}{
			"content": content,
		})
		sendResult(msg.ID, resultBytes)

	default:
		sendError(msg.ID, -32601, "Method not found")
	}
}

func sendResult(id interface{}, result json.RawMessage) {
	resp := JSONRPCMessage{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	}
	respBytes, _ := json.Marshal(resp)
	fmt.Println(string(respBytes))
}

func sendError(id interface{}, code int, message string) {
	resp := JSONRPCMessage{
		JSONRPC: "2.0",
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
		},
		ID: id,
	}
	respBytes, _ := json.Marshal(resp)
	fmt.Println(string(respBytes))
}
