package keeper

import (
	"context"
	"strconv"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/lavanet/lava/utils"
	"github.com/lavanet/lava/x/conflict/types"
	tendermintcrypto "github.com/tendermint/tendermint/crypto"
	"golang.org/x/exp/slices"
)

func DetectionIndex(msg *types.MsgDetection, epochStart uint64) string {
	return msg.Creator + msg.ResponseConflict.ConflictRelayData0.Request.RelaySession.Provider + msg.ResponseConflict.ConflictRelayData1.Request.RelaySession.Provider + strconv.FormatUint(epochStart, 10)
}

func (k msgServer) Detection(goCtx context.Context, msg *types.MsgDetection) (*types.MsgDetectionResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	logger := k.Keeper.Logger(ctx)
	clientAddr, err := sdk.AccAddressFromBech32(msg.Creator)
	if err != nil {
		return nil, utils.LavaError(ctx, logger, "conflict_detection", map[string]string{"client": msg.Creator, "error": err.Error()}, "parsing client address")
	}
	if msg.FinalizationConflict != nil && msg.ResponseConflict == nil && msg.SameProviderConflict == nil {
		err := k.Keeper.ValidateFinalizationConflict(ctx, msg.FinalizationConflict, clientAddr)
		if err != nil {
			return nil, utils.LavaError(ctx, logger, "Finalization_conflict_detection", map[string]string{"client": msg.Creator, "error": err.Error()}, "Simulation: finalization conflict detection error")
		}
	} else if msg.FinalizationConflict == nil && msg.ResponseConflict == nil && msg.SameProviderConflict != nil {
		err := k.Keeper.ValidateSameProviderConflict(ctx, msg.SameProviderConflict, clientAddr)
		if err != nil {
			return nil, utils.LavaError(ctx, logger, "same_provider_conflict_detection", map[string]string{"client": msg.Creator, "error": err.Error()}, "Simulation: same provider conflict detection error")
		}
	} else if msg.FinalizationConflict == nil && msg.ResponseConflict != nil && msg.SameProviderConflict == nil {
		err := k.Keeper.ValidateResponseConflict(ctx, msg.ResponseConflict, clientAddr)
		if err != nil {
			return nil, utils.LavaError(ctx, logger, "response_conflict_detection", map[string]string{"client": msg.Creator, "error": err.Error()}, "Simulation: response conflict detection error")
		}

		// the conflict detection transaction is valid!, start a vote
		// TODO: 1. start a vote, with vote ID (unique, list index isn't good because its changing, use a map)
		// 2. create an event to declare vote
		// 3. accept incoming commit transactions for this vote,
		// 4. after vote ends, accept reveal transactions, strike down every provider that voted (only valid if there was a commit)
		// 5. majority wins, minority gets penalised
		epochStart, _, err := k.epochstorageKeeper.GetEpochStartForBlock(ctx, uint64(msg.ResponseConflict.ConflictRelayData0.Request.RelaySession.Epoch))
		if err != nil {
			return nil, utils.LavaError(ctx, logger, "response_conflict_detection", map[string]string{"client": msg.Creator, "provider0": msg.ResponseConflict.ConflictRelayData0.Request.RelaySession.Provider, "provider1": msg.ResponseConflict.ConflictRelayData1.Request.RelaySession.Provider}, "Simulation: could not get EpochStart for specific block")
		}
		index := DetectionIndex(msg, epochStart)
		found := k.Keeper.AllocateNewConflictVote(ctx, index)
		if found {
			return nil, utils.LavaError(ctx, logger, "response_conflict_detection", map[string]string{"client": msg.Creator, "provider0": msg.ResponseConflict.ConflictRelayData0.Request.RelaySession.Provider, "provider1": msg.ResponseConflict.ConflictRelayData1.Request.RelaySession.Provider}, "Simulation: conflict with is already open for this client and providers in this epoch")
		}
		conflictVote := types.ConflictVote{}
		conflictVote.Index = index
		conflictVote.VoteState = types.StateCommit
		conflictVote.VoteStartBlock = uint64(msg.ResponseConflict.ConflictRelayData0.Request.RelaySession.Epoch)
		epochBlocks, err := k.epochstorageKeeper.EpochBlocks(ctx, uint64(ctx.BlockHeight()))
		if err != nil {
			return nil, utils.LavaError(ctx, logger, "response_conflict_detection", map[string]string{"client": msg.Creator, "provider0": msg.ResponseConflict.ConflictRelayData0.Request.RelaySession.Provider, "provider1": msg.ResponseConflict.ConflictRelayData1.Request.RelaySession.Provider}, "Simulation: could not get epochblocks")
		}

		voteDeadline, err := k.Keeper.epochstorageKeeper.GetNextEpoch(ctx, uint64(ctx.BlockHeight())+k.VotePeriod(ctx)*epochBlocks)
		if err != nil {
			return nil, utils.LavaError(ctx, logger, "response_conflict_detection", map[string]string{"client": msg.Creator, "provider0": msg.ResponseConflict.ConflictRelayData0.Request.RelaySession.Provider, "provider1": msg.ResponseConflict.ConflictRelayData1.Request.RelaySession.Provider}, "Simulation: could not get NextEpoch")
		}
		conflictVote.VoteDeadline = voteDeadline
		conflictVote.ApiUrl = msg.ResponseConflict.ConflictRelayData0.Request.RelayData.ApiUrl
		conflictVote.ClientAddress = msg.Creator
		conflictVote.ChainID = msg.ResponseConflict.ConflictRelayData0.Request.RelaySession.SpecId
		conflictVote.RequestBlock = uint64(msg.ResponseConflict.ConflictRelayData0.Request.RelayData.RequestBlock)
		conflictVote.RequestData = msg.ResponseConflict.ConflictRelayData0.Request.RelayData.Data

		conflictVote.FirstProvider.Account = msg.ResponseConflict.ConflictRelayData0.Request.RelaySession.Provider
		conflictVote.FirstProvider.Response = tendermintcrypto.Sha256(msg.ResponseConflict.ConflictRelayData0.Reply.Data)
		conflictVote.SecondProvider.Account = msg.ResponseConflict.ConflictRelayData1.Request.RelaySession.Provider
		conflictVote.SecondProvider.Response = tendermintcrypto.Sha256(msg.ResponseConflict.ConflictRelayData1.Reply.Data)
		conflictVote.Votes = []types.Vote{}
		voters := k.Keeper.LotteryVoters(goCtx, epochStart, conflictVote.ChainID, []string{conflictVote.FirstProvider.Account, conflictVote.SecondProvider.Account})
		for _, voter := range voters {
			conflictVote.Votes = append(conflictVote.Votes, types.Vote{Address: voter, Hash: []byte{}, Result: types.NoVote})
		}

		k.SetConflictVote(ctx, conflictVote)

		eventData := map[string]string{"client": msg.Creator}
		eventData["voteID"] = conflictVote.Index
		eventData["chainID"] = conflictVote.ChainID
		eventData["connectionType"] = msg.ResponseConflict.ConflictRelayData0.Request.RelayData.ConnectionType
		eventData["apiURL"] = conflictVote.ApiUrl
		eventData["requestData"] = string(conflictVote.RequestData)
		eventData["requestBlock"] = strconv.FormatUint(conflictVote.RequestBlock, 10)
		eventData["voteDeadline"] = strconv.FormatUint(conflictVote.VoteDeadline, 10)
		eventData["voters"] = strings.Join(voters, ",")
		eventData["apiInterface"] = msg.ResponseConflict.ConflictRelayData0.Request.RelayData.ApiInterface

		utils.LogLavaEvent(ctx, logger, types.ConflictVoteDetectionEventName, eventData, "Simulation: Got a new valid conflict detection from consumer, starting new vote")
		return &types.MsgDetectionResponse{}, nil
	}

	eventData := map[string]string{"client": msg.Creator}
	utils.LogLavaEvent(ctx, logger, types.ConflictDetectionRecievedEventName, eventData, "Simulation: Got a new valid conflict detection from consumer")
	return &types.MsgDetectionResponse{}, nil
}

func (k Keeper) LotteryVoters(goCtx context.Context, epoch uint64, chainID string, exemptions []string) []string {
	ctx := sdk.UnwrapSDKContext(goCtx)
	entries, err := k.epochstorageKeeper.GetStakeEntryForAllProvidersEpoch(ctx, chainID, epoch)
	if err != nil {
		return make([]string, 0)
	}

	voters := make([]string, 0)
	for _, entry := range *entries {
		if !slices.Contains(exemptions, entry.Address) {
			voters = append(voters, entry.Address)
		}
	}

	return voters
}
