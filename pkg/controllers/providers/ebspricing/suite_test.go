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

package ebspricing_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	awspricing "github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/pricing/types"
	"github.com/samber/lo"
	coreoptions "sigs.k8s.io/karpenter/pkg/operator/options"
	coretest "sigs.k8s.io/karpenter/pkg/test"

	"github.com/aws/karpenter-provider-aws/pkg/apis"
	controllersebspricing "github.com/aws/karpenter-provider-aws/pkg/controllers/providers/ebspricing"
	"github.com/aws/karpenter-provider-aws/pkg/fake"
	"github.com/aws/karpenter-provider-aws/pkg/operator/options"
	"github.com/aws/karpenter-provider-aws/pkg/providers/ebspricing"
	"github.com/aws/karpenter-provider-aws/pkg/test"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "sigs.k8s.io/karpenter/pkg/test/expectations"
	. "sigs.k8s.io/karpenter/pkg/utils/testing"
)

var ctx context.Context
var stop context.CancelFunc
var env *coretest.Environment
var awsEnv *test.Environment
var controller *controllersebspricing.Controller

func TestAWS(t *testing.T) {
	ctx = TestContextWithLogger(t)
	RegisterFailHandler(Fail)
	RunSpecs(t, "EBSPricing")
}

var _ = BeforeSuite(func() {
	env = coretest.NewEnvironment(coretest.WithCRDs(apis.CRDs...))
	ctx = coreoptions.ToContext(ctx, coretest.Options())
	ctx = options.ToContext(ctx, test.Options())
	ctx, stop = context.WithCancel(ctx)
	awsEnv = test.NewEnvironment(ctx, env)
	controller = controllersebspricing.NewController(awsEnv.EBSPricingProvider)
})

var _ = AfterSuite(func() {
	stop()
	Expect(env.Stop()).To(Succeed(), "Failed to stop environment")
})

var _ = BeforeEach(func() {
	ctx = coreoptions.ToContext(ctx, coretest.Options())
	ctx = options.ToContext(ctx, test.Options())
	awsEnv.Reset()
})

// pricingAPIResponse returns EBS rates that differ from the static pricing, so tests can tell them apart
func pricingAPIResponse() *awspricing.GetProductsOutput {
	return &awspricing.GetProductsOutput{
		PriceList: []string{
			// Instance pricing is returned by the same API and should be ignored
			fake.NewOnDemandPrice("c98.large", 1.20),
			// Usage types outside of us-east-1 are prefixed with the region
			fake.NewEBSPrice("gp3", "USW2-EBS:VolumeUsage.gp3", "GB-Mo", 1.0),
			fake.NewEBSPrice("gp3", "USW2-EBS:VolumeP-IOPS.gp3", "IOPS-Mo", 0.01),
			// gp3 throughput is priced per GiBps-month
			fake.NewEBSPrice("gp3", "USW2-EBS:VolumeP-Throughput.gp3", "GiBps-mo", 1024),
			fake.NewEBSPrice("io2", "EBS:VolumeUsage.io2", "GB-month", 2.0),
			fake.NewEBSPrice("io2", "EBS:VolumeP-IOPS.io2", "IOPS-Mo", 0.1),
			fake.NewEBSPrice("io2", "EBS:VolumeP-IOPS.io2.tier2", "IOPS-Mo", 0.01),
			fake.NewEBSPrice("io2", "EBS:VolumeP-IOPS.io2.tier3", "IOPS-Mo", 0.001),
			// Snapshots aren't volume pricing and should be ignored
			fake.NewEBSPrice("gp3", "EBS:SnapshotUsage", "GB-Mo", 100),
		},
	}
}

