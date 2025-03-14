package settler

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v9"
	"github.com/redis/go-redis/v9"
	"github.com/rss3-network/global-indexer/common/httputil"
	"github.com/rss3-network/global-indexer/common/txmgr"
	"github.com/rss3-network/global-indexer/contract/l2"
	stakingv2 "github.com/rss3-network/global-indexer/contract/l2/staking/v2"
	"github.com/rss3-network/global-indexer/internal/client/ethereum"
	"github.com/rss3-network/global-indexer/internal/config"
	"github.com/rss3-network/global-indexer/internal/config/flag"
	"github.com/rss3-network/global-indexer/internal/database"
	"github.com/rss3-network/global-indexer/internal/service"
	"github.com/rss3-network/global-indexer/schema"
	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

const Name = "settler"

type Server struct {
	txManager             txmgr.TxManager
	checkpoint            uint64
	chainID               *big.Int
	mutex                 *redsync.Mutex
	currentEpoch          uint64
	ethereumClient        *ethclient.Client
	databaseClient        database.Client
	stakingContract       *stakingv2.Staking
	settlementContract    *l2.Settlement
	networkParamsContract *l2.NetworkParams
	config                *config.File
	httpClient            httputil.Client
}

func (s *Server) Name() string {
	return Name
}

func (s *Server) Run(ctx context.Context) error {
	errorPool := pool.New().WithContext(ctx).WithCancelOnError().WithFirstError()

	// Listen epoch event
	errorPool.Go(func(ctx context.Context) error {
		if err := s.listenEpochEvent(ctx); err != nil {
			zap.L().Error("listen epoch event", zap.Error(err))

			return err
		}

		return nil
	})

	errorChan := make(chan error)
	go func() { errorChan <- errorPool.Wait() }()

	select {
	case err := <-errorChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) listenEpochEvent(ctx context.Context) error {
	epochInterval := time.Duration(s.config.Settler.EpochIntervalInHours) * time.Hour

	timer := time.NewTimer(0)
	<-timer.C

	defer timer.Stop()

	for {
		// Load checkpoint and latest block number.
		indexedBlock, latestBlock, err := s.loadCheckpoint(ctx)
		if err != nil {
			zap.L().Error("get checkpoint and latest block number", zap.Error(err), zap.Any("chain_id", s.chainID),
				zap.Any("checkpoint", indexedBlock), zap.Uint64("block_number_latest", latestBlock))

			return err
		}

		// If indexer is lagging behind the latest block by more than 5 blocks
		// impose a 5-second delay to allow the indexer to catch up
		if int(latestBlock-indexedBlock) > 5 {
			zap.L().Error("indexer encountered errors or is still catching up with the latest block", zap.Uint64("checkpoint", indexedBlock),
				zap.Uint64("last checkpoint", s.checkpoint), zap.Uint64("block_number_latest", latestBlock))

			timer.Reset(5 * time.Second)
			<-timer.C

			continue
		}

		s.checkpoint = indexedBlock

		// Find the latest epoch event from database
		lastEpoch, err := s.databaseClient.FindEpochs(ctx, &schema.FindEpochsQuery{Limit: lo.ToPtr(1)})
		if err != nil && !errors.Is(err, database.ErrorRowNotFound) {
			zap.L().Error("get latest epoch event from database", zap.Error(err))

			return err
		}

		// Find the latest epoch submitEpochProof from database.
		lastEpochTrigger, err := s.databaseClient.FindLatestEpochTrigger(ctx)
		if err != nil && !errors.Is(err, database.ErrorRowNotFound) {
			zap.L().Error("get latest epoch submitEpochProof from database", zap.Error(err))

			return err
		}

		var lastEpochTime, lastEpochTriggerTime time.Time

		// Make sure the lastEpoch exists
		if len(lastEpoch) > 0 {
			lastEpochTime = time.Unix(lastEpoch[0].BlockTimestamp, 0)
			s.currentEpoch = lastEpoch[0].ID
		}

		// Set lastEpochTriggerTime time
		if lastEpochTrigger != nil {
			lastEpochTriggerTime = lastEpochTrigger.CreatedAt
		}

		now := time.Now()

		// The time elapsed since the last epoch event was included on the VSL
		timeSinceLastEpoch := now.Sub(lastEpochTime)
		// The time elapsed since the last epoch trigger was sent
		timeSinceLastTrigger := now.Sub(lastEpochTriggerTime)

		// Special case for genesis epoch
		// No longer applicable for the subsequent epochs
		if genesisEpoch, exist := l2.GenesisEpochMap[s.chainID.Uint64()]; exist {
			genesisEpochTime := time.Unix(genesisEpoch, 0)

			// Wait for genesis epoch
			if now.Sub(genesisEpochTime) < epochInterval-1*time.Hour {
				zap.L().Info("wait for genesis epoch start", zap.Time("genesis_epoch_time", genesisEpochTime),
					zap.Time("estimated_epoch_start_time", now.Add(epochInterval-1*time.Hour-now.Sub(genesisEpochTime))))

				timer.Reset(epochInterval - 1*time.Hour - now.Sub(genesisEpochTime))
				<-timer.C

				zap.L().Info("genesis epoch start", zap.Time("start_time", time.Now()))
			}
		}

		// Wait for the finalization of the last epoch
		if len(lastEpoch) > 0 && !lastEpoch[0].Finalized {
			zap.L().Info("wait for the finalization of the last epoch", zap.Int64("block_number", lastEpoch[0].BlockNumber.Int64()))

			timer.Reset(time.Minute)
			<-timer.C

			continue
		}

		// Check if epochInterval has passed since the last epoch event
		if timeSinceLastEpoch >= epochInterval {
			// Check if the epochInterval has passed since the last epoch trigger
			if timeSinceLastTrigger >= epochInterval {
				// Submit proof of a new epoch
				if err := s.submitEpochProof(ctx, s.currentEpoch+1); err != nil {
					zap.L().Error("trigger new epoch", zap.Error(err))

					return err
				}
			} else if timeSinceLastTrigger < epochInterval {
				// Get current epoch
				currentEpoch, err := s.settlementContract.CurrentEpoch(&bind.CallOpts{Context: ctx})
				if err != nil {
					zap.L().Error("get current epoch from chain", zap.Error(err))

					return err
				}

				if lastEpochTrigger != nil {
					if lastEpochTrigger.EpochID == currentEpoch.Uint64() {
						// If the epoch trigger has already fired for the current epoch, wait for the epoch event indexer to catch up
						zap.L().Info("wait for epoch event indexer", zap.Time("last_epoch_event_time", lastEpochTime),
							zap.Time("last_epoch_trigger_time", lastEpochTriggerTime))

						timer.Reset(5 * time.Second)
						<-timer.C
					} else if lastEpochTrigger.EpochID > currentEpoch.Uint64() {
						// If a block reorganization occurs, the epoch trigger's ID is greater than the current epoch, retry this epoch trigger.
						zap.L().Info("retry epoch trigger", zap.Uint64("epoch_id", lastEpochTrigger.EpochID))

						if err := s.retryEpochProof(ctx, lastEpochTrigger.EpochID); err != nil {
							zap.L().Error("retry epoch trigger", zap.Error(err))

							return err
						}
					}
				}
			}
		} else if timeSinceLastEpoch < epochInterval {
			// If epochInterval has NOT passed since the last epoch event
			// Wait for the remaining time until the next epoch event
			remainingTime := epochInterval - now.Sub(lastEpochTime)
			timer.Reset(remainingTime)
			<-timer.C

			if err := s.submitEpochProof(ctx, s.currentEpoch+1); err != nil {
				zap.L().Error("submitEpochProof new epoch", zap.Error(err))
				return err
			}
		}
	}
}

func (s *Server) loadCheckpoint(ctx context.Context) (uint64, uint64, error) {
	// Load checkpoint from database.
	// A checkpoint is basically the last indexed block
	indexedBlock, err := s.databaseClient.FindCheckpoint(ctx, s.chainID.Uint64())
	if err != nil {
		if errors.Is(err, database.ErrorRowNotFound) {
			return 0, 0, nil
		}

		return 0, 0, fmt.Errorf("get checkpoint from database: %w", err)
	}

	// Load latest finalized block number from RPC.
	latestFinalizedBlock, err := s.ethereumClient.BlockByNumber(ctx, big.NewInt(rpc.FinalizedBlockNumber.Int64()))
	if err != nil {
		return 0, 0, fmt.Errorf("get latest finalized block from rpc: %w", err)
	}

	return indexedBlock.BlockNumber, latestFinalizedBlock.NumberU64(), nil
}

func NewServer(databaseClient database.Client, redisClient *redis.Client, ethereumMultiChainClient *ethereum.MultiChainClient, config *config.File, txManager *txmgr.SimpleTxManager, httpClient httputil.Client) (service.Server, error) {
	redisPool := goredis.NewPool(redisClient)
	rs := redsync.New(redisPool)

	chainID := new(big.Int).SetUint64(viper.GetUint64(flag.KeyChainIDL2))

	ethereumClient, err := ethereumMultiChainClient.Get(chainID.Uint64())
	if err != nil {
		return nil, fmt.Errorf("load l2 ethereum client: %w", err)
	}

	contractAddresses := l2.ContractMap[chainID.Uint64()]
	if contractAddresses == nil {
		return nil, fmt.Errorf("contract address not found for chain id: %d", chainID)
	}

	stakingContract, err := stakingv2.NewStaking(contractAddresses.AddressStakingProxy, ethereumClient)
	if err != nil {
		return nil, fmt.Errorf("new staking contract: %w", err)
	}

	settlementContract, err := l2.NewSettlement(contractAddresses.AddressSettlementProxy, ethereumClient)
	if err != nil {
		return nil, fmt.Errorf("new settlement contract: %w", err)
	}

	networkParamsContract, err := l2.NewNetworkParams(contractAddresses.AddressNetworkParamsProxy, ethereumClient)
	if err != nil {
		return nil, fmt.Errorf("new network params contract: %w", err)
	}

	server := &Server{
		chainID:               chainID,
		mutex:                 rs.NewMutex(Name, redsync.WithExpiry(5*time.Minute)),
		ethereumClient:        ethereumClient,
		databaseClient:        databaseClient,
		txManager:             txManager,
		stakingContract:       stakingContract,
		settlementContract:    settlementContract,
		config:                config,
		httpClient:            httpClient,
		networkParamsContract: networkParamsContract,
	}

	return server, nil
}
