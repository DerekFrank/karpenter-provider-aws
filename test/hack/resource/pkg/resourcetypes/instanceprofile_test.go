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

	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
)

func TestClusterInstanceProfiles(t *testing.T) {
	profiles := lo.Map([]string{"drift-1_123", "drift-1_456", "drift-12_789", "drift-1", "KarpenterNodeInstanceProfile-drift-1", "other_1"}, func(name string, _ int) iamtypes.InstanceProfile {
		return iamtypes.InstanceProfile{InstanceProfileName: lo.ToPtr(name)}
	})
	got := lo.Map(clusterInstanceProfiles(profiles, "drift-1"), func(p iamtypes.InstanceProfile, _ int) string { return lo.FromPtr(p.InstanceProfileName) })
	want := []string{"drift-1_123", "drift-1_456"}
	if !slices.Equal(got, want) {
		t.Errorf("clusterInstanceProfiles() = %v, want %v", got, want)
	}
}

func TestNodeRoleProfileCluster(t *testing.T) {
	karpenterTags := func(cluster, region string) []iamtypes.Tag {
		return []iamtypes.Tag{
			{Key: lo.ToPtr(karpenterClusterNameTag), Value: lo.ToPtr(cluster)},
			{Key: lo.ToPtr(v1.LabelTopologyRegion), Value: lo.ToPtr(region)},
			{Key: lo.ToPtr("kubernetes.io/cluster/" + cluster), Value: lo.ToPtr("owned")},
		}
	}
	profile := func(name string, roles ...string) iamtypes.InstanceProfile {
		return iamtypes.InstanceProfile{
			InstanceProfileName: lo.ToPtr(name),
			Roles:               lo.Map(roles, func(r string, _ int) iamtypes.Role { return iamtypes.Role{RoleName: lo.ToPtr(r)} }),
		}
	}
	for _, tc := range []struct {
		name    string
		profile iamtypes.InstanceProfile
		tags    []iamtypes.Tag
		want    string
		wantOK  bool
	}{
		{"holds node role", profile("termination-1_123", "KarpenterNodeRole-termination-1"), karpenterTags("termination-1", "us-east-2"), "termination-1", true},
		{"other region", profile("termination-1_123", "KarpenterNodeRole-termination-1"), karpenterTags("termination-1", "us-west-2"), "", false},
		{"no role", profile("termination-1_123"), karpenterTags("termination-1", "us-east-2"), "", false},
		{"other role", profile("termination-1_123", "SomeOtherRole"), karpenterTags("termination-1", "us-east-2"), "", false},
		{"name not from cluster", profile("custom-profile", "KarpenterNodeRole-termination-1"), karpenterTags("termination-1", "us-east-2"), "", false},
		{"no cluster tag", profile("termination-1_123", "KarpenterNodeRole-termination-1"), []iamtypes.Tag{{Key: lo.ToPtr(v1.LabelTopologyRegion), Value: lo.ToPtr("us-east-2")}}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := nodeRoleProfileCluster(tc.profile, tc.tags, "us-east-2")
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("nodeRoleProfileCluster() = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
