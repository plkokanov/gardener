// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package vpagarden

import (
	_ "embed"
)

//go:embed assets/scrapeconfigs/cadvisor.yaml
var cAdvisor string

// AdditionalScrapeConfigs returns the additional scrape configs for the garden prometheus.
func AdditionalScrapeConfigs() []string {
	return []string{cAdvisor}
}
