package rpcprovider

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"github.com/lavanet/lava/protocol/chainlib"
	"github.com/lavanet/lava/protocol/lavasession"
	"github.com/lavanet/lava/utils"
	pairingtypes "github.com/lavanet/lava/x/pairing/types"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	grpc "google.golang.org/grpc"
	wrapperspb "google.golang.org/protobuf/types/known/wrapperspb"
)

type ProviderListener struct {
	networkAddress string
	relayServer    *relayServer
	httpServer     http.Server
}

func (pl *ProviderListener) Key() string {
	return pl.networkAddress
}

func (pl *ProviderListener) RegisterReceiver(existingReceiver RelayReceiver, endpoint *lavasession.RPCProviderEndpoint) error {
	listen_endpoint := lavasession.RPCEndpoint{ChainID: endpoint.ChainID, ApiInterface: endpoint.ApiInterface}
	pl.relayServer.lock.Lock()
	defer pl.relayServer.lock.Unlock()
	_, ok := pl.relayServer.relayReceivers[listen_endpoint.Key()]
	if ok {
		// there was already a receiver defined
		return utils.LavaFormatError("double_receiver_setup receiver already defined on this address with the same chainID and apiInterface", nil, utils.Attribute{Key: "chainID", Value: endpoint.ChainID}, utils.Attribute{Key: "apiInterface", Value: endpoint.ApiInterface})
	}
	pl.relayServer.relayReceivers[listen_endpoint.Key()] = existingReceiver
	utils.LavaFormatInfo("Provider Listening on Address", utils.Attribute{Key: "chainID", Value: endpoint.ChainID}, utils.Attribute{Key: "apiInterface", Value: endpoint.ApiInterface}, utils.Attribute{Key: "Address", Value: endpoint.NetworkAddress})
	return nil
}

func (pl *ProviderListener) Shutdown(shutdownCtx context.Context) error {
	if err := pl.httpServer.Shutdown(shutdownCtx); err != nil {
		utils.LavaFormatFatal("Provider failed to shutdown", err)
	}
	return nil
}

func NewProviderListener(ctx context.Context, networkAddress string) *ProviderListener {
	pl := &ProviderListener{networkAddress: networkAddress}

	// GRPC
	lis := chainlib.GetListenerWithRetryGrpc("tcp", networkAddress)
	grpcServer := grpc.NewServer()

	wrappedServer := grpcweb.WrapServer(grpcServer)
	handler := func(resp http.ResponseWriter, req *http.Request) {
		// Set CORS headers
		resp.Header().Set("Access-Control-Allow-Origin", "*")
		resp.Header().Set("Access-Control-Allow-Headers", "Content-Type,x-grpc-web")

		wrappedServer.ServeHTTP(resp, req)
	}

	pl.httpServer = http.Server{
		Handler: h2c.NewHandler(http.HandlerFunc(handler), &http2.Server{}),
	}
	relayServer := &relayServer{relayReceivers: map[string]RelayReceiver{}}
	pl.relayServer = relayServer
	pairingtypes.RegisterRelayerServer(grpcServer, relayServer)
	go func() {
		utils.LavaFormatInfo("New provider listener active", utils.Attribute{Key: "address", Value: networkAddress})
		if err := pl.httpServer.Serve(lis); !errors.Is(err, http.ErrServerClosed) {
			utils.LavaFormatFatal("provider failed to serve", err, utils.Attribute{Key: "Address", Value: lis.Addr().String()})
		}
		utils.LavaFormatInfo("listener closed server", utils.Attribute{Key: "address", Value: networkAddress})
	}()
	return pl
}

type relayServer struct {
	pairingtypes.UnimplementedRelayerServer
	relayReceivers map[string]RelayReceiver
	lock           sync.RWMutex
}

type RelayReceiver interface {
	Relay(ctx context.Context, request *pairingtypes.RelayRequest) (*pairingtypes.RelayReply, error)
	RelaySubscribe(request *pairingtypes.RelayRequest, srv pairingtypes.Relayer_RelaySubscribeServer) error
}

func (rs *relayServer) Relay(ctx context.Context, request *pairingtypes.RelayRequest) (*pairingtypes.RelayReply, error) {
	relayReceiver, err := rs.findReceiver(request)
	if err != nil {
		return nil, err
	}
	return relayReceiver.Relay(ctx, request)
}

func (rs *relayServer) Probe(ctx context.Context, probeReq *wrapperspb.UInt64Value) (*wrapperspb.UInt64Value, error) {
	return probeReq, nil
}

func (rs *relayServer) RelaySubscribe(request *pairingtypes.RelayRequest, srv pairingtypes.Relayer_RelaySubscribeServer) error {
	relayReceiver, err := rs.findReceiver(request)
	if err != nil {
		return err
	}
	return relayReceiver.RelaySubscribe(request, srv)
}

func (rs *relayServer) findReceiver(request *pairingtypes.RelayRequest) (RelayReceiver, error) {
	apiInterface := request.RelayData.ApiInterface
	chainID := request.RelaySession.SpecId
	endpoint := lavasession.RPCEndpoint{ChainID: chainID, ApiInterface: apiInterface}
	rs.lock.RLock()
	defer rs.lock.RUnlock()
	relayReceiver, ok := rs.relayReceivers[endpoint.Key()]
	if !ok {
		keys := make([]string, 0, len(rs.relayReceivers))
		for k := range rs.relayReceivers {
			keys = append(keys, k)
		}
		return nil, utils.LavaFormatError("got called with unhandled relay receiver", nil, utils.Attribute{Key: "requested_receiver", Value: endpoint.Key()}, utils.Attribute{Key: "handled_receivers", Value: strings.Join(keys, ",")})
	}
	return relayReceiver, nil
}
