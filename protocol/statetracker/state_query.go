package statetracker

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/dgraph-io/ristretto"
	reliabilitymanager "github.com/lavanet/lava/protocol/rpcprovider/reliabilitymanager"
	"github.com/lavanet/lava/protocol/rpcprovider/rewardserver"
	"github.com/lavanet/lava/utils"
	conflicttypes "github.com/lavanet/lava/x/conflict/types"
	epochstoragetypes "github.com/lavanet/lava/x/epochstorage/types"
	pairingtypes "github.com/lavanet/lava/x/pairing/types"
	spectypes "github.com/lavanet/lava/x/spec/types"
)

const (
	CacheMaxCost                = 10 * 1024 // 10K cost
	CacheNumCounters            = 100000    // expect 10K items
	DefaultTimeToLiveExpiration = 30 * time.Minute
	PairingRespKey              = "pairing-resp"
	VerifyPairingRespKey        = "verify-pairing-resp"
	VrfPkAndMaxCuResponseKey    = "vrf-and-max-cu-resp"
)

type StateQuery struct {
	SpecQueryClient         spectypes.QueryClient
	PairingQueryClient      pairingtypes.QueryClient
	EpochStorageQueryClient epochstoragetypes.QueryClient
	ResponsesCache          *ristretto.Cache
}

func NewStateQuery(ctx context.Context, clientCtx client.Context) *StateQuery {
	sq := &StateQuery{}
	sq.SpecQueryClient = spectypes.NewQueryClient(clientCtx)
	sq.PairingQueryClient = pairingtypes.NewQueryClient(clientCtx)
	sq.EpochStorageQueryClient = epochstoragetypes.NewQueryClient(clientCtx)
	cache, err := ristretto.NewCache(&ristretto.Config{NumCounters: CacheNumCounters, MaxCost: CacheMaxCost, BufferItems: 64})
	if err != nil {
		utils.LavaFormatFatal("failed setting up cache for queries", err)
	}
	sq.ResponsesCache = cache
	return sq
}

func (csq *StateQuery) GetSpec(ctx context.Context, chainID string) (*spectypes.Spec, error) {
	spec, err := csq.SpecQueryClient.Spec(ctx, &spectypes.QueryGetSpecRequest{
		ChainID: chainID,
	})
	if err != nil {
		return nil, utils.LavaFormatError("Failed Querying spec for chain", err, utils.Attribute{Key: "ChainID", Value: chainID})
	}
	return &spec.Spec, nil
}

type ConsumerStateQuery struct {
	StateQuery
	clientCtx   client.Context
	lastChainID string
}

func NewConsumerStateQuery(ctx context.Context, clientCtx client.Context) *ConsumerStateQuery {
	csq := &ConsumerStateQuery{StateQuery: *NewStateQuery(ctx, clientCtx), clientCtx: clientCtx, lastChainID: ""}
	return csq
}

func (csq *ConsumerStateQuery) GetPairing(ctx context.Context, chainID string, latestBlock int64) (pairingList []epochstoragetypes.StakeEntry, epoch uint64, nextBlockForUpdate uint64, errRet error) {
	if chainID == "" {
		if csq.lastChainID != "" {
			chainID = csq.lastChainID
		}
		if chainID == "" {
			chainID = "LAV1"
			utils.LavaFormatWarning("failed to run get pairing as there is no entry for empty chainID call, using default chainID", nil, utils.Attribute{Key: "chainID", Value: chainID})
		}
	}

	cachedInterface, found := csq.ResponsesCache.Get(PairingRespKey + chainID)
	if found && cachedInterface != nil {
		if cachedResp, ok := cachedInterface.(*pairingtypes.QueryGetPairingResponse); ok {
			if cachedResp.BlockOfNextPairing > uint64(latestBlock) {
				return cachedResp.Providers, cachedResp.CurrentEpoch, cachedResp.BlockOfNextPairing, nil
			}
		} else {
			utils.LavaFormatError("invalid cache entry - failed casting response", nil, utils.Attribute{Key: "castingType", Value: "*pairingtypes.QueryGetPairingResponse"}, utils.Attribute{Key: "type", Value: cachedInterface})
		}
	}

	pairingResp, err := csq.PairingQueryClient.GetPairing(ctx, &pairingtypes.QueryGetPairingRequest{
		ChainID: chainID,
		Client:  csq.clientCtx.FromAddress.String(),
	})
	if err != nil {
		return nil, 0, 0, utils.LavaFormatError("Failed in get pairing query", err, utils.Attribute{})
	}
	csq.lastChainID = chainID
	csq.ResponsesCache.SetWithTTL(PairingRespKey+chainID, pairingResp, 1, DefaultTimeToLiveExpiration)
	return pairingResp.Providers, pairingResp.CurrentEpoch, pairingResp.BlockOfNextPairing, nil
}

