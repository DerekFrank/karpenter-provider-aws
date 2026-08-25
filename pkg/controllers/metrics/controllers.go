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

import "sigs.k8s.io/karpenter/pkg/metrics"

// Provider controller names, centralized as first-class metrics.Value vars (see
// the core pkg/metrics/controllers.go for the rationale). ControllerValues is
// unioned with core's for the `controller` dimension's value set.
var (
	NodeClassController = metrics.Value{Name: "nodeclass", Help: "Resolves the subnets, security groups, AMIs, and instance profile an EC2NodeClass selects."}
)

// ControllerValues enumerates the provider controllers for the `controller` dimension.
var ControllerValues = []metrics.Value{
	NodeClassController,
}
