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

package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/awslabs/operatorpkg/serrors"
	"github.com/samber/lo"

	"sigs.k8s.io/karpenter/pkg/metrics"
)

type metricInfo struct {
	namespace string
	subsystem string
	name      string
	help      string
	labels    []string
	// labelScope, when non-empty, selects a scoped label registry to resolve this
	// metric's dimensions before falling back to the global registry. It
	// disambiguates dimension names reused with different meanings across code
	// bases (e.g. `reason` means a karpenter action reason in karpenter metrics
	// but a status-condition reason in operatorpkg metrics).
	labelScope string
	// labelValues overrides, per dimension name, the set of documented values for
	// THIS metric — used when a dimension's value set is metric-specific rather than
	// global (e.g. the `type` dimension of operator_<kind>_status_condition_* metrics,
	// whose values are that object's status condition types).
	labelValues map[string][]valueInfo
}

var (
	// stringSymbols maps a package-level string constant/variable name to its value
	// (e.g. ReasonLabel -> "reason"), used to resolve metric label names that are
	// declared as named constants rather than string literals.
	stringSymbols = map[string]string{}
	// sliceSymbols maps a package-level []string constant/variable name to its resolved
	// values (e.g. the aws-sdk-go-prometheus `labels` var -> {"service","action","code"}).
	sliceSymbols = map[string][]string{}
	// valueSliceSymbols maps a package-level []Value constant/variable name to its
	// resolved values (e.g. operatorpkg's conditionStatusValues), so a Label whose
	// Values field references a shared var resolves like an inline literal.
	valueSliceSymbols = map[string][]valueInfo{}
	// valueSymbols maps a package-level Value variable name to its resolved value, so
	// a []Value literal may reference first-class Value vars by name instead of
	// repeating an inline {Name, Help} literal.
	valueSymbols = map[string]valueInfo{}
	// conditionTypesByKind maps an object Kind (e.g. "NodeClaim") to the status
	// condition types it sets, sourced from karpenter's metrics.ConditionTypeValues
	// map. It is the per-object value set of the `type` dimension on that object's
	// status-condition metrics.
	conditionTypesByKind = map[string][]valueInfo{}
	// controllerValues is the union of the ControllerValues registries (core +
	// provider) — the documented values of the `controller` dimension. The
	// operatorpkg status/events controllers are added from their registration sites.
	controllerValues []valueInfo
	// ambiguous names resolved to conflicting values across packages; treated as unresolvable.
	ambiguousStrings = map[string]bool{}
	ambiguousSlices  = map[string]bool{}

	// labelRegistry maps a resolved label name (dimension key) to its documentation,
	// sourced from metrics.Label{...} var declarations across the scanned packages
	// (both karpenter core and karpenter-provider-aws). It is the source of truth for
	// per-dimension help text and stable values.
	labelRegistry = map[string]labelInfo{}
	// scopedLabelRegistry holds the same Label documentation keyed first by a scope
	// (the code base a Label was declared in, e.g. "operatorpkg") and then by name.
	// A metric with a matching labelScope resolves its dimensions here first, so a
	// name reused with a different meaning across code bases (e.g. `reason`) gets
	// the documentation from its own code base rather than whichever was scanned
	// first into the global registry.
	scopedLabelRegistry = map[string]map[string]labelInfo{}
)

// labelScopeForFile returns the scope a Label declaration belongs to, based on
// the file it was declared in. operatorpkg-declared Labels document the
// dimensions of operatorpkg's status/termination/event metrics. Matching an
// "operatorpkg" path segment (rather than any substring) avoids misattributing
// files that merely sit under a checkout/branch dir whose name contains it.
func labelScopeForFile(file string) string {
	if strings.Contains(file, "/operatorpkg/") || strings.Contains(file, "/operatorpkg@") {
		return "operatorpkg"
	}
	return ""
}

// labelInfo is the resolved documentation for a metric dimension.
type labelInfo struct {
	help   string
	values []valueInfo
}

// valueInfo is the resolved documentation for a single dimension value.
type valueInfo struct {
	name string
	help string
}

// labelInjections supplies documentation for dimensions of THIRD-PARTY metrics
// whose labels cannot be described with a metrics.Label in karpenter code
// (aws-sdk-go-prometheus, controller-runtime, client-go). It is keyed by
// subsystem first so that a name reused with different meanings across subsystems
// (e.g. `code` is an HTTP status here but a Kubernetes eviction response elsewhere)
// resolves correctly. All values are hardcoded, well-known constants for the
// respective third-party library.
var labelInjections = map[string]map[string]labelInfo{
	"aws_sdk_go": {
		"service": {help: "The AWS service the request was made to, e.g. `EC2`."},
		"action":  {help: "The AWS API operation invoked, e.g. `DescribeSubnets`."},
		"code":    {help: "The HTTP status code of the response, e.g. `200`, `503`."},
	},
	"controller_runtime": {
		"controller": {help: "The name of the controller that owns the reconcile loop."},
		"name":       {help: "The name of the controller instance."},
		"result":     {help: "The outcome of the reconcile call.", values: []valueInfo{{name: "success"}, {name: "error"}, {name: "requeue"}, {name: "requeue_after"}}},
	},
	"client_go": {
		"verb":   {help: "The HTTP verb of the Kubernetes API request, e.g. `GET`, `POST`."},
		"code":   {help: "The HTTP status code of the Kubernetes API response."},
		"method": {help: "The HTTP method of the Kubernetes API request."},
		"host":   {help: "The Kubernetes API server host the request was made to."},
	},
}

