package inmem

import (
	"context"
	"sync"
	"time"

	"github.com/mshindle/ratelimit"
)

type accountLimit struct {
	limit      int64
	window     time.Duration
	tokens     int64
	lastFilled time.Time
}

type Limiter struct {
	mutex     *sync.RWMutex
	resources map[string]*accountLimit
}

func New() *Limiter {
	l := &Limiter{
		mutex:     &sync.RWMutex{},
		resources: make(map[string]*accountLimit),
	}

	return l
}

func (ml *Limiter) SetLimit(_ context.Context, resourceName, accountID string, limit int64, windowSec float64) error {
	var al *accountLimit
	var ok bool

	key := ratelimit.CreateKey(resourceName, accountID)

	ml.mutex.Lock()
	defer ml.mutex.Unlock()

	if al, ok = ml.resources[key]; !ok {
		al = &accountLimit{}
		ml.resources[key] = al
	}

	al.limit = limit
	al.window = time.Duration(windowSec * float64(time.Second))

	return nil
}

func (ml *Limiter) GetLimit(_ context.Context, resourceName, accountID string) (limit int64, window float64, err error) {
	var al *accountLimit
	var ok bool

	key := ratelimit.CreateKey(resourceName, accountID)

	ml.mutex.RLock()
	defer ml.mutex.RUnlock()

	if al, ok = ml.resources[key]; !ok {
		err = ratelimit.ErrNoLimitSet
		return
	}
	limit = al.limit
	window = float64(al.window) / float64(time.Second)
	return
}

func (ml *Limiter) GetToken(ctx context.Context, resourceName, accountID string) (bool, error) {
	return ml.GetTokens(ctx, resourceName, accountID, 1)
}

func (ml *Limiter) GetTokens(_ context.Context, resourceName, accountID string, requested int64) (bool, error) {
	var al *accountLimit
	var ok bool

	// lock the process
	ml.mutex.Lock()
	defer ml.mutex.Unlock()

	// grab the current limits
	key := ratelimit.CreateKey(resourceName, accountID)
	if al, ok = ml.resources[key]; !ok {
		return false, ratelimit.ErrNoLimitSet
	}

	// determine how much time has passed
	delta := time.Now().Sub(al.lastFilled)
	if delta < 0 {
		delta = 0
	}

	// determine new token amount with fill
	gainedTokens := int64(delta / al.window)
	filledTokens := minInt64(al.limit, al.tokens+gainedTokens)
	lastFilled := al.lastFilled.Add(al.window * time.Duration(gainedTokens))
	allowed := filledTokens >= requested
	if allowed {
		filledTokens -= requested
	}

	al.tokens = filledTokens
	al.lastFilled = lastFilled

	return allowed, nil
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