func (csq *ConsumerStateQuery) GetMaxCUForUser(ctx context.Context, chainID string, epoch uint64) (maxCu uint64, err error) {
	address := csq.clientCtx.FromAddress.String()
	UserEntryRes, err := csq.PairingQueryClient.UserEntry(ctx, &pairingtypes.QueryUserEntryRequest{ChainID: chainID, Address: address, Block: epoch})
	if err != nil {
		return 0, utils.LavaFormatError("failed querying StakeEntry for consumer", err, utils.Attribute{Key: "chainID", Value: chainID}, utils.Attribute{Key: "address", Value: address}, utils.Attribute{Key: "block", Value: epoch})
	}
	return UserEntryRes.GetMaxCU(), nil
}

type ProviderStateQuery struct {
	StateQuery
	clientCtx client.Context
}

func NewProviderStateQuery(ctx context.Context, clientCtx client.Context) *ProviderStateQuery {
	csq := &ProviderStateQuery{StateQuery: *NewStateQuery(ctx, clientCtx), clientCtx: clientCtx}
	return csq
}

func (psq *ProviderStateQuery) GetVrfPkAndMaxCuForUser(ctx context.Context, consumerAddress string, chainID string, epoch uint64) (vrfPk *utils.VrfPubKey, maxCu uint64, err error) {
	key := psq.entryKey(consumerAddress, chainID, epoch, "")
	cachedInterface, found := psq.ResponsesCache.Get(VrfPkAndMaxCuResponseKey + key)
	var userEntryRes *pairingtypes.QueryUserEntryResponse = nil
	if found && cachedInterface != nil {
		if cachedResp, ok := cachedInterface.(*pairingtypes.QueryUserEntryResponse); ok {
			userEntryRes = cachedResp
		} else {
			utils.LavaFormatError("invalid cache entry - failed casting response", nil, utils.Attribute{Key: "castingType", Value: "*pairingtypes.QueryUserEntryResponse"}, utils.Attribute{Key: "type", Value: fmt.Sprintf("%T", cachedInterface)})
		}
	}
	if userEntryRes == nil {
		userEntryRes, err = psq.PairingQueryClient.UserEntry(ctx, &pairingtypes.QueryUserEntryRequest{ChainID: chainID, Address: consumerAddress, Block: epoch})
		if err != nil {
			return nil, 0, utils.LavaFormatError("StakeEntry querying for consumer failed", err, utils.Attribute{Key: "chainID", Value: chainID}, utils.Attribute{Key: "address", Value: consumerAddress}, utils.Attribute{Key: "block", Value: epoch})
		}
		psq.ResponsesCache.SetWithTTL(VrfPkAndMaxCuResponseKey+key, userEntryRes, 1, DefaultTimeToLiveExpiration)
	}
	vrfPk = &utils.VrfPubKey{}
	vrfPk, err = vrfPk.DecodeFromBech32(userEntryRes.GetConsumer().Vrfpk)
	if err != nil {
		err = utils.LavaFormatError("decoding vrfpk from bech32", err, utils.Attribute{Key: "chainID", Value: chainID}, utils.Attribute{Key: "address", Value: consumerAddress}, utils.Attribute{Key: "block", Value: epoch}, utils.Attribute{Key: "UserEntryRes", Value: userEntryRes})
	}
	return vrfPk, userEntryRes.GetMaxCU(), err
}

