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
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/samber/lo"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
	"github.com/aws/karpenter-provider-aws/pkg/providers/ebspricing"
)

func TestOffering(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Offering")
}

var _ = Describe("Block Device Pricing", func() {
	var nodeClass *v1.EC2NodeClass
	var ebsPricingProvider *ebspricing.DefaultProvider

	BeforeEach(func() {
		nodeClass = &v1.EC2NodeClass{Spec: v1.EC2NodeClassSpec{
			AMISelectorTerms: []v1.AMISelectorTerm{{Alias: "al2023@latest"}},
		}}
		// Without pricing API responses, the provider uses its static us-east-1 pricing
		ebsPricingProvider = ebspricing.NewDefaultProvider(nil, "us-east-1", false)
	})

	It("should price block devices per instance type", func() {
		calls := map[ec2types.InstanceType]int{}
		// Size the root volume at 10GiB per vCPU
		resolve := func(_ context.Context, _ NodeClass, info ec2types.InstanceTypeInfo) ([]*v1.BlockDeviceMapping, error) {
			calls[info.InstanceType]++
			return []*v1.BlockDeviceMapping{{
				DeviceName: aws.String("/dev/xvda"),
				EBS:        &v1.BlockDevice{VolumeType: aws.String("gp3"), VolumeSize: lo.ToPtr(fmt.Sprintf("%dGi", 10*lo.FromPtr(info.VCpuInfo.DefaultVCpus)))},
			}}, nil
		}
		rc := &ResolveContext{blockDevicePricer: &blockDevicePricer{
			ctx:                context.Background(),
			nodeClass:          nodeClass,
			ebsPricingProvider: ebsPricingProvider,
			resolve:            resolve,
			prices:             map[ec2types.InstanceType]float64{},
		}}
		large := ec2types.InstanceTypeInfo{InstanceType: "m5.large", VCpuInfo: &ec2types.VCpuInfo{DefaultVCpus: aws.Int32(2)}}
		xlarge := ec2types.InstanceTypeInfo{InstanceType: "m5.xlarge", VCpuInfo: &ec2types.VCpuInfo{DefaultVCpus: aws.Int32(4)}}

		Expect(rc.BlockDeviceMappingsPrice(large)).To(BeNumerically("~", 20*0.08/ebspricing.HoursPerMonth, 1e-12))
		Expect(rc.BlockDeviceMappingsPrice(xlarge)).To(BeNumerically("~", 40*0.08/ebspricing.HoursPerMonth, 1e-12))

		// Each instance type's mappings are resolved once and shared by every resolver
		Expect(rc.BlockDeviceMappingsPrice(large)).To(BeNumerically("~", 20*0.08/ebspricing.HoursPerMonth, 1e-12))
		Expect(calls).To(Equal(map[ec2types.InstanceType]int{"m5.large": 1, "m5.xlarge": 1}))
	})
	It("should default to the NodeClass' block device mappings for every instance type", func() {
		nodeClass.Spec.BlockDeviceMappings = []*v1.BlockDeviceMapping{{
			DeviceName: aws.String("/dev/xvda"),
			EBS:        &v1.BlockDevice{VolumeType: aws.String("gp3"), VolumeSize: lo.ToPtr("100Gi")},
		}}
		Expect(lo.Must(DefaultBlockDeviceMappings(context.Background(), nodeClass, ec2types.InstanceTypeInfo{InstanceType: "m5.large"}))).To(Equal(nodeClass.Spec.BlockDeviceMappings))
		Expect(lo.Must(DefaultBlockDeviceMappings(context.Background(), nodeClass, ec2types.InstanceTypeInfo{InstanceType: "m5.24xlarge"}))).To(Equal(nodeClass.Spec.BlockDeviceMappings))
	})
	It("should fall back to the AMI family's default block device mappings", func() {
		bdms := lo.Must(DefaultBlockDeviceMappings(context.Background(), nodeClass, ec2types.InstanceTypeInfo{InstanceType: "m5.large"}))
		Expect(bdms).To(HaveLen(1))
		Expect(lo.FromPtr(bdms[0].EBS.VolumeType)).To(Equal("gp3"))
		Expect(lo.FromPtr(bdms[0].EBS.VolumeSize)).To(Equal("20Gi"))
	})
	It("should include the block device price in the offering cache key", func() {
		b := newCacheKeyBuilder(nodeClass, nil, nil, nil)
		it := &cloudprovider.InstanceType{Name: "m5.large", Requirements: scheduling.NewRequirements()}
		Expect(b.cacheKeyFromInstanceType(it, 0.1)).ToNot(Equal(b.cacheKeyFromInstanceType(it, 0.2)))
		Expect(b.cacheKeyFromInstanceType(it, 0.1)).To(Equal(b.cacheKeyFromInstanceType(it, 0.1)))
	})
})
