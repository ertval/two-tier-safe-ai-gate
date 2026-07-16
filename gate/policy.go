package gate

import (
	"errors"
	"fmt"
)

// Transaction represents the proposed financial payload from Tier 1.
type Transaction struct {
	ID        string  `json:"id"`
	UserID    string  `json:"user_id"`
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
	RiskScore float64 `json:"risk_score"`
}

// VerificationVerdict represents the outcome of the policy gate.
type VerificationVerdict struct {
	Approved      bool   `json:"approved"`
	RequiresHITL  bool   `json:"requires_hitl"` // Human-in-the-loop approval
	Justification string `json:"justification"`
}

// VerifyPolicy runs deterministic checks on the proposed transaction payload.
func VerifyPolicy(tx Transaction) (VerificationVerdict, error) {
	if tx.Amount <= 0 {
		return VerificationVerdict{Approved: false, Justification: "Transaction amount must be positive"}, errors.New("invalid amount")
	}

	if tx.RiskScore > 0.8 {
		return VerificationVerdict{
			Approved:      false,
			RequiresHITL:  false,
			Justification: fmt.Sprintf("Transaction rejected: Risk score %.2f exceeds maximum threshold (0.80)", tx.RiskScore),
		}, nil
	}

	if tx.Amount > 500.0 {
		return VerificationVerdict{
			Approved:      false,
			RequiresHITL:  true,
			Justification: fmt.Sprintf("Transaction size ($%.2f) exceeds autonomous limit ($500.00). Requesting Human-in-the-Loop authorization.", tx.Amount),
		}, nil
	}

	return VerificationVerdict{
		Approved:      true,
		RequiresHITL:  false,
		Justification: "Transaction automatically approved by policy bounds.",
	}, nil
}
