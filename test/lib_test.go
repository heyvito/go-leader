package test

import (
	"context"
	"fmt"
	"github.com/heyvito/go-leader/leader"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type faultyClient struct {
	r        *redis.Client
	breaking bool
}

func (f *faultyClient) Get(ctx context.Context, key string) *redis.StringCmd {
	r := &redis.StringCmd{}
	if f.breaking {
		r.SetErr(fmt.Errorf("faulty client is faulty"))
	} else {
		r = f.r.Get(ctx, key)
	}

	return r
}

func (f *faultyClient) EvalSha(ctx context.Context, sha string, keys []string, args ...any) *redis.Cmd {
	r := &redis.Cmd{}
	if f.breaking {
		r.SetErr(fmt.Errorf("faulty client is faulty"))
	} else {
		r = f.r.EvalSha(ctx, sha, keys, args...)
	}

	return r
}

func (f *faultyClient) ScriptLoad(ctx context.Context, script string) *redis.StringCmd {
	r := &redis.StringCmd{}
	if f.breaking {
		r.SetErr(fmt.Errorf("faulty client is faulty"))
	} else {
		r = f.r.ScriptLoad(ctx, script)
	}

	return r
}

type leaderWatcher struct {
	promoted, demoted, errored bool
	lastError                  error
}

func (l *leaderWatcher) reset() {
	l.promoted = false
	l.demoted = false
	l.errored = false
	l.lastError = nil
}

func makeRedis(t *testing.T) *redis.Client {
	opts, err := redis.ParseURL("redis://localhost:6379/10")
	require.NoError(t, err)

	cli := redis.NewClient(opts)
	err = cli.FlushDB(context.Background()).Err()
	require.NoError(t, err)
	err = cli.ScriptFlush(context.Background()).Err()
	require.NoError(t, err)

	return cli
}

func makeWatcher(opts leader.Opts) (leader.Leader, *leaderWatcher) {
	lead, promote, demote, err := leader.NewLeader(opts)
	watcher := &leaderWatcher{}
	go func() {
		for {
			select {
			case <-promote:
				watcher.promoted = true
			case <-demote:
				watcher.demoted = true
			case err := <-err:
				watcher.errored = true
				watcher.lastError = err
			}
		}
	}()
	lead.Start()
	return lead, watcher
}

func TestLib(t *testing.T) {
	cli := makeRedis(t)

	cli1 := &faultyClient{r: cli}
	leader1, watch1 := makeWatcher(leader.Opts{
		Redis:    cli1,
		TTL:      1 * time.Second,
		Wait:     2 * time.Second,
		JitterMS: 50,
		Key:      "test1",
	})
	leader1.Start()

	// Wait a sec until l1 is done
	time.Sleep(100 * time.Millisecond)

	cli2 := &faultyClient{r: cli}
	leader2, watch2 := makeWatcher(leader.Opts{
		Redis:    cli2,
		TTL:      1 * time.Second,
		Wait:     2 * time.Second,
		JitterMS: 50,
		Key:      "test1",
	})
	leader2.Start()
	time.Sleep(100 * time.Millisecond)

	require.True(t, watch1.promoted)
	require.False(t, watch2.promoted)

	t.Run("leader 1 retains its lease", func(t *testing.T) {
		// Here we will wait three seconds and see whether 1 is still the leader.
		// In that case, no watcher should have changed.
		watch1.reset()
		watch2.reset()
		time.Sleep(3 * time.Second)
		assert.False(t, watch1.errored, "leader1 should not have errored")
		assert.Nil(t, watch1.lastError)
		assert.False(t, watch1.demoted, "leader1 should not be demoted")
		assert.False(t, watch1.promoted, "leader1 should not be promoted")

		assert.False(t, watch2.errored, "leader2 should not have errored")
		assert.Nil(t, watch2.lastError)
		assert.False(t, watch2.demoted, "leader2 should not be demoted")
		assert.False(t, watch2.promoted, "leader2 should not be promoted")
	})

	t.Run("it automatically takes the place of another leader", func(t *testing.T) {
		// Break leader1, wait leader2 take its place. may take up to three seconds.
		cli1.breaking = true
		watch1.reset()
		watch2.reset()
		time.Sleep(3 * time.Second)
		assert.True(t, watch1.demoted, "leader1 should be demoted")
		assert.False(t, watch1.promoted, "leader1 should not be promoted")

		assert.False(t, watch2.errored, "leader2 should not have errored")
		assert.Nil(t, watch2.lastError)
		assert.False(t, watch2.demoted, "leader2 should not be demoted")
		assert.True(t, watch2.promoted, "leader2 should be promoted")
	})

	t.Run("leader 2 retains its lease", func(t *testing.T) {
		// Here we will wait three seconds and see whether 2 is still the leader.
		// In that case, no watcher should have changed.
		watch1.reset()
		watch2.reset()
		time.Sleep(3 * time.Second)
		assert.False(t, watch1.demoted, "leader1 should not be demoted")
		assert.False(t, watch1.promoted, "leader1 should not be promoted")

		assert.False(t, watch2.errored, "leader2 should not have errored")
		assert.Nil(t, watch2.lastError)
		assert.False(t, watch2.demoted, "leader2 should not be demoted")
		assert.False(t, watch2.promoted, "leader2 should not be promoted")
	})

	t.Run("leader 1 takes its state back once leader2 breaks", func(t *testing.T) {
		cli1.breaking = false
		cli2.breaking = true
		watch1.reset()
		watch2.reset()
		time.Sleep(3 * time.Second)
		assert.False(t, watch1.errored, "leader1 should not have errored")
		assert.Nil(t, watch1.lastError)
		assert.False(t, watch1.demoted, "leader1 should not be demoted")
		assert.True(t, watch1.promoted, "leader1 be promoted")

		assert.True(t, watch2.demoted, "leader2 should be demoted")
		assert.False(t, watch2.promoted, "leader2 should not be promoted")
	})

	t.Run("Stop", func(t *testing.T) {
		cli2.breaking = false
		cli1.breaking = false

		err := leader1.Stop()
		assert.NoError(t, err)
		err = leader2.Stop()
		assert.NoError(t, err)
	})
}
