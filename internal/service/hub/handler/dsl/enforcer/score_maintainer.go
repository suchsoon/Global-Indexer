package enforcer

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/redis/go-redis/v9"
	"github.com/rss3-network/global-indexer/internal/cache"
	"github.com/rss3-network/global-indexer/internal/service/hub/handler/dsl/model"
	"github.com/rss3-network/global-indexer/schema"
	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	"go.uber.org/zap"
)

// ScoreMaintainer is a structure used to maintain a sorted set and a quick lookup map.
// It uses Redis to keep a sorted set based on node scores,
// and a map in memory for fast access to each node endpoint's cached data.
// This structure helps in quickly and efficiently updating and retrieving scores and statuses of nodes in distributed systems.
type ScoreMaintainer struct {
	cacheClient cache.Client
	// This map is used to record the nodes currently in the sorted set.
	// It can only be updated or reduced, not increased.
	nodeEndpointCaches map[string]*EndpointCache
	lock               sync.RWMutex
}

type EndpointCache struct {
	Endpoint    string
	AccessToken string
}

// addOrUpdateScore updates or adds a nodeEndpointCache in the data structure.
// If the invalidCount is greater than or equal to DemotionCountBeforeSlashing, the nodeEndpointCache is removed.
func (sm *ScoreMaintainer) addOrUpdateScore(ctx context.Context, setKey string, nodeStat *schema.Stat) error {
	sm.lock.RLock()
	_, exists := sm.nodeEndpointCaches[nodeStat.Address.String()]
	sm.lock.RUnlock()

	if !exists {
		return nil
	}

	if nodeStat.EpochInvalidRequest >= int64(model.DemotionCountBeforeSlashing) {
		// Fixme: add redis lock
		// Remove from sorted set.
		return sm.cacheClient.ZRem(ctx, setKey, nodeStat.Address.String())
	}

	return sm.cacheClient.ZAdd(ctx, setKey, redis.Z{
		Member: nodeStat.Address.String(),
		Score:  nodeStat.Score,
	})
}

// retrieveQualifiedNodes returns the top n NodeEndpointCaches from the sorted set.
func (sm *ScoreMaintainer) retrieveQualifiedNodes(ctx context.Context, setKey string, n int) ([]*model.NodeEndpointCache, error) {
	// Get the top n nodes from the sorted set.
	result, err := sm.cacheClient.ZRevRangeWithScores(ctx, setKey, 0, int64(n-1))
	if err != nil {
		return nil, err
	}

	qualifiedNodes := make([]*model.NodeEndpointCache, 0, n)

	sm.lock.RLock()
	defer sm.lock.RUnlock()

	for _, item := range result {
		if endpointCache, ok := sm.nodeEndpointCaches[item.Member.(string)]; ok {
			qualifiedNodes = append(qualifiedNodes, &model.NodeEndpointCache{
				Address:     item.Member.(string),
				Endpoint:    endpointCache.Endpoint,
				AccessToken: endpointCache.AccessToken,
			})
		}
	}

	return qualifiedNodes, nil
}

// updateQualifiedNodesMap replaces the current nodeEndpointCaches.
func (sm *ScoreMaintainer) updateQualifiedNodesMap(ctx context.Context, nodeStats []*schema.Stat) error {
	nodeStatsCaches := make(map[string]*EndpointCache, len(nodeStats))

	var mu sync.Mutex

	statsPool := pool.New().WithContext(ctx).WithMaxGoroutines(lo.Ternary(len(nodeStats) < 20*runtime.NumCPU() && len(nodeStats) > 0, len(nodeStats), 20*runtime.NumCPU()))

	for _, stat := range nodeStats {
		stat := stat

		statsPool.Go(func(_ context.Context) error {
			if stat.EpochInvalidRequest < int64(model.DemotionCountBeforeSlashing) {
				mu.Lock()
				nodeStatsCaches[stat.Address.String()] = &EndpointCache{
					Endpoint:    stat.Endpoint,
					AccessToken: stat.AccessToken,
				}
				mu.Unlock()
			}

			return nil
		})
	}

	if err := statsPool.Wait(); err != nil {
		return err
	}

	sm.lock.Lock()
	sm.nodeEndpointCaches = nodeStatsCaches
	sm.lock.Unlock()

	return nil
}

// newScoreMaintainer creates a new ScoreMaintainer with the nodeEndpointCaches and redis sorted set.
func newScoreMaintainer(ctx context.Context, setKey string, nodeStats []*schema.Stat, cacheClient cache.Client) (*ScoreMaintainer, error) {
	// Prepare the node caches and members for the sorted set.
	nodeEndpointCaches, newMembers, err := prepareNodeCachesAndMembers(ctx, nodeStats, cacheClient)
	if err != nil {
		return nil, err
	}

	// Adjust the members in the sorted set.
	if err = adjustMembersToSet(ctx, setKey, newMembers, nodeEndpointCaches, cacheClient); err != nil {
		return nil, err
	}

	return &ScoreMaintainer{
		cacheClient:        cacheClient,
		nodeEndpointCaches: nodeEndpointCaches,
	}, nil
}

