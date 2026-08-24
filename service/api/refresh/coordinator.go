// Package refresh coalesces concurrent access-token refreshes into a single flight, so N
// requests hitting an expired token trigger one POST /auth/refresh, not N. See RedisCoordinator.
package refresh

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/cash-track/gateway/headers/cookie"
)

const (
	keyPrefix = "CT:refresh"
	resultTtl = 10 * time.Second

	// defaultLockTtl falls back when no lock TTL is given; production always passes an
	// explicit one sized to the real worst case — see service/api.RefreshLockTTL.
	defaultLockTtl = 5 * time.Second
)

// waitBudgetSlack and pollInterval are vars, not consts, only so tests can shrink them.
// waitBudget must always be lockTtl + waitBudgetSlack, never set independently: a waiter
// that times out before the leader's worst case elapses would replay the leader's
// already-consumed refresh token, causing a spurious 401 and logout.
var (
	waitBudgetSlack = 2 * time.Second
	pollInterval    = 50 * time.Millisecond
)

// unlockScript deletes the lock only if it is still owned by the caller (ARGV[1]).
// Never a bare DEL: by the time a leader gets here the lock may have already expired
// and been re-acquired by a different instance, and DEL-ing it would release a lock
// that isn't ours.
const unlockScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
else
	return 0
