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

package resourcetypes

import (
	"slices"
	"testing"

	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/samber/lo"
)

func testBranchENI(id string, tags map[string]string) ec2types.NetworkInterface {
	return ec2types.NetworkInterface{
		NetworkInterfaceId: lo.ToPtr(id),
		TagSet: lo.MapToSlice(tags, func(k, v string) ec2types.Tag {
			return ec2types.Tag{Key: lo.ToPtr(k), Value: lo.ToPtr(v)}
		}),
	}
}

func TestBranchENIClusterName(t *testing.T) {
	for _, tc := range []struct {
		name      string
		tags      map[string]string
		want      string
		wantFound bool
		wantE2E   bool
	}{
		{"e2e cluster", map[string]string{eniOwnerTag: vpcResourceController, k8sClusterTagPrefix + "integration-283743684": "owned"}, "integration-283743684", true, true},
		{"soak cluster", map[string]string{k8sClusterTagPrefix + "soak-periodic-123": "owned"}, "soak-periodic-123", true, true},
		{"non e2e cluster", map[string]string{k8sClusterTagPrefix + "my-cluster": "owned"}, "my-cluster", true, false},
		{"kept cluster", map[string]string{k8sClusterTagPrefix + "stk-drift-1": "owned"}, "stk-drift-1", true, false},
		{"no cluster tag", map[string]string{k8sClusterTag: "integration-1"}, "", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, found := branchENIClusterName(testBranchENI("eni-1", tc.tags))
			if got != tc.want || found != tc.wantFound {
				t.Errorf("branchENIClusterName() = (%q, %v), want (%q, %v)", got, found, tc.want, tc.wantFound)
			}
			if found && isE2ECluster(got) != tc.wantE2E {
				t.Errorf("isE2ECluster(%q) = %v, want %v", got, !tc.wantE2E, tc.wantE2E)
			}
		})
	}
}

func TestOrphanedBranchENIs(t *testing.T) {
	enis := []ec2types.NetworkInterface{
		testBranchENI("eni-live", map[string]string{branchENIInstanceTag: "i-live"}),
		testBranchENI("eni-gone", map[string]string{branchENIInstanceTag: "i-gone"}),
		testBranchENI("eni-no-node", map[string]string{}),
	}
	got := orphanedBranchENIs(enis, map[string]struct{}{"i-live": {}})
	want := []string{"eni-gone"}
	if !slices.Equal(got, want) {
		t.Errorf("orphanedBranchENIs() = %v, want %v", got, want)
	}
}
