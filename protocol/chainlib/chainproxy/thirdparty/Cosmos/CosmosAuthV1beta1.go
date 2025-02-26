package cosmos_thirdparty

import (
	"context"
	"encoding/json"

	pb_pkg "cosmossdk.io/api/cosmos/auth/v1beta1"
	"github.com/golang/protobuf/proto"
	"github.com/lavanet/lava/utils"
)

type implementedCosmosAuthV1beta1 struct {
	pb_pkg.UnimplementedQueryServer
	cb func(ctx context.Context, method string, reqBody []byte) ([]byte, error)
}

// this line is used by grpc_scaffolder #implementedCosmosAuthV1beta1

func (is *implementedCosmosAuthV1beta1) Account(ctx context.Context, req *pb_pkg.QueryAccountRequest) (*pb_pkg.QueryAccountResponse, error) {
	reqMarshaled, err := json.Marshal(req)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to proto.Marshal(req)", err)
	}
	res, err := is.cb(ctx, "cosmos.auth.v1beta1.Query/Account", reqMarshaled)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to SendRelay cb", err)
	}
	result := &pb_pkg.QueryAccountResponse{}
	err = proto.Unmarshal(res, result)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to proto.Unmarshal", err)
	}
	return result, nil
}

// this line is used by grpc_scaffolder #Method

func (is *implementedCosmosAuthV1beta1) Accounts(ctx context.Context, req *pb_pkg.QueryAccountsRequest) (*pb_pkg.QueryAccountsResponse, error) {
	reqMarshaled, err := json.Marshal(req)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to proto.Marshal(req)", err)
	}
	res, err := is.cb(ctx, "cosmos.auth.v1beta1.Query/Accounts", reqMarshaled)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to SendRelay cb", err)
	}
	result := &pb_pkg.QueryAccountsResponse{}
	err = proto.Unmarshal(res, result)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to proto.Unmarshal", err)
	}
	return result, nil
}

// this line is used by grpc_scaffolder #Method

func (is *implementedCosmosAuthV1beta1) Params(ctx context.Context, req *pb_pkg.QueryParamsRequest) (*pb_pkg.QueryParamsResponse, error) {
	reqMarshaled, err := json.Marshal(req)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to proto.Marshal(req)", err)
	}
	res, err := is.cb(ctx, "cosmos.auth.v1beta1.Query/Params", reqMarshaled)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to SendRelay cb", err)
	}
	result := &pb_pkg.QueryParamsResponse{}
	err = proto.Unmarshal(res, result)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to proto.Unmarshal", err)
	}
	return result, nil
}

// this line is used by grpc_scaffolder #Method

func (is *implementedCosmosAuthV1beta1) ModuleAccounts(ctx context.Context, req *pb_pkg.QueryModuleAccountsRequest) (*pb_pkg.QueryModuleAccountsResponse, error) {
	reqMarshaled, err := json.Marshal(req)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to proto.Marshal(req)", err)
	}
	res, err := is.cb(ctx, "cosmos.auth.v1beta1.Query/ModuleAccounts", reqMarshaled)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to SendRelay cb", err)
	}
	result := &pb_pkg.QueryModuleAccountsResponse{}
	err = proto.Unmarshal(res, result)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to proto.Unmarshal", err)
	}
	return result, nil
}

// this line is used by grpc_scaffolder #Method

func (is *implementedCosmosAuthV1beta1) ModuleAccountByName(ctx context.Context, req *pb_pkg.QueryModuleAccountByNameRequest) (*pb_pkg.QueryModuleAccountByNameResponse, error) {
	reqMarshaled, err := json.Marshal(req)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to proto.Marshal(req)", err)
	}
	res, err := is.cb(ctx, "cosmos.auth.v1beta1.Query/ModuleAccountByName", reqMarshaled)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to SendRelay cb", err)
	}
	result := &pb_pkg.QueryModuleAccountByNameResponse{}
	err = proto.Unmarshal(res, result)
	if err != nil {
		return nil, utils.LavaFormatError("Failed to proto.Unmarshal", err)
	}
	return result, nil
}

// this line is used by grpc_scaffolder #Method

// this line is used by grpc_scaffolder #Methods
