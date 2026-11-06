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

package offering

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
	"github.com/aws/karpenter-provider-aws/pkg/providers/pricing"
)

func TestOffering(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Offering")
}

var _ = Describe("Block Device Pricing", func() {
	var nodeClass *v1.EC2NodeClass
	var pricingProvider *pricing.DefaultProvider

	BeforeEach(func() {
		nodeClass = &v1.EC2NodeClass{Spec: v1.EC2NodeClassSpec{
			AMISelectorTerms: []v1.AMISelectorTerm{{Alias: "al2023@latest"}},
		}}
		// Without pricing API responses, the provider uses its static us-east-1 pricing
		pricingProvider = pricing.NewDefaultProvider(nil, nil, "us-east-1", false)
	})

	It("should price block devices per instance type", func() {
		calls := map[ec2types.InstanceType]int{}
		// Size the root volume at 10GiB per vCPU
		resolve := func(_ NodeClass, info ec2types.InstanceTypeInfo) []*v1.BlockDeviceMapping {
			calls[info.InstanceType]++
			size := resource.MustParse("10Gi")
			size.Mul(int64(lo.FromPtr(info.VCpuInfo.DefaultVCpus)))
			return []*v1.BlockDeviceMapping{{
				DeviceName: aws.String("/dev/xvda"),
				EBS:        &v1.BlockDevice{VolumeType: aws.String("gp3"), VolumeSize: &size},
			}}
		}
		rc := &ResolveContext{blockDevicePricer: &blockDevicePricer{
			nodeClass:       nodeClass,
			pricingProvider: pricingProvider,
			resolve:         resolve,
			prices:          map[ec2types.InstanceType]float64{},
		}}
		large := ec2types.InstanceTypeInfo{InstanceType: "m5.large", VCpuInfo: &ec2types.VCpuInfo{DefaultVCpus: aws.Int32(2)}}
		xlarge := ec2types.InstanceTypeInfo{InstanceType: "m5.xlarge", VCpuInfo: &ec2types.VCpuInfo{DefaultVCpus: aws.Int32(4)}}

		Expect(rc.BlockDeviceMappingsPrice(large)).To(BeNumerically("~", 20*0.08/pricing.HoursPerMonth, 1e-12))
		Expect(rc.BlockDeviceMappingsPrice(xlarge)).To(BeNumerically("~", 40*0.08/pricing.HoursPerMonth, 1e-12))

		// Each instance type's mappings are resolved once and shared by every resolver
		Expect(rc.BlockDeviceMappingsPrice(large)).To(BeNumerically("~", 20*0.08/pricing.HoursPerMonth, 1e-12))
		Expect(calls).To(Equal(map[ec2types.InstanceType]int{"m5.large": 1, "m5.xlarge": 1}))
	})
	It("should default to the NodeClass' block device mappings for every instance type", func() {
		size := resource.MustParse("100Gi")
		nodeClass.Spec.BlockDeviceMappings = []*v1.BlockDeviceMapping{{
			DeviceName: aws.String("/dev/xvda"),
			EBS:        &v1.BlockDevice{VolumeType: aws.String("gp3"), VolumeSize: &size},
		}}
		Expect(DefaultBlockDeviceMappings(nodeClass, ec2types.InstanceTypeInfo{InstanceType: "m5.large"})).To(Equal(nodeClass.Spec.BlockDeviceMappings))
		Expect(DefaultBlockDeviceMappings(nodeClass, ec2types.InstanceTypeInfo{InstanceType: "m5.24xlarge"})).To(Equal(nodeClass.Spec.BlockDeviceMappings))
	})
	It("should fall back to the AMI family's default block device mappings", func() {
		bdms := DefaultBlockDeviceMappings(nodeClass, ec2types.InstanceTypeInfo{InstanceType: "m5.large"})
		Expect(bdms).To(HaveLen(1))
		Expect(lo.FromPtr(bdms[0].EBS.VolumeType)).To(Equal("gp3"))
		Expect(bdms[0].EBS.VolumeSize.String()).To(Equal("20Gi"))
	})
	It("should include the block device price in the offering cache key", func() {
		b := newCacheKeyBuilder(nodeClass, nil, nil, nil)
		it := &cloudprovider.InstanceType{Name: "m5.large", Requirements: scheduling.NewRequirements()}
		Expect(b.cacheKeyFromInstanceType(it, 0.1)).ToNot(Equal(b.cacheKeyFromInstanceType(it, 0.2)))
		Expect(b.cacheKeyFromInstanceType(it, 0.1)).To(Equal(b.cacheKeyFromInstanceType(it, 0.1)))
	})
})