var _ = Describe("EBSPricing", func() {
	It("should serve static pricing before any update", func() {
		price, ok := awsEnv.EBSPricingProvider.VolumePrice("gp3", 20, 0, 0)
		Expect(ok).To(BeTrue())
		Expect(price).To(BeNumerically("~", 20*0.08/ebspricing.HoursPerMonth, 1e-9))
	})
	It("should serve static pricing for every generated region", func() {
		for region, prices := range lo.Assign(ebspricing.InitialPricesAWS, ebspricing.InitialPricesUSGov) {
			provider := ebspricing.NewDefaultProvider(awsEnv.PricingAPI, region, false)
			for volumeType, rates := range prices {
				price, ok := provider.VolumePrice(volumeType, 100, 0, 0)
				Expect(ok).To(BeTrue())
				Expect(price).To(BeNumerically("~", 100*rates.GBMonth/ebspricing.HoursPerMonth, 1e-9))
			}
		}
	})
	It("should not serve static pricing in the CN partition", func() {
		provider := ebspricing.NewDefaultProvider(awsEnv.PricingAPI, "cn-north-1", false)
		_, ok := provider.VolumePrice("gp3", 20, 0, 0)
		Expect(ok).To(BeFalse())
	})
	It("should not serve pricing for unknown volume types", func() {
		_, ok := awsEnv.EBSPricingProvider.VolumePrice("gp9", 20, 0, 0)
		Expect(ok).To(BeFalse())
	})
	It("should update pricing with the response from the pricing API", func() {
		awsEnv.PricingAPI.GetProductsBehavior.Output.Set(pricingAPIResponse())
		result := ExpectSingletonReconciled(ctx, controller)
		Expect(result.RequeueAfter).To(Equal(12 * time.Hour))

		// IOPS and throughput within gp3's included baseline aren't charged
		price, ok := awsEnv.EBSPricingProvider.VolumePrice("gp3", 10, 3000, 125)
		Expect(ok).To(BeTrue())
		Expect(price).To(BeNumerically("~", 10*1.0/ebspricing.HoursPerMonth, 1e-9))

		price, ok = awsEnv.EBSPricingProvider.VolumePrice("gp3", 10, 4000, 225)
		Expect(ok).To(BeTrue())
		Expect(price).To(BeNumerically("~", (10*1.0+1000*0.01+100*1.0)/ebspricing.HoursPerMonth, 1e-9))

		price, ok = awsEnv.EBSPricingProvider.VolumePrice("io2", 10, 70000, 0)
		Expect(ok).To(BeTrue())
		Expect(price).To(BeNumerically("~", (10*2.0+32000*0.1+32000*0.01+6000*0.001)/ebspricing.HoursPerMonth, 1e-9))

		// Volume types missing from the response keep their previous pricing
		price, ok = awsEnv.EBSPricingProvider.VolumePrice("gp2", 10, 0, 0)
		Expect(ok).To(BeTrue())
		Expect(price).To(BeNumerically("~", 10*0.1/ebspricing.HoursPerMonth, 1e-9))
	})
	It("should query each EBS product family for the provider's region", func() {
		awsEnv.PricingAPI.GetProductsBehavior.Output.Set(pricingAPIResponse())
		ExpectSingletonReconciled(ctx, controller)
		var families []string
		awsEnv.PricingAPI.GetProductsBehavior.CalledWithInput.ForEach(func(in *awspricing.GetProductsInput) {
			Expect(in.Filters).To(ContainElement(types.Filter{Field: lo.ToPtr("regionCode"), Type: "TERM_MATCH", Value: lo.ToPtr(fake.DefaultRegion)}))
			f, _ := lo.Find(in.Filters, func(f types.Filter) bool { return lo.FromPtr(f.Field) == "productFamily" })
			families = append(families, lo.FromPtr(f.Value))
		})
		Expect(families).To(ConsistOf("Storage", "System Operation", "Provisioned Throughput"))
	})
	It("should keep serving static pricing when the pricing API fails", func() {
		awsEnv.PricingAPI.GetProductsBehavior.Error.Set(fmt.Errorf("failed"))
		_ = ExpectSingletonReconcileFailed(ctx, controller)
		price, ok := awsEnv.EBSPricingProvider.VolumePrice("gp3", 20, 0, 0)
		Expect(ok).To(BeTrue())
		Expect(price).To(BeNumerically("~", 20*0.08/ebspricing.HoursPerMonth, 1e-9))
	})
	It("should keep serving previously fetched pricing when a later update fails", func() {
		awsEnv.PricingAPI.GetProductsBehavior.Output.Set(pricingAPIResponse())
		ExpectSingletonReconciled(ctx, controller)

		awsEnv.PricingAPI.GetProductsBehavior.Error.Set(fmt.Errorf("failed"))
		_ = ExpectSingletonReconcileFailed(ctx, controller)
		price, ok := awsEnv.EBSPricingProvider.VolumePrice("gp3", 10, 0, 0)
		Expect(ok).To(BeTrue())
		Expect(price).To(BeNumerically("~", 10*1.0/ebspricing.HoursPerMonth, 1e-9))
	})
	It("should fail the update and keep its pricing when the pricing API returns no EBS pricing", func() {
		awsEnv.PricingAPI.GetProductsBehavior.Output.Set(&awspricing.GetProductsOutput{
			PriceList: []string{fake.NewOnDemandPrice("c98.large", 1.20)},
		})
		_ = ExpectSingletonReconcileFailed(ctx, controller)
		price, ok := awsEnv.EBSPricingProvider.VolumePrice("gp3", 20, 0, 0)
		Expect(ok).To(BeTrue())
		Expect(price).To(BeNumerically("~", 20*0.08/ebspricing.HoursPerMonth, 1e-9))
	})
	It("should serve static pricing without calling the pricing API in an isolated VPC", func() {
		provider := ebspricing.NewDefaultProvider(awsEnv.PricingAPI, "us-east-1", true)
		awsEnv.PricingAPI.GetProductsBehavior.Output.Set(pricingAPIResponse())
		Expect(provider.UpdatePricing(ctx)).To(Succeed())
		Expect(awsEnv.PricingAPI.GetProductsBehavior.CalledWithInput.Len()).To(Equal(0))
		price, ok := provider.VolumePrice("gp3", 20, 0, 0)
		Expect(ok).To(BeTrue())
		Expect(price).To(BeNumerically("~", 20*0.08/ebspricing.HoursPerMonth, 1e-9))
	})
	It("should serve static pricing without calling the pricing API in an AWS GovCloud region", func() {
		provider := ebspricing.NewDefaultProvider(awsEnv.PricingAPI, "us-gov-east-1", false)
		Expect(provider.UpdatePricing(ctx)).To(Succeed())
		Expect(awsEnv.PricingAPI.GetProductsBehavior.CalledWithInput.Len()).To(Equal(0))
		_, ok := provider.VolumePrice("gp3", 20, 0, 0)
		Expect(ok).To(BeTrue())
	})
})
