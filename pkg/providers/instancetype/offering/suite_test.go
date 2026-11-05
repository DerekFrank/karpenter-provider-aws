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

package offering_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	arcsdk "github.com/aws/aws-sdk-go-v2/service/arczonalshift"
	arctypes "github.com/aws/aws-sdk-go-v2/service/arczonalshift/types"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	clock "k8s.io/utils/clock/testing"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	coreoptions "sigs.k8s.io/karpenter/pkg/operator/options"
	"sigs.k8s.io/karpenter/pkg/scheduling"
	coretest "sigs.k8s.io/karpenter/pkg/test"
	. "sigs.k8s.io/karpenter/pkg/utils/testing"

	v1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
	awscache "github.com/aws/karpenter-provider-aws/pkg/cache"
	"github.com/aws/karpenter-provider-aws/pkg/fake"
	"github.com/aws/karpenter-provider-aws/pkg/providers/arczonalshift"
	"github.com/aws/karpenter-provider-aws/pkg/providers/instancetype/offering"
)

const (
	zoneA   = "test-zone-1a"
	zoneB   = "test-zone-1b"
	zoneIDA = "tstz1-1a"
	zoneIDB = "tstz1-1b"
)

var ctx context.Context

func TestOffering(t *testing.T) {
	ctx = TestContextWithLogger(t)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Offering")
}

type stubPricing struct{}

func (stubPricing) LivenessProbe(*http.Request) error                       { return nil }
func (stubPricing) InstanceTypes() []ec2types.InstanceType                  { return nil }
func (stubPricing) OnDemandPrice(ec2types.InstanceType) (float64, bool)     { return 1, true }
func (stubPricing) SpotPrice(ec2types.InstanceType, string) (float64, bool) { return 0.5, true }
func (stubPricing) UpdateOnDemandPricing(context.Context) error             { return nil }
func (stubPricing) UpdateSpotPricing(context.Context) error                 { return nil }

// updateAfterSnapshot runs onSnapshot once, right after ShiftedZones returns, to model the zonal shift
// controller updating shifts while InjectOfferings is resolving offerings.
type updateAfterSnapshot struct {
	*arczonalshift.DefaultProvider
	onSnapshot func()
}

func (u *updateAfterSnapshot) ShiftedZones(ctx context.Context) sets.Set[string] {
	s := u.DefaultProvider.ShiftedZones(ctx)
	if u.onSnapshot != nil {
		u.onSnapshot()
		u.onSnapshot = nil
	}
	return s
}

var _ = Describe("Zonal Shift", func() {
	var arcAPI *fake.ARCZonalShiftAPI
	var fakeClock *clock.FakeClock
	var zonalShiftProvider *updateAfterSnapshot
	var offeringProvider *offering.DefaultProvider
	var nodeClass *v1.EC2NodeClass
	var instanceTypes []*cloudprovider.InstanceType

	setShiftedZones := func(zoneIDs ...string) {
		arcAPI.GetManagedResourceBehavior.Output.Set(&arcsdk.GetManagedResourceOutput{
			ZonalShifts: lo.Map(zoneIDs, func(id string, _ int) arctypes.ZonalShiftInResource {
				return arctypes.ZonalShiftInResource{
					AwayFrom:      aws.String(id),
					ExpiryTime:    aws.Time(fakeClock.Now().Add(time.Hour)),
					AppliedStatus: arctypes.AppliedStatusApplied,
				}
			}),
		})
	}
	isZoneBAvailable := func() bool {
		its := offeringProvider.InjectOfferings(ctx, instanceTypes, map[ec2types.InstanceType]ec2types.InstanceTypeInfo{}, nodeClass, sets.New(zoneA, zoneB))
		o, ok := lo.Find(its[0].Offerings, func(o *cloudprovider.Offering) bool { return o.Zone() == zoneB })
		Expect(ok).To(BeTrue())
		return o.Available
	}

	BeforeEach(func() {
		ctx = coreoptions.ToContext(ctx, coretest.Options())
		arcAPI = fake.NewARCZonalShiftAPI()
		fakeClock = clock.NewFakeClock(time.Now())
		zonalShiftProvider = &updateAfterSnapshot{DefaultProvider: arczonalshift.NewProvider(arcAPI, fakeClock, "test-cluster-arn")}
		offeringProvider = offering.NewDefaultProvider(
			stubPricing{},
			nil,
			nil,
			awscache.NewUnavailableOfferings(),
			cache.New(awscache.InstanceTypesZonesAndOfferingsTTL, awscache.DefaultCleanupInterval),
			zonalShiftProvider,
			nil,
		)
		nodeClass = &v1.EC2NodeClass{Status: v1.EC2NodeClassStatus{Subnets: []v1.Subnet{
			{ID: "subnet-a", Zone: zoneA, ZoneID: zoneIDA},
			{ID: "subnet-b", Zone: zoneB, ZoneID: zoneIDB},
		}}}
		instanceTypes = []*cloudprovider.InstanceType{{
			Name: string(ec2types.InstanceTypeM5Large),
			Requirements: scheduling.NewRequirements(
				scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zoneA, zoneB),
				scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, karpv1.CapacityTypeOnDemand),
			),
		}}
	})

	It("should make a zone available as soon as a shift that was active before hydration ends", func() {
		// The zone is shifted when the controller starts, before the zonal shift provider has hydrated.
		setShiftedZones(zoneIDB)
		Expect(isZoneBAvailable()).To(BeFalse())

		setShiftedZones()
		Expect(zonalShiftProvider.UpdateZonalShifts(ctx)).To(Succeed())
		Expect(isZoneBAvailable()).To(BeTrue())
	})
	It("should mark a zone unavailable when it is shifted again after a shift ended mid-resolution", func() {
		setShiftedZones(zoneIDB)
		Expect(zonalShiftProvider.UpdateZonalShifts(ctx)).To(Succeed())
		zonalShiftProvider.onSnapshot = func() {
			setShiftedZones()
			Expect(zonalShiftProvider.UpdateZonalShifts(ctx)).To(Succeed())
		}
		Expect(isZoneBAvailable()).To(BeFalse())

		// The same zone is shifted away from again before the offering cache entry expires.
		setShiftedZones(zoneIDB)
		Expect(zonalShiftProvider.UpdateZonalShifts(ctx)).To(Succeed())
		Expect(isZoneBAvailable()).To(BeFalse())
	})
})