// describeLabel resolves documentation for a metric's dimension, preferring a
// subsystem-scoped third-party injection, then a code-base-scoped label (when the
// metric declares a labelScope), then the global code-sourced label registry.
func describeLabel(subsystem, name, scope string) (labelInfo, bool) {
	if inj, ok := labelInjections[subsystem]; ok {
		if li, ok := inj[name]; ok {
			return li, true
		}
	}
	if scope != "" {
		if scoped, ok := scopedLabelRegistry[scope]; ok {
			if li, ok := scoped[name]; ok {
				return li, true
			}
		}
	}
	if li, ok := labelRegistry[name]; ok {
		return li, true
	}
	return labelInfo{}, false
}

var (
	stableMetrics = []string{"controller_runtime", "aws_sdk_go", "client_go", "leader_election", "interruption", "cluster_state", "workqueue", "karpenter_build_info", "karpenter_nodepool_usage", "karpenter_nodepool_limit",
		"karpenter_nodeclaims_terminated_total", "karpenter_nodeclaims_created_total", "karpenter_nodes_terminated_total", "karpenter_nodes_created_total", "karpenter_pods_startup_duration_seconds",
		"karpenter_scheduler_scheduling_duration_seconds", "karpenter_provisioner_scheduling_duration_seconds", "karpenter_nodepool_allowed_disruptions", "karpenter_voluntary_disruption_decisions_total"}
	betaMetrics = []string{"cloudprovider", "cloudprovider_batcher", "karpenter_nodeclaims_termination_duration_seconds", "karpenter_nodeclaims_instance_termination_duration_seconds",
		"karpenter_nodes_total_pod_requests", "karpenter_nodes_total_pod_limits", "karpenter_nodes_total_daemon_requests", "karpenter_nodes_total_daemon_limits", "karpenter_nodes_termination_duration_seconds",
		"karpenter_nodes_system_overhead", "karpenter_nodes_allocatable", "karpenter_pods_state", "karpenter_scheduler_queue_depth", "karpenter_voluntary_disruption_queue_failures_total",
		"karpenter_voluntary_disruption_decision_evaluation_duration_seconds", "karpenter_voluntary_disruption_eligible_nodes", "karpenter_voluntary_disruption_consolidation_timeouts_total",
		// Per-object status condition and termination metrics from operatorpkg
		"nodeclaim_status_condition", "nodeclaim_termination",
		"nodepool_status_condition", "nodepool_termination",
		"ec2nodeclass_status_condition", "ec2nodeclass_termination"}
	// Deprecated generic status condition and termination metrics (without object name prefix).
	// These are still emitted at runtime but are superseded by per-object variants.
	deprecatedMetrics = []string{"status_condition", "termination"}
)

func (i metricInfo) qualifiedName() string {
	return strings.Join(lo.Compact([]string{i.namespace, i.subsystem, i.name}), "_")
}

// metrics_gen_docs is used to parse the source code for Prometheus metrics and automatically generate markdown documentation
// based on the naming and help provided in the source code.

func main() {
	flag.Parse()
	if flag.NArg() < 2 {
		log.Fatalf("Usage: %s path/to/metrics/controller path/to/metrics/controller2 path/to/markdown.md", os.Args[0])
	}
	var allPackages []*ast.Package
	for i := 0; i < flag.NArg()-1; i++ {
		allPackages = append(allPackages, getPackages(flag.Arg(i))...)
	}
	// Build symbol tables for string and []string constants/variables across all
	// packages so we can resolve metric label names declared as named identifiers.
	collectSymbols(allPackages)
	// Build the label documentation registry from metrics.Label{...} declarations
	// (must run after collectSymbols so Name/Values identifiers resolve).
	collectLabels(allPackages)
	allMetrics := getMetricsFromPackages(allPackages...)

	// operatorpkg's status controller creates per-object metrics at runtime from the
	// type parameter of status.NewController[T](); they can't be read from a metric
	// declaration, so synthesize them from the parsed registration sites (plus the
	// deprecated generic variants and the unparseable client_go metrics).
	statusObjects := parseStatusControllerObjects(allPackages)
	// Finalize the `controller` dimension values (registries + operatorpkg controllers).
	attachControllerValues(statusObjects)
	allMetrics = append(allMetrics, perObjectStatusMetrics(statusObjects)...)
	allMetrics = append(allMetrics, deprecatedStatusMetrics(statusObjects)...)
	allMetrics = append(allMetrics, hardcodedMetrics()...)

	// Dedupe metrics
	allMetrics = lo.UniqBy(allMetrics, func(m metricInfo) string {
		return fmt.Sprintf("%s/%s/%s", m.namespace, m.subsystem, m.name)
	})

	// Drop some metrics
	for _, subsystem := range []string{"rest_client", "certwatcher_read", "controller_runtime_webhook"} {
		allMetrics = lo.Reject(allMetrics, func(m metricInfo, _ int) bool {
			return strings.HasPrefix(m.name, subsystem)
		})
	}

	// Controller Runtime and AWS SDK Go for Prometheus naming is different in that they don't specify a namespace or subsystem
	// Getting the metrics requires special parsing logic
	for _, subsystem := range []string{"controller_runtime", "aws_sdk_go", "client_go", "leader_election"} {
		for i := range allMetrics {
			if allMetrics[i].subsystem == "" && strings.HasPrefix(allMetrics[i].name, fmt.Sprintf("%s_", subsystem)) {
				allMetrics[i].subsystem = subsystem
				allMetrics[i].name = strings.TrimPrefix(allMetrics[i].name, fmt.Sprintf("%s_", subsystem))
			}
		}
	}
	sort.Slice(allMetrics, bySubsystem(allMetrics))

	// Sanity check: fail loudly if the metric count drops below expected.
	// This catches silent regressions where new identifier mappings are needed
	// or metric declaration patterns change. Update this threshold when metrics
	// are intentionally removed.
	const minExpectedMetrics = 100
	if len(allMetrics) < minExpectedMetrics {
		log.Fatalf("expected at least %d metrics but only found %d; the generator may be silently dropping metrics due to unrecognized identifiers or new declaration patterns", minExpectedMetrics, len(allMetrics))
	}

	outputFileName := flag.Arg(flag.NArg() - 1)
	f, err := os.Create(outputFileName)
	if err != nil {
		log.Fatalf("error creating output file %s, %s", outputFileName, err)
	}

	log.Println("writing output to", outputFileName)
	fmt.Fprintf(f, `---
title: "Metrics"
linkTitle: "Metrics"
weight: 7

description: >
  Inspect Karpenter Metrics
---
`)
	fmt.Fprintf(f, "<!-- this document is generated from hack/docs/metrics_gen/main.go -->\n")
	fmt.Fprintf(f, "Karpenter makes several metrics available in Prometheus format to allow monitoring cluster provisioning status. "+
		"These metrics are available by default at `karpenter.kube-system.svc.cluster.local:8080/metrics` configurable via the `METRICS_PORT` environment variable documented [here](../settings)\n")
	previousSubsystem := ""

	for _, metric := range allMetrics {
		if metric.subsystem != previousSubsystem {
			if metric.subsystem != "" {
				subsystemTitle := strings.Join(lo.Map(strings.Split(metric.subsystem, "_"), func(s string, _ int) string {
					if s == "sdk" || s == "aws" {
						return strings.ToUpper(s)
					} else {
						return fmt.Sprintf("%s%s", strings.ToUpper(s[0:1]), s[1:])
					}
				}), " ")
				fmt.Fprintf(f, "## %s Metrics\n", subsystemTitle)
				fmt.Fprintln(f)
			}
			previousSubsystem = metric.subsystem
		}
		fmt.Fprintf(f, "### `%s`\n", metric.qualifiedName())
		fmt.Fprintf(f, "%s\n", metric.help)
		switch {
		case slices.Contains(deprecatedMetrics, metric.subsystem) || slices.Contains(deprecatedMetrics, metric.qualifiedName()):
			fmt.Fprintf(f, "- Stability Level: %s\n", "DEPRECATED")
		case slices.Contains(stableMetrics, metric.subsystem) || slices.Contains(stableMetrics, metric.qualifiedName()):
			fmt.Fprintf(f, "- Stability Level: %s\n", "STABLE")
		case slices.Contains(betaMetrics, metric.subsystem) || slices.Contains(betaMetrics, metric.qualifiedName()):
			fmt.Fprintf(f, "- Stability Level: %s\n", "BETA")
		default:
			fmt.Fprintf(f, "- Stability Level: %s\n", "ALPHA")
		}
		if dims := formatDimensions(metric); dims != "" {
			fmt.Fprintf(f, "- Dimensions:%s\n", dims)
		}
		fmt.Fprintln(f)
	}

}

