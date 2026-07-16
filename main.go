package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/ertval/two-tier-safe-ai-gate/agent"
	"github.com/ertval/two-tier-safe-ai-gate/gate"
	"github.com/ertval/two-tier-safe-ai-gate/mcp"
	"github.com/ertval/two-tier-safe-ai-gate/workflow"
	"github.com/inngest/inngestgo"
)

func main() {
	mcpFlag := flag.Bool("mcp", false, "Start the stdio Model Context Protocol (MCP) server")
	flag.Parse()

	if *mcpFlag {
		ctx := context.Background()
		mcp.StartMCPServer(ctx)
		return
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	// 1. Initialize Inngest Client
	inngestClient, err := inngestgo.NewClient(inngestgo.ClientOpts{
		AppID: "two-tier-safe-ai-gate",
	})
	if err != nil {
		log.Fatalf("Failed to initialize Inngest client: %v", err)
	}

	// 2. Register workflows
	workflow.RegisterPayoutWorkflow(inngestClient)

	// 3. Set up routes
	http.Handle("/inngest", inngestClient.Serve())
	http.HandleFunc("/run", handleAgentRun(inngestClient))
	http.HandleFunc("/approve", handleCfoApproval(inngestClient))

	log.Printf("Starting Two-Tier Safe AI Gate server on :%s...", port)
	log.Printf("Inngest handler endpoints served at http://localhost:%s/inngest", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// handleAgentRun executes Tier 1 (exploration) locally and sends the proposal to Inngest.
func handleAgentRun(client inngestgo.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Only POST requests are allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Instruction string `json:"instruction"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.Instruction == "" {
			http.Error(w, "Instruction must not be empty", http.StatusBadRequest)
			return
		}

		ctx := r.Context()

		// Execute Tier 1: Autonomous Proposal Extraction
		proposal, err := agent.ProposePayout(ctx, req.Instruction)
		if err != nil {
			http.Error(w, fmt.Sprintf("Tier 1 planning loop failed: %v", err), http.StatusInternalServerError)
			return
		}

		// Trigger Inngest to execute Tier 2 durably
		_, err = client.Send(ctx, inngestgo.GenericEvent[gate.Transaction]{
			Name: "agent/payout.proposed",
			Data: proposal,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to send proposal event to Inngest event bus: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":             "submitted_to_payout_pipeline",
			"transaction_id":     proposal.ID,
			"extracted_proposal": proposal,
		})
	}
}

// handleCfoApproval sends the CFO approval event to release pending payments.
func handleCfoApproval(client inngestgo.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Only POST requests are allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			TransactionID string `json:"transaction_id"`
			Approved      bool   `json:"approved"`
			Reason        string `json:"denial_reason,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.TransactionID == "" {
			http.Error(w, "Transaction ID must not be empty", http.StatusBadRequest)
			return
		}

		ctx := r.Context()

		// Send approval/denial event to Inngest
		_, err := client.Send(ctx, inngestgo.GenericEvent[any]{
			Name: "agent/payout.approved",
			Data: map[string]interface{}{
				"transaction_id": req.TransactionID,
				"approved":       req.Approved,
				"denial_reason":  req.Reason,
			},
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to send approval event to Inngest: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":         "decision_submitted",
			"transaction_id": req.TransactionID,
			"approved":       req.Approved,
		})
	}
}
