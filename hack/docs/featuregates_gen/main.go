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
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/awslabs/operatorpkg/docs"
	coreoptions "sigs.k8s.io/karpenter/pkg/operator/options"

	"github.com/aws/karpenter-provider-aws/pkg/operator/options"
)

// featuregates_gen renders core's and the AWS provider's feature gates into the feature gate tables of the settings page.
func main() {
	if len(os.Args) != 2 {
		log.Fatalf("Usage: %s path/to/markdown.md", os.Args[0])
	}
	outputFileName := os.Args[1]
	mdFile, err := os.ReadFile(outputFileName)
	if err != nil {
		log.Fatalf("error reading output file %s, %s", outputFileName, err)
	}
	doc := replaceSection(string(mdFile), "core", table(coreoptions.KarpenterFeatureGates))
	doc = replaceSection(doc, "AWS", table(options.AWSFeatureGates))
	if err := os.WriteFile(outputFileName, []byte(doc), 0644); err != nil {
		log.Fatalf("error writing output file %s, %s", outputFileName, err)
	}
}

// replaceSection replaces the content between a section's generated comment markers.
func replaceSection(doc, section, content string) string {
	genStart := fmt.Sprintf("[comment]: <> (the %s feature gates below are generated from hack/docs/featuregates_gen/main.go)", section)
	genEnd := fmt.Sprintf("[comment]: <> (end %s feature gates generated from hack/docs/featuregates_gen/main.go)", section)
	startDocSections := strings.Split(doc, genStart)
	if len(startDocSections) != 2 {
		log.Fatalf("expected one %s generated comment block start but got %d", section, len(startDocSections)-1)
	}
	endDocSections := strings.Split(doc, genEnd)
	if len(endDocSections) != 2 {
		log.Fatalf("expected one %s generated comment block end but got %d", section, len(endDocSections)-1)
	}
	return fmt.Sprintf("%s%s\n\n%s\n%s%s", startDocSections[0], genStart, content, genEnd, endDocSections[1])
}

func table(gates []coreoptions.FeatureGate) string {
	var b strings.Builder
	b.WriteString("| Feature | Default | Stage | Description |\n")
	b.WriteString("|---------|---------|-------|-------------|\n")
	for _, g := range gates {
		fmt.Fprintf(&b, "| %s | %t | %s | %s |\n", g.Name, g.Default, stage(g.Stage), g.Help)
	}
	return b.String()
}

func stage(s docs.Stage) string {
	if s == docs.GA {
		return "GA"
	}
	return strings.ToUpper(string(s[:1])) + string(s[1:])
}