end
`

// Coordinator runs a token refresh exactly once per key within this process (tier 1).
// Concurrent callers with the same key block until the leader finishes and receive its
// result. Implementations may additionally coalesce across gateway instances (tier 2).
type Coordinator interface {
	Do(ctx context.Context, key string, refresh func(context.Context) (cookie.Auth, error)) (cookie.Auth, error)
}

// call is one in-flight (or just-finished) refresh, shared by every caller coalesced
// onto it within this process.
type call struct {
	done chan struct{}
	auth cookie.Auth
	err  error
}

// RedisCoordinator coalesces concurrent refreshes for the same key within this process
// (tier 1, always active) and, when Redis is available and reachable, across every
// gateway instance sharing it (tier 2). Any Redis failure degrades to tier-1-only
// coalescing — it never fails the request itself and never deadlocks a waiter.
type RedisCoordinator struct {
	redis      *redis.Client
	instanceID string
	lockTtl    time.Duration

	// waitBudget is always lockTtl + waitBudgetSlack, set at construction — see NewRedis.
	waitBudget time.Duration

	mu    sync.Mutex
	calls map[string]*call
}

// NewRedis builds a coordinator. client may be nil for tier-1 (in-process) only.
//
// lockTtl must outlive the worst-case refresh call, or a second instance can acquire
// the lock mid-refresh and duplicate it — see service/api.RefreshLockTTL, which derives
// a safe value. Zero falls back to defaultLockTtl.
func NewRedis(client *redis.Client, lockTtl time.Duration) *RedisCoordinator {
	if lockTtl <= 0 {
		lockTtl = defaultLockTtl
	}

	instanceID := newInstanceID()

	return &RedisCoordinator{
		redis:      client,
		instanceID: instanceID,
		lockTtl:    lockTtl,
		waitBudget: lockTtl + waitBudgetSlack,
		calls:      make(map[string]*call),
	}
}

// WaitBudget returns how long a coalesced waiter blocks before degrading to its own
// refresh — exposed so tests can check it against external constraints.
func (c *RedisCoordinator) WaitBudget() time.Duration {
	return c.waitBudget
}

// uuidNewV7 is a seam over uuid.NewV7 so tests can force its (otherwise
// crypto/rand-failure-only) error path.
var uuidNewV7 = uuid.NewV7

func newInstanceID() string {
	id, err := uuidNewV7()
	if err != nil {
		// Timestamp-based fallback still keeps lock ownership checks correct for this
		// process's lifetime.
		return fmt.Sprintf("gateway-%d", time.Now().UnixNano())
	}

	return id.String()
}

// Do implements Coordinator.
func (c *RedisCoordinator) Do(
	ctx context.Context, key string, refresh func(context.Context) (cookie.Auth, error),
) (cookie.Auth, error) {
	c.mu.Lock()
	if existing, ok := c.calls[key]; ok {
		c.mu.Unlock()

		return c.wait(ctx, existing, refresh)
	}

	leaderCall := &call{done: make(chan struct{})}
	c.calls[key] = leaderCall
	c.mu.Unlock()

	// Unconditionally clean up and wake waiters even if c.lead panics — otherwise the map
	// entry orphans forever and every future request for this key blocks a full
	// waitBudget. Capture the panic into leaderCall.err first so waiters see an error,
	// not a false-success zero-value cookie.Auth{}.
	defer func() {
		r := recover()
		if r != nil {
			leaderCall.err = fmt.Errorf("refresh coordinator: leader panicked: %v", r)
		}

		c.mu.Lock()
		delete(c.calls, key)
		c.mu.Unlock()

		close(leaderCall.done)

		if r != nil {
			panic(r)
		}
	}()

	leaderCall.auth, leaderCall.err = c.lead(ctx, key, refresh)

	return leaderCall.auth, leaderCall.err
}

// wait blocks until the local leader finishes, the wait budget expires, or ctx is
// cancelled.
//
//   - Leader finishes: return its result as-is to every waiter.
//   - Budget expires: degrade to running our own refresh.
//   - ctx cancelled: client is gone, return immediately instead of parking or refreshing.
func (c *RedisCoordinator) wait(
	ctx context.Context, leaderCall *call, refresh func(context.Context) (cookie.Auth, error),
) (cookie.Auth, error) {
	timer := time.NewTimer(c.waitBudget)
	defer timer.Stop()

	select {
	case <-leaderCall.done:
		refreshesCoalescedTotal.Inc()

		return leaderCall.auth, leaderCall.err
	case <-ctx.Done():
		return cookie.Auth{}, ctx.Err()
	case <-timer.C:
		refreshWaitTimeoutsTotal.Inc()

		return c.runRefresh(ctx, refresh)
	}
}

// lead runs (or delegates) the actual refresh for the local leader of key. With no
// Redis client it just runs refresh directly (tier 1 only). With Redis, it also
// contends for a cluster-wide lock so at most one gateway instance calls the real
// refresh endpoint per key; the loser polls for the winner's published result instead.
func (c *RedisCoordinator) lead(
	ctx context.Context, key string, refresh func(context.Context) (cookie.Auth, error),
) (cookie.Auth, error) {
	if c.redis == nil {
		return c.runRefresh(ctx, refresh)
	}

	lockKey := fmt.Sprintf("%s:lock:%s", keyPrefix, key)

	acquired, err := c.redis.SetNX(ctx, lockKey, c.instanceID, c.lockTtl).Result()
	if err != nil {
		slog.Warn("refresh coordinator: redis lock acquisition failed, falling back to tier-1", "error", err)
		redisLockFailuresTotal.Inc()

		return c.runRefresh(ctx, refresh)
	}

	if !acquired {
		return c.followRedisLeader(ctx, key, refresh)
	}

	// Unconditionally release the lock even on panic, or it leaks for its full lockTtl,
	// leaving every follower on every other instance polling instead of re-electing.
	defer func() {
		r := recover()

		c.releaseLock(ctx, lockKey)

		if r != nil {
			panic(r)
		}
	}()

	auth, err := c.runRefresh(ctx, refresh)
	if err == nil {
		c.publishResult(ctx, key, auth)
	}

	return auth, err
}

// followRedisLeader polls for the result the lock-winning instance publishes. If it
// never shows, is corrupt, or Redis errors — within the wait budget — this instance
// degrades to running its own refresh.
func (c *RedisCoordinator) followRedisLeader(
	ctx context.Context, key string, refresh func(context.Context) (cookie.Auth, error),
) (cookie.Auth, error) {
	resultKey := fmt.Sprintf("%s:result:%s", keyPrefix, key)
	deadline := time.Now().Add(c.waitBudget)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return cookie.Auth{}, ctx.Err()
		case <-ticker.C:
			payload, err := c.redis.Get(ctx, resultKey).Result()
			switch {
			case err == nil:
				auth, decErr := decryptAuth(key, payload)
				if decErr == nil {
					refreshesCoalescedTotal.Inc()

					return auth, nil
				}

				slog.Warn("refresh coordinator: failed to decrypt published result", "error", decErr)
			case !errors.Is(err, redis.Nil):
				slog.Warn("refresh coordinator: redis error while polling for result, falling back", "error", err)
				redisLockFailuresTotal.Inc()

				return c.runRefresh(ctx, refresh)
			}

			if time.Now().After(deadline) {
				refreshWaitTimeoutsTotal.Inc()

				return c.runRefresh(ctx, refresh)
			}
		}
	}
}

func (c *RedisCoordinator) runRefresh(
	ctx context.Context, refresh func(context.Context) (cookie.Auth, error),
) (cookie.Auth, error) {
	refreshesLedTotal.Inc()

	return refresh(ctx)
}

// publishResult stores the leader's result for followers to pick up. Best-effort: the
// leader's own caller already has the correct result; only followers miss out on failure.
func (c *RedisCoordinator) publishResult(ctx context.Context, key string, auth cookie.Auth) {
	resultKey := fmt.Sprintf("%s:result:%s", keyPrefix, key)

	payload, err := encryptAuth(key, auth)
	if err != nil {
		slog.Warn("refresh coordinator: failed to encrypt result for publish", "error", err)

		return
	}

	if err := c.redis.Set(ctx, resultKey, payload, resultTtl).Err(); err != nil {
		slog.Warn("refresh coordinator: failed to publish result", "error", err)
		redisLockFailuresTotal.Inc()
	}
}

// releaseLock runs the compare-and-delete Lua script so only a lock still owned by this
// instance is released.
func (c *RedisCoordinator) releaseLock(ctx context.Context, lockKey string) {
	if err := c.redis.Eval(ctx, unlockScript, []string{lockKey}, c.instanceID).Err(); err != nil {
		slog.Warn("refresh coordinator: failed to release lock", "error", err)
	}
}
