package nta

import (
	"encoding/json"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/rss3-network/global-indexer/schema"
	"github.com/shopspring/decimal"
)

type GetStakeTransactionsRequest struct {
	Cursor  *common.Hash                 `query:"cursor"`
	Staker  *common.Address              `query:"staker"`
	Node    *common.Address              `query:"node"`
	Type    *schema.StakeTransactionType `query:"type"`
	Pending *bool                        `query:"pending"`
	Limit   int                          `query:"limit" default:"50" min:"1" max:"100"`
}

type GetStakeTransactionRequest struct {
	TransactionHash *common.Hash                 `param:"transaction_hash"`
	Type            *schema.StakeTransactionType `query:"type"`
}

type GetStakeTransactionsResponseData []*StakeTransaction

type GetStakeTransactionResponseData *StakeTransaction

type StakeTransaction struct {
	ID        common.Hash                `json:"id"`
	Staker    common.Address             `json:"staker"`
	Node      common.Address             `json:"node"`
	Value     decimal.Decimal            `json:"value"`
	ChipIDs   []*big.Int                 `json:"chip_ids,omitempty"`
	Event     StakeTransactionEventTypes `json:"event"`
	Finalized bool                       `json:"finalized"`
}

type StakeTransactionEventTypes struct {
	Deposit    *StakeTransactionEventTypeDeposit    `json:"deposit,omitempty"`
	Withdraw   *StakeTransactionEventTypeWithdraw   `json:"withdraw,omitempty"`
	Stake      *StakeTransactionEventTypeStake      `json:"stake,omitempty"`
	Unstake    *StakeTransactionEventTypeUnstake    `json:"unstake,omitempty"`
	MergeChips *StakeTransactionEventTypeMergeChips `json:"merge_chips,omitempty"`
}

type StakeTransactionEventTypeDeposit struct {
	Deposited *StakeTransactionEvent `json:"deposited,omitempty"`
}

type StakeTransactionEventTypeWithdraw struct {
	Requested *StakeTransactionEvent `json:"requested,omitempty"`
	Claimed   *StakeTransactionEvent `json:"claimed,omitempty"`
}

type StakeTransactionEventTypeStake struct {
	Staked *StakeTransactionEvent `json:"staked,omitempty"`
}

type StakeTransactionEventTypeUnstake struct {
	Requested *StakeTransactionEvent `json:"requested,omitempty"`
	Claimed   *StakeTransactionEvent `json:"claimed,omitempty"`
}

type StakeTransactionEventTypeMergeChips struct {
	Merged *StakeTransactionEvent   `json:"merged,omitempty"`
	Burned []*StakeTransactionEvent `json:"burned,omitempty"`
}

type StakeTransactionEvent struct {
	Block       TransactionEventBlock       `json:"block"`
	Transaction TransactionEventTransaction `json:"transaction"`
	Metadata    json.RawMessage             `json:"metadata,omitempty"`
}

func NewStakeTransaction(transaction *schema.StakeTransaction, events []*schema.StakeEvent) GetStakeTransactionResponseData {
	transactionModel := StakeTransaction{
		ID:        transaction.ID,
		Staker:    transaction.User,
		Node:      transaction.Node,
		ChipIDs:   transaction.ChipIDs,
		Value:     decimal.NewFromBigInt(transaction.Value, 0),
		Finalized: transaction.Finalized,
	}

	switch transaction.Type {
	case schema.StakeTransactionTypeDeposit:
		transactionModel.Event.Deposit = new(StakeTransactionEventTypeDeposit)
	case schema.StakeTransactionTypeWithdraw:
		transactionModel.Event.Withdraw = new(StakeTransactionEventTypeWithdraw)
	case schema.StakeTransactionTypeStake:
		transactionModel.Event.Stake = new(StakeTransactionEventTypeStake)
	case schema.StakeTransactionTypeUnstake:
		transactionModel.Event.Unstake = new(StakeTransactionEventTypeUnstake)
	case schema.StakeTransactionTypeMergeChips:
		transactionModel.Event.MergeChips = new(StakeTransactionEventTypeMergeChips)
	}

	for _, event := range events {
		eventModel := StakeTransactionEvent{
			Block: TransactionEventBlock{
				Hash:      event.BlockHash,
				Number:    event.BlockNumber,
				Timestamp: event.BlockTimestamp.Unix(),
			},
			Transaction: TransactionEventTransaction{
				Hash:  event.TransactionHash,
				Index: event.TransactionIndex,
			},
			Metadata: event.Metadata,
		}

		switch transaction.Type {
		case schema.StakeTransactionTypeDeposit:
			switch event.Type {
			case schema.StakeEventTypeDepositDeposited:
				transactionModel.Event.Deposit.Deposited = &eventModel
			}
		case schema.StakeTransactionTypeWithdraw:
			switch event.Type {
			case schema.StakeEventTypeWithdrawRequested:
				transactionModel.Event.Withdraw.Requested = &eventModel
			case schema.StakeEventTypeWithdrawClaimed:
				transactionModel.Event.Withdraw.Claimed = &eventModel
			}
		case schema.StakeTransactionTypeStake:
			switch event.Type {
			case schema.StakeEventTypeStakeStaked:
				transactionModel.Event.Stake.Staked = &eventModel
			}
		case schema.StakeTransactionTypeUnstake:
			switch event.Type {
			case schema.StakeEventTypeUnstakeRequested:
				transactionModel.Event.Unstake.Requested = &eventModel
			case schema.StakeEventTypeUnstakeClaimed:
				transactionModel.Event.Unstake.Claimed = &eventModel
			}
		case schema.StakeTransactionTypeMergeChips:
			switch event.Type {
			case schema.StakeEventTypeChipsMerged:
				transactionModel.Event.MergeChips.Merged = &eventModel
			}
		}
	}

	return &transactionModel
}
