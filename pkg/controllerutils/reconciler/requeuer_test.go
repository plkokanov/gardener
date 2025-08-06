// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package reconciler_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/gomega"

	. "github.com/gardener/gardener/pkg/controllerutils/reconciler"
)

var _ = Describe("RequeueRateLimiter", func() {
	It("should increase the requeue counter", func() {
		fakeRateLimiter := &fakeRateLimiter{}

		reconcileFunc(context.TODO(), fakeRateLimiter, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "foo", Name: "bar"}}, true, nil)
		Expect(fakeRateLimiter.counter).To(Equal(1))

		reconcileFunc(context.TODO(), fakeRateLimiter, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "foo", Name: "bar"}}, true, nil)
		Expect(fakeRateLimiter.counter).To(Equal(2))

		reconcileFunc(context.TODO(), fakeRateLimiter, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "foo", Name: "bar"}}, true, fmt.Errorf("fake error"))
		Expect(fakeRateLimiter.counter).To(Equal(3))

		reconcileFunc(context.TODO(), fakeRateLimiter, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "foo", Name: "bar"}}, false, fmt.Errorf("fake error"))
		Expect(fakeRateLimiter.counter).To(Equal(4))

		reconcileFunc(context.TODO(), fakeRateLimiter, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "foo", Name: "bar"}}, false, nil)
		Expect(fakeRateLimiter.counter).To(Equal(0))
	})
})

type fakeRateLimiter struct {
	counter int
}

func (r *fakeRateLimiter) NumRequeues(req reconcile.Request) int {
	return 0
}

func (r *fakeRateLimiter) When(req reconcile.Request) time.Duration {
	r.counter++
	return time.Second * time.Duration(r.counter)
}

func (r *fakeRateLimiter) Forget(req reconcile.Request) {
	r.counter = 0
}

func reconcileFunc(ctx context.Context, rateLimiter *fakeRateLimiter, req reconcile.Request, requeue bool, someErr error) (result reconcile.Result, err error) {
	requeuer := NewRequeueRateLimiter(rateLimiter, RateLimitOnError(&err))
	defer requeuer.Forget(req)

	if requeue {
		return reconcile.Result{RequeueAfter: requeuer.When(req)}, nil
	}

	if someErr != nil {
		return reconcile.Result{}, someErr
	}

	return
}
