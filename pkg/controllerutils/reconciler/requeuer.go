package reconciler

import (
	"time"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type RequeueRateLimiter interface {
	workqueue.TypedRateLimiter[reconcile.Request]
	RequeueOnError(*error)
}

type defaultRequeueRateLimiter struct {
	err         *error
	rateLimiter workqueue.TypedRateLimiter[reconcile.Request]
	requeue     bool
}

type RequeueRateLimiterOption func(r RequeueRateLimiter)

func RateLimitOnError(err *error) RequeueRateLimiterOption {
	return func(r RequeueRateLimiter) {
		r.RequeueOnError(err)
	}
}

func NewRequeueRateLimiter(rateLimiter workqueue.TypedRateLimiter[reconcile.Request], opts ...RequeueRateLimiterOption) RequeueRateLimiter {
	r := &defaultRequeueRateLimiter{rateLimiter: rateLimiter}
	for _, opt := range opts {
		opt(r)
	}

	return r
}

func (r *defaultRequeueRateLimiter) RequeueOnError(err *error) {
	r.err = err
}

func (r *defaultRequeueRateLimiter) When(req reconcile.Request) time.Duration {
	r.requeue = true
	return r.rateLimiter.When(req)
}

func (r *defaultRequeueRateLimiter) Forget(req reconcile.Request) {
	if r.err != nil && *r.err != nil && !r.requeue {
		// If an error has occurred increase the rate limit
		r.rateLimiter.When(req)
		return
	}

	if !r.requeue {
		// If we don't have to requeue with backoff, remove item from rateLimiter
		r.rateLimiter.Forget(req)
	}
}

func (r *defaultRequeueRateLimiter) NumRequeues(req reconcile.Request) int {
	return r.rateLimiter.NumRequeues(req)
}
