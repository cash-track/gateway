package refresh

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-redis/redismock/v9"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"

	"github.com/cash-track/gateway/headers/cookie"
)

// withPollInterval shrinks the package-level poll interval for the duration of a test,
// restoring the production default afterwards. Tests are not parallel, so this
// mutation of shared package state is safe.
func withPollInterval(t *testing.T, poll time.Duration) {
	t.Helper()

	orig := pollInterval
	pollInterval = poll
	t.Cleanup(func() {
		pollInterval = orig
	})
}

// withWaitBudgetSlack shrinks the package-level slack for a test via NewRedis, restoring
// it after. Tests building a RedisCoordinator by struct literal set waitBudget directly
// instead. Tests aren't parallel, so mutating this shared state is safe.
func withWaitBudgetSlack(t *testing.T, slack time.Duration) {
	t.Helper()

	orig := waitBudgetSlack
	waitBudgetSlack = slack
	t.Cleanup(func() {
		waitBudgetSlack = orig
	})
}

func TestDoCoalescesConcurrentCallersOntoOneRefresh(t *testing.T) {
	c := NewRedis(nil, 0)

	var refreshCount int32
	wantAuth := cookie.Auth{AccessToken: "new-access-token"}

	refresh := func(ctx context.Context) (cookie.Auth, error) {
		atomic.AddInt32(&refreshCount, 1)
		time.Sleep(30 * time.Millisecond)

		return wantAuth, nil
	}

	const n = 20
	var wg sync.WaitGroup
	results := make([]cookie.Auth, n)
	errs := make([]error, n)

	// A start barrier so every goroutine calls Do at roughly the same time, giving
	// the coordinator a real chance to coalesce them.
	start := make(chan struct{})

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = c.Do(context.Background(), "shared-key", refresh)
		}(i)
	}
	close(start)
	wg.Wait()

	assert.EqualValues(t, 1, atomic.LoadInt32(&refreshCount))
	for i := 0; i < n; i++ {
		assert.NoError(t, errs[i])
		assert.Equal(t, wantAuth, results[i])
	}
}

func TestDoDistinctKeysDoNotBlockEachOther(t *testing.T) {
	c := NewRedis(nil, 0)

	started := make(chan string, 2)
	release := make(chan struct{})

	refresh := func(key string) func(context.Context) (cookie.Auth, error) {
		return func(ctx context.Context) (cookie.Auth, error) {
			started <- key
			<-release

			return cookie.Auth{AccessToken: key}, nil
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = c.Do(context.Background(), "key-a", refresh("key-a"))
	}()
	go func() {
		defer wg.Done()
		_, _ = c.Do(context.Background(), "key-b", refresh("key-b"))
	}()

	// Both leaders must start without waiting on each other, proving keys don't
	// share a lock. If they were serialized, the second "started" would never
	// arrive before release is closed.
	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case k := <-started:
			seen[k] = true
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for both distinct-key leaders to start")
		}
	}
	assert.True(t, seen["key-a"])
	assert.True(t, seen["key-b"])

	close(release)
	wg.Wait()
}

func TestDoErrorPropagatesIdenticallyToAllWaiters(t *testing.T) {
	c := NewRedis(nil, 0)

	wantErr := errors.New("api unreachable")
	refresh := func(ctx context.Context) (cookie.Auth, error) {
		time.Sleep(20 * time.Millisecond)

		return cookie.Auth{}, wantErr
	}

	const n = 10
	var wg sync.WaitGroup
	errs := make([]error, n)
	start := make(chan struct{})

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = c.Do(context.Background(), "err-key", refresh)
		}(i)
	}
	close(start)
	wg.Wait()

	for i := 0; i < n; i++ {
		assert.ErrorIs(t, errs[i], wantErr)
	}
}

