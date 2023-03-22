package leader

import (
	"crypto/sha1"
	"encoding/base32"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

type Leader interface {
	Start()
	Stop() error
	IsLeading() bool
}

type Opts struct {
	Redis    RedisCompatible
	TTL      time.Duration
	Wait     time.Duration
	JitterMS int
	Key      string
}

func makeKey(input string) string {
	sha := sha1.New()
	sha.Write([]byte(input))
	return "leader:" + base32.StdEncoding.EncodeToString(sha.Sum(nil))
}

func clientID() string {
	src := rand.New(rand.NewSource(time.Now().UnixMicro()))
	id := make([]byte, 16)
	src.Read(id)

	return base32.StdEncoding.EncodeToString(id)
}

func NewLeader(opts Opts) (leader Leader, onPromote <-chan time.Time, onDemote <-chan time.Time, onError <-chan error) {
	if opts.TTL == 0 {
		panic("NewLeader received a zero TTL")
	}

	if opts.Wait == 0 {
		panic("NewLeader received a zero Wait value")
	}

	prom := make(chan time.Time, 10)
	demo := make(chan time.Time, 10)
	err := make(chan error, 10)

	return &impl{
		redis:   &redisWrapper{opts.Redis},
		leading: false,
		key:     makeKey(opts.Key),
		ttl:     opts.TTL,
		wait:    opts.Wait,
		jitter:  time.Duration(opts.JitterMS) * time.Millisecond,
		id:      clientID(),

		startStopLock: &sync.Mutex{},
		electionLock:  &sync.Mutex{},

		promoteCh: prom,
		demoteCh:  demo,
		errorCh:   err,
	}, prom, demo, err
}

type impl struct {
	redis     *redisWrapper
	key       string
	id        string
	promoteCh chan time.Time
	demoteCh  chan time.Time
	errorCh   chan error
	ttl       time.Duration
	wait      time.Duration
	jitter    time.Duration

	running           bool
	leading           bool
	cancelRenewRun    func()
	cancelElectionRun func()
	startStopLock     *sync.Mutex
	electionLock      *sync.Mutex
	stopCh            chan bool
}

func randomJitter(valMS time.Duration) time.Duration {
	if valMS == 0 {
		return 0
	}
	return time.Duration(rand.Intn(int(valMS.Milliseconds()))) * time.Millisecond
}

func (i *impl) renew() {
	i.electionLock.Lock()
	defer i.electionLock.Unlock()

	i.cancelRenewRun = nil

	leading, err := i.redis.atomicRenew(i.key, i.id, i.ttl)
	if err != nil {
		// This is bad, as we failed to renew our lease. Pretend we just lost
		// the lease, and we will retry to become leader in case the error
		// goes away on the next election.
		i.errorCh <- fmt.Errorf("trying to renew lease: %w", err)

		if i.leading {
			i.leading = false
			i.demoteCh <- time.Now()
		}

		i.cancelElectionRun = runAfter(i.wait, i.runElection)
		return
	}

	if leading {
		// We are still leading. Just prepare to the next renew.
		i.cancelRenewRun = runAfter((i.ttl/2)+randomJitter(i.jitter), i.renew)
		return
	}
	// We are no longer leading. Announce the demotion, and look for the
	// next election.
	if i.leading {
		i.leading = false
		i.demoteCh <- time.Now()
	}
	i.cancelElectionRun = runAfter(i.wait, i.runElection)
}

func (i *impl) runElection() {
	i.electionLock.Lock()
	defer i.electionLock.Unlock()
	i.cancelElectionRun = nil

	set, err := i.redis.atomicSet(i.key, i.id, i.ttl)
	if err != nil {
		i.errorCh <- fmt.Errorf("trying to run election: %w", err)
		i.cancelElectionRun = runAfter(i.wait, i.runElection)
		return
	}

	if set {
		i.leading = true
		i.promoteCh <- time.Now()

		i.cancelRenewRun = runAfter((i.ttl/2)+randomJitter(i.jitter), i.renew)
	} else {
		if i.leading {
			i.demoteCh <- time.Now()
		}
		i.leading = false
		if i.cancelRenewRun != nil {
			i.cancelRenewRun()
			i.cancelRenewRun = nil
		}
		i.cancelElectionRun = runAfter(i.wait, i.runElection)
	}

}

func (i *impl) voidTimers() {
	if i.cancelElectionRun != nil {
		i.cancelElectionRun()
		i.cancelRenewRun = nil
	}
	if i.cancelRenewRun != nil {
		i.cancelRenewRun()
		i.cancelRenewRun = nil
	}
}

func (i *impl) resign() error {
	i.electionLock.Lock()
	defer i.electionLock.Unlock()
	i.voidTimers()

	leading, err := i.redis.atomicDelete(i.key, i.id)
	if err != nil {
		return err
	}

	i.running = false
	if leading {
		i.demoteCh <- time.Now()
	}

	i.leading = false

	return nil
}

func (i *impl) Start() {
	i.startStopLock.Lock()
	defer i.startStopLock.Unlock()
	if i.running {
		return
	}

	i.running = true
	go i.runElection()
	return
}

func (i *impl) Stop() error {
	i.startStopLock.Lock()
	defer i.startStopLock.Unlock()
	if !i.running {
		return nil
	}
	return i.resign()
}

func (i *impl) IsLeading() bool {
	return i.leading
}