// formatDimensions renders a metric's dimensions as a markdown sub-list. Each
// dimension carries its help text (sourced from the code-defined metrics.Label or
// a third-party injection) and, where the set of values is known and stable, the
// list of values. The returned string begins with a leading newline so it renders
// as a nested list under the "- Dimensions:" line.
func formatDimensions(m metricInfo) string {
	if len(m.labels) == 0 {
		return ""
	}
	var b strings.Builder
	for _, l := range m.labels {
		b.WriteString(fmt.Sprintf("\n  - `%s`", l))
		info, ok := describeLabel(m.subsystem, l, m.labelScope)
		if ok && info.help != "" {
			b.WriteString(fmt.Sprintf(" — %s", info.help))
		}
		// A metric-specific value set (e.g. per-object condition types) overrides the
		// dimension's global values.
		values := info.values
		if override, ok := m.labelValues[l]; ok {
			values = override
		}
		// Render each documented value as a nested sub-list item, with its own help
		// where available.
		for _, v := range values {
			b.WriteString(fmt.Sprintf("\n    - `%s`", v.name))
			if v.help != "" {
				b.WriteString(fmt.Sprintf(" — %s", v.help))
			}
		}
	}
	return b.String()
}

func getPackages(root string) []*ast.Package {
	var packages []*ast.Package
	fset := token.NewFileSet()

	// walk our metrics controller directory
	log.Println("parsing code in", root)
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if d == nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		// parse the packagers that we find
		pkgs, err := parser.ParseDir(fset, path, func(info fs.FileInfo) bool {
			return !strings.HasSuffix(info.Name(), "_test.go")
		}, parser.AllErrors)
		if err != nil {
			log.Fatalf("error parsing, %s", err)
		}
		for _, pkg := range pkgs {
			if strings.HasSuffix(pkg.Name, "_test") {
				continue
			}
			packages = append(packages, pkg)
		}
		return nil
	})
	return packages
}

func getMetricsFromPackages(packages ...*ast.Package) []metricInfo {
	var allMetrics []metricInfo
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				ce, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if m, ok := metricFromCallExpr(ce); ok {
					allMetrics = append(allMetrics, m)
				}
				return true
			})
		}
	}
	return allMetrics
}

func bySubsystem(metrics []metricInfo) func(i int, j int) bool {
	// Higher ordering comes first. If a value isn't designated here then the subsystem will be given a default of 0.
	// Metrics without a subsystem come first since there is no designation for the bucket they fall under
	subSystemSortOrder := map[string]int{
		"":                              100,
		"nodepool":                      10,
		"nodeclaims":                    9,
		"nodeclaim_status_condition":    8,
		"nodeclaim_termination":         8,
		"nodes":                         7,
		"node_status_condition":         6,
		"node_termination":              6,
		"pods":                          5,
		"nodepool_status_condition":     4,
		"nodepool_termination":          4,
		"ec2nodeclass_status_condition": 3,
		"ec2nodeclass_termination":      3,
		"status_condition":              -1,
		"termination":                   -1,
		"workqueue":                     -1,
		"client_go":                     -1,
		"aws_sdk_go":                    -1,
		"leader_election":               -2,
	}

	return func(i, j int) bool {
		lhs := metrics[i]
		rhs := metrics[j]
		if subSystemSortOrder[lhs.subsystem] != subSystemSortOrder[rhs.subsystem] {
			return subSystemSortOrder[lhs.subsystem] > subSystemSortOrder[rhs.subsystem]
		}
		return lhs.qualifiedName() > rhs.qualifiedName()
	}
}

