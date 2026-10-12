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
	"time"

	cloudformationtypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/samber/lo"
)

func testStack(name string, status cloudformationtypes.StackStatus, created time.Time, tags map[string]string) cloudformationtypes.Stack {
	return cloudformationtypes.Stack{
		StackName:    lo.ToPtr(name),
		StackStatus:  status,
		CreationTime: lo.ToPtr(created),
		Tags: lo.MapToSlice(tags, func(k, v string) cloudformationtypes.Tag {
			return cloudformationtypes.Tag{Key: lo.ToPtr(k), Value: lo.ToPtr(v)}
		}),
	}
}

func TestExpiredStacks(t *testing.T) {
	now := time.Now()
	expiration := now.Add(-12 * time.Hour)
	old := now.Add(-24 * time.Hour)
	recent := now.Add(-1 * time.Hour)
	stacks := []cloudformationtypes.Stack{
		testStack("tagged-old", cloudformationtypes.StackStatusCreateComplete, old, map[string]string{karpenterTestingTag: "drift-1"}),
		testStack("run-url-old", cloudformationtypes.StackStatusCreateComplete, old, map[string]string{githubRunURLTag: "https://github.com/x"}),
		testStack("delete-failed-old", cloudformationtypes.StackStatusDeleteFailed, old, map[string]string{karpenterTestingTag: "drift-2"}),
		testStack("tagged-recent", cloudformationtypes.StackStatusCreateComplete, recent, map[string]string{karpenterTestingTag: "drift-3"}),
		testStack("deleting", cloudformationtypes.StackStatusDeleteInProgress, old, map[string]string{karpenterTestingTag: "drift-4"}),
		testStack("excluded", cloudformationtypes.StackStatusCreateComplete, old, map[string]string{karpenterTestingTag: "keep-1"}),
		testStack("untagged", cloudformationtypes.StackStatusCreateComplete, old, nil),
		testStack("iam-storage-1", cloudformationtypes.StackStatusReviewInProgress, old, nil),
		testStack("iam-storage-2", cloudformationtypes.StackStatusReviewInProgress, recent, nil),
		testStack("iam-keep-1", cloudformationtypes.StackStatusReviewInProgress, old, nil),
		testStack("other-review", cloudformationtypes.StackStatusReviewInProgress, old, nil),
	}
	got := expiredStacks(stacks, expiration, []string{"keep-1"})
	want := []string{"tagged-old", "run-url-old", "delete-failed-old", "iam-storage-1"}
	if !slices.Equal(got, want) {
		t.Errorf("expiredStacks() = %v, want %v", got, want)
	}
}

func TestClusterStacks(t *testing.T) {
	now := time.Now()
	stacks := []cloudformationtypes.Stack{
		testStack("eksctl-drift-1-cluster", cloudformationtypes.StackStatusDeleteFailed, now, map[string]string{karpenterTestingTag: "drift-1"}),
		testStack("iam-drift-1", cloudformationtypes.StackStatusReviewInProgress, now, nil),
		testStack("iam-drift-12", cloudformationtypes.StackStatusReviewInProgress, now, nil),
		testStack("eksctl-drift-12-cluster", cloudformationtypes.StackStatusCreateComplete, now, map[string]string{karpenterTestingTag: "drift-12"}),
		testStack("iam-drift-1-untagged", cloudformationtypes.StackStatusCreateComplete, now, nil),
	}
	got := clusterStacks(stacks, "drift-1")
	want := []string{"eksctl-drift-1-cluster", "iam-drift-1"}
	if !slices.Equal(got, want) {
		t.Errorf("clusterStacks() = %v, want %v", got, want)
	}
}

func TestE2EStackCluster(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name   string
		stack  cloudformationtypes.Stack
		want   string
		wantOK bool
	}{
		{"eksctl nodegroup", testStack("eksctl-integration-1005423756-nodegroup-integration-1005423756-system-pool", cloudformationtypes.StackStatusDeleteFailed, now, map[string]string{karpenterTestingTag: "integration-1005423756"}), "integration-1005423756", true},
		{"iam stack", testStack("iam-drift-123", cloudformationtypes.StackStatusDeleteFailed, now, map[string]string{karpenterTestingTag: "drift-123"}), "drift-123", true},
		{"untagged iam stack", testStack("iam-drift-123", cloudformationtypes.StackStatusDeleteFailed, now, nil), "drift-123", true},
		{"kept cluster", testStack("iam-stk-drift-1", cloudformationtypes.StackStatusDeleteFailed, now, map[string]string{karpenterTestingTag: "stk-drift-1"}), "stk-drift-1", false},
		{"tag doesn't match name", testStack("some-other-stack", cloudformationtypes.StackStatusDeleteFailed, now, map[string]string{karpenterTestingTag: "drift-123"}), "", false},
		{"non e2e cluster", testStack("eksctl-my-cluster-cluster", cloudformationtypes.StackStatusDeleteFailed, now, map[string]string{karpenterTestingTag: "my-cluster"}), "my-cluster", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := e2eStackCluster(tc.stack)
			if ok != tc.wantOK || (ok && got != tc.want) {
				t.Errorf("e2eStackCluster() = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
