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
	"regexp"
	"testing"
	"time"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

// TestRepairPolicyDefaults pins the AWS provider's default repair-policy set. RepairPolicies/RepairTiming return static
// literals, so a zero-value receiver is sufficient — no cluster or provider wiring needed.
func TestRepairPolicyDefaults(t *testing.T) {
	c := &CloudProvider{}
	policies := c.RepairPolicies()

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

	timing := c.RepairTiming()
	// The AIMD durations MUST scale together at the production ratio (dwell:floor:ceiling ≈ 5:1:10, clawback ≈ 4×dwell).
	if timing.Dwell != 5*time.Minute || timing.CooldownFloor != 1*time.Minute ||
		timing.CooldownCeiling != 10*time.Minute || timing.ClawbackWindow != 20*time.Minute {
		t.Errorf("unexpected RepairTiming: %+v", timing)
	}
}

// TestAcceleratedHardwareReasonMatching verifies the reason-aware split on AcceleratedHardwareReady: known GPU XID
// families match their policies (10m confidence delay) while an unrecognized reason only matches the empty-matcher
// fallback (30m). Matching mirrors core's whole-string anchoring (^(?:pattern)$).
func TestAcceleratedHardwareReasonMatching(t *testing.T) {
	policies := (&CloudProvider{}).RepairPolicies()
	ahr := lo.Filter(policies, func(p cloudprovider.RepairPolicy, _ int) bool {
		return p.ConditionType == "AcceleratedHardwareReady" && p.ConditionStatus == corev1.ConditionFalse
	})

	// Exactly one condition-level fallback (empty ReasonMatcher), and it uses the longer 30m delay.
	fallbacks := lo.Filter(ahr, func(p cloudprovider.RepairPolicy, _ int) bool { return p.ReasonMatcher == "" })
	if len(fallbacks) != 1 {
		t.Fatalf("expected exactly one empty-ReasonMatcher fallback for AcceleratedHardwareReady, got %d", len(fallbacks))
	}
	if fallbacks[0].TolerationDuration != 30*time.Minute {
		t.Errorf("AcceleratedHardwareReady fallback toleration = %s, want 30m", fallbacks[0].TolerationDuration)
	}

	// matchedToleration returns the toleration of the most specific (non-empty matcher first) policy that matches the
	// reason, or -1 if only the fallback matches / nothing matches.
	matchedSpecific := func(reason string) (time.Duration, bool) {
		for _, p := range ahr {
			if p.ReasonMatcher == "" {
				continue
			}
			if regexp.MustCompile("^(?:" + p.ReasonMatcher + ")$").MatchString(reason) {
				return p.TolerationDuration, true
			}
		}
		return 0, false
	}

	for _, reason := range []string{"XID48", "GPU-XID140-fault", "XID64", "XID119"} {
		if tol, ok := matchedSpecific(reason); !ok {
			t.Errorf("reason %q matched no XID-family policy; it would only get the 30m fallback", reason)
		} else if tol != 10*time.Minute {
			t.Errorf("reason %q: XID-family toleration = %s, want 10m", reason, tol)
		}
	}
	// An unrecognized reason (and the healthy/DCGM advisory codes we intentionally don't enumerate) must NOT match a
	// fast XID policy — it falls through to the 30m fallback.
	for _, reason := range []string{"SomeUnknownReason", "DCGM_FI_DEV_XID_ERRORS", "XID13"} {
		if _, ok := matchedSpecific(reason); ok {
			t.Errorf("reason %q unexpectedly matched a fast XID-family policy", reason)
		}
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