// statusObject is a type that has a status controller registered via
// status.NewController[T](), parsed from the registration sites.
type statusObject struct {
	kind      string // the object Kind, e.g. "NodeClaim"
	subsystem string // the metric subsystem prefix, e.g. "nodeclaim"
}

// parseStatusControllerObjects finds status.NewController[T]() registrations across
// the scanned packages and returns the object types they register. This is the
// authoritative, self-maintaining source for which per-object status/termination
// metrics exist (replacing a hardcoded list).
func parseStatusControllerObjects(packages []*ast.Package) []statusObject {
	seen := map[string]statusObject{}
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				ce, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				// A generic call fn[T](...) parses as a call whose Fun is an IndexExpr
				// (one type arg) or IndexListExpr (several). status.NewController[T]().
				var fun ast.Expr
				var typeArg ast.Expr
				switch idx := ce.Fun.(type) {
				case *ast.IndexExpr:
					fun, typeArg = idx.X, idx.Index
				case *ast.IndexListExpr:
					if len(idx.Indices) == 0 {
						return true
					}
					fun, typeArg = idx.X, idx.Indices[0]
				default:
					return true
				}
				// Match operatorpkg's status.NewController[T](), not any generic function
				// that happens to be named NewController.
				sel, ok := fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "NewController" {
					return true
				}
				if pkgIdent, ok := sel.X.(*ast.Ident); !ok || pkgIdent.Name != "status" {
					return true
				}
				if kind := identName(typeArg); kind != "" {
					seen[kind] = statusObject{kind: kind, subsystem: strings.ToLower(kind)}
				}
				return true
			})
		}
	}
	objs := lo.Values(seen)
	sort.Slice(objs, func(i, j int) bool { return objs[i].subsystem < objs[j].subsystem })
	return objs
}

// statusMetricTemplate describes one operatorpkg status/termination metric family:
// the subsystem suffix, metric name, help, and the base dimensions it always carries
// (in operatorpkg's declared order — see operatorpkg status/metrics.go). Per-object
// controllers may append registration-specific labels at runtime that can't be
// determined statically; those are omitted.
type statusMetricTemplate struct {
	subsystemSuffix string
	name            string
	help            string
	labels          []string
}

func statusMetricTemplates() []statusMetricTemplate {
	return []statusMetricTemplate{
		{"status_condition", "transition_seconds", "The amount of time a condition was in a given state before transitioning. e.g. Alarm := P99(Updated=False) > 5 minutes", []string{"type", "status", "to_status"}},
		{"status_condition", "count", "The number of a condition for a given object, type and status. e.g. Alarm := Available=False > 0", []string{"namespace", "name", "type", "status", "reason"}},
		{"status_condition", "current_status_seconds", "The current amount of time in seconds that a status condition has been in a specific state. Alarm := P99(Updated=Unknown) > 5 minutes", []string{"namespace", "name", "type", "status", "reason"}},
		{"status_condition", "transitions_total", "The count of transitions of a given object, type and status.", []string{"type", "status", "reason"}},
		{"termination", "current_time_seconds", "The current amount of time in seconds that an object has been in terminating state.", []string{"namespace", "name"}},
		{"termination", "duration_seconds", "The amount of time taken by an object to terminate completely.", nil},
	}
}

// perObjectStatusMetrics synthesizes the metrics operatorpkg's status controller
// creates per registered object type. operatorpkg creates them at runtime from the
// type parameter of status.NewController[T](), so they can't be read from a metric
// declaration; objects is the set parsed from those registration sites.
func perObjectStatusMetrics(objects []statusObject) []metricInfo {
	var out []metricInfo
	for _, obj := range objects {
		for _, t := range statusMetricTemplates() {
			// The `type` dimension of an object's status-condition metrics enumerates
			// that object's condition types.
			var labelValues map[string][]valueInfo
			if types, ok := conditionTypesByKind[obj.kind]; ok && slices.Contains(t.labels, "type") {
				labelValues = map[string][]valueInfo{"type": types}
			}
			out = append(out, metricInfo{
				namespace:   "operator",
				subsystem:   fmt.Sprintf("%s_%s", obj.subsystem, t.subsystemSuffix),
				name:        t.name,
				help:        t.help,
				labels:      t.labels,
				labelScope:  "operatorpkg",
				labelValues: labelValues,
			})
		}
	}
	return out
}

// deprecatedStatusMetrics synthesizes the deprecated generic status/termination
// metrics (no object-name prefix), still emitted when emitDeprecatedMetrics is set.
// They carry group/kind labels instead of baking the object into the subsystem, so
// their kind/type dimensions span every registered object.
func deprecatedStatusMetrics(objects []statusObject) []metricInfo {
	allKinds := lo.Map(objects, func(o statusObject, _ int) valueInfo { return valueInfo{name: o.kind} })
	var allTypes []valueInfo
	for _, o := range objects {
		allTypes = append(allTypes, conditionTypesByKind[o.kind]...)
	}
	// A condition type (e.g. ValidationSucceeded) can be set by more than one object;
	// dedupe by name so the union lists each once.
	allTypes = lo.UniqBy(allTypes, func(v valueInfo) string { return v.name })

	var out []metricInfo
	for _, t := range statusMetricTemplates() {
		labelValues := map[string][]valueInfo{"kind": allKinds}
		if slices.Contains(t.labels, "type") && len(allTypes) > 0 {
			labelValues["type"] = allTypes
		}
		out = append(out, metricInfo{
			namespace:   "operator",
			subsystem:   t.subsystemSuffix,
			name:        t.name,
			help:        t.help,
			labels:      slices.Concat(t.labels, []string{"group", "kind"}),
			labelScope:  "operatorpkg",
			labelValues: labelValues,
		})
	}
	return out
}