func (psq *ProviderStateQuery) entryKey(consumerAddress string, chainID string, epoch uint64, providerAddress string) string {
	return consumerAddress + chainID + strconv.FormatUint(epoch, 10) + providerAddress
}

func (psq *ProviderStateQuery) CurrentEpochStart(ctx context.Context) (uint64, error) {
	epochDetails, err := psq.EpochStorageQueryClient.EpochDetails(ctx, &epochstoragetypes.QueryGetEpochDetailsRequest{})
	if err != nil {
		return 0, utils.LavaFormatError("Failed Querying EpochDetails", err)
	}
	details := epochDetails.GetEpochDetails()
	return details.StartBlock, nil
}

func (psq *ProviderStateQuery) PaymentEvents(ctx context.Context, latestBlock int64) (payments []*rewardserver.PaymentRequest, err error) {
	blockResults, err := psq.clientCtx.Client.BlockResults(ctx, &latestBlock)
	if err != nil {
		return nil, err
	}
	transactionResults := blockResults.TxsResults
	for _, tx := range transactionResults {
		events := tx.Events
		for _, event := range events {
			if event.Type == "lava_relay_payment" {
				payment, err := rewardserver.BuildPaymentFromRelayPaymentEvent(event, latestBlock)
				if err != nil {
					return nil, utils.LavaFormatError("failed relay_payment_event parsing", err, utils.Attribute{Key: "event", Value: event})
				}
				utils.LavaFormatDebug("relay_payment_event", utils.Attribute{Key: "payment", Value: payment})
				payments = append(payments, payment)
			}
		}
	}
	return payments, nil
}

func (psq *ProviderStateQuery) VoteEvents(ctx context.Context, latestBlock int64) (votes []*reliabilitymanager.VoteParams, err error) {
	blockResults, err := psq.clientCtx.Client.BlockResults(ctx, &latestBlock)
	if err != nil {
		return nil, err
	}
	transactionResults := blockResults.TxsResults
	for _, tx := range transactionResults {
		events := tx.Events
		for _, event := range events {
			if event.Type == utils.EventPrefix+conflicttypes.ConflictVoteDetectionEventName {
				vote, err := reliabilitymanager.BuildVoteParamsFromDetectionEvent(event)
				if err != nil {
					return nil, utils.LavaFormatError("failed conflict_vote_detection_event parsing", err, utils.Attribute{Key: "event", Value: event})
				}
				utils.LavaFormatDebug("conflict_vote_detection_event", utils.Attribute{Key: "voteID", Value: vote.VoteID})
				votes = append(votes, vote)
			}
		}
	}

	beginBlockEvents := blockResults.BeginBlockEvents
	for _, event := range beginBlockEvents {
		if event.Type == utils.EventPrefix+conflicttypes.ConflictVoteRevealEventName {
			voteID, voteDeadline, err := reliabilitymanager.BuildBaseVoteDataFromEvent(event)
			if err != nil {
				return nil, utils.LavaFormatError("failed conflict_vote_reveal_event parsing", err, utils.Attribute{Key: "event", Value: event})
			}
			vote_reveal := &reliabilitymanager.VoteParams{VoteID: voteID, VoteDeadline: voteDeadline, ParamsType: reliabilitymanager.RevealVoteType}
			utils.LavaFormatDebug("conflict_vote_reveal_event", utils.Attribute{Key: "voteID", Value: voteID})
			votes = append(votes, vote_reveal)
		}
		if event.Type == utils.EventPrefix+conflicttypes.ConflictVoteResolvedEventName {
			voteID, _, err := reliabilitymanager.BuildBaseVoteDataFromEvent(event)
			if err != nil {
				if !reliabilitymanager.NoVoteDeadline.Is(err) {
					return nil, utils.LavaFormatError("failed conflict_vote_resolved_event parsing", err, utils.Attribute{Key: "event", Value: event})
				}
			}
			vote_resolved := &reliabilitymanager.VoteParams{VoteID: voteID, VoteDeadline: 0, ParamsType: reliabilitymanager.CloseVoteType, CloseVote: true}
			votes = append(votes, vote_resolved)
			utils.LavaFormatDebug("conflict_vote_resolved_event", utils.Attribute{Key: "voteID", Value: voteID})
		}
	}
	return votes, err
}

