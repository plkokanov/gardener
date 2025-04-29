// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package vparecommendercentral

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
)

var (
	//go:embed assets/scrapeconfigs/cadvisor-seed.yaml
	cAdvisorSeed string
	//go:embed assets/scrapeconfigs/cadvisor-garden.yaml
	cAdvisorGarden string
)

// Data represents the data for the template.
type Data struct {
	IsManagedSeed bool
}

// AdditionalScrapeConfigsSeed returns the additional scrape configs for the cache prometheus.
// TODO(plkokanov): Largely copied from `github.com/gardener/gardener/pkg/component/observability/monitoring/prometheus/cache/scrapeconfigs.go`
// Check if we can reuse this somehow instead of copying it
func AdditionalScrapeConfigsSeed(isManagedSeed bool) ([]string, error) {
	var out []string

	if result, err := process(cAdvisorSeed, isManagedSeed); err != nil {
		return nil, fmt.Errorf("failed processing cadvisor scrape config template: %w", err)
	} else {
		out = append(out, result)
	}
	return out, nil
}

func AdditionalScrapeConfigsGarden() []string {
	return []string{cAdvisorGarden}
}

func process(text string, isManagedSeed bool) (string, error) {
	data := Data{
		IsManagedSeed: isManagedSeed,
	}

	tmpl, err := template.New("Template").Parse(text)
	if err != nil {
		return "", fmt.Errorf("failed parsing template: %w", err)
	}

	var result bytes.Buffer
	if err := tmpl.Execute(&result, data); err != nil {
		return "", fmt.Errorf("failed rendering template: %w", err)
	}

	return result.String(), nil
}