// hardcodedMetrics are metrics that can't be parsed from any declaration: operatorpkg
// registers the client_go metrics inside RegisterClientMetrics() via unqualified
// NewPrometheus* calls (same package), which the generator doesn't recognize.
func hardcodedMetrics() []metricInfo {
	return []metricInfo{
		{name: "client_go_request_duration_seconds", help: "Request latency in seconds. Broken down by verb, group, version, kind, and subresource."},
		{name: "client_go_request_total", help: "Number of HTTP requests, partitioned by status code and method."},
	}
}

// metricFromCallExpr attempts to extract metric info from a call expression. It
// recognizes prometheus.New*() and opmetrics.NewPrometheus*() calls. operatorpkg's
// own metric constructors (the pmetrics.* alias) are deliberately NOT parsed here:
// they build per-object metrics dynamically from a runtime type parameter, so their
// opts (e.g. a computed Subsystem) can't be resolved statically; those metrics are
// synthesized instead by perObjectStatusMetrics.
func metricFromCallExpr(ce *ast.CallExpr) (metricInfo, bool) {
	funcPkg := getFuncPackage(ce.Fun)
	// Determine the index of the opts argument based on the package.
	// prometheus.New*() calls pass opts as Args[0], while
	// opmetrics.NewPrometheus*() calls from operatorpkg pass
	// (registry, opts, labelNames), so opts is Args[1].
	var optsIdx int
	switch funcPkg {
	case "prometheus":
		optsIdx = 0
	case "opmetrics":
		optsIdx = 1
	default:
		return metricInfo{}, false
	}
	if len(ce.Args) <= optsIdx {
		return metricInfo{}, false
	}
	arg, ok := ce.Args[optsIdx].(*ast.CompositeLit)
	if !ok {
		return metricInfo{}, false
	}
	keyValuePairs := map[string]string{}
	for _, el := range arg.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key := fmt.Sprintf("%s", kv.Key)
		switch key {
		case "Namespace", "Subsystem", "Name", "Help":
		default:
			// skip any keys we don't care about
			continue
		}
		value := ""
		switch val := kv.Value.(type) {
		case *ast.BasicLit:
			value = val.Value
		case *ast.SelectorExpr:
			selector := fmt.Sprintf("%s.%s", val.X, val.Sel)
			// Prefer the curated identifier mapping (which intentionally overrides
			// some values, e.g. pluralizing subsystems), then fall back to the
			// resolved package-level string constant.
			if v, err := getIdentMapping(selector); err == nil {
				value = v
			} else if s, ok := resolveStringExpr(val); ok {
				value = s
			} else {
				log.Fatalf("unresolvable selector %s for key %s: %s", selector, key, err)
			}
		case *ast.Ident:
			if v, err := getIdentMapping(val.String()); err == nil {
				value = v
			} else if s, ok := resolveStringExpr(val); ok {
				value = s
			} else {
				log.Fatalf("unresolvable identifier %q for key %s: %s", val.String(), key, err)
			}
		case *ast.BinaryExpr:
			value = getBinaryExpr(val)
		default:
			// Unknown value expression type; skip this metric.
			return metricInfo{}, false
		}
		keyValuePairs[key] = strings.TrimFunc(value, func(r rune) bool {
			return r == '"'
		})
	}
	// The label names (dimensions) are the argument immediately after the opts
	// argument: prometheus.New*Vec(opts, labels) and
	// opmetrics.NewPrometheus*(registry, opts, labels). Non-vector metrics have no
	// such argument. Resolution is best-effort: metrics whose labels cannot be fully
	// resolved simply omit the Dimensions line rather than emit partial/incorrect data.
	var labels []string
	if labelsIdx := optsIdx + 1; len(ce.Args) > labelsIdx {
		if resolved, ok := resolveLabels(ce.Args[labelsIdx]); ok {
			labels = resolved
		}
	}
	return metricInfo{
		namespace: keyValuePairs["Namespace"],
		subsystem: keyValuePairs["Subsystem"],
		name:      keyValuePairs["Name"],
		help:      keyValuePairs["Help"],
		labels:    labels,
	}, true
}

