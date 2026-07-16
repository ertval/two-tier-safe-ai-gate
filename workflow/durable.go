package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/ertval/two-tier-safe-ai-gate/gate"
	"github.com/inngest/inngestgo"
	"github.com/inngest/inngestgo/step"
)

// RegisterPayoutWorkflow creates and registers the durable two-tier safe payout workflow.
func RegisterPayoutWorkflow(client inngestgo.Client) {
	_, err := inngestgo.CreateFunction(
		client,
		inngestgo.FunctionOpts{
			ID:   "payout-reconciliation-flow",
			Name: "Two-Tier Durable Payout Flow",
		},
		inngestgo.EventTrigger("agent/payout.proposed", nil),
		func(ctx context.Context, input inngestgo.Input[gate.Transaction]) (any, error) {
			tx := input.Event.Data

			// 1. Run deterministic Policy Gate check (Tier 1 -> Tier 2 boundary)
			verdict, err := step.Run(ctx, "verify-policy-bounds", func(ctx context.Context) (gate.VerificationVerdict, error) {
				v, err := gate.VerifyPolicy(tx)
				if err != nil {
					return gate.VerificationVerdict{}, err
				}
				return v, nil
			})
			if err != nil {
				return nil, fmt.Errorf("policy check failed: %w", err)
			}

			// If rejected, record audit trail and abort
			if !verdict.Approved && !verdict.RequiresHITL {
				_, _ = step.Run(ctx, "log-audit-rejection", func(ctx context.Context) (string, error) {
					return fmt.Sprintf("Audit Log: Payout rejected for user %s. Reason: %s", tx.UserID, verdict.Justification), nil
				})
				return map[string]any{
					"status":        "rejected",
					"justification": verdict.Justification,
				}, nil
			}

			// 2. If HITL approval is required, pause the workflow durably
			if verdict.RequiresHITL {
				// Log that we are entering a paused state
				_, _ = step.Run(ctx, "log-audit-paused", func(ctx context.Context) (string, error) {
					return fmt.Sprintf("Audit Log: Payout of $%.2f for user %s paused. Reason: %s", tx.Amount, tx.UserID, verdict.Justification), nil
				})

				// Wait for approval event (up to 24 hours)
				approvalEvent, err := step.WaitForEvent[inngestgo.Event](ctx, "wait-for-cfo-approval", step.WaitForEventOpts{
					Event:   "agent/payout.approved",
					Timeout: 24 * time.Hour,
					If:      inngestgo.StrPtr(fmt.Sprintf("event.data.transaction_id == '%s'", tx.ID)),
				})

				if err != nil {
					// Timeout reached, mark as cancelled
					_, _ = step.Run(ctx, "log-audit-timeout", func(ctx context.Context) (string, error) {
						return fmt.Sprintf("Audit Log: CFO approval timed out for transaction %s. Cancelling transaction.", tx.ID), nil
					})
					return map[string]any{
						"status":        "cancelled",
						"justification": "CFO approval timeout (24h exceeded)",
					}, nil
				}

				// Check CFO action
				isApproved := approvalEvent.Data["approved"].(bool)
				if !isApproved {
					denialReason, _ := approvalEvent.Data["denial_reason"].(string)
					_, _ = step.Run(ctx, "log-audit-denied", func(ctx context.Context) (string, error) {
						return fmt.Sprintf("Audit Log: CFO denied transaction %s. Justification: %s", tx.ID, denialReason), nil
					})
					return map[string]any{
						"status":        "denied",
						"justification": denialReason,
					}, nil
				}
			}

			// 3. Bounded Execution (Tier 2): Safe and guaranteed payout
			payoutResult, err := step.Run(ctx, "execute-erp-payout", func(ctx context.Context) (string, error) {
				// Simulating ERP payout integration
				// In production, this would make an idempotent HTTP call to the payment gateway or database ledger.
				time.Sleep(100 * time.Millisecond) // Mock network latency
				return fmt.Sprintf("SUCCESS: $%.2f transferred to User %s. Transaction Reference: TX-%s", tx.Amount, tx.UserID, tx.ID), nil
			})
			if err != nil {
				// Inngest automatically retries this step on failure based on default backoff rules
				return nil, err
			}

			// 4. Update Audit Ledger
			_, _ = step.Run(ctx, "update-audit-ledger", func(ctx context.Context) (string, error) {
				return fmt.Sprintf("LEDGER WRITE: Confirmed transaction %s. Details: %s", tx.ID, payoutResult), nil
			})

			return map[string]any{
				"status": "completed",
				"result": payoutResult,
			}, nil
		},
	)
	if err != nil {
		panic(fmt.Sprintf("failed to create function: %s", err))
	}
}
