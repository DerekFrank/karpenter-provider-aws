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

package metrics

import (
	v1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/metrics"
)

// ConditionTypeValues documents the status condition types the EC2NodeClass sets.
// It is the `type` dimension's value set on the ec2nodeclass_status_condition_*
// metrics, and a cheat-sheet of which conditions the provider reports and why.
// Keys are the object Kind (matching status.NewController[T]); every value's Name
// references a ConditionType* const so the set cannot drift from the API.
var ConditionTypeValues = map[string][]metrics.Value{
	"EC2NodeClass": {
		{Name: v1.ConditionTypeSubnetsReady, Help: "The selected subnets were resolved."},
		{Name: v1.ConditionTypeSecurityGroupsReady, Help: "The selected security groups were resolved."},
		{Name: v1.ConditionTypeAMIsReady, Help: "The selected AMIs were resolved."},
		{Name: v1.ConditionTypeInstanceProfileReady, Help: "The instance profile was resolved or created."},
		{Name: v1.ConditionTypeCapacityReservationsReady, Help: "The selected capacity reservations were resolved."},
		{Name: v1.ConditionTypeValidationSucceeded, Help: "The EC2NodeClass configuration passed validation."},
		{Name: v1.ConditionTypePlacementGroupReady, Help: "The selected placement group was resolved."},
	},
}