// collectSymbols scans all package-level constant and variable declarations across
// the parsed packages, recording string-valued names (for label constants such as
// ReasonLabel = "reason") and []string-valued names (for shared label slices such as
// the aws-sdk-go-prometheus `labels` var). Names that resolve to conflicting values
// across packages are marked ambiguous and treated as unresolvable.
func collectSymbols(packages []*ast.Package) {
	// Pass 1: string constants/variables.
	forEachValueSpec(packages, func(_, name string, value ast.Expr) {
		if s, ok := stringLiteralValue(value); ok {
			if existing, seen := stringSymbols[name]; seen && existing != s {
				ambiguousStrings[name] = true
				return
			}
			stringSymbols[name] = s
		}
	})
	// Pass 1b: resolve alias declarations such as `const X = Y` or
	// `const X = pkg.Y`, where Y is itself a known string symbol. This lets label
	// name consts that dedupe to a shared const (e.g. metricLabelController =
	// metrics.ControllerLabel) resolve to their underlying value. A few iterations
	// reach a fixpoint that handles aliases-of-aliases.
	const aliasResolutionPasses = 3
	for range aliasResolutionPasses {
		changed := false
		forEachValueSpec(packages, func(_, name string, value ast.Expr) {
			if _, seen := stringSymbols[name]; seen {
				return
			}
			switch value.(type) {
			case *ast.Ident, *ast.SelectorExpr:
				if s, ok := resolveStringExpr(value); ok {
					stringSymbols[name] = s
					changed = true
				}
			}
		})
		if !changed {
			break
		}
	}
	// Pass 2: []string composite literals (may reference the string symbols above).
	forEachValueSpec(packages, func(_, name string, value ast.Expr) {
		cl, ok := value.(*ast.CompositeLit)
		if !ok {
			return
		}
		vals, ok := stringSliceFromCompositeLit(cl)
		if !ok {
			return
		}
		if existing, seen := sliceSymbols[name]; seen && !slices.Equal(existing, vals) {
			ambiguousSlices[name] = true
			return
		}
		sliceSymbols[name] = vals
	})
	// Pass 3: single Value vars (first-class dimension values referenced by name from
	// a []Value literal, e.g. a metrics-owned error category).
	forEachValueSpec(packages, func(_, name string, value ast.Expr) {
		cl, ok := value.(*ast.CompositeLit)
		if !ok || identName(cl.Type) != "Value" {
			return
		}
		if v, ok := valueFromCompositeLit(cl); ok {
			valueSymbols[name] = v
		}
	})
	// Pass 4: []Value composite literals (shared value sets referenced by a Label's
	// Values field, e.g. operatorpkg's conditionStatusValues). Runs after Pass 3 so
	// elements that reference a single Value var resolve.
	forEachValueSpec(packages, func(_, name string, value ast.Expr) {
		cl, ok := value.(*ast.CompositeLit)
		if !ok || !isValueSliceType(cl.Type) {
			return
		}
		if vals, ok := valueSliceFromCompositeLit(cl); ok {
			valueSliceSymbols[name] = vals
		}
	})
	// Pass 5: the per-Kind condition-type registry (karpenter's
	// metrics.ConditionTypeValues map), used to document the `type` dimension of
	// each object's status-condition metrics.
	collectConditionTypes(packages)
	// Pass 6: the controller-name registries (metrics.ControllerValues in core and
	// the provider), used to document the `controller` dimension.
	collectControllerValues(packages)
}

// collectControllerValues unions every metrics.ControllerValues ([]Value) registry
// across the scanned packages into the controller-name value set.
func collectControllerValues(packages []*ast.Package) {
	forEachValueSpec(packages, func(_, name string, value ast.Expr) {
		if name != "ControllerValues" {
			return
		}
		cl, ok := value.(*ast.CompositeLit)
		if !ok || !isValueSliceType(cl.Type) {
			return
		}
		if vals, ok := valueSliceFromCompositeLit(cl); ok {
			controllerValues = append(controllerValues, vals...)
		}
	})
}

// attachControllerValues finalizes the `controller` dimension's value set — the
// ControllerValues registries plus the operatorpkg status/events controllers, which
// name themselves operatorpkg.<kind>.status / .events at runtime — and attaches it
// wherever `controller` is documented (the code Label for karpenter metrics and the
// controller_runtime third-party injection).
func attachControllerValues(objects []statusObject) {
	vals := slices.Clone(controllerValues)
	for _, o := range objects {
		vals = append(vals,
			valueInfo{name: "operatorpkg." + o.subsystem + ".status", help: fmt.Sprintf("operatorpkg status-condition controller for %s.", o.kind)},
			valueInfo{name: "operatorpkg." + o.subsystem + ".events", help: fmt.Sprintf("operatorpkg events controller for %s.", o.kind)},
		)
	}
	if len(vals) == 0 {
		return
	}
	if li, ok := labelRegistry["controller"]; ok {
		li.values = vals
		labelRegistry["controller"] = li
	}
	if inj, ok := labelInjections["controller_runtime"]; ok {
		e := inj["controller"]
		e.values = vals
		inj["controller"] = e
	}
}

// collectConditionTypes scans for a map[string][]Value composite literal (karpenter's
// metrics.ConditionTypeValues) and records each Kind's status condition types.
func collectConditionTypes(packages []*ast.Package) {
	forEachValueSpec(packages, func(_, _ string, value ast.Expr) {
		cl, ok := value.(*ast.CompositeLit)
		if !ok {
			return
		}
		mt, ok := cl.Type.(*ast.MapType)
		if !ok {
			return
		}
		if key, ok := mt.Key.(*ast.Ident); !ok || key.Name != "string" {
			return
		}
		if !isValueSliceType(mt.Value) {
			return
		}
		for _, el := range cl.Elts {
			kv, ok := el.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			kind, ok := resolveStringExpr(kv.Key)
			if !ok {
				continue
			}
			if vals, ok := resolveValues(kv.Value); ok {
				conditionTypesByKind[kind] = vals
			}
		}
	})
}

// collectLabels scans package-level var declarations for metrics.Label{...}
// composite literals and records each label's help text and stable values keyed by
// its resolved Name. This is the code-sourced registry the docs generator uses to
// annotate each metric dimension. Entries are merged by name: an entry that carries
// help text is preferred over one that does not, so a fully-documented shared Label
// wins over a bare reference elsewhere.
func collectLabels(packages []*ast.Package) {
	forEachValueSpec(packages, func(file, _ string, value ast.Expr) {
		cl, ok := value.(*ast.CompositeLit)
		if !ok || !isLabelType(cl.Type) {
			return
		}
		fields := namedFields(cl)
		name, ok := resolveStringExpr(fields["Name"])
		if !ok {
			return
		}
		help, _ := resolveStringExpr(fields["Help"])
		values, _ := resolveValues(fields["Values"])
		info := labelInfo{help: help, values: values}
		// A Label is recorded in the registry for its declaring code base. Labels from
		// a scoped code base (operatorpkg) go ONLY into that scope, not the global
		// registry, so a name reused with a different meaning (e.g. `reason`) doesn't
		// leak operatorpkg's docs onto unrelated karpenter/third-party metrics; only
		// metrics that opt into the scope resolve there. First entry with help wins.
		registry := labelRegistry
		if scope := labelScopeForFile(file); scope != "" {
			if scopedLabelRegistry[scope] == nil {
				scopedLabelRegistry[scope] = map[string]labelInfo{}
			}
			registry = scopedLabelRegistry[scope]
		}
		if existing, seen := registry[name]; !seen || (existing.help == "" && help != "") {
			registry[name] = info
		}
	})
}

