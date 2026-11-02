/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ebspricing

import (
	"context"
	"fmt"
	"time"

	"github.com/awslabs/operatorpkg/reconciler"
	"github.com/awslabs/operatorpkg/singleton"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/karpenter/pkg/operator/injection"

	"github.com/aws/karpenter-provider-aws/pkg/providers/ebspricing"
)

// Controller periodically refreshes EBS volume pricing. It's separate from instance pricing so that an EBS pricing
// failure only retries the EBS pricing fetch, while the provider keeps serving the pricing it already has.
type Controller struct {
	ebsPricingProvider ebspricing.Provider
}

func NewController(ebsPricingProvider ebspricing.Provider) *Controller {
	return &Controller{
		ebsPricingProvider: ebsPricingProvider,
	}
}

func (c *Controller) Reconcile(ctx context.Context) (reconciler.Result, error) {
	ctx = injection.WithControllerName(ctx, "providers.ebspricing")
	if err := c.ebsPricingProvider.UpdatePricing(ctx); err != nil {
		return reconciler.Result{}, fmt.Errorf("updating ebs pricing, %w", err)
	}
	return reconciler.Result{RequeueAfter: 12 * time.Hour}, nil
}

func (c *Controller) Register(_ context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		Named("providers.ebspricing").
		WatchesRawSource(singleton.Source()).
		Complete(singleton.AsReconciler(c))
}
