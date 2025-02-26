package cosmos_thirdparty

import (
	"context"
	"encoding/json"

	// add protobuf here as pb_pkg
	pb_pkg "cosmossdk.io/api/cosmos/feegrant/v1beta1"
	"github.com/golang/protobuf/proto"
	"github.com/lavanet/lava/utils"
)

type implementedCosmosFeegrantV1beta1 struct {
	pb_pkg.UnimplementedQueryServer
	cb func(ctx context.Context, method string, reqBody []byte) ([]byte, error)
}

// this line is used by grpc_scaffolder #implementedCosmosFeegrantV1beta1

func (is *implementedCosmosFeegrantV1beta1) Allowance(ctx context.Context, req *pb_pkg.QueryAllowanceRequest) (*pb_pkg.QueryAllowanceResponse, error) {
	reqMarshaled, err := json.Marshal(req)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to proto.Marshal(req)", err)
	}
	res, err := is.cb(ctx, "cosmos.feegrant.v1beta1.Query/Allowance", reqMarshaled)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to SendRelay cb", err)
	}
	result := &pb_pkg.QueryAllowanceResponse{}
	err = proto.Unmarshal(res, result)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to proto.Unmarshal", err)
	}
	return result, nil
}

// this line is used by grpc_scaffolder #Method

func (is *implementedCosmosFeegrantV1beta1) Allowances(ctx context.Context, req *pb_pkg.QueryAllowancesRequest) (*pb_pkg.QueryAllowancesResponse, error) {
	reqMarshaled, err := json.Marshal(req)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to proto.Marshal(req)", err)
	}
	res, err := is.cb(ctx, "cosmos.feegrant.v1beta1.Query/Allowances", reqMarshaled)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to SendRelay cb", err)
	}
	result := &pb_pkg.QueryAllowancesResponse{}
	err = proto.Unmarshal(res, result)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to proto.Unmarshal", err)
	}
	return result, nil
}

// this line is used by grpc_scaffolder #Method

func (is *implementedCosmosFeegrantV1beta1) AllowancesByGranter(ctx context.Context, req *pb_pkg.QueryAllowancesByGranterRequest) (*pb_pkg.QueryAllowancesByGranterResponse, error) {
	reqMarshaled, err := json.Marshal(req)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to proto.Marshal(req)", err)
	}
	res, err := is.cb(ctx, "cosmos.feegrant.v1beta1.Query/AllowancesByGranter", reqMarshaled)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to SendRelay cb", err)
	}
	result := &pb_pkg.QueryAllowancesByGranterResponse{}
	err = proto.Unmarshal(res, result)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to proto.Unmarshal", err)
	}
	return result, nil
}

// this line is used by grpc_scaffolder #Method

// this line is used by grpc_scaffolder #Methods
