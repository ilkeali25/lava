package keeper

import (
	"fmt"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/lavanet/lava/utils"
	epochstoragetypes "github.com/lavanet/lava/x/epochstorage/types"
	"github.com/lavanet/lava/x/pairing/types"
	spectypes "github.com/lavanet/lava/x/spec/types"
)

func (k Keeper) UnstakeEntry(ctx sdk.Context, provider bool, chainID string, creator string, unstakeDescription string) error {
	logger := k.Logger(ctx)
	var stake_type string
	if provider {
		stake_type = epochstoragetypes.ProviderKey
	} else {
		stake_type = epochstoragetypes.ClientKey
	}
	// TODO: validate chainID basic validation

	// we can unstake disabled specs, but not missing ones
	_, found := k.specKeeper.GetSpec(ctx, chainID)
	if !found {
		return utils.LavaError(ctx, logger, "unstake_spec_missing", map[string]string{"spec": chainID}, "trying to unstake an entry on missing spec")
	}
	senderAddr, err := sdk.AccAddressFromBech32(creator)
	if err != nil {
		details := map[string]string{stake_type: creator, "error": err.Error()}
		return utils.LavaError(ctx, logger, "unstake_"+stake_type+"_addr", details, "invalid "+stake_type+" address")
	}

	existingEntry, entryExists, indexInStakeStorage := k.epochStorageKeeper.GetStakeEntryByAddressCurrent(ctx, stake_type, chainID, senderAddr)
	if !entryExists {
		details := map[string]string{stake_type: creator, "spec": chainID}
		return utils.LavaError(ctx, logger, stake_type+"_unstake_entry", details, "can't unstake Entry, stake entry not found for address")
	}
	err = k.epochStorageKeeper.RemoveStakeEntryCurrent(ctx, stake_type, chainID, indexInStakeStorage)
	if err != nil {
		details := map[string]string{stake_type: creator, "spec": chainID, "index": strconv.FormatUint(indexInStakeStorage, 10)}
		return utils.LavaError(ctx, logger, stake_type+"_unstake_entry", details, "can't remove stake Entry, stake entry not found in index")
	}

	details := map[string]string{
		"address":     existingEntry.GetAddress(),
		"chainID":     existingEntry.GetChain(),
		"geolocation": strconv.FormatUint(existingEntry.GetGeolocation(), 10),
		"moniker":     existingEntry.GetMoniker(),
		"stake":       existingEntry.GetStake().Amount.String(),
	}
	utils.LogLavaEvent(ctx, logger, types.UnstakeCommitNewEventName(provider), details, unstakeDescription)

	unstakeHoldBlocks, err := k.unstakeHoldBlocks(ctx, existingEntry.Chain, provider)
	if err != nil {
		return err
	}

	return k.epochStorageKeeper.AppendUnstakeEntry(ctx, stake_type, existingEntry, unstakeHoldBlocks)
}

func (k Keeper) CheckUnstakingForCommit(ctx sdk.Context) error {
	// this pops all the entries that had their deadline pass
	unstakingEntriesToCredit := k.epochStorageKeeper.PopUnstakeEntries(ctx, epochstoragetypes.ProviderKey, uint64(ctx.BlockHeight()))

	if unstakingEntriesToCredit != nil {
		err := k.creditUnstakingEntries(ctx, true, unstakingEntriesToCredit) // true for providers
		if err != nil {
			panic(err.Error())
		}
	}
	// no providers entries to handle, check clients
	unstakingEntriesToCredit = k.epochStorageKeeper.PopUnstakeEntries(ctx, epochstoragetypes.ClientKey, uint64(ctx.BlockHeight()))
	if unstakingEntriesToCredit != nil {
		err := k.creditUnstakingEntries(ctx, false, unstakingEntriesToCredit) // false for clients
		if err != nil {
			panic(err.Error())
		}
	}
	return nil
}

func (k Keeper) creditUnstakingEntries(ctx sdk.Context, provider bool, entriesToUnstake []epochstoragetypes.StakeEntry) error {
	logger := k.Logger(ctx)
	var stake_type string
	if provider {
		stake_type = epochstoragetypes.ProviderKey
	} else {
		stake_type = epochstoragetypes.ClientKey
	}
	verifySufficientAmountAndSendFromModuleToAddress := func(ctx sdk.Context, k Keeper, addr sdk.AccAddress, neededAmount sdk.Coin) (bool, error) {
		moduleBalance := k.bankKeeper.GetBalance(ctx, k.accountKeeper.GetModuleAddress(types.ModuleName), epochstoragetypes.TokenDenom)
		if moduleBalance.IsLT(neededAmount) {
			return false, fmt.Errorf("insufficient balance for unstaking %s current balance: %s", neededAmount, moduleBalance)
		}
		err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, addr, []sdk.Coin{neededAmount})
		if err != nil {
			return false, fmt.Errorf("invalid transfer coins from module, %s to account %s", err, addr)
		}
		return true, nil
	}
	for _, unstakingEntry := range entriesToUnstake {
		details := map[string]string{"spec": unstakingEntry.Chain, stake_type: unstakingEntry.Address, "stake": unstakingEntry.Stake.String()}
		if unstakingEntry.StakeAppliedBlock <= uint64(ctx.BlockHeight()) {
			// found an entry that needs handling
			receiverAddr, err := sdk.AccAddressFromBech32(unstakingEntry.Address)
			if err != nil {
				panic(fmt.Sprintf("error getting AccAddress from : %s error: %s", unstakingEntry.Address, err))
			}
			if unstakingEntry.Stake.Amount.GT(sdk.ZeroInt()) {
				// transfer stake money to the stake entry account
				valid, err := verifySufficientAmountAndSendFromModuleToAddress(ctx, k, receiverAddr, unstakingEntry.Stake)
				if !valid {
					details["error"] = err.Error()
					utils.LavaError(ctx, logger, stake_type+"_unstaking_credit", details, "verifySufficientAmountAndSendFromModuleToAddress Failed,")
					panic(fmt.Sprintf("error unstaking : %s", err))
				}
				utils.LogLavaEvent(ctx, logger, types.UnstakeCommitNewEventName(provider), details, "Unstaking Providers Commit")
			}
		} else {
			// found an entry that isn't handled now, but later because its stakeAppliedBlock isnt current block
			utils.LavaError(ctx, logger, stake_type+"_unstaking", details, "trying to unstake while its stakeAppliedBlock wasn't reached")
		}
	}
	return nil
}

func (k Keeper) unstakeHoldBlocks(ctx sdk.Context, chainID string, isProvider bool) (uint64, error) {
	spec, found := k.specKeeper.GetSpec(ctx, chainID)
	if !found {
		return 0, fmt.Errorf("coult not find spec %s", chainID)
	}

	if isProvider && spec.ProvidersTypes == spectypes.Spec_static {
		return k.epochStorageKeeper.UnstakeHoldBlocksStatic(ctx, uint64(ctx.BlockHeight())), nil
	} else {
		return k.epochStorageKeeper.UnstakeHoldBlocks(ctx, uint64(ctx.BlockHeight())), nil
	}
}
