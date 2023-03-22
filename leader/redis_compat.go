package leader

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"strings"
	"time"
)

type RedisCompatible interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	EvalSha(ctx context.Context, sha string, keys []string, args ...any) *redis.Cmd
	ScriptLoad(ctx context.Context, script string) *redis.StringCmd
}

type redisWrapper struct {
	redis RedisCompatible
}

func (r *redisWrapper) installScripts() error {
	for _, script := range []string{atomicSetScript, atomicRenewScript, atomicDeleteScript} {
		if err := r.redis.ScriptLoad(context.Background(), script).Err(); err != nil {
			return err
		}
	}
	return nil
}

func hasPrefix(err error, prefix string) bool {
	msg := strings.TrimPrefix(err.Error(), "ERR ") // KVRocks adds such prefix
	return strings.HasPrefix(msg, prefix)
}

func (r *redisWrapper) recoverScriptFlush(fn func() error) error {
	retry := true
	for {
		err := fn()
		if err != nil && hasPrefix(err, "NOSCRIPT") && retry {
			retry = false
			if installErr := r.installScripts(); installErr != nil {
				return fmt.Errorf("failed installing scripts: %w", installErr)
			}
			continue
		}
		return err
	}
}

func (r *redisWrapper) atomicSet(key string, id string, timeout time.Duration) (bool, error) {
	status := false
	err := r.recoverScriptFlush(func() error {
		v, err := r.redis.EvalSha(context.Background(), atomicSetSHA, []string{key}, id, timeout.Milliseconds()).Result()
		if err != nil {
			return err
		}

		if i, ok := v.(int64); ok {
			status = i == 1
		}
		return nil
	})
	return status, err
}

func (r *redisWrapper) atomicDelete(key string, id string) (bool, error) {
	status := false
	err := r.recoverScriptFlush(func() error {
		v, err := r.redis.EvalSha(context.Background(), atomicDeleteSHA, []string{key}, id).Result()
		if err != nil {
			return err
		}

		if i, ok := v.(int64); ok {
			status = i == 1
		}
		return nil
	})
	return status, err
}

func (r *redisWrapper) atomicRenew(key string, id string, timeout time.Duration) (bool, error) {
	status := false
	err := r.recoverScriptFlush(func() error {
		v, err := r.redis.EvalSha(context.Background(), atomicRenewSHA, []string{key}, id, timeout.Milliseconds()).Result()
		if err != nil {
			return err
		}

		if i, ok := v.(int64); ok {
			status = i == 1
		}
		return nil
	})
	return status, err
}