// prepareNodeCachesAndMembers set node request caches and prepares the members for the sorted set.
func prepareNodeCachesAndMembers(ctx context.Context, nodeStats []*schema.Stat, cacheClient cache.Client) (map[string]*EndpointCache, []redis.Z, error) {
	var mu sync.Mutex

	nodeEndpointMap := make(map[string]*EndpointCache, len(nodeStats))
	members := make([]redis.Z, 0, len(nodeStats))

	statsPool := pool.New().WithContext(ctx).WithMaxGoroutines(lo.Ternary(len(nodeStats) < 20*runtime.NumCPU() && len(nodeStats) > 0, len(nodeStats), 20*runtime.NumCPU()))

	for _, stat := range nodeStats {
		stat := stat

		statsPool.Go(func(ctx context.Context) error {
			var (
				invalidCount int64
				validCount   int64
			)

			// Get the latest invalid request counts from the cache.
			if err := getCacheCount(ctx, cacheClient, model.InvalidRequestCount, stat.Address, &invalidCount, stat.EpochInvalidRequest); err != nil {
				zap.L().Error("failed to get invalid request count from cache ", zap.Error(err), zap.String("address", stat.Address.String()))

				return err
			}
			// Get the latest valid request counts from the cache.
			if err := getCacheCount(ctx, cacheClient, model.ValidRequestCount, stat.Address, &validCount, stat.EpochRequest); err != nil {
				zap.L().Error("failed to get valid request count from cache ", zap.Error(err), zap.String("address", stat.Address.String()))

				return err
			}

			// Update the invalid request count in the current epoch.
			stat.EpochInvalidRequest = invalidCount

			// Update the total request count.
			if stat.EpochRequest < validCount {
				stat.TotalRequest += validCount - stat.EpochRequest
			}
			// Update the valid request count in the current epoch.
			stat.EpochRequest = validCount

			// If the invalid request count is less than the demotion count, add the node to the map and sorted set.
			if invalidCount < int64(model.DemotionCountBeforeSlashing) {
				// Calculate the reliability score.
				calculateReliabilityScore(stat)

				mu.Lock()
				nodeEndpointMap[stat.Address.String()] = &EndpointCache{
					Endpoint:    stat.Endpoint,
					AccessToken: stat.AccessToken,
				}

				members = append(members, redis.Z{
					Member: stat.Address.String(),
					Score:  stat.Score,
				})
				mu.Unlock()
			}

			return nil
		})
	}

	if err := statsPool.Wait(); err != nil {
		return nil, nil, err
	}

	return nodeEndpointMap, members, nil
}

// getCacheCount retrieves the count associated with a specific key from the cache.
// If the key is not found in the cache, it initializes the cache with the provided statCount value.
// This ensures that all keys have a corresponding value in the cache for accurate tracking and operations.
func getCacheCount(ctx context.Context, cacheClient cache.Client, key string, address common.Address, resCount *int64, statCount int64) error {
	if err := cacheClient.Get(ctx, formatNodeStatRedisKey(key, address.String()), resCount); err != nil {
		if errors.Is(err, redis.Nil) {
			*resCount = statCount
			return cacheClient.Set(ctx, formatNodeStatRedisKey(key, address.String()), resCount, 0)
		}

		return err
	}

	return nil
}

// adjustMembersToSet modifies the existing members of a sorted set based on specific criteria or updates.
// This function may add, update, or remove members to ensure the set reflects current data states or conditions.
func adjustMembersToSet(ctx context.Context, setKey string, newMembers []redis.Z, nodeEndpointCaches map[string]*EndpointCache, cacheClient cache.Client) error {
	if len(newMembers) > 0 {
		if err := cacheClient.ZAdd(ctx, setKey, newMembers...); err != nil {
			return err
		}
	}

	members, err := cacheClient.ZRevRangeWithScores(ctx, setKey, 0, -1)
	if err != nil {
		return err
	}

	membersToRemove := filterMembersToRemove(members, nodeEndpointCaches)
	if len(membersToRemove) > 0 {
		if err = cacheClient.ZRem(ctx, setKey, membersToRemove); err != nil {
			return err
		}
	}

	return nil
}

// filterMembers filters out the members that are not in the nodeEndpointCaches.
func filterMembersToRemove(members []redis.Z, nodeEndpointCaches map[string]*EndpointCache) []string {
	membersToRemove := make([]string, 0, len(members))

	for _, member := range members {
		if _, ok := nodeEndpointCaches[member.Member.(string)]; !ok {
			membersToRemove = append(membersToRemove, member.Member.(string))
		}
	}

	return membersToRemove
}

// formatNodeStatRedisKey formats the redis key.
func formatNodeStatRedisKey(key string, address string) string {
	return fmt.Sprintf("%s:%s", key, address)
}
