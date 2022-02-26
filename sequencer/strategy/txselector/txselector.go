package txselector

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/hermeznetwork/hermez-core/pool"
	"github.com/hermeznetwork/hermez-core/state"
)

// TxSelector interface for different types of selection
type TxSelector interface {
	// SelectTxs selecting txs and returning selected txs, hashes of the selected txs (to not build array multiple times) and hashes of invalid txs
	SelectTxs(batchProcessor batchProcessor, pendingTxs []pool.Transaction, sequencerAddress common.Address) ([]*types.Transaction, []string, []string, error)
}

// AcceptAll that accept all transactions
type AcceptAll struct{}

// NewTxSelectorAcceptAll init function
func NewTxSelectorAcceptAll() TxSelector {
	return &AcceptAll{}
}

// SelectTxs selects all transactions and don't check anything
func (s *AcceptAll) SelectTxs(batchProcessor batchProcessor, pendingTxs []pool.Transaction, sequencerAddress common.Address) ([]*types.Transaction, []string, []string, error) {
	selectedTxs := make([]*types.Transaction, 0, len(pendingTxs))
	selectedTxsHashes := make([]string, 0, len(pendingTxs))
	for _, tx := range pendingTxs {
		t := tx.Transaction
		selectedTxs = append(selectedTxs, &t)
		selectedTxsHashes = append(selectedTxsHashes, tx.Hash().Hex())
	}
	return selectedTxs, selectedTxsHashes, nil, nil
}

// Base tx selector with basic selection algorithm. Accepts different tx sorting and tx profitability checking structs
type Base struct {
	TxSorter TxSorter
}

// NewTxSelectorBase init function
func NewTxSelectorBase(cfg Config) TxSelector {
	var sorter TxSorter

	switch cfg.TxSorterType {
	case ByCostAndTime:
		sorter = &TxSorterByCostAndTime{}
	case ByCostAndNonce:
		sorter = &TxSorterByCostAndNonce{}
	}

	return &Base{
		TxSorter: sorter,
	}
}

// SelectTxs process txs and split valid txs into batches of txs. This process should be completed in less than selectionTime
func (b *Base) SelectTxs(batchProcessor batchProcessor, pendingTxs []pool.Transaction, sequencerAddress common.Address) ([]*types.Transaction, []string, []string, error) {
	sortedTxs := b.TxSorter.SortTxs(pendingTxs)
	var (
		selectedTxs                         []*types.Transaction
		selectedTxsHashes, invalidTxsHashes []string
	)
	for _, tx := range sortedTxs {
		t := tx.Transaction
		result := batchProcessor.ProcessTransaction(&t, sequencerAddress)
		if result.Failed() {
			err := result.Err
			if state.InvalidTxErrors[err.Error()] {
				invalidTxsHashes = append(invalidTxsHashes, tx.Hash().Hex())
			} else if errors.Is(err, state.ErrNonceIsBiggerThanAccountNonce) {
				// this means, that this tx could be valid in the future, but can't be selected at this moment
				continue
			} else if errors.Is(err, state.ErrInvalidCumulativeGas) {
				// this means, that cumulative gas from txs is exceeded max amount
				return selectedTxs, selectedTxsHashes, invalidTxsHashes, nil
			} else {
				return nil, nil, nil, err
			}
		} else {
			selectedTxs = append(selectedTxs, &t)
			selectedTxsHashes = append(selectedTxsHashes, t.Hash().Hex())
		}
	}

	return selectedTxs, selectedTxsHashes, invalidTxsHashes, nil
}