func (psq *ProviderStateQuery) VerifyPairing(ctx context.Context, consumerAddress string, providerAddress string, epoch uint64, chainID string) (valid bool, index, total int64, err error) {
	key := psq.entryKey(consumerAddress, chainID, epoch, providerAddress)
	extractedResultFromCache := false
	cachedInterface, found := psq.ResponsesCache.Get(VerifyPairingRespKey + key)
	var verifyResponse *pairingtypes.QueryVerifyPairingResponse = nil
	if found && cachedInterface != nil {
		if cachedResp, ok := cachedInterface.(*pairingtypes.QueryVerifyPairingResponse); ok {
			verifyResponse = cachedResp
			extractedResultFromCache = true
		} else {
			utils.LavaFormatError("invalid cache entry - failed casting response", nil, utils.Attribute{Key: "castingType", Value: "*pairingtypes.QueryVerifyPairingResponse"}, utils.Attribute{Key: "type", Value: fmt.Sprintf("%T", cachedInterface)})
		}
	}
	if verifyResponse == nil {
		verifyResponse, err = psq.PairingQueryClient.VerifyPairing(context.Background(), &pairingtypes.QueryVerifyPairingRequest{
			ChainID:  chainID,
			Client:   consumerAddress,
			Provider: providerAddress,
			Block:    epoch,
		})
		if err != nil {
			return false, 0, 0, err
		}
		psq.ResponsesCache.SetWithTTL(VerifyPairingRespKey+key, verifyResponse, 1, DefaultTimeToLiveExpiration)
	}
	if !verifyResponse.Valid {
		return false, 0, 0, utils.LavaFormatError("invalid self pairing with consumer", nil, utils.Attribute{Key: "provider", Value: providerAddress}, utils.Attribute{Key: "consumer address", Value: consumerAddress}, utils.Attribute{Key: "epoch", Value: epoch}, utils.Attribute{Key: "from_cache", Value: extractedResultFromCache})
	}
	return verifyResponse.Valid, verifyResponse.GetIndex(), int64(verifyResponse.GetPairedProviders()), nil
}

func (psq *ProviderStateQuery) GetProvidersCountForConsumer(ctx context.Context, consumerAddress string, epoch uint64, chainID string) (uint32, error) {
	res, err := psq.PairingQueryClient.Params(ctx, &pairingtypes.QueryParamsRequest{})
	if err != nil {
		return 0, err
	}
	return uint32(res.GetParams().ServicersToPairCount), nil
}

func (psq *ProviderStateQuery) GetEpochSize(ctx context.Context) (uint64, error) {
	res, err := psq.EpochStorageQueryClient.Params(ctx, &epochstoragetypes.QueryParamsRequest{})
	if err != nil {
		return 0, err
	}
	return res.Params.EpochBlocks, nil
}

func (psq *ProviderStateQuery) EarliestBlockInMemory(ctx context.Context) (uint64, error) {
	res, err := psq.EpochStorageQueryClient.EpochDetails(ctx, &epochstoragetypes.QueryGetEpochDetailsRequest{})
	if err != nil {
		return 0, err
	}
	return res.EpochDetails.EarliestStart, nil
}

func (psq *ProviderStateQuery) GetRecommendedEpochNumToCollectPayment(ctx context.Context) (uint64, error) {
	res, err := psq.PairingQueryClient.Params(ctx, &pairingtypes.QueryParamsRequest{})
	if err != nil {
		return 0, err
	}
	return res.GetParams().RecommendedEpochNumToCollectPayment, nil
}

func (psq *ProviderStateQuery) GetEpochSizeMultipliedByRecommendedEpochNumToCollectPayment(ctx context.Context) (uint64, error) {
	epochSize, err := psq.GetEpochSize(ctx)
	if err != nil {
		return 0, err
	}
	recommendedEpochNumToCollectPayment, err := psq.GetRecommendedEpochNumToCollectPayment(ctx)
	if err != nil {
		return 0, err
	}
	return epochSize * recommendedEpochNumToCollectPayment, nil
}
