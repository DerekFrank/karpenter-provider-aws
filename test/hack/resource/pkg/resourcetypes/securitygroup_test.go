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
	"testing"

	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/samber/lo"
)

func TestEKSClusterSGCluster(t *testing.T) {
	sg := func(name, cluster string) ec2types.SecurityGroup {
		group := ec2types.SecurityGroup{GroupName: lo.ToPtr(name)}
		if cluster != "" {
			group.Tags = []ec2types.Tag{{Key: lo.ToPtr(eksClusterSGNameTag), Value: lo.ToPtr(cluster)}}
		}
		return group
	}
	for _, tc := range []struct {
		name   string
		sg     ec2types.SecurityGroup
		want   string
		wantOK bool
	}{
		{"cluster sg", sg("eks-cluster-sg-scale-41015412-1838923873", "scale-41015412"), "scale-41015412", true},
		{"no cluster tag", sg("eks-cluster-sg-scale-41015412-1838923873", ""), "", false},
		{"tag doesn't match name", sg("eks-cluster-sg-scale-41015412-1838923873", "scale-4101541"), "", false},
		{"other sg", sg("security-group-drift", "scale-41015412"), "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := eksClusterSGCluster(tc.sg)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("eksClusterSGCluster() = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