func TestDoWaiterRespectsContextCancellation(t *testing.T) {
	c := NewRedis(nil, 0)

	leaderStarted := make(chan struct{})
	releaseLeader := make(chan struct{})
	leaderRefresh := func(ctx context.Context) (cookie.Auth, error) {
		close(leaderStarted)
		<-releaseLeader

		return cookie.Auth{AccessToken: "leader-result"}, nil
	}

	go func() {
		_, _ = c.Do(context.Background(), "cancel-key", leaderRefresh)
	}()

	<-leaderStarted

	waiterCtx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	var waiterErr error
	go func() {
		_, waiterErr = c.Do(waiterCtx, "cancel-key", func(context.Context) (cookie.Auth, error) {
			t.Error("waiter must not run its own refresh after cancellation")

			return cookie.Auth{}, nil
		})
		close(done)
	}()

	cancel()

	select {
	case <-done:
		assert.ErrorIs(t, waiterErr, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("cancelled waiter did not return promptly")
	}

	close(releaseLeader)
}

func TestDoWaiterDegradesToOwnRefreshAfterWaitBudget(t *testing.T) {
	withWaitBudgetSlack(t, 5*time.Millisecond)

	c := NewRedis(nil, 10*time.Millisecond) // waitBudget = lockTtl + slack = 15ms; tier 1 only, pollInterval unused

	before := testutil.ToFloat64(refreshWaitTimeoutsTotal)

	leaderStarted := make(chan struct{})
	releaseLeader := make(chan struct{})
	leaderRefresh := func(ctx context.Context) (cookie.Auth, error) {
		close(leaderStarted)
		<-releaseLeader

		return cookie.Auth{AccessToken: "leader-result"}, nil
	}

	go func() {
		_, _ = c.Do(context.Background(), "timeout-key", leaderRefresh)
	}()
	<-leaderStarted

	waiterRan := int32(0)
	waiterRefresh := func(ctx context.Context) (cookie.Auth, error) {
		atomic.AddInt32(&waiterRan, 1)

		return cookie.Auth{AccessToken: "degraded-result"}, nil
	}

	auth, err := c.Do(context.Background(), "timeout-key", waiterRefresh)

	assert.NoError(t, err)
	assert.Equal(t, cookie.Auth{AccessToken: "degraded-result"}, auth)
	assert.EqualValues(t, 1, atomic.LoadInt32(&waiterRan))
	assert.Equal(t, before+1, testutil.ToFloat64(refreshWaitTimeoutsTotal))

	close(releaseLeader)
}

// --- Tier 2 (Redis) ---

func TestLeadNoRedisClientRunsRefreshDirectly(t *testing.T) {
	c := NewRedis(nil, 0)

	called := false
	auth, err := c.lead(context.Background(), "some-key", func(context.Context) (cookie.Auth, error) {
		called = true

		return cookie.Auth{AccessToken: "direct"}, nil
	})

	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, cookie.Auth{AccessToken: "direct"}, auth)
}