// identName returns the identifier name of a (possibly pointer, possibly
// package-qualified) type/name expression, e.g. Label -> "Label",
// metrics.Value -> "Value", *v1.NodeClaim -> "NodeClaim".
func identName(t ast.Expr) string {
	switch v := t.(type) {
	case *ast.StarExpr:
		return identName(v.X)
	case *ast.SelectorExpr:
		return v.Sel.Name
	case *ast.Ident:
		return v.Name
	}
	return ""
}

// isLabelType reports whether a composite literal's type is a metrics.Label.
func isLabelType(t ast.Expr) bool { return identName(t) == "Label" }

// isValueSliceType reports whether an expression's type is []Value (a metrics.Value slice).
func isValueSliceType(t ast.Expr) bool {
	at, ok := t.(*ast.ArrayType)
	return ok && identName(at.Elt) == "Value"
}

// namedFields returns the key->value expressions of a struct composite literal's
// keyed fields, e.g. {Name: x, Help: y} -> {"Name": x, "Help": y}.
func namedFields(cl *ast.CompositeLit) map[string]ast.Expr {
	out := map[string]ast.Expr{}
	for _, el := range cl.Elts {
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			if key, ok := kv.Key.(*ast.Ident); ok {
				out[key.Name] = kv.Value
			}
		}
	}
	return out
}

// valueFromCompositeLit resolves a single Value{Name: ..., Help: ...} composite
// literal. Name is resolved through the string symbol table so it may reference a
// const; a Value whose Name cannot be resolved is reported as not ok.
func valueFromCompositeLit(cl *ast.CompositeLit) (valueInfo, bool) {
	fields := namedFields(cl)
	name, ok := resolveStringExpr(fields["Name"])
	if !ok {
		return valueInfo{}, false
	}
	help, _ := resolveStringExpr(fields["Help"])
	return valueInfo{name: name, help: help}, true
}

// valueSliceFromCompositeLit resolves a []Value composite literal to its documented
// values. Each element is either an inline Value{...} literal or a reference to a
// named Value var. An element whose Name cannot be resolved (e.g. it references a
// const in an unscanned package) is skipped rather than discarding the whole slice.
func valueSliceFromCompositeLit(cl *ast.CompositeLit) ([]valueInfo, bool) {
	if cl.Type != nil && !isValueSliceType(cl.Type) {
		return nil, false
	}
	out := make([]valueInfo, 0, len(cl.Elts))
	for _, el := range cl.Elts {
		if v, ok := resolveValue(el); ok {
			out = append(out, v)
		}
	}
	return out, true
}

// resolveValue resolves a single []Value element: an inline Value{...} literal or a
// reference to a named Value var.
func resolveValue(expr ast.Expr) (valueInfo, bool) {
	if cl, ok := expr.(*ast.CompositeLit); ok {
		return valueFromCompositeLit(cl)
	}
	if v, ok := valueSymbols[identName(expr)]; ok {
		return v, true
	}
	return valueInfo{}, false
}

// resolveValues resolves a Label's Values field, handling both an inline []Value
// composite literal and a reference to a named []Value variable (e.g. a shared
// conditionStatusValues var).
func resolveValues(expr ast.Expr) ([]valueInfo, bool) {
	switch v := expr.(type) {
	case *ast.CompositeLit:
		return valueSliceFromCompositeLit(v)
	case *ast.Ident:
		if vals, ok := valueSliceSymbols[v.Name]; ok {
			return vals, true
		}
	case *ast.SelectorExpr:
		if vals, ok := valueSliceSymbols[v.Sel.Name]; ok {
			return vals, true
		}
	}
	return nil, false
}

// forEachValueSpec invokes fn for every (file, name, value) tuple in package-level
// const/var declarations across the given packages. file is the path of the source
// file the declaration was parsed from, used to attribute a declaration to a code base.
func forEachValueSpec(packages []*ast.Package, fn func(file, name string, value ast.Expr)) {
	for _, pkg := range packages {
		// Iterate files in a stable order; pkg.Files is a map, so ranging it directly
		// would make "first entry wins" resolution (and thus the docs) nondeterministic.
		for _, filePath := range slices.Sorted(maps.Keys(pkg.Files)) {
			file := pkg.Files[filePath]
			for _, decl := range file.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || (gd.Tok != token.CONST && gd.Tok != token.VAR) {
					continue
				}
				for _, spec := range gd.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, nm := range vs.Names {
						if i < len(vs.Values) {
							fn(filePath, nm.Name, vs.Values[i])
						}
					}
				}
			}
		}
	}
}

