/*
Copyright The Kubernetes Authors.

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

package cloudprovider

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

// TestRepairPolicyDefaults pins the AWS provider's default repair-policy set. RepairPolicies returns a static literal,
// so a zero-value receiver is sufficient — no cluster or provider wiring needed.
func TestRepairPolicyDefaults(t *testing.T) {
	policies := (&CloudProvider{}).RepairPolicies()

	for _, p := range policies {
		// Every repair policy MUST bound its drain so repair is never the unbounded (~19-day) NodeClaim
		// TerminationGracePeriod hang.
		if p.TerminationGracePeriod == nil {
			t.Errorf("policy %s/%s has a nil TerminationGracePeriod; repair drain must be bounded", p.ConditionType, p.ConditionStatus)
		}
	}

	// Ready=Unknown is a lost kubelet heartbeat: the drain can never make progress, so it must be forceful (0).
	assertTGP(t, policies, corev1.NodeReady, corev1.ConditionUnknown, 0)
	// A live-but-NotReady kubelet and the NMA conditions all get a bounded graceful drain.
	assertTGP(t, policies, corev1.NodeReady, corev1.ConditionFalse, 10*time.Minute)
	for _, cond := range []corev1.NodeConditionType{"AcceleratedHardwareReady", "StorageReady", "NetworkingReady", "KernelReady", "ContainerRuntimeReady"} {
		assertTGP(t, policies, cond, corev1.ConditionFalse, 10*time.Minute)
	}
}

func assertTGP(t *testing.T, policies []cloudprovider.RepairPolicy, cond corev1.NodeConditionType, status corev1.ConditionStatus, want time.Duration) {
	t.Helper()
	for _, p := range policies {
		if p.ConditionType == cond && p.ConditionStatus == status {
			if p.TerminationGracePeriod == nil {
				t.Errorf("policy %s/%s: expected TerminationGracePeriod %s, got nil", cond, status, want)
				return
			}
			if *p.TerminationGracePeriod != want {
				t.Errorf("policy %s/%s: expected TerminationGracePeriod %s, got %s", cond, status, want, *p.TerminationGracePeriod)
			}
			return
		}
	}
	t.Errorf("no repair policy found for %s/%s", cond, status)
}