func TestLeadAcquiresLockRunsRefreshPublishesAndReleasesLock(t *testing.T) {
	client, mock := redismock.NewClientMock()
	key := Key("refresh-token-lead")
	lockKey := fmt.Sprintf("%s:lock:%s", keyPrefix, key)
	resultKey := fmt.Sprintf("%s:result:%s", keyPrefix, key)

	mock.ExpectSetNX(lockKey, "inst-1", defaultLockTtl).SetVal(true)
	mock.CustomMatch(func(expected, actual []interface{}) error {
		return nil // payload is random (nonce); only the key/ttl framing is verified below
	}).ExpectSet(resultKey, nil, resultTtl).SetVal("OK")
	mock.ExpectEval(unlockScript, []string{lockKey}, "inst-1").SetVal(int64(1))

	c := &RedisCoordinator{redis: client, instanceID: "inst-1", lockTtl: defaultLockTtl, calls: make(map[string]*call)}

	called := false
	wantAuth := cookie.Auth{AccessToken: "leader-token"}
	auth, err := c.lead(context.Background(), key, func(context.Context) (cookie.Auth, error) {
		called = true

		return wantAuth, nil
	})

	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, wantAuth, auth)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestLeadRefreshErrorSkipsPublishButStillReleasesLock(t *testing.T) {
	client, mock := redismock.NewClientMock()
	key := Key("refresh-token-lead-err")
	lockKey := fmt.Sprintf("%s:lock:%s", keyPrefix, key)

	mock.ExpectSetNX(lockKey, "inst-1", defaultLockTtl).SetVal(true)
	mock.ExpectEval(unlockScript, []string{lockKey}, "inst-1").SetVal(int64(1))

	c := &RedisCoordinator{redis: client, instanceID: "inst-1", lockTtl: defaultLockTtl, calls: make(map[string]*call)}

	wantErr := errors.New("refresh failed")
	_, err := c.lead(context.Background(), key, func(context.Context) (cookie.Auth, error) {
		return cookie.Auth{}, wantErr
	})

	assert.ErrorIs(t, err, wantErr)
	// No ExpectSet was queued: publishResult must not be called on a failed refresh.
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestLeadLockAcquisitionErrorFallsBackToTier1(t *testing.T) {
	client, mock := redismock.NewClientMock()
	key := Key("refresh-token-lock-err")
	lockKey := fmt.Sprintf("%s:lock:%s", keyPrefix, key)

	mock.ExpectSetNX(lockKey, "inst-1", defaultLockTtl).SetErr(errors.New("connection refused"))

	c := &RedisCoordinator{redis: client, instanceID: "inst-1", lockTtl: defaultLockTtl, calls: make(map[string]*call)}

	before := testutil.ToFloat64(redisLockFailuresTotal)

	called := false
	auth, err := c.lead(context.Background(), key, func(context.Context) (cookie.Auth, error) {
		called = true

		return cookie.Auth{AccessToken: "fallback"}, nil
	})

	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, cookie.Auth{AccessToken: "fallback"}, auth)
	assert.Equal(t, before+1, testutil.ToFloat64(redisLockFailuresTotal))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestLeadLockContendedFollowsAndGetsPublishedResult(t *testing.T) {
	withPollInterval(t, 5*time.Millisecond)

	client, mock := redismock.NewClientMock()
	key := Key("refresh-token-contended")
	lockKey := fmt.Sprintf("%s:lock:%s", keyPrefix, key)
	resultKey := fmt.Sprintf("%s:result:%s", keyPrefix, key)

	wantAuth := cookie.Auth{AccessToken: "published-token"}
	payload, err := encryptAuth(key, wantAuth)
	assert.NoError(t, err)

	mock.ExpectSetNX(lockKey, "inst-2", defaultLockTtl).SetVal(false)
	mock.ExpectGet(resultKey).SetVal(payload)

	c := &RedisCoordinator{
		redis: client, instanceID: "inst-2", lockTtl: defaultLockTtl, waitBudget: time.Second,
		calls: make(map[string]*call),
	}

	called := false
	auth, err := c.lead(context.Background(), key, func(context.Context) (cookie.Auth, error) {
		called = true

		return cookie.Auth{}, nil
	})

	assert.NoError(t, err)
	assert.False(t, called, "follower must not run its own refresh once it sees the leader's result")
	assert.Equal(t, wantAuth, auth)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFollowRedisLeaderCorruptPayloadDegradesAfterBudget(t *testing.T) {
	// pollInterval > waitBudget: the very first tick already lands past the
	// deadline, giving a deterministic single poll before degrading.
	withPollInterval(t, 30*time.Millisecond)

	client, mock := redismock.NewClientMock()
	key := Key("refresh-token-corrupt")
	lockKey := fmt.Sprintf("%s:lock:%s", keyPrefix, key)
	resultKey := fmt.Sprintf("%s:result:%s", keyPrefix, key)

	mock.ExpectSetNX(lockKey, "inst-3", defaultLockTtl).SetVal(false)
	mock.ExpectGet(resultKey).SetVal("not-a-valid-encrypted-payload")

	c := &RedisCoordinator{
		redis: client, instanceID: "inst-3", lockTtl: defaultLockTtl, waitBudget: 5 * time.Millisecond,
		calls: make(map[string]*call),
	}

	before := testutil.ToFloat64(refreshWaitTimeoutsTotal)

	called := false
	auth, err := c.lead(context.Background(), key, func(context.Context) (cookie.Auth, error) {
		called = true

		return cookie.Auth{AccessToken: "degraded"}, nil
	})

	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, cookie.Auth{AccessToken: "degraded"}, auth)
	assert.Equal(t, before+1, testutil.ToFloat64(refreshWaitTimeoutsTotal))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFollowRedisLeaderNeverPublishedDegradesAfterBudget(t *testing.T) {
	// pollInterval > waitBudget: see TestFollowRedisLeaderCorruptPayloadDegradesAfterBudget.
	withPollInterval(t, 30*time.Millisecond)

	client, mock := redismock.NewClientMock()
	key := Key("refresh-token-never-published")
	lockKey := fmt.Sprintf("%s:lock:%s", keyPrefix, key)
	resultKey := fmt.Sprintf("%s:result:%s", keyPrefix, key)

	mock.ExpectSetNX(lockKey, "inst-4", defaultLockTtl).SetVal(false)
	mock.ExpectGet(resultKey).RedisNil()

	c := &RedisCoordinator{
		redis: client, instanceID: "inst-4", lockTtl: defaultLockTtl, waitBudget: 5 * time.Millisecond,
		calls: make(map[string]*call),
	}

	called := false
	auth, err := c.lead(context.Background(), key, func(context.Context) (cookie.Auth, error) {
		called = true

		return cookie.Auth{AccessToken: "degraded"}, nil
	})

	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, cookie.Auth{AccessToken: "degraded"}, auth)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFollowRedisLeaderRedisErrorFallsBackToTier1(t *testing.T) {
	withPollInterval(t, 5*time.Millisecond)

	client, mock := redismock.NewClientMock()
	key := Key("refresh-token-poll-err")
	lockKey := fmt.Sprintf("%s:lock:%s", keyPrefix, key)
	resultKey := fmt.Sprintf("%s:result:%s", keyPrefix, key)

	mock.ExpectSetNX(lockKey, "inst-5", defaultLockTtl).SetVal(false)
	mock.ExpectGet(resultKey).SetErr(errors.New("connection refused"))

	c := &RedisCoordinator{
		redis: client, instanceID: "inst-5", lockTtl: defaultLockTtl, waitBudget: time.Second,
		calls: make(map[string]*call),
	}

	before := testutil.ToFloat64(redisLockFailuresTotal)

	called := false
	auth, err := c.lead(context.Background(), key, func(context.Context) (cookie.Auth, error) {
		called = true

		return cookie.Auth{AccessToken: "fallback"}, nil
	})

	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, cookie.Auth{AccessToken: "fallback"}, auth)
	assert.Equal(t, before+1, testutil.ToFloat64(redisLockFailuresTotal))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFollowRedisLeaderRespectsContextCancellation(t *testing.T) {
	withPollInterval(t, time.Second) // long poll interval: ctx.Done() must win first

	client, mock := redismock.NewClientMock()
	key := Key("refresh-token-follow-cancel")
	lockKey := fmt.Sprintf("%s:lock:%s", keyPrefix, key)

	mock.ExpectSetNX(lockKey, "inst-6", defaultLockTtl).SetVal(false)

	c := &RedisCoordinator{
		redis: client, instanceID: "inst-6", lockTtl: defaultLockTtl, waitBudget: time.Second,
		calls: make(map[string]*call),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	called := false
	_, err := c.lead(ctx, key, func(context.Context) (cookie.Auth, error) {
		called = true

		return cookie.Auth{}, nil
	})

	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, called)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPublishResultEncryptFailureIsLoggedNotFatal(t *testing.T) {
	client, mock := redismock.NewClientMock()

	// An invalid (non-32-byte) key makes encryptAuth fail before any Redis call.
	c := &RedisCoordinator{redis: client, instanceID: "inst-7", calls: make(map[string]*call)}

	c.publishResult(context.Background(), "not-a-valid-hex-key", cookie.Auth{AccessToken: "x"})

	// No Redis command expected: encryption failed before Set was attempted.
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPublishResultSetFailureIncrementsMetric(t *testing.T) {
	client, mock := redismock.NewClientMock()
	key := Key("refresh-token-publish-fail")
	resultKey := fmt.Sprintf("%s:result:%s", keyPrefix, key)

	mock.CustomMatch(func(expected, actual []interface{}) error {
		return nil
	}).ExpectSet(resultKey, nil, resultTtl).SetErr(errors.New("connection refused"))

	c := &RedisCoordinator{redis: client, instanceID: "inst-8", calls: make(map[string]*call)}

	before := testutil.ToFloat64(redisLockFailuresTotal)

	c.publishResult(context.Background(), key, cookie.Auth{AccessToken: "x"})

	assert.Equal(t, before+1, testutil.ToFloat64(redisLockFailuresTotal))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestReleaseLockOnlyReleasesWhenOwned(t *testing.T) {
	client, mock := redismock.NewClientMock()
	lockKey := fmt.Sprintf("%s:lock:%s", keyPrefix, "some-key")

	// Simulates the lock having already expired and been re-acquired by another
	// instance: the CAS script must see ARGV[1] mismatch and return 0 (no-op),
	// never a bare DEL.
	mock.ExpectEval(unlockScript, []string{lockKey}, "inst-9").SetVal(int64(0))

	c := &RedisCoordinator{redis: client, instanceID: "inst-9", calls: make(map[string]*call)}
	c.releaseLock(context.Background(), lockKey)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestReleaseLockErrorIsLoggedNotFatal(t *testing.T) {
	client, mock := redismock.NewClientMock()
	lockKey := fmt.Sprintf("%s:lock:%s", keyPrefix, "some-key")

	mock.ExpectEval(unlockScript, []string{lockKey}, "inst-10").SetErr(errors.New("connection refused"))

	c := &RedisCoordinator{redis: client, instanceID: "inst-10", calls: make(map[string]*call)}
	c.releaseLock(context.Background(), lockKey) // must not panic

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNewRedisGeneratesDistinctInstanceIDs(t *testing.T) {
	a := NewRedis(nil, 0)
	b := NewRedis(nil, 0)

	assert.NotEmpty(t, a.instanceID)
	assert.NotEqual(t, a.instanceID, b.instanceID)
}

func TestWaitBudgetIsLockTtlPlusSlack(t *testing.T) {
	c := NewRedis(nil, 10*time.Second)

	assert.Equal(t, 10*time.Second+waitBudgetSlack, c.WaitBudget())
}

func TestNewInstanceIDFallsBackWhenUuidNewV7Fails(t *testing.T) {
	orig := uuidNewV7
	t.Cleanup(func() { uuidNewV7 = orig })
	uuidNewV7 = func() (uuid.UUID, error) {
		return uuid.UUID{}, errors.New("forced uuid failure")
	}

	id := newInstanceID()

	assert.NotEmpty(t, id)
	assert.True(t, strings.HasPrefix(id, "gateway-"))
}

func TestNewRedisUsesProvidedLockTtl(t *testing.T) {
	c := NewRedis(nil, 20*time.Second)

	assert.Equal(t, 20*time.Second, c.lockTtl)
}

func TestNewRedisFallsBackToDefaultLockTtlWhenNotPositive(t *testing.T) {
	for _, ttl := range []time.Duration{0, -time.Second} {
		c := NewRedis(nil, ttl)

		assert.Equal(t, defaultLockTtl, c.lockTtl)
	}
}

// --- waitBudget / lockTtl invariant ---

// TestWaitBudgetNeverShorterThanLockTtl asserts the *relationship* holds for any lockTtl
// a caller passes to NewRedis, not just one snapshot pair of numbers — waitBudget is
// derived from lockTtl (never set independently), so this can't drift out of sync the
// way the original fixed 5s waitBudget did once lockTtl grew to 15s in production.
func TestWaitBudgetNeverShorterThanLockTtl(t *testing.T) {
	for _, lockTtl := range []time.Duration{
		0, // falls back to defaultLockTtl inside NewRedis
		time.Millisecond,
		100 * time.Millisecond,
		defaultLockTtl,
		15 * time.Second, // production RefreshLockTTL()
		time.Minute,
	} {
		c := NewRedis(nil, lockTtl)

		assert.GreaterOrEqualf(t, c.waitBudget, c.lockTtl,
			"waitBudget (%s) must never be shorter than lockTtl (%s), or a waiter can give "+
				"up and run a duplicate refresh while the elected leader is still legitimately "+
				"in flight", c.waitBudget, c.lockTtl)
	}
}

// TestDoWaiterDoesNotFallBackWhileLeaderStillWithinLockTtl guards the round-2 regression
// this waitBudget/lockTtl tie prevents: a waiter must not give up while the leader is
// still within its lockTtl allowance, or it replays the leader's already-consumed
// refresh token — spurious 401, spurious logout, double-invoked CSRF Seed.
func TestDoWaiterDoesNotFallBackWhileLeaderStillWithinLockTtl(t *testing.T) {
	withWaitBudgetSlack(t, 20*time.Millisecond)

	c := NewRedis(nil, 40*time.Millisecond) // waitBudget = 40ms + 20ms = 60ms

	leaderStarted := make(chan struct{})
	leaderRan := int32(0)
	leaderRefresh := func(ctx context.Context) (cookie.Auth, error) {
		atomic.AddInt32(&leaderRan, 1)
		close(leaderStarted)
		time.Sleep(35 * time.Millisecond) // close to lockTtl, comfortably under waitBudget

		return cookie.Auth{AccessToken: "leader-result"}, nil
	}

	go func() {
		_, _ = c.Do(context.Background(), "boundary-key", leaderRefresh)
	}()
	<-leaderStarted

	waiterRan := int32(0)
	auth, err := c.Do(context.Background(), "boundary-key", func(context.Context) (cookie.Auth, error) {
		atomic.AddInt32(&waiterRan, 1)

		return cookie.Auth{AccessToken: "duplicate-refresh"}, nil
	})

	assert.NoError(t, err)
	assert.EqualValues(t, 1, atomic.LoadInt32(&leaderRan))
	assert.EqualValues(t, 0, atomic.LoadInt32(&waiterRan),
		"waiter must not run its own refresh while the leader is still within its lockTtl window")
	assert.Equal(t, cookie.Auth{AccessToken: "leader-result"}, auth)
}

// --- Panic safety ---

// TestDoLeaderPanicCleansUpAndPropagates guards against the leader registration
// (c.calls[key]) and its done channel leaking forever if refresh panics: every future
// request sharing this key would otherwise block in wait() for a full waitBudget,
// forever, since nothing would ever close leaderCall.done or delete the map entry.
func TestDoLeaderPanicCleansUpAndPropagates(t *testing.T) {
	c := NewRedis(nil, 0)

	panicking := func(context.Context) (cookie.Auth, error) {
		panic("boom")
	}

	assert.PanicsWithValue(t, "boom", func() {
		_, _ = c.Do(context.Background(), "panic-key", panicking)
	})

	c.mu.Lock()
	_, stillRegistered := c.calls["panic-key"]
	c.mu.Unlock()
	assert.False(t, stillRegistered, "leader registration must not leak after a panic")

	// A subsequent call for the same key must run its own refresh immediately, not
	// block for a wait budget waiting on a leader that will never finish.
	ran := false
	auth, err := c.Do(context.Background(), "panic-key", func(context.Context) (cookie.Auth, error) {
		ran = true

		return cookie.Auth{AccessToken: "recovered"}, nil
	})

	assert.NoError(t, err)
	assert.True(t, ran)
	assert.Equal(t, cookie.Auth{AccessToken: "recovered"}, auth)
}

// TestDoWaiterGetsErrorWhenLeaderPanics guards against a coalesced waiter reading the
// leader's zero-value cookie.Auth{} and nil error as a false "success" once done is
// closed during panic cleanup — it must see an error instead.
func TestDoWaiterGetsErrorWhenLeaderPanics(t *testing.T) {
	c := NewRedis(nil, 0)

	leaderStarted := make(chan struct{})
	panicking := func(context.Context) (cookie.Auth, error) {
		close(leaderStarted)
		time.Sleep(20 * time.Millisecond) // give the waiter time to register

		panic("boom")
	}

	leaderDone := make(chan struct{})
	go func() {
		defer func() {
			_ = recover() // this goroutine only needs to observe completion, not the panic

			close(leaderDone)
		}()
		_, _ = c.Do(context.Background(), "panic-waiter-key", panicking)
	}()
	<-leaderStarted

	var waiterErr error
	waiterDone := make(chan struct{})
	go func() {
		_, waiterErr = c.Do(context.Background(), "panic-waiter-key", func(context.Context) (cookie.Auth, error) {
			t.Error("waiter must not run its own refresh — it should get the leader's panic error")

			return cookie.Auth{}, nil
		})
		close(waiterDone)
	}()

	<-leaderDone
	<-waiterDone

	assert.Error(t, waiterErr)
	assert.Contains(t, waiterErr.Error(), "boom")
}

// TestLeadReleasesLockWhenRefreshPanics guards against a panicking leader leaking the
// cross-instance lock for its full lockTtl, leaving every follower polling instead of
// re-electing.
func TestLeadReleasesLockWhenRefreshPanics(t *testing.T) {
	client, mock := redismock.NewClientMock()
	key := Key("refresh-token-lead-panic")
	lockKey := fmt.Sprintf("%s:lock:%s", keyPrefix, key)

	mock.ExpectSetNX(lockKey, "inst-11", defaultLockTtl).SetVal(true)
	mock.ExpectEval(unlockScript, []string{lockKey}, "inst-11").SetVal(int64(1))

	c := &RedisCoordinator{redis: client, instanceID: "inst-11", lockTtl: defaultLockTtl, calls: make(map[string]*call)}

	assert.PanicsWithValue(t, "boom", func() {
		_, _ = c.lead(context.Background(), key, func(context.Context) (cookie.Auth, error) {
			panic("boom")
		})
	})

	// No ExpectSet was queued: publishResult must not run when refresh panics before
	// returning. The queued ExpectEval (release) being consumed proves releaseLock
	// still ran despite the panic.
	assert.NoError(t, mock.ExpectationsWereMet())
}