// stringLiteralValue returns the unquoted value of a string literal expression.
// It uses strconv.Unquote so that only the delimiters are removed (and escapes
// are decoded); a naive trim would strip backticks that are part of the content,
// e.g. help text like "... or `expired`".
func stringLiteralValue(expr ast.Expr) (string, bool) {
	bl, ok := expr.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(bl.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

// stringSliceFromCompositeLit resolves a []string composite literal to its string
// values, resolving element identifiers via the string symbol table.
func stringSliceFromCompositeLit(cl *ast.CompositeLit) ([]string, bool) {
	if at, ok := cl.Type.(*ast.ArrayType); ok {
		if id, ok := at.Elt.(*ast.Ident); !ok || id.Name != "string" {
			return nil, false
		}
	} else if cl.Type != nil {
		return nil, false
	}
	out := make([]string, 0, len(cl.Elts))
	for _, el := range cl.Elts {
		s, ok := resolveStringExpr(el)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

// resolveLabels resolves a metric's label-names argument to a slice of label names,
// handling both an inline []string composite literal and a reference to a named
// []string variable.
func resolveLabels(expr ast.Expr) ([]string, bool) {
	switch v := expr.(type) {
	case *ast.CompositeLit:
		return stringSliceFromCompositeLit(v)
	case *ast.Ident:
		if ambiguousSlices[v.Name] {
			return nil, false
		}
		if s, ok := sliceSymbols[v.Name]; ok {
			return s, true
		}
	case *ast.SelectorExpr:
		if ambiguousSlices[v.Sel.Name] {
			return nil, false
		}
		if s, ok := sliceSymbols[v.Sel.Name]; ok {
			return s, true
		}
	}
	return nil, false
}

// resolveStringExpr resolves a single expression to a string value, handling string
// literals and references to named string constants (bare or package-qualified).
func resolveStringExpr(expr ast.Expr) (string, bool) {
	switch v := expr.(type) {
	case *ast.BasicLit:
		return stringLiteralValue(v)
	case *ast.Ident:
		if ambiguousStrings[v.Name] {
			return "", false
		}
		if s, ok := stringSymbols[v.Name]; ok {
			return s, true
		}
	case *ast.SelectorExpr:
		if ambiguousStrings[v.Sel.Name] {
			return "", false
		}
		if s, ok := stringSymbols[v.Sel.Name]; ok {
			return s, true
		}
	case *ast.CallExpr:
		// Unwrap string(X) conversions, used when a Label value references a
		// string-based named type such as disruption.Decision or v1.ConsolidationPolicy.
		if fn, ok := v.Fun.(*ast.Ident); ok && fn.Name == "string" && len(v.Args) == 1 {
			return resolveStringExpr(v.Args[0])
		}
	case *ast.BinaryExpr:
		// Resolve string concatenation ("a" + "b" + const), used to wrap long
		// Label.Help text across multiple source lines.
		if v.Op == token.ADD {
			if l, ok := resolveStringExpr(v.X); ok {
				if r, ok := resolveStringExpr(v.Y); ok {
					return l + r, true
				}
			}
		}
	}
	return "", false
}

func getFuncPackage(fun ast.Expr) string {
	if pexpr, ok := fun.(*ast.ParenExpr); ok {
		return getFuncPackage(pexpr.X)
	}
	if sexpr, ok := fun.(*ast.StarExpr); ok {
		return getFuncPackage(sexpr.X)
	}
	if sel, ok := fun.(*ast.SelectorExpr); ok {
		return fmt.Sprintf("%s", sel.X)
	}
	if ident, ok := fun.(*ast.Ident); ok {
		return ident.String()
	}
	if iexpr, ok := fun.(*ast.IndexExpr); ok {
		return getFuncPackage(iexpr.X)
	}
	return ""
}

func getBinaryExpr(b *ast.BinaryExpr) string {
	var x, y string
	switch val := b.X.(type) {
	case *ast.BasicLit:
		x = strings.Trim(val.Value, `"`)
	case *ast.BinaryExpr:
		x = getBinaryExpr(val)
	default:
		log.Fatalf("unsupported value %T %v", val, val)
	}
	switch val := b.Y.(type) {
	case *ast.BasicLit:
		y = strings.Trim(val.Value, `"`)
	case *ast.BinaryExpr:
		y = getBinaryExpr(val)
	default:
		log.Fatalf("unsupported value %T %v", val, val)
	}
	return x + y
}

// we cannot get the value of an Identifier directly so we map it manually instead
func getIdentMapping(identName string) (string, error) {
	identMapping := map[string]string{
		"metrics.Namespace": metrics.Namespace,
		"Namespace":         metrics.Namespace,

		"pmetrics.Namespace":         "operator",
		"MetricNamespace":            "operator",
		"MetricSubsystem":            "status_condition",
		"TerminationSubsystem":       "termination",
		"WorkQueueSubsystem":         "workqueue",
		"DepthKey":                   "depth",
		"AddsKey":                    "adds_total",
		"QueueLatencyKey":            "queue_duration_seconds",
		"WorkDurationKey":            "work_duration_seconds",
		"UnfinishedWorkKey":          "unfinished_work_seconds",
		"LongestRunningProcessorKey": "longest_running_processor_seconds",
		"RetriesKey":                 "retries_total",

		"metrics.PodSubsystem":       "pods",
		"NodeSubsystem":              "nodes",
		"metrics.NodeSubsystem":      "nodes",
		"machineSubsystem":           "machines",
		"NodeClaimSubsystem":         "nodeclaims",
		"metrics.NodeClaimSubsystem": "nodeclaims",
		// TODO @joinnis: We should eventually change this subsystem to be
		// plural so that it aligns with the other subsystems
		"nodePoolSubsystem":            "nodepools",
		"metrics.NodePoolSubsystem":    "nodepools",
		"interruptionSubsystem":        "interruption",
		"deprovisioningSubsystem":      "deprovisioning",
		"voluntaryDisruptionSubsystem": "voluntary_disruption",
		"batcherSubsystem":             "cloudprovider_batcher",
		"cloudProviderSubsystem":       "cloudprovider",
		"stateSubsystem":               "cluster_state",
		"schedulerSubsystem":           "scheduler",
		"nodeClassSubsystem":           "ec2nodeclasses",
	}
	if v, ok := identMapping[identName]; ok {
		return v, nil
	}
	return "", serrors.Wrap(fmt.Errorf("no identifier mapping exists"), "identifier", identName)
}
