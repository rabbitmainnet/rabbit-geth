// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package misc

import (
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

// ApplyRabbitVRFCoordinatorV1 installs the canonical Rabbit VRF Coordinator V1
// runtime as an irregular protocol state transition.
//
// The coordinator address becomes protocol-reserved at activation. Any balance
// sent to the address before activation is preserved, while pre-existing code,
// nonce and storage are deliberately discarded so they cannot contaminate the
// consensus-owned coordinator state.
func ApplyRabbitVRFCoordinatorV1(statedb vm.StateDB) {
	addr := params.RabbitVRFCoordinatorV1Address

	// Clone the balance before replacing the state object. GetBalance returns
	// state-owned data and must not be retained across CreateAccount.
	balance := new(uint256.Int).Set(statedb.GetBalance(addr))

	// This is an intentional irregular hard-fork transition. Replacing the
	// account guarantees a clean storage root, empty code and zero nonce.
	statedb.CreateAccount(addr)

	if !balance.IsZero() {
		statedb.AddBalance(addr, balance, tracing.BalanceChangeUnspecified)
	}

	statedb.CreateContract(addr)
	statedb.SetCode(addr, params.RabbitVRFCoordinatorV1Code, tracing.CodeChangeUnspecified)
	statedb.SetNonce(addr, 1, tracing.NonceChangeNewContract)
}
